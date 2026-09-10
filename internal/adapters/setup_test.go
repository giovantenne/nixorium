package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyMaterialRequiresPrivateModesAndNoSymlinks(t *testing.T) {
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, "keys"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]os.FileMode{
		"secret-key": 0600,
		filepath.Join("keys", "cache-public-key"): 0644,
		"admin-ssh":                                   0644,
		filepath.Join("keys", "admin-ssh.pub"):        0644,
		"veyon-private-key.pem":                       0600,
		filepath.Join("keys", "veyon-public-key.pem"): 0644,
	}
	for path, mode := range files {
		if err := os.WriteFile(filepath.Join(repository, path), []byte("key"), mode); err != nil {
			t.Fatal(err)
		}
	}
	states := (Local{}).KeyMaterial(repository)
	if len(states) != 3 || !states[0].Ready() || states[1].Ready() || !states[2].Ready() {
		t.Fatalf("states = %+v", states)
	}

	target := filepath.Join(repository, "actual-public-key")
	if err := os.WriteFile(target, []byte("key"), 0644); err != nil {
		t.Fatal(err)
	}
	publicPath := filepath.Join(repository, "keys", "cache-public-key")
	if err := os.Remove(publicPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, publicPath); err != nil {
		t.Fatal(err)
	}
	states = (Local{}).KeyMaterial(repository)
	if states[0].PublicPresent || states[0].Problem == "" {
		t.Fatalf("symlink public key was accepted: %+v", states[0])
	}
}
