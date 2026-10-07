package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantPrivateTokenAndIndependentConsent(t *testing.T) {
	store := AssistantStateStore{Directory: t.TempDir()}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	if token, err := store.ReadToken(); err != nil || token != "" {
		t.Fatalf("missing: %q %v", token, err)
	}
	token := strings.Repeat("t", 43)
	if err := store.WriteToken(token); err != nil {
		t.Fatal(err)
	}
	if saved, err := store.ReadToken(); err != nil || saved != token {
		t.Fatal("token did not round trip")
	}
	if accepted, err := store.ReadConsent(); err != nil || accepted {
		t.Fatal("activation inherited consent")
	}
	if err := store.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	if accepted, err := store.ReadConsent(); err != nil || !accepted {
		t.Fatal("consent not saved")
	}
	for _, name := range []string{"token", "consent.json"} {
		info, err := os.Stat(filepath.Join(store.Directory, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe %s", name)
		}
	}
	if err := store.WriteToken(strings.Repeat("u", 43)); err != nil {
		t.Fatal(err)
	}
	if accepted, _ := store.ReadConsent(); accepted {
		t.Fatal("new activation reused old consent")
	}
	if err := store.DeleteToken(); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteToken(); err != nil {
		t.Fatal(err)
	}
	if saved, _ := store.ReadToken(); saved != "" {
		t.Fatal("token not deleted")
	}
	entries, _ := os.ReadDir(store.Directory)
	if len(entries) != 0 {
		t.Fatal("temporary state files left behind")
	}
}

func TestAssistantStateRefusesSymlinksAndUnsafePermissions(t *testing.T) {
	store := AssistantStateStore{Directory: t.TempDir()}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte(strings.Repeat("t", 43)), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory, "token")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadToken(); err == nil {
		t.Fatal("read followed symlink")
	}
	if err := store.WriteToken(strings.Repeat("u", 43)); err == nil {
		t.Fatal("write replaced symlink")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("t", 43)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadToken(); err == nil {
		t.Fatal("public token accepted")
	}
	if err := os.Chmod(store.Directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadConsent(); err == nil {
		t.Fatal("public directory accepted")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != strings.Repeat("t", 43) {
		t.Fatal("outside file changed")
	}
}
