package adapters

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Exercise real Nix evaluation in the unprivileged management VM, including
// a legacy deployment without workspace hooks and ignored secret sentinels.
func TestTemplateResetRealNixCandidate(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_TEMPLATE_RESET_NIX") != "1" {
		t.Skip("requires explicit real-Nix integration environment")
	}
	repo := workspaceRepository(t)
	writeGitReviewFile(t, repo, "lab-settings.json", "{}")
	writeGitReviewFile(t, repo, "lab-software.json", `{"schemaVersion":1,"packages":[]}`)
	writeGitReviewFile(t, repo, "secret-key", "ignored-private-reset-sentinel")
	writeGitReviewFile(t, repo, "identity.nix", `{ disk = "/dev/vda"; publicKey = "public-fixture"; }`)
	writeGitReviewFile(t, repo, "flake.nix", resetIntegrationFlake)
	workspaceTestGit(t, repo, "add", "lab-settings.json", "lab-software.json", "identity.nix", "flake.nix")
	workspaceTestGit(t, repo, "commit", "-qm", "legacy deployment fixture")
	if _, err := resetNix(t.Context(), repo, `f.nixoriumValidateWorkspaceCandidate "{}"`); err == nil {
		t.Fatal("legacy fixture unexpectedly has a workspace hook")
	}
	root, err := openWorkspaceRoot(repo, unix.LOCK_SH)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	p, err := inspectResetRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	p.Original[domain.WorkspaceFileName] = domain.TemplateFile{Mode: "100644", Data: []byte(`{"schemaVersion":1}`)}
	p.Original["flake.nix"] = domain.TemplateFile{Mode: "100644", Data: []byte(strings.Replace(resetIntegrationFlake, "# WORKSPACE-HOOKS", resetIntegrationHooks, 1))}
	candidate, err := isolatedResetCandidate(t.Context(), p.Original)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(candidate)
	if err := validateResetCandidate(t.Context(), repo, candidate); err != nil {
		flake, _ := deploymentFlakeReference(t.Context(), repo)
		command := exec.CommandContext(t.Context(), "nix", "--extra-experimental-features", "nix-command flakes", "eval", "--impure", "--json", "--no-write-lock-file", "--no-update-lock-file", "--expr", `let f = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE"); in `+resetIdentityExpression)
		command.Env = append(workspaceEnvironment(), "NIXORIUM_DEPLOYMENT_FLAKE="+flake)
		output, _ := command.CombinedOutput()
		t.Logf("Disposable identity fixture only: %s", output)
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(candidate, "secret-key")); !os.IsNotExist(err) {
		t.Fatal("candidate copied ignored secret")
	}
	writeGitReviewFile(t, candidate, "identity.nix", `{ disk = "/dev/vdb"; publicKey = "public-fixture"; }`)
	if err := validateResetCandidate(t.Context(), repo, candidate); err == nil {
		t.Fatal("changed disk accepted")
	}
	writeGitReviewFile(t, candidate, "identity.nix", `{ disk = "/dev/vda"; publicKey = "different-key"; }`)
	if err := validateResetCandidate(t.Context(), repo, candidate); err == nil {
		t.Fatal("changed public key accepted")
	}
	writeGitReviewFile(t, candidate, "identity.nix", `{ disk = "/dev/vda"; publicKey = "public-fixture"; }`)
	writeGitReviewFile(t, candidate, "flake.nix", strings.Replace(strings.Replace(resetIntegrationFlake, "# WORKSPACE-HOOKS", resetIntegrationHooks, 1), "raw: true", "raw: false", 1))
	if err := validateResetCandidate(t.Context(), repo, candidate); err == nil {
		t.Fatal("false validation hook accepted")
	}
}

const resetIntegrationFlake = `{
  outputs = { self }: let identity = import ./identity.nix; in
  assert !(builtins.pathExists (./. + "/secret-key"));
  {
    labMeta = {
      schemaVersion = 2; deploymentMode = "controller";
      controller.name = "controller";
      clients = { count = 0; hosts = []; groups = {}; };
      users = { student = "student"; teacher = "teacher"; };
    };
    deploymentStatus = { ready = false; controller = { ready = true; issues = []; }; };
    nixosConfigurations.controller.config = {
      system = { stateVersion = "25.11"; build.toplevel.drvPath = "/nix/store/fixture.drv"; build.diskoScript.drvPath = "/nix/store/disko-fixture.drv"; };
      networking.hostName = "controller";
      disko.devices.disk.main.device = identity.disk;
      fileSystems = {}; swapDevices = [];
      users.users.admin = { uid = 1000; group = "users"; home = "/home/admin"; hashedPassword = "!"; openssh.authorizedKeys.keys = [ identity.publicKey ]; };
      nix.settings.trusted-public-keys = [ "fixture" ];
      environment.etc = {};
    };
    nixoriumValidateCandidate = raw: true;
    nixoriumValidateSoftwareCandidate = raw: true;
    # WORKSPACE-HOOKS
  };
}`
const resetIntegrationHooks = `nixoriumValidateWorkspaceCandidate = raw: true;
    nixoriumResolveWorkspaceCandidate = raw: { declared = builtins.fromJSON raw; };`

func TestTemplateResetRealNixKeepsExternalCredentials(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_TEMPLATE_RESET_NIX") != "1" {
		t.Skip("requires real Nix")
	}
	repo := workspaceRepository(t)
	writeGitReviewFile(t, repo, ".gitignore", "secret-key\nadmin-ssh\nlab-credentials.json\n")
	if err := (Local{}).WriteSettings(repo, adapterSettings()); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repo, "lab-software.json", `{"schemaVersion":1,"packages":[]}`)
	writeGitReviewFile(t, repo, "identity.nix", `{ disk = "/dev/vda"; publicKey = "public-fixture"; }`)
	flake := strings.Replace(resetIntegrationFlake, "# WORKSPACE-HOOKS", resetIntegrationHooks, 1)
	flake = strings.Replace(flake, `hashedPassword = "!"`, `hashedPassword = (builtins.fromJSON (builtins.readFile ./lab-settings.json)).lab.adminPassword`, 1)
	writeGitReviewFile(t, repo, "flake.nix", flake)
	workspaceTestGit(t, repo, "add", ".")
	workspaceTestGit(t, repo, "commit", "-qm", "external credential fixture")
	root, err := openWorkspaceRoot(repo, unix.LOCK_SH)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	proposal, err := inspectResetRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Original[domain.WorkspaceFileName] = domain.TemplateFile{Mode: "100644", Data: []byte(`{"schemaVersion":1}`)}
	candidate, err := isolatedResetCandidate(t.Context(), proposal.Original)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(candidate)
	if err := validateResetCandidate(t.Context(), repo, candidate); err != nil {
		source, sourceErr := (Local{}).EvaluationSource(t.Context(), repo, "")
		t.Logf("disposable source: %s, error: %v", source, sourceErr)
		if sourceErr == nil {
			command := exec.CommandContext(t.Context(), "nix", "--extra-experimental-features", "nix-command flakes", "eval", "--impure", "--json", "--expr", "let f = builtins.getFlake "+workspaceUpdateNixString(source)+"; in "+resetIdentityExpression)
			output, _ := command.CombinedOutput()
			t.Logf("disposable fixture diagnostic: %s", output)
		}
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(candidate, settingsFileName))
	if err != nil || strings.Contains(string(data), "$6$") {
		t.Fatal("reset proposal contains hashes")
	}
	if _, err := os.Lstat(filepath.Join(candidate, credentialsFile)); !os.IsNotExist(err) {
		t.Fatal("reset left temporary credentials")
	}
}
