package adapters

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// This test also runs in the management VM, using its actual unprivileged Git,
// Nix daemon and filesystem. Ordinary package tests never require a Nix store.
func TestWorkspaceRealNixSave(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKSPACE_NIX") != "1" {
		t.Skip("requires explicit real-Nix integration environment")
	}
	repository := workspaceRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", workspaceIntegrationFlake)
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; version = "1.0"; }`)
	writeGitReviewFile(t, repository, "secret-key", "ignored-private-sentinel")
	workspaceTestGit(t, repository, "add", "flake.nix", "policy.nix")
	workspaceTestGit(t, repository, "commit", "-qm", "workspace fixture")
	// An unrelated staged edit must survive both review and save unchanged.
	writeGitReviewFile(t, repository, "module.nix", "# unrelated operator edit\n")
	workspaceTestGit(t, repository, "add", "module.nix")
	indexBefore, err := os.ReadFile(filepath.Join(repository, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	lockBefore := readGitReviewFile(t, repository, "flake.lock")
	manager := app.NewWorkspaceManager(Local{})
	candidate := []byte(`{"schemaVersion":1,"desktop":{"colorScheme":"dark"}}`)
	plan := manager.Plan(t.Context(), repository, candidate)
	if plan.HasErrors() || plan.State != "ready" || plan.Inspection.Base != nil {
		// Only this disposable fixture may print evaluator diagnostics; the
		// production report must not expose arbitrary private deployment traces.
		flake, _ := deploymentFlakeReference(repository)
		command := exec.CommandContext(t.Context(), "nix", "--extra-experimental-features", "nix-command flakes", "eval", "--impure", "--json", "--no-write-lock-file", "--no-update-lock-file", "--expr", workspaceCandidateExpression)
		command.Env = append(workspaceEnvironment(), "NIXORIUM_DEPLOYMENT_FLAKE="+flake, "NIXORIUM_WORKSPACE_CANDIDATE="+string(candidate))
		output, _ := command.CombinedOutput()
		t.Logf("fixture evaluation: %s", output)
		t.Fatalf("first candidate: %+v", plan)
	}
	if _, err := os.Lstat(filepath.Join(repository, domain.WorkspaceFileName)); !os.IsNotExist(err) {
		t.Fatalf("review wrote a profile: %v", err)
	}
	result := manager.Apply(t.Context(), repository, candidate, plan.ReviewToken)
	if result.HasErrors() || result.State != "saved" {
		t.Fatalf("save: %+v", result)
	}
	indexAfter, _ := os.ReadFile(filepath.Join(repository, ".git/index"))
	if string(indexBefore) != string(indexAfter) || lockBefore != readGitReviewFile(t, repository, "flake.lock") {
		t.Fatal("review/save changed the index or lock")
	}
	if workspaceTestGit(t, repository, "ls-files", "--", domain.WorkspaceFileName) != "" {
		t.Fatal("save staged the profile")
	}
	if readGitReviewFile(t, repository, "secret-key") != "ignored-private-sentinel" {
		t.Fatal("private file changed")
	}
	if result := manager.Apply(t.Context(), repository, candidate, plan.ReviewToken); result.State != "conflict" {
		t.Fatalf("consumed first-save token was accepted: %+v", result)
	}
	current := manager.Plan(t.Context(), repository, candidate)
	if current.HasErrors() || current.State != "unchanged" {
		t.Fatalf("unchanged: %+v", current)
	}
	// Bind tracked content even when HEAD/index and resolved destinations are
	// unchanged. The source includes this unrelated, unstaged module edit.
	writeGitReviewFile(t, repository, "module.nix", "# changed after workspace review\n")
	if result := manager.Apply(t.Context(), repository, candidate, current.ReviewToken); result.State != "conflict" {
		t.Fatalf("source drift accepted: %+v", result)
	}
	// A deployment validator can impose extra policy beyond core resolution;
	// returning false must fail just as a thrown exception would.
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = false; version = "1.0"; }`)
	if rejected := manager.Plan(t.Context(), repository, candidate); !rejected.HasErrors() {
		t.Fatalf("false deployment validator ignored: %+v", rejected)
	}
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = throw "private-diagnostic-sentinel"; version = "1.0"; }`)
	if rejected := manager.Plan(t.Context(), repository, candidate); !rejected.HasErrors() || strings.Contains(rejected.Issues[0].Message, "private-diagnostic-sentinel") {
		t.Fatalf("validator failure ignored/leaked: %+v", rejected)
	}
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; version = "2.0"; }`)
	// Test the last locked recheck independently of the application's fresh
	// evaluation: a candidate validated earlier cannot authorize a stale write.
	next := []byte(`{"schemaVersion":1,"desktop":{"colorScheme":"light"}}`)
	fresh := manager.Plan(t.Context(), repository, next)
	if fresh.HasErrors() {
		t.Fatalf("fresh review: %+v", fresh)
	}
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; version = "3.0"; }`)
	err = (Local{}).WriteWorkspaceIfUnchanged(t.Context(), repository, fresh.Inspection.Snapshot, *fresh.Candidate)
	if err == nil {
		t.Fatal("writer accepted stale source")
	}
	stored := readGitReviewFile(t, repository, domain.WorkspaceFileName)
	if !strings.Contains(stored, "dark") {
		t.Fatal("stale writer replaced profile")
	}
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; version = "2.0"; }`)
	// Also exercise replacement when the managed file is tracked by Git.
	workspaceTestGit(t, repository, "add", domain.WorkspaceFileName)
	fresh = manager.Plan(t.Context(), repository, next)
	if fresh.HasErrors() {
		t.Fatalf("tracked profile: %+v", fresh)
	}
	result = manager.Apply(t.Context(), repository, next, fresh.ReviewToken)
	if result.HasErrors() || result.State != "saved" {
		t.Fatalf("tracked replacement: %+v", result)
	}
	if !strings.Contains(readGitReviewFile(t, repository, domain.WorkspaceFileName), "light") {
		t.Fatal("replacement missing")
	}
	assertWorkspaceDraftsRemoved(t, repository)
}

const workspaceIntegrationFlake = `{
  outputs = { self }:
    let policy = import ./policy.nix; in
    assert !(builtins.pathExists (./. + "/secret-key"));
    {
      nixoriumValidateWorkspaceCandidate = raw: policy.allow;
      nixoriumResolveWorkspaceCandidate = raw: {
        schemaVersion = 1;
        state = "prepared";
        managedFile = "workspace-profile.json";
        studentUser = "learner";
        seed = "/nix/store/00000000000000000000000000000000-home";
        declared = builtins.fromJSON raw;
        effective = builtins.fromJSON raw;
        catalog = {
          schemaVersion = 1;
          baseline = { schemaVersion = 1; };
          applications = [];
          extensions = [];
        };
        requiredPackages = [ "gnome-shell" ];
        packages = [ { package = "gnome-shell"; version = policy.version; } ];
        extensions = [];
        targets = [ { name = "controller"; role = "controller"; } ];
      };
    };
}
`
