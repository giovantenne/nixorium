package adapters

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDeploymentHistoryRoundTripMergesHostsAndSeparatesRepositories(t *testing.T) {
	stateRoot := t.TempDir()
	repository := filepath.Join(t.TempDir(), "deployment")
	otherRepository := filepath.Join(t.TempDir(), "deployment")
	firstTime := time.Date(2026, 9, 14, 12, 30, 0, 0, time.FixedZone("test", 2*60*60))
	first := domain.LastSuccessfulDeployment{
		Revision:   "0123456789abcdef0123456789abcdef01234567",
		SystemPath: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system",
		VerifiedAt: firstTime,
	}
	if err := writeDeploymentHistory(stateRoot, repository, map[string]domain.LastSuccessfulDeployment{"pc01": first}); err != nil {
		t.Fatal(err)
	}
	second := domain.LastSuccessfulDeployment{
		Revision:   "fedcba9876543210fedcba9876543210fedcba98",
		SystemPath: "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-pc02-system",
		VerifiedAt: firstTime.Add(time.Hour),
	}
	if err := writeDeploymentHistory(stateRoot, repository, map[string]domain.LastSuccessfulDeployment{"pc02": second}); err != nil {
		t.Fatal(err)
	}

	history, err := readDeploymentHistory(stateRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Hosts) != 2 || history.Hosts["pc01"].Revision != first.Revision || history.Hosts["pc02"].SystemPath != second.SystemPath {
		t.Fatalf("history = %+v, want merged pc01 and pc02 records", history)
	}
	if history.Hosts["pc01"].VerifiedAt.Location() != time.UTC || !history.Hosts["pc01"].VerifiedAt.Equal(firstTime) {
		t.Fatalf("verified time = %v, want normalized UTC instant", history.Hosts["pc01"].VerifiedAt)
	}
	_, path, err := deploymentHistoryLocation(stateRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("history mode = %04o, want 0600", info.Mode().Perm())
	}
	other, err := readDeploymentHistory(stateRoot, otherRepository)
	if err != nil || len(other.Hosts) != 0 {
		t.Fatalf("other repository history = %+v, error = %v", other, err)
	}
}

func TestDeploymentHistoryRejectsCorruptionWithoutOverwritingIt(t *testing.T) {
	stateRoot := t.TempDir()
	repository := filepath.Join(t.TempDir(), "deployment")
	directory, path, err := deploymentHistoryLocation(stateRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"schemaVersion":1,"repository":"wrong","hosts":{}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDeploymentHistory(stateRoot, repository); err == nil {
		t.Fatal("repository-mismatched history was accepted")
	}
	update := map[string]domain.LastSuccessfulDeployment{
		"pc01": {
			Revision:   "0123456789abcdef0123456789abcdef01234567",
			SystemPath: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system",
			VerifiedAt: time.Now(),
		},
	}
	if err := writeDeploymentHistory(stateRoot, repository, update); err == nil {
		t.Fatal("corrupt history was silently overwritten")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != string(original) {
		t.Fatalf("corrupt history changed to %q, error = %v", content, err)
	}
}

func TestDeploymentHistoryRejectsSymlinkAndUnsafeMode(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(string) error
	}{
		{
			name: "symlink",
			make: func(path string) error {
				target := filepath.Join(filepath.Dir(path), "target")
				if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
					return err
				}
				return os.Symlink(target, path)
			},
		},
		{
			name: "unsafe mode",
			make: func(path string) error {
				return os.WriteFile(path, []byte("{}"), 0644)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := t.TempDir()
			repository := filepath.Join(t.TempDir(), "deployment")
			directory, path, err := deploymentHistoryLocation(stateRoot, repository)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := test.make(path); err != nil {
				t.Fatal(err)
			}
			if _, err := readDeploymentHistory(stateRoot, repository); err == nil {
				t.Fatalf("%s history was accepted", test.name)
			}
		})
	}
}

func TestDeploymentHistoryRejectsUnsafeDirectory(t *testing.T) {
	stateRoot := t.TempDir()
	repository := filepath.Join(t.TempDir(), "deployment")
	directory, path, err := deploymentHistoryLocation(stateRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"repository":"`+repository+`","hosts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := readDeploymentHistory(stateRoot, repository); err == nil {
		t.Fatal("history in an unsafe directory was accepted")
	}
}
