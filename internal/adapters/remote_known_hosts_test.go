package adapters

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestVerifiedKnownHostRotationPreservesAliasesHashedEntriesAndBackup(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	knownHostsPath := filepath.Join(directory, "known_hosts")
	backupRoot := filepath.Join(directory, "backups")
	oldKey := testKnownHostKey(t)
	newKey := testKnownHostKey(t)
	host := "10.0.0.1"
	hashedHost := knownhosts.HashHostname(host)
	original := "other.example," + host + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(oldKey))) + " shared\n" +
		hashedHost + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(oldKey))) + " hashed\n" +
		"unrelated.example " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(oldKey))) + " keep\n"
	if err := os.WriteFile(knownHostsPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	newLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(newKey)))
	conflict, err := KnownHostConflict(knownHostsPath, host, newLine)
	if err != nil || !conflict {
		t.Fatalf("conflict=%t error=%v", conflict, err)
	}
	operationID := "0123456789abcdef0123456789abcdef"
	if err := MergeVerifiedKnownHost(knownHostsPath, backupRoot, host, newLine, operationID, false); err == nil {
		t.Fatal("unreviewed known-host rotation succeeded")
	}
	if err := MergeVerifiedKnownHost(knownHostsPath, backupRoot, host, newLine, operationID, true); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, "other.example "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(oldKey)))) ||
		!strings.Contains(text, "unrelated.example ") || !strings.Contains(text, host+" "+newLine) ||
		strings.Contains(text, hashedHost) {
		t.Fatalf("unexpected merged known-hosts:\n%s", text)
	}
	backup, err := os.ReadFile(filepath.Join(backupRoot, "known_hosts-"+operationID+".bak"))
	if err != nil || string(backup) != original {
		t.Fatalf("backup=%q error=%v", backup, err)
	}
	conflict, err = KnownHostConflict(knownHostsPath, host, newLine)
	if err != nil || conflict {
		t.Fatalf("post-merge conflict=%t error=%v", conflict, err)
	}
}

func testKnownHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	return key
}
