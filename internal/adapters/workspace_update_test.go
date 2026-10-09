package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func workspaceUpdateFixture(version string) *domain.WorkspaceResolution {
	p := domain.WorkspaceProfile{SchemaVersion: 1}
	return &domain.WorkspaceResolution{
		SchemaVersion: 1, State: "prepared", ManagedFile: domain.WorkspaceFileName,
		StudentUser: "student", Seed: "/nix/store/00000000000000000000000000000000-home", Declared: p, Effective: p,
		Catalog:    domain.WorkspaceCatalog{SchemaVersion: 1, Baseline: p},
		Packages:   []domain.WorkspacePackage{{Package: "vscode", Version: version}},
		Extensions: []domain.WorkspaceExtension{{ID: "example.extension", Version: version}},
		Targets:    []domain.WorkspaceTarget{{Name: "controller", Role: "controller"}, {Name: "pc01", Role: "client"}},
	}
}

func TestWorkspaceUpdateReadsBothPinsAndRejectsMigration(t *testing.T) {
	for _, mode := range []string{"versions", "missing", "dropped", "student", "targets", "schema", "malformed", "private-error", "validator-false"} {
		t.Run(mode, func(t *testing.T) {
			before, after := workspaceUpdateFixture("1.0"), workspaceUpdateFixture("2.0")
			switch mode {
			case "missing":
				before, after = nil, nil
			case "dropped":
				after = nil
			case "student":
				after.StudentUser = "different"
			case "targets":
				after.Targets = after.Targets[:1]
			case "schema":
				after.SchemaVersion = 2
			}
			bin := t.TempDir()
			for name, value := range map[string]*domain.WorkspaceResolution{"before": before, "after": after} {
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "malformed" && name == "after" {
					data = []byte(`{"invalid":`)
				}
				if err := os.WriteFile(filepath.Join(bin, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$NIXORIUM_WORKSPACE_TEST_BIN/calls"
case " $* " in
  *"#nixoriumValidateWorkspaceCandidate "*)
    if [ "$NIXORIUM_WORKSPACE_TEST_MODE" = validator-false ]; then printf 'false\n'; else printf 'true\n'; fi ;;
  *" --reference-lock-file "*)
    if [ "$NIXORIUM_WORKSPACE_TEST_MODE" = private-error ]; then echo 'private-sentinel' >&2; exit 1; fi
    cat "$NIXORIUM_WORKSPACE_TEST_BIN/after" ;;
  *) cat "$NIXORIUM_WORKSPACE_TEST_BIN/before" ;;
esac
`
			if err := os.WriteFile(filepath.Join(bin, "nix"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("NIXORIUM_WORKSPACE_TEST_BIN", bin)
			t.Setenv("NIXORIUM_WORKSPACE_TEST_MODE", mode)
			impact, err := inspectWorkspaceUpdate(context.Background(), "git+file:///fixture", []string{"--reference-lock-file", "/candidate-lock", "--no-write-lock-file"})
			valid := mode == "versions"
			if (err == nil) != valid {
				t.Fatalf("impact=%+v err=%v", impact, err)
			}
			if err != nil && strings.Contains(err.Error(), "private-sentinel") {
				t.Fatal("evaluator stderr exposed")
			}
			if mode == "versions" && (impact.Current.Extensions[0].Version != "1.0" || impact.Proposed.Extensions[0].Version != "2.0") {
				t.Fatal("version comparison lost")
			}
			calls, _ := os.ReadFile(filepath.Join(bin, "calls"))
			expected := 3
			if mode == "missing" {
				expected = 1
			}
			if mode == "dropped" || mode == "schema" || mode == "malformed" || mode == "private-error" {
				expected = 2
			}
			if strings.Count(string(calls), "--no-update-lock-file") != expected || strings.Count(string(calls), "--reference-lock-file") != expected-1 {
				t.Fatalf("wrong pin boundary: %s", calls)
			}
		})
	}
}

func TestWorkspaceUpdateRealNixProjection(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKSPACE_NIX") != "1" {
		t.Skip("requires explicit real-Nix integration environment")
	}
	repo := workspaceRepository(t)
	renamed := filepath.Join(t.TempDir(), "deployment & ${literal}")
	if err := os.Rename(repo, renamed); err != nil {
		t.Fatal(err)
	}
	repo = renamed
	versionInput := func(version string) string {
		directory := t.TempDir()
		if err := os.WriteFile(filepath.Join(directory, "flake.nix"), []byte(fmt.Sprintf(`{ outputs = { self }: { value = %q; }; }`, version)), 0600); err != nil {
			t.Fatal(err)
		}
		return "path:" + directory
	}
	currentInput, candidateInput := versionInput("1.0"), versionInput("2.0")
	data, err := json.Marshal(workspaceUpdateFixture("1.0"))
	if err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repo, "metadata.json", string(data))
	declaration := fmt.Sprintf(`{
  inputs.version.url = %q;
  outputs = { self, version }: assert !(builtins.pathExists (./. + "/secret-key")); {
    nixoriumWorkspace = (builtins.fromJSON (builtins.readFile ./metadata.json)) // {
      packages = [ { package = "vscode"; version = version.value; } ];
    };
    nixoriumValidateWorkspaceCandidate = raw: (builtins.fromJSON raw).schemaVersion == 1;
  };
}`, currentInput)
	writeGitReviewFile(t, repo, "flake.nix", declaration)
	writeGitReviewFile(t, repo, "secret-key", "private-sentinel")
	workspaceTestGit(t, repo, "add", "flake.nix", "metadata.json")
	flake, err := deploymentFlakeReference(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runBoundedNix(t.Context(), 64*1024, "flake", "lock", flake); err != nil {
		t.Fatal(err)
	}
	workspaceTestGit(t, repo, "add", "flake.lock")
	workspaceTestGit(t, repo, "commit", "-qm", "update workspace fixture")
	candidateLock := filepath.Join(t.TempDir(), "candidate.lock")
	if _, err := runBoundedNix(t.Context(), 64*1024, "flake", "lock", flake, "--override-input", "version", candidateInput, "--output-lock-file", candidateLock); err != nil {
		t.Fatal(err)
	}
	lock := readGitReviewFile(t, repo, "flake.lock")
	impact, err := inspectWorkspaceUpdate(t.Context(), flake, []string{"--override-input", "version", candidateInput, "--reference-lock-file", candidateLock, "--no-write-lock-file"})
	if err != nil || impact == nil || impact.Current.Packages[0].Version != "1.0" || impact.Proposed.Packages[0].Version != "2.0" {
		t.Fatalf("impact=%+v err=%v", impact, err)
	}
	if lock != readGitReviewFile(t, repo, "flake.lock") {
		t.Fatal("read-only comparison changed the pin")
	}
	writeGitReviewFile(t, repo, "flake.nix", strings.Replace(declaration, "(builtins.fromJSON raw).schemaVersion == 1", "false", 1))
	if _, err := readUpdateWorkspace(t.Context(), flake, nil); err == nil {
		t.Fatal("local validator rejection ignored")
	}
}
