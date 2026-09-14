package adapters

import (
	"context"
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

func TestPrepareUpdateUsesExternalCandidateLockAndRepresentativeBuilds(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", "{\n  inputs.nixorium.url = \"github:owner/project/v1.0.0\";\n}\n")
	writeGitReviewFile(t, repository, "flake.lock", `{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"1111111111111111111111111111111111111111"}}}}`+"\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "nix.log")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$NIXORIUM_TEST_NIX_LOG"
case " $* " in
  *" flake lock "*)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --output-lock-file ]; then
        printf '%s\n' '{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"2222222222222222222222222222222222222222"}}}}' > "$2"
        exit 0
      fi
      shift
    done
    exit 2
    ;;
  *"#labMeta "*)
    printf '%s\n' '{"schemaVersion":2,"controller":{"name":"pc99"},"clients":{"count":1,"hosts":[{"name":"pc01","ip":"10.0.0.1"}]}}'
    ;;
  *"#deploymentStatus "*)
    printf '%s\n' '{"ready":true,"issues":[]}'
    ;;
  *" build "*) exit 0 ;;
  *) exit 3 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "nix"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NIXORIUM_TEST_NIX_LOG", logPath)
	proposal, err := (Local{}).PrepareUpdate(context.Background(), repository, "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Checks) != 7 || !strings.Contains(string(proposal.FlakeContent), "/v1.1.0") || !strings.Contains(string(proposal.LockContent), strings.Repeat("2", 40)) || !strings.Contains(proposal.Diff.Content, "flake.nix") || !strings.Contains(proposal.Diff.Content, "flake.lock") {
		t.Fatalf("proposal = %+v", proposal)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 8 || strings.Count(string(log), " build ") != 5 || strings.Count(string(log), "--no-link") != 5 || strings.Count(string(log), "--reference-lock-file") != 7 || strings.Count(string(log), "--no-write-lock-file") != 7 {
		t.Fatalf("unexpected Nix invocations (%d):\n%s", len(lines), log)
	}
	if strings.Contains(string(log), repository+"/secret-key") {
		t.Fatalf("private path entered Nix arguments:\n%s", log)
	}
}
