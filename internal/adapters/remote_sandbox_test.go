package adapters

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Run in the real worker filesystem sandbox, not in the ordinary unit suite.
// Only disposable fixture files are written, so this also supports local checks.
func TestRemoteWorkerFilesystemSandbox(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKER_SANDBOX") != "1" {
		t.Skip("requires the worker systemd sandbox")
	}
	for _, path := range []string{"/home/admin/.ssh", "/home/admin/.local/state", "/home/admin/.local/state/nixorium"} {
		file, err := os.CreateTemp(path, ".nixorium-sandbox-denied-*")
		if err == nil {
			file.Close()
			os.Remove(file.Name())
			t.Fatalf("sandbox unexpectedly permits creating files in %s", path)
		}
	}
	for _, path := range []string{"/home/admin/.ssh/id_ed25519", "/home/admin/.ssh/id_ed25519.pub"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			file.Close()
			t.Fatalf("sandbox permits writing %s", path)
		}
	}
	fixture, err := os.CreateTemp(filepath.Dir(ManagedKnownHostsPath), ".sandbox-known-hosts-*")
	if err != nil {
		t.Fatal(err)
	}
	path := fixture.Name()
	fixture.Close()
	defer os.Remove(path)
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(randomID[:])
	key := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(testKnownHostKey(t))))
	backupRoot := t.TempDir()
	if err := os.Chmod(backupRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := MergeVerifiedKnownHost(path, backupRoot, "192.0.2.1", key, id, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "192.0.2.1 "+key) {
		t.Fatal("host merge was not published")
	}
	name, err := PublishRemoteOperationLog(id, []byte("sandbox regression check\n"), "completed")
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(os.Getenv("XDG_STATE_HOME"), "nixorium", "operations", name)
	defer os.Remove(logPath)
	content, err = os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(content), "Result: completed") {
		t.Fatal("operation log was not published")
	}
}
