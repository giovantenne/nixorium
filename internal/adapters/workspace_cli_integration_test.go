package adapters

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWorkspaceRealCLI(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKSPACE_CLI") != "1" {
		t.Skip("requires the packaged CLI in the management VM")
	}
	repository := workspaceRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", workspaceIntegrationFlake)
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; version = "1.0"; }`)
	workspaceTestGit(t, repository, "add", "flake.nix", "policy.nix")
	workspaceTestGit(t, repository, "commit", "-qm", "workspace CLI fixture")
	candidate := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(candidate, []byte(`{"schemaVersion":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(arguments ...string) ([]byte, error) {
		t.Helper()
		command := exec.CommandContext(t.Context(), "nixorium", append(arguments, "--repo", repository, "--file", candidate, "--json")...)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		t.Logf("CLI %v: %s", arguments, &stderr)
		return stdout.Bytes(), err
	}
	data, err := run("workspace", "plan")
	var plan domain.WorkspacePlanReport
	if err != nil || json.Unmarshal(data, &plan) != nil || plan.HasErrors() || plan.ReviewToken == "" {
		t.Fatalf("CLI plan: %s (%v)", data, err)
	}
	if _, err := os.Lstat(filepath.Join(repository, domain.WorkspaceFileName)); !os.IsNotExist(err) {
		t.Fatal("CLI plan wrote the profile")
	}
	if _, err := run("workspace", "apply", "--expect", plan.ReviewToken); err == nil {
		t.Fatal("noninteractive apply did not require --yes")
	}
	if _, err := os.Lstat(filepath.Join(repository, domain.WorkspaceFileName)); !os.IsNotExist(err) {
		t.Fatal("unconfirmed apply wrote the profile")
	}
	data, err = run("workspace", "apply", "--expect", "stale", "--yes")
	var result domain.WorkspaceApplyReport
	if err == nil || json.Unmarshal(data, &result) != nil || result.State != "conflict" {
		t.Fatalf("CLI stale review: %s (%v)", data, err)
	}
	indexBefore, err := os.ReadFile(filepath.Join(repository, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	data, err = run("workspace", "apply", "--expect", plan.ReviewToken, "--yes")
	if err != nil || json.Unmarshal(data, &result) != nil || result.State != "saved" || result.HasErrors() {
		t.Fatalf("CLI save: %s (%v)", data, err)
	}
	indexAfter, _ := os.ReadFile(filepath.Join(repository, ".git/index"))
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("CLI save changed the Git index")
	}
	if _, issues := domain.DecodeWorkspaceProfile([]byte(readGitReviewFile(t, repository, domain.WorkspaceFileName))); len(issues) != 0 {
		t.Fatalf("CLI wrote an invalid profile: %v", issues)
	}
	data, err = run("workspace", "apply", "--expect", plan.ReviewToken, "--yes")
	if err == nil || json.Unmarshal(data, &result) != nil || result.State != "conflict" {
		t.Fatalf("CLI reused consumed first-save token: %s (%v)", data, err)
	}
}
