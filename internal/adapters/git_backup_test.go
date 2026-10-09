package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gitBackupFixture(t *testing.T) (GitBackup, string, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	installKeyTestCommands(t)
	repo := t.TempDir()
	workspaceTestGit(t, repo, "init", "-q")
	workspaceTestGit(t, repo, "config", "user.name", "Test")
	workspaceTestGit(t, repo, "config", "user.email", "test@example.invalid")
	for name, data := range map[string]string{"flake.nix": "{}\n", "flake.lock": "{}\n", ".gitignore": "secret-key\nadmin-ssh\nlab-credentials.json\n"} {
		writeGitReviewFile(t, repo, name, data)
	}
	if err := (Local{}).WriteSettings(repo, adapterSettings()); err != nil {
		t.Fatal(err)
	}
	if err := (Local{}).ReconcileKeyMaterial(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	workspaceTestGit(t, repo, "add", ".")
	workspaceTestGit(t, repo, "commit", "-m", "fixture")
	remote := filepath.Join(t.TempDir(), "remote.git")
	workspaceTestGit(t, repo, "init", "--bare", remote)
	return GitBackup{allowLocal: true}, repo, remote
}

func TestGitBackupRoundTripAndReminder(t *testing.T) {
	g, repo, remote := gitBackupFixture(t)
	// Restore must protect clear credentials even when the source repository
	// relied on a local Git exclusion instead of its tracked ignore file.
	writeGitReviewFile(t, repo, ".gitignore", "secret-key\nadmin-ssh\n")
	writeGitReviewFile(t, repo, ".git/info/exclude", "lab-credentials.json\n")
	workspaceTestGit(t, repo, "add", ".gitignore")
	workspaceTestGit(t, repo, "commit", "-m", "use local credential exclusion")
	if _, due := (Local{}).BackupDue(repo); !due {
		t.Fatal("missing backup accepted")
	}
	trust := filepath.Join(os.Getenv("HOME"), ".ssh", "nixorium-known-hosts")
	if err := os.MkdirAll(trust, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trust, "known_hosts"), []byte("pc01 ssh-ed25519 trusted\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := g.Plan(t.Context(), repo, remote, "main")
	if err != nil {
		t.Fatal(err)
	}
	pass := []byte("correct horse battery")
	result := g.Publish(t.Context(), p, pass)
	if result.HasErrors() {
		t.Fatalf("publish: %+v", result)
	}
	if reason, due := (Local{}).BackupDue(repo); due {
		t.Fatal(reason)
	}
	if status := workspaceTestGit(t, repo, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatal(status)
	}
	if head := strings.TrimSpace(workspaceTestGit(t, repo, "ls-remote", remote, "refs/heads/main")); !strings.HasPrefix(head, result.Revision+"\t") {
		t.Fatal(head)
	}
	ciphertext, err := os.ReadFile(filepath.Join(repo, gitRecoveryFile))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, pass) || bytes.Contains(ciphertext, []byte("private-cache-key")) {
		t.Fatal("plaintext recovery data")
	}
	p, err = g.Plan(t.Context(), repo, remote, "main")
	if err != nil {
		t.Fatal(err)
	}
	if retry := g.Publish(t.Context(), p, pass); retry.HasErrors() || retry.Revision != result.Revision {
		t.Fatalf("retry: %+v", retry)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if bad := g.Restore(t.Context(), remote, "main", target, []byte("wrong")); !bad.HasErrors() {
		t.Fatal("wrong passphrase accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("failed restore published directory")
	}
	t.Setenv("HOME", t.TempDir())
	emptyTrust := filepath.Join(os.Getenv("HOME"), ".ssh", "nixorium-known-hosts")
	if err := os.MkdirAll(emptyTrust, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyTrust, "known_hosts"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if restored := g.Restore(t.Context(), remote, "main", target, pass); restored.HasErrors() {
		t.Fatalf("restore: %+v", restored)
	}
	if status := workspaceTestGit(t, target, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatal("restore exposed private files to Git", status)
	}
	for _, name := range privateDeploymentPaths {
		a, _ := os.ReadFile(filepath.Join(repo, name))
		b, _ := os.ReadFile(filepath.Join(target, name))
		if !bytes.Equal(a, b) {
			t.Fatalf("key %s changed", name)
		}
		info, _ := os.Stat(filepath.Join(target, name))
		if info.Mode().Perm() != 0600 {
			t.Fatal("unsafe private mode")
		}
	}
	if content, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", "nixorium-known-hosts", "known_hosts")); err != nil || !strings.Contains(string(content), "pc01") {
		t.Fatal("trusted key not restored", err)
	}
	if err := os.WriteFile(filepath.Join(emptyTrust, "known_hosts"), []byte("different client identity"), 0600); err != nil {
		t.Fatal(err)
	}
	conflictTarget := filepath.Join(t.TempDir(), "conflicting-trust")
	if report := g.Restore(t.Context(), remote, "main", conflictTarget, pass); !report.HasErrors() {
		t.Fatal("different trust overwritten")
	}
	if _, err := os.Stat(conflictTarget); !os.IsNotExist(err) {
		t.Fatal("conflicting trust published a target")
	}
	if second := g.Restore(t.Context(), remote, "main", target, pass); !second.HasErrors() {
		t.Fatal("existing deployment replaced")
	}
	if err := os.WriteFile(filepath.Join(repo, "secret-key"), []byte("rotated"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, due := (Local{}).BackupDue(repo); !due {
		t.Fatal("rotation did not require backup")
	}
}

func TestGitBackupRefusesStaleDirtyAndLeakedHistory(t *testing.T) {
	g, repo, remote := gitBackupFixture(t)
	p, err := g.Plan(t.Context(), repo, remote, "main")
	if err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repo, "extra.nix", "{}")
	if _, err := g.Plan(t.Context(), repo, remote, "main"); err == nil {
		t.Fatal("untracked file ignored")
	}
	workspaceTestGit(t, repo, "add", "extra.nix")
	workspaceTestGit(t, repo, "commit", "-m", "change")
	if report := g.Publish(t.Context(), p, []byte("correct horse battery")); !report.HasErrors() {
		t.Fatal("stale plan pushed")
	}
	workspaceTestGit(t, repo, "add", "-f", "secret-key")
	workspaceTestGit(t, repo, "commit", "-m", "bad historical key")
	workspaceTestGit(t, repo, "rm", "--cached", "secret-key")
	workspaceTestGit(t, repo, "commit", "-m", "remove key")
	if _, err := g.Plan(t.Context(), repo, remote, "main"); err == nil || !strings.Contains(err.Error(), "history") {
		t.Fatal("historical key accepted", err)
	}
}

func TestGitBackupRemoteFailureDoesNotRecordSuccess(t *testing.T) {
	g, repo, _ := gitBackupFixture(t)
	remote := filepath.Join(t.TempDir(), "missing.git")
	p, err := g.Plan(t.Context(), repo, remote, "main")
	if err != nil {
		t.Fatal(err)
	}
	if result := g.Publish(t.Context(), p, []byte("correct horse battery")); !result.HasErrors() {
		t.Fatal("missing remote accepted")
	}
	if _, due := (Local{}).BackupDue(repo); !due {
		t.Fatal("failed push recorded")
	}
	if _, err := os.Stat(filepath.Join(repo, gitRecoveryFile)); err != nil {
		t.Fatal("recoverable ciphertext lost")
	}
}

func TestGitBackupRemoteValidationAndUnsafeRecovery(t *testing.T) {
	g := GitBackup{}
	for _, url := range []string{"/tmp/backup.git", "https://token@github.com/org/repo", "ext::command", "ssh://git:password@github.com/org/repo", "git@host:repo\ncommand"} {
		if err := g.validateDestination(url, "main"); err == nil {
			t.Fatalf("accepted %q", url)
		}
	}
	for _, url := range []string{"git@github.com:org/repo.git", "git@gitlab.com:org/repo.git", "ssh://git@gitlab.school:2222/lab/repo.git"} {
		if err := g.validateDestination(url, "main"); err != nil {
			t.Fatal(err)
		}
	}
	r := gitRecovery{SchemaVersion: 1, Keys: map[string][]byte{"secret-key": []byte("a"), "admin-ssh": []byte("b")}, KnownHosts: map[string][]byte{"../escape": []byte("bad")}}
	pass := []byte("correct horse battery")
	encrypted, err := encryptedRecovery(r, pass)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptRecovery(encrypted, pass); err == nil {
		t.Fatal("traversal accepted")
	}
}
