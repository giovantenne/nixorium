package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"filippo.io/age"
	"github.com/giovantenne/nixorium/internal/domain"
	"io/fs"
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
	if reason, due := local.BackupDue(repository); !due || !strings.Contains(reason, "No backup is recorded") {
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
	if reason, due := local.BackupDue(repository); !due || !strings.Contains(reason, "keys") {
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

func TestRestoreRefusesLinksThatLeaveTheDeployment(t *testing.T) {
	passphrase := []byte("correct horse battery")
	craft := func(t *testing.T, files []domain.BackupFile, origins map[string]string) string {
		t.Helper()
		recipient, err := age.NewScryptRecipient(string(passphrase))
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(t.TempDir(), "crafted.age")
		output, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		manifest := domain.BackupManifest{SchemaVersion: domain.BackupSchemaVersion, CreatedAt: time.Now().UTC(), Files: files}
		if err := writeBackupArchive(output, recipient, manifest, origins); err != nil {
			t.Fatal(err)
		}
		return name
	}
	content := filepath.Join(t.TempDir(), "content")
	if err := os.WriteFile(content, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("payload"))
	regular := func(name string) domain.BackupFile {
		return domain.BackupFile{Path: name, Mode: 0o600, Size: 7, SHA256: hex.EncodeToString(digest[:])}
	}
	directory := domain.BackupFile{Path: "deployment", Mode: uint32(fs.ModeDir) | 0o700}

	outside := t.TempDir()
	through := craft(t, []domain.BackupFile{directory, {Path: "deployment/x", Link: outside}, regular("deployment/x/passwd")}, map[string]string{"deployment/x/passwd": content})
	if report := (Local{}).RestoreBackup(through, passphrase, filepath.Join(t.TempDir(), "restored")); report.State == "completed" {
		t.Fatal("a link to an absolute path was restored")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("restore wrote outside the target: %v", entries)
	}
	escaping := craft(t, []domain.BackupFile{directory, {Path: "deployment/up", Link: "../../.."}}, nil)
	if report := (Local{}).RestoreBackup(escaping, passphrase, filepath.Join(t.TempDir(), "restored")); report.State == "completed" {
		t.Fatal("a link escaping the restore root was restored")
	}
	// Each link stays inside on its own, but c/b is created where c points.
	chained := craft(t, []domain.BackupFile{directory, {Path: "deployment/c", Link: "."}, {Path: "deployment/c/b", Link: "../.."}, {Path: "deployment/c/b/z", Link: "x"}}, nil)
	chainedTarget := filepath.Join(t.TempDir(), "restored")
	if report := (Local{}).RestoreBackup(chained, passphrase, chainedTarget); report.State == "completed" {
		t.Fatal("a link inside another link was restored")
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(chainedTarget), "z")); err == nil {
		t.Fatal("restore created a link outside the target")
	}
	// Each target stays inside on its own, but b passes through a, a link to
	// the parent directory, and so leaves the target.
	viaLink := craft(t, []domain.BackupFile{directory, {Path: "deployment/a", Link: ".."}, {Path: "deployment/b", Link: "a/../../etc/passwd"}}, nil)
	if report := (Local{}).RestoreBackup(viaLink, passphrase, filepath.Join(t.TempDir(), "restored")); report.State == "completed" {
		t.Fatal("a link leaving the target through another link was restored")
	}
	looping := craft(t, []domain.BackupFile{directory, {Path: "deployment/x", Link: "y"}, {Path: "deployment/y", Link: "x"}}, nil)
	if report := (Local{}).RestoreBackup(looping, passphrase, filepath.Join(t.TempDir(), "restored")); report.State == "completed" {
		t.Fatal("a link loop was restored")
	}
	inside := craft(t, []domain.BackupFile{directory, {Path: "deployment/link", Link: "real"}, regular("deployment/real")}, map[string]string{"deployment/real": content})
	target := filepath.Join(t.TempDir(), "restored")
	if report := (Local{}).RestoreBackup(inside, passphrase, target); report.State != "completed" {
		t.Fatalf("a relative link inside the tree was refused: %+v", report)
	}
	if data, err := os.ReadFile(filepath.Join(target, "deployment", "link")); err != nil || string(data) != "payload" {
		t.Fatalf("link = %q %v", data, err)
	}
}
