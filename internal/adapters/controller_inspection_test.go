package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControllerInspectionRealNixKeepsPrivateFilesOutAndRechecks(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKSPACE_NIX") != "1" {
		t.Skip("requires explicit real-Nix integration environment")
	}
	repo := workspaceRepository(t)
	renamed := filepath.Join(t.TempDir(), "deployment & ${literal}")
	if err := os.Rename(repo, renamed); err != nil {
		t.Fatal(err)
	}
	repo = renamed
	declaration := `{
  outputs = { self }: assert !(builtins.pathExists (./. + "/secret-key")); {
    labMeta.controller.name = "pc99";
    deploymentStatus = { ready = true; issues = []; };
    nixosConfigurations.pc99.config.system.build.toplevel = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-inspection-test-system";
  };
}`
	writeGitReviewFile(t, repo, "flake.nix", declaration)
	writeGitReviewFile(t, repo, "secret-key", "private-sentinel")
	workspaceTestGit(t, repo, "add", "flake.nix")
	workspaceTestGit(t, repo, "commit", "-qm", "controller inspection fixture")
	local := Local{}
	inspection, err := local.InspectController(t.Context(), repo)
	if err != nil || inspection.Meta.Controller.Name != "pc99" || !inspection.Deployment.Ready || inspection.Current {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	writeGitReviewFile(t, repo, "flake.nix", strings.Replace(declaration, "ready = true", "ready = false", 1))
	inspection, err = local.InspectController(t.Context(), repo)
	if err != nil || inspection.Deployment.Ready {
		t.Fatalf("previous readiness was reused: %+v err=%v", inspection, err)
	}
	writeGitReviewFile(t, repo, "flake.nix", strings.Replace(declaration, "ready = true", `ready = throw "invalid workspace"`, 1))
	if _, err := local.InspectController(t.Context(), repo); err == nil {
		t.Fatal("failed evaluation yielded a usable controller inspection")
	}
	workspaceTestGit(t, repo, "add", "-f", "secret-key")
	if _, err := local.InspectController(t.Context(), repo); err == nil || !strings.Contains(err.Error(), "private key file is tracked") {
		t.Fatalf("tracked secret was not refused before Nix: %v", err)
	}
}
