package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestHostKeyFailureIsTypedWithoutConfusingOtherSSHFailures(t *testing.T) {
	for _, test := range []struct {
		output    string
		condition domain.HostKeyCondition
	}{
		{"WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!", domain.HostKeyChanged},
		{"Host key for 10.0.0.1 has changed and you have requested strict checking.", domain.HostKeyChanged},
		{"Connection refused", ""}, {"Permission denied (publickey)", ""}, {"Host key verification failed.", ""},
	} {
		if got := sshHostKeyCondition(test.output); got != test.condition {
			t.Fatalf("%q: %s", test.output, got)
		}
	}
}
func TestHostTrustInspectAndAtomicBaseRecheck(t *testing.T) {
	oldKey, newKey := testKnownHostKey(t), testKnownHostKey(t)
	address := "10.0.0.1"
	content := []byte(knownhosts.Line([]string{knownhosts.HashHostname(address)}, oldKey) + "\n" + knownhosts.Line([]string{"other.example"}, oldKey) + "\n")
	public := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(newKey)))
	review, err := inspectHostTrust(content, address, public)
	if err != nil || len(review.Recorded) != 1 || review.Recorded[0] != ssh.FingerprintSHA256(oldKey) || review.Offered != ssh.FingerprintSHA256(newKey) {
		t.Fatalf("review: %+v err=%v", review, err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "known_hosts")
	backup := filepath.Join(dir, "backups")
	id := "0123456789abcdef0123456789abcdef"
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := mergeVerifiedKnownHost(path, backup, address, public, id, true, "stale"); err == nil {
		t.Fatal("stale base accepted")
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(content) {
		t.Fatal("stale review changed trust")
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("stale review wrote a backup")
	}
	if err := mergeVerifiedKnownHost(path, backup, address, public, id, true, review.BaseFingerprint); err != nil {
		t.Fatal(err)
	}
	updated, _ := os.ReadFile(path)
	saved, _ := os.ReadFile(filepath.Join(backup, "known_hosts-"+id+".bak"))
	if !strings.Contains(string(updated), knownhosts.Line([]string{"other.example"}, oldKey)) || string(saved) != string(content) {
		t.Fatal("lost unrelated trust or original backup")
	}
}
