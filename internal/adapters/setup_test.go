package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyMaterialRequiresPrivateModesAndNoSymlinks(t *testing.T) {
	installKeyTestCommands(t)
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
	states := (Local{}).KeyMaterial(context.Background(), repository)
	if len(states) != 3 || states[0].Ready() || states[1].Ready() || states[2].Ready() {
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
	states = (Local{}).KeyMaterial(context.Background(), repository)
	if states[0].PublicPresent || states[0].Problem == "" {
		t.Fatalf("symlink public key was accepted: %+v", states[0])
	}
}

func TestReconcileKeyMaterialCreatesVerifiesAndReusesPairs(t *testing.T) {
	installKeyTestCommands(t)
	repository := t.TempDir()
	local := Local{}

	if err := local.ReconcileKeyMaterial(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	states := local.KeyMaterial(context.Background(), repository)
	if len(states) != 3 {
		t.Fatalf("states = %+v", states)
	}
	for _, state := range states {
		if !state.Ready() || state.PrivateMode != 0600 {
			t.Fatalf("key state = %+v", state)
		}
	}

	paths := []string{
		"secret-key", filepath.Join("keys", "cache-public-key"),
		"admin-ssh", filepath.Join("keys", "admin-ssh.pub"),
		"veyon-private-key.pem", filepath.Join("keys", "veyon-public-key.pem"),
	}
	original := map[string]string{}
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(repository, path))
		if err != nil {
			t.Fatal(err)
		}
		original[path] = string(content)
	}
	if err := local.ReconcileKeyMaterial(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(repository, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != original[path] {
			t.Fatalf("%s changed during idempotent reconciliation", path)
		}
	}

	privatePath := filepath.Join(repository, "secret-key")
	if err := os.Chmod(privatePath, 0644); err != nil {
		t.Fatal(err)
	}
	if err := local.ReconcileKeyMaterial(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private mode = %04o", info.Mode().Perm())
	}
}

func TestReconcileKeyMaterialRefusesMismatchesAndPublicOnlyPairs(t *testing.T) {
	installKeyTestCommands(t)
	local := Local{}
	repository := t.TempDir()
	if err := local.ReconcileKeyMaterial(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	publicPath := filepath.Join(repository, "keys", "admin-ssh.pub")
	const mismatch = "ssh-ed25519 WRONG existing\n"
	if err := os.WriteFile(publicPath, []byte(mismatch), 0644); err != nil {
		t.Fatal(err)
	}
	if err := local.ReconcileKeyMaterial(context.Background(), repository); err == nil || !strings.Contains(err.Error(), "refuse to replace mismatched ssh public key") {
		t.Fatalf("mismatch error = %v", err)
	}
	content, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != mismatch {
		t.Fatalf("mismatched public key was replaced: %q", content)
	}

	publicOnly := t.TempDir()
	if err := os.Mkdir(filepath.Join(publicOnly, "keys"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicOnly, "keys", "cache-public-key"), []byte("nixorium-cache:PUBLIC\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := local.ReconcileKeyMaterial(context.Background(), publicOnly); err == nil || !strings.Contains(err.Error(), "while a public key already exists") {
		t.Fatalf("public-only error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(publicOnly, "secret-key")); !os.IsNotExist(err) {
		t.Fatalf("private key unexpectedly created: %v", err)
	}
}

func TestKeyCommandErrorsDoNotExposeCommandOutput(t *testing.T) {
	directory := t.TempDir()
	writeExecutable(t, filepath.Join(directory, "unsafe-command"), "#!/bin/sh\nprintf 'private-material' >&2\nexit 9\n")
	t.Setenv("PATH", directory)
	_, err := runKeyCommand(context.Background(), []byte("secret input"), "unsafe-command")
	if err == nil || strings.Contains(err.Error(), "private-material") || strings.Contains(err.Error(), "secret input") {
		t.Fatalf("error leaked key material: %v", err)
	}
}

func installKeyTestCommands(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	writeExecutable(t, filepath.Join(directory, "nix"), `#!/bin/sh
case "$*" in
  *generate-secret*) printf 'nixorium-cache:PRIVATE\n' ;;
  *convert-secret-to-public*) cat >/dev/null; printf 'nixorium-cache:PUBLIC\n' ;;
  *) exit 2 ;;
esac
`)
	writeExecutable(t, filepath.Join(directory, "ssh-keygen"), `#!/bin/sh
case "$1" in
  -q)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-f" ]; then shift; target="$1"; fi
      shift
    done
    printf 'ssh-private\n' > "$target"
    chmod 600 "$target"
    printf 'ssh-ed25519 AAAATEST admin@controller\n' > "$target.pub"
    ;;
  -y) printf 'ssh-ed25519 AAAATEST\n' ;;
  *) exit 2 ;;
esac
`)
	writeExecutable(t, filepath.Join(directory, "openssl"), `#!/bin/sh
case "$1" in
  genrsa) printf '%s\n' '-----BEGIN PRIVATE KEY-----' 'VEYONPRIVATE' '-----END PRIVATE KEY-----' ;;
  pkey) cat >/dev/null; printf '%s\n' '-----BEGIN PUBLIC KEY-----' 'VEYONPUBLIC' '-----END PUBLIC KEY-----' ;;
  *) exit 2 ;;
esac
`)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
}
