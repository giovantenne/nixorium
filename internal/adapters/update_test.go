package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectUpdateInputAndRenderTarget(t *testing.T) {
	repository := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repository, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("flake.nix", "{\n  inputs.nixorium.url = \"github:giovantenne/nixorium/v1.2.3\";\n}\n")
	write("flake.lock", `{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"0123456789012345678901234567890123456789"}}}}`)
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil || snapshot.SourcePrefix != "giovantenne/nixorium" || snapshot.CurrentRef != "v1.2.3" || len(snapshot.CurrentRev) != 40 {
		t.Fatalf("snapshot = %+v, error = %v", snapshot, err)
	}
	proposed, err := ProposedUpdateFlake(snapshot, "v1.3.0")
	if err != nil || !strings.Contains(string(proposed), "github:giovantenne/nixorium/v1.3.0") || strings.Contains(string(proposed), "v1.2.3") {
		t.Fatalf("proposed flake = %q, error = %v", proposed, err)
	}
}

func TestInspectUpdateInputRefusesAmbiguousComputedAndSymlinkSources(t *testing.T) {
	for _, content := range []string{
		"{ inputs.nixorium.url = target; }\n",
		"inputs.nixorium.url = \"github:one/nixorium/v1.0.0\";\ninputs.nixorium.url = \"github:two/nixorium/v1.0.0\";\n",
		"inputs.nixorium.url = \"path:../nixorium\";\n",
	} {
		repository := t.TempDir()
		if err := os.WriteFile(filepath.Join(repository, "flake.nix"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := (Local{}).InspectUpdateInput(repository); err == nil {
			t.Fatalf("unsafe source accepted: %q", content)
		}
	}
	repository := t.TempDir()
	target := filepath.Join(t.TempDir(), "flake.nix")
	if err := os.WriteFile(target, []byte("inputs.nixorium.url = \"github:owner/repo/v1.0.0\";\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(repository, "flake.nix")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).InspectUpdateInput(repository); err == nil {
		t.Fatal("symlinked flake.nix was accepted")
	}
}
