package adapters

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestBackupAccessKeyIsCreatedOnceAndPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, err := BackupAccessKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "ssh-ed25519 ") || !strings.HasSuffix(first, " nixorium-backup") {
		t.Fatalf("public key = %q", first)
	}
	second, err := BackupAccessKey()
	if err != nil || second != first {
		t.Fatalf("key changed on second use: %q %v", second, err)
	}
	info, err := os.Stat(filepath.Join(home, ".ssh", "nixorium-backup"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private key mode: %v %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(home, ".ssh")); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("ssh directory mode: %v %v", info, err)
	}
	// A lost public half is derived again from the same private key.
	if err := os.Remove(filepath.Join(home, ".ssh", "nixorium-backup.pub")); err != nil {
		t.Fatal(err)
	}
	if again, err := BackupAccessKey(); err != nil || again != first {
		t.Fatalf("public key not derived again: %q %v", again, err)
	}
}

func TestBackupSSHCommandChecksHostsAndOffersTheBackupKeyFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := BackupAccessKey(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	command := backupSSHCommand("/tmp/pinned")
	for _, want := range []string{"-oBatchMode=yes", "-oStrictHostKeyChecking=yes", "'-oUserKnownHostsFile=" + home + "/.ssh/known_hosts " + home + "/.ssh/nixorium-git-hosts /tmp/pinned'"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command lacks %q: %s", want, command)
		}
	}
	backup := strings.Index(command, "nixorium-backup'")
	own := strings.Index(command, "id_ed25519'")
	if backup < 0 || own < 0 || backup > own || strings.Contains(command, "id_rsa") {
		t.Fatalf("key order: %s", command)
	}
	for _, line := range pinnedGitHosts {
		if !strings.HasPrefix(line, "github.com ") && !strings.HasPrefix(line, "gitlab.com ") {
			t.Fatalf("unexpected pinned host: %s", line)
		}
	}
}

func TestExplainBackupGitErrorNamesTheFix(t *testing.T) {
	for message, want := range map[string]string{
		"git: No ED25519 host key is known for github.com and you have requested strict checking.\nHost key verification failed.": "not verified the Git server",
		"git: git@github.com: Permission denied (publickey).":                                                                     "with write access",
		"git: ERROR: The key you are authenticating with has been marked as read only.":                                           "cannot write",
		"git: ERROR: Repository not found.\nfatal: Could not read from remote repository.":                                        "not found",
		"git: ssh: Could not resolve hostname github.com: Name or service not known":                                              "cannot be reached",
		"git: ! [rejected] main -> main (fetch first)":                                                                            "different history",
		"git: remote: error: GH006: Protected branch update failed for refs/heads/main.":                                          "branch is protected",
		"git: error: unable to create file: Read-only file system":                                                                "Git said: error: unable to create file",
		"git: something else": "Git said: something else",
	} {
		if got := explainBackupGitError(message).Error(); !strings.Contains(got, want) {
			t.Errorf("%q -> %q, want %q", message, got, want)
		}
	}
}

// gitHostFixture serves SSH handshakes on loopback with one host key.
func gitHostFixture(t *testing.T) (remote string, key ssh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
		return nil, errors.New("no access")
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, _, _, _ = ssh.NewServerConn(connection, config)
			}()
		}
	}()
	return "ssh://git@" + listener.Addr().String() + "/lab.git", signer.PublicKey()
}

func TestUnknownGitHostIsTrustedOnlyWithTheComparedFingerprint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	remote, key := gitHostFixture(t)
	unknown, err := scanGitHost(t.Context(), remote)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Algorithm != "ED25519" || unknown.Fingerprint != ssh.FingerprintSHA256(key) || !strings.HasPrefix(unknown.Host, "127.0.0.1:") {
		t.Fatalf("scan = %+v", unknown)
	}
	var asError error = unknown
	if !errors.As(asError, &domain.UnknownGitHostError{}) || !strings.Contains(asError.Error(), "trust-host") {
		t.Fatalf("error = %v", asError)
	}
	if err := TrustGitHost(t.Context(), remote, "SHA256:another"); err == nil {
		t.Fatal("a different fingerprint was trusted")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", gitHostsFileName)); !os.IsNotExist(err) {
		t.Fatal("refused trust wrote the hosts file")
	}
	if err := TrustGitHost(t.Context(), remote, unknown.Fingerprint); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", gitHostsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "[127.0.0.1]:") || !strings.Contains(string(data), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))) {
		t.Fatalf("hosts file = %q", data)
	}
	if info, err := os.Stat(filepath.Join(home, ".ssh", gitHostsFileName)); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("hosts file mode: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "known_hosts")); !os.IsNotExist(err) {
		t.Fatal("known_hosts was written")
	}
	if err := TrustGitHost(t.Context(), remote, unknown.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(filepath.Join(home, ".ssh", gitHostsFileName)); string(again) != string(data) {
		t.Fatal("trusting twice added a second line")
	}
	if err := TrustGitHost(t.Context(), "ssh://git@*.example/lab.git", unknown.Fingerprint); err == nil {
		t.Fatal("a host pattern was accepted")
	}
	if command := backupSSHCommand("/tmp/pinned"); !strings.Contains(command, filepath.Join(home, ".ssh", gitHostsFileName)) {
		t.Fatalf("trusted hosts not consulted: %s", command)
	}
}

func TestUnknownHostErrorComesFromHostVerification(t *testing.T) {
	err := explainBackupGitError("git: No ED25519 host key is known for git.school and you have requested strict checking.\nHost key verification failed.")
	if !errors.Is(err, errUnknownGitHost) {
		t.Fatalf("error = %v", err)
	}
	if errors.Is(explainBackupGitError("git: WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!\nHost key verification failed."), errUnknownGitHost) {
		t.Fatal("a changed key was offered for trust")
	}
}

func TestRealSSHAcceptsATrustedGitHostOnlyAfterTrust(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh is not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := BackupAccessKey(); err != nil {
		t.Fatal(err)
	}
	remote, _ := gitHostFixture(t)
	err := checkBackupAccess(t.Context(), remote, "main")
	var unknown domain.UnknownGitHostError
	if !errors.As(err, &unknown) {
		t.Fatalf("unknown server not offered for trust: %v", err)
	}
	if err := TrustGitHost(t.Context(), remote, unknown.Fingerprint); err != nil {
		t.Fatal(err)
	}
	// The host key is now accepted; the fixture refuses every key.
	err = checkBackupAccess(t.Context(), remote, "main")
	if err == nil || !strings.Contains(err.Error(), "refused this controller's backup key") {
		t.Fatalf("after trust: %v", err)
	}
}

func TestChangedGitHostKeyNamesTheFileToFix(t *testing.T) {
	message := "git: @@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@\nOffending ED25519 key in /home/admin/.ssh/nixorium-git-hosts:2\nHost key verification failed."
	got := explainBackupGitError(message).Error()
	if !strings.Contains(got, "ssh-keygen -R <server> -f /home/admin/.ssh/nixorium-git-hosts") || errors.Is(explainBackupGitError(message), errUnknownGitHost) {
		t.Fatalf("changed key message = %q", got)
	}
}
