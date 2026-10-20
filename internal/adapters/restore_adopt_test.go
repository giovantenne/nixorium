package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func restoredLabFixture(t *testing.T, path, adminKey string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"flake.nix": "{}", "lab-settings.json": "{}", "admin-ssh": adminKey} {
		if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdoptRestoredLabKeepsThePreviousLab(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "nixorium-deployment")
	restored := filepath.Join(root, "nixorium-restored")
	restoredLabFixture(t, repository, "fresh key")
	restoredLabFixture(t, restored, "original key")
	installed := filepath.Join(root, "id_ed25519")
	if err := os.WriteFile(installed, []byte("other key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).AdoptRestoredLab(repository, restored, installed, time.Now()); err == nil || !strings.Contains(err.Error(), "own lab keys") {
		t.Fatalf("different installed key accepted: %v", err)
	}
	if err := os.WriteFile(installed, []byte("original key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 19, 56, 0, 0, time.Local)
	previous, err := (Local{}).AdoptRestoredLab(repository, restored, installed, now)
	if err != nil {
		t.Fatal(err)
	}
	if previous != repository+"-before-restore-20261020-195600" {
		t.Fatalf("previous = %q", previous)
	}
	if key, _ := os.ReadFile(filepath.Join(repository, "admin-ssh")); string(key) != "original key" {
		t.Fatal("restored lab not in place")
	}
	if key, _ := os.ReadFile(filepath.Join(previous, "admin-ssh")); string(key) != "fresh key" {
		t.Fatal("previous lab not kept")
	}
	if _, err := os.Lstat(restored); !os.IsNotExist(err) {
		t.Fatal("restored folder still present")
	}
	if _, err := (Local{}).AdoptRestoredLab(repository, restored, "", now); err == nil {
		t.Fatal("missing restored lab accepted")
	}
}

func TestRestoreFolderAndBackupFiles(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "nixorium-deployment")
	if got := RestoreFolder(repository); got != filepath.Join(root, "nixorium-restored") {
		t.Fatalf("folder = %q", got)
	}
	if err := os.Mkdir(filepath.Join(root, "nixorium-restored"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := RestoreFolder(repository); got != filepath.Join(root, "nixorium-restored-2") {
		t.Fatalf("second folder = %q", got)
	}
	for _, name := range []string{"nixorium-backup-20261001-090000.age", "nixorium-backup-20261020-153344.age", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := FindBackupFiles(root, "")
	if len(files) != 2 || filepath.Base(files[0]) != "nixorium-backup-20261020-153344.age" {
		t.Fatalf("backup files = %v", files)
	}
}
