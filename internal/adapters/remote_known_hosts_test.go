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

func TestVerifiedKnownHostRetryPreservesExistingBackup(t *testing.T) {
	for _, scenario := range []string{"empty", "matching", "different", "symlink", "public"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "known_hosts")
			backupRoot := filepath.Join(directory, "backups")
			if err := os.Mkdir(backupRoot, 0700); err != nil {
				t.Fatal(err)
			}
			id := "0123456789abcdef0123456789abcdef"
			backup := filepath.Join(backupRoot, "known_hosts-"+id+".bak")
			content := []byte("# retained original\n")
			if scenario == "empty" {
				content = []byte{}
			}
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
			saved := content
			if scenario == "different" {
				saved = []byte("different recovery evidence\n")
			}
			if scenario == "symlink" {
				if err := os.Symlink(path, backup); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(backup, saved, 0600); err != nil {
					t.Fatal(err)
				}
				if scenario == "public" {
					if err := os.Chmod(backup, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			key := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(testKnownHostKey(t))))
			err := MergeVerifiedKnownHost(path, backupRoot, "10.0.0.1", key, id, false)
			wantSuccess := scenario == "empty" || scenario == "matching"
			if (err == nil) != wantSuccess {
				t.Fatalf("retry error = %v", err)
			}
			if !wantSuccess {
				current, readErr := os.ReadFile(path)
				if readErr != nil || string(current) != string(content) {
					t.Fatal("failed retry changed known_hosts")
				}
			}
			retained, readErr := os.ReadFile(backup)
			if readErr != nil || string(retained) != string(saved) {
				t.Fatal("retry changed recovery evidence")
			}
		})
	}
}
