package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func adapterInstallationSession(repository string) domain.InstallationSessionRecord {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	return domain.InstallationSessionRecord{
		SchemaVersion: domain.InstallationSessionSchemaVersion,
		Repository:    repository,
		State:         domain.InstallationSessionActive,
		Revision:      strings.Repeat("a", 40),
		Selected:      "pc01",
		StartedAt:     now,
		UpdatedAt:     now,
		Evidence:      []domain.InstallationEvidence{},
	}
}

func TestInstallationSessionRoundTripUsesPrivateAtomicState(t *testing.T) {
	stateRoot := t.TempDir()
	repository := filepath.Join(t.TempDir(), "deployment")
	record := adapterInstallationSession(repository)
	if err := writeInstallationSession(stateRoot, repository, record); err != nil {
		t.Fatal(err)
	}
	read, present, err := readInstallationSession(stateRoot, repository)
	if err != nil || !present || read.Selected != "pc01" {
		t.Fatalf("read session = %+v present=%t error=%v", read, present, err)
	}
	directory, path, err := installationSessionLocation(stateRoot, repository)
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]os.FileMode{filepath.Dir(directory): 0700, directory: 0700, path: 0600} {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("mode %s = %04o, want %04o", target, info.Mode().Perm(), want)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(directory, ".installation-session.*")); len(matches) != 0 {
		t.Fatalf("temporary session files remain: %v", matches)
	}
}

func TestInstallationSessionRejectsCorruptionSymlinkAndUnsafeMode(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "unsafe mode", mutate: func(path string) error { return os.Chmod(path, 0644) }},
		{name: "corrupt", mutate: func(path string) error { return os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0600) }},
		{name: "symlink", mutate: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Symlink("/etc/passwd", path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := t.TempDir()
			repository := filepath.Join(t.TempDir(), "deployment")
			if err := writeInstallationSession(stateRoot, repository, adapterInstallationSession(repository)); err != nil {
				t.Fatal(err)
			}
			_, path, err := installationSessionLocation(stateRoot, repository)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(path); err != nil {
				t.Fatal(err)
			}
			if _, _, err := readInstallationSession(stateRoot, repository); err == nil {
				t.Fatal("unsafe installation session was accepted")
			}
		})
	}
}

func TestInstallationSessionSeparatesRepositoriesAndRejectsMismatchedWrites(t *testing.T) {
	stateRoot := t.TempDir()
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := writeInstallationSession(stateRoot, first, adapterInstallationSession(first)); err != nil {
		t.Fatal(err)
	}
	if _, present, err := readInstallationSession(stateRoot, second); err != nil || present {
		t.Fatalf("second repository leaked session: present=%t error=%v", present, err)
	}
	if err := writeInstallationSession(stateRoot, second, adapterInstallationSession(first)); err == nil {
		t.Fatal("repository-mismatched session was written")
	}
}
