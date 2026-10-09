package adapters

import (
	"github.com/giovantenne/nixorium/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupRoundTripKeepsHistoryAndPrivateKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repository := workspaceRepository(t)
	writeGitReviewFile(t, repository, "lab-settings.json", "{\"lab\":{}}\n")
	local := Local{}
	if _, due := local.BackupDue(repository); due {
		t.Fatal("a backup is due before any laboratory key exists")
	}
	if err := os.WriteFile(filepath.Join(repository, "secret-key"), []byte("private cache key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nix/store/0000-result", filepath.Join(repository, "result")); err != nil {
		t.Fatal(err)
	}
	if reason, due := local.BackupDue(repository); !due || !strings.Contains(reason, "No current backup") {
		t.Fatalf("due = %q %v", reason, due)
	}
	destination := t.TempDir()
	passphrase := []byte("correct horse battery")
	if report := local.CreateBackup(t.Context(), repository, destination, []byte("short"), "test"); report.State == "completed" {
		t.Fatal("a short passphrase was accepted")
	}
	report := local.CreateBackup(t.Context(), repository, destination, passphrase, "test")
	if report.State != "completed" || report.Path == "" || len(report.PrivateKeys) != 1 || report.PrivateKeys[0] != "secret-key" {
		t.Fatalf("create = %+v", report)
	}
	entries, _ := os.ReadDir(destination)
	if len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".") {
		t.Fatalf("destination = %v", entries)
	}
	if reason, due := local.BackupDue(repository); due {
		t.Fatalf("archive did not clear the reminder: %s", reason)
	}
	record, _ := local.LastBackup()
	for _, change := range []struct {
		name string
		edit func(*domain.BackupRecord)
	}{
		{"another laboratory", func(r *domain.BackupRecord) { r.Repository = t.TempDir() }},
		{"expired", func(r *domain.BackupRecord) { r.CreatedAt = time.Now().Add(-31 * 24 * time.Hour) }},
		{"different revision", func(r *domain.BackupRecord) { r.Revision = "old" }},
		{"different settings", func(r *domain.BackupRecord) { r.SettingsHash = "old" }},
	} {
		stale := record
		change.edit(&stale)
		recordBackup(stale)
		if _, due := local.BackupDue(repository); !due {
			t.Fatalf("%s archive hid reminder", change.name)
		}
	}
	recordBackup(record)
	if verify := local.VerifyBackup(report.Path, []byte("wrong passphrase!!")); verify.State == "completed" {
		t.Fatal("a wrong passphrase was accepted")
	}
	if verify := local.VerifyBackup(report.Path, passphrase); verify.State != "completed" || verify.Revision != report.Revision {
		t.Fatalf("verify = %+v", verify)
	}
	target := filepath.Join(t.TempDir(), "restored")
	restored := local.RestoreBackup(report.Path, passphrase, target)
	if restored.State != "completed" {
		t.Fatalf("restore = %+v", restored)
	}
	key, err := os.ReadFile(filepath.Join(target, "deployment", "secret-key"))
	if err != nil || string(key) != "private cache key" {
		t.Fatalf("private key = %q %v", key, err)
	}
	if info, _ := os.Stat(filepath.Join(target, "deployment", "secret-key")); info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %v", info.Mode())
	}
	if _, err := os.Lstat(filepath.Join(target, "deployment", "result")); !os.IsNotExist(err) {
		t.Fatal("a Nix store result link was backed up")
	}
	if head := strings.TrimSpace(workspaceTestGit(t, filepath.Join(target, "deployment"), "rev-parse", "HEAD")); head != report.Revision {
		t.Fatalf("restored HEAD = %s, want %s", head, report.Revision)
	}
	if again := local.RestoreBackup(report.Path, passphrase, target); again.State == "completed" {
		t.Fatal("restore wrote into a non-empty directory")
	}
	if err := os.WriteFile(filepath.Join(repository, "secret-key"), []byte("rotated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reason, due := local.BackupDue(repository); !due || !strings.Contains(reason, "private keys") {
		t.Fatalf("key change not noticed: %q %v", reason, due)
	}
}

func TestBackupRefusesADestinationInsideTheRepository(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repository := workspaceRepository(t)
	if report := (Local{}).CreateBackup(t.Context(), repository, repository, []byte("correct horse battery"), "test"); report.State == "completed" {
		t.Fatal("backup written inside the repository")
	}
}
