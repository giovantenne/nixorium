package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestInspectUpdateInputAndRenderTarget(t *testing.T) {
	repository := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repository, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("flake.nix", "{\n  inputs.nixpkgs.url = \"github:NixOS/nixpkgs/nixos-26.05\";\n  inputs.nixorium.url = \"github:giovantenne/nixorium/v1.2.3\";\n  inputs.nixorium.inputs.nixpkgs.follows = \"nixpkgs\";\n}\n")
	write("flake.lock", `{"root":"root","nodes":{"root":{"inputs":{"nixpkgs":"nixpkgs","nixorium":"nixorium"}},"nixpkgs":{"locked":{"owner":"NixOS","repo":"nixpkgs","rev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"nixorium":{"locked":{"rev":"0123456789012345678901234567890123456789"}}}}`)
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil || snapshot.SourcePrefix != "giovantenne/nixorium" || snapshot.CurrentRef != "v1.2.3" || len(snapshot.CurrentRev) != 40 {
		t.Fatalf("snapshot = %+v, error = %v", snapshot, err)
	}
	proposed, err := ProposedUpdateFlake(snapshot, "v1.3.0")
	if err != nil || !strings.Contains(string(proposed), "github:giovantenne/nixorium/v1.3.0") || strings.Contains(string(proposed), "v1.2.3") || !strings.Contains(string(proposed), "github:NixOS/nixpkgs/nixos-26.05") || !strings.Contains(string(proposed), `inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs"`) {
		t.Fatalf("proposed flake = %q, error = %v", proposed, err)
	}
}

func TestInspectUpdateInputAcceptsNestedTemplateCompatibilityForm(t *testing.T) {
	repository := t.TempDir()
	flake := "{\n  inputs = {\n    nixpkgs.url = \"github:NixOS/nixpkgs/nixos-26.05\";\n    nixorium.url = \"github:giovantenne/nixorium/master\";\n    nixorium.inputs.nixpkgs.follows = \"nixpkgs\";\n  };\n}\n"
	if err := os.WriteFile(filepath.Join(repository, "flake.nix"), []byte(flake), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil || snapshot.SourcePrefix != "giovantenne/nixorium" || snapshot.CurrentRef != "master" {
		t.Fatalf("snapshot = %+v, error = %v", snapshot, err)
	}
	proposed, err := ProposedUpdateFlake(snapshot, "v2.0.0")
	if err != nil || !strings.Contains(string(proposed), `    nixorium.url = "github:giovantenne/nixorium/v2.0.0";`) {
		t.Fatalf("proposed flake = %q, error = %v", proposed, err)
	}
}

func TestSiteTemplateExposesCanonicalManagedUpdateInput(t *testing.T) {
	template, err := os.ReadFile("../../templates/site/flake.nix")
	if err != nil {
		t.Fatal(err)
	}
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "flake.nix"), template, 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil || snapshot.SourcePrefix != "giovantenne/nixorium" || snapshot.CurrentRef != "master" {
		t.Fatalf("template snapshot = %+v, error = %v", snapshot, err)
	}
	if !strings.Contains(string(template), `inputs.nixorium.url = "github:giovantenne/nixorium/master";`) {
		t.Fatal("site template does not use the canonical managed input assignment")
	}
}

func TestFrameworkUpdatePreservesDeploymentOwnedPackageBaseNode(t *testing.T) {
	before := []byte(`{"root":"root","nodes":{"root":{"inputs":{"nixpkgs":"nixpkgs","nixorium":"nixorium"}},"nixpkgs":{"locked":{"owner":"NixOS","repo":"nixpkgs","rev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},"nixorium":{"locked":{"rev":"1111111111111111111111111111111111111111"}}}}`)
	after := []byte(`{"nodes":{"nixorium":{"locked":{"rev":"2222222222222222222222222222222222222222"}},"nixpkgs":{"locked":{"rev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repo":"nixpkgs","owner":"NixOS"}},"root":{"inputs":{"nixorium":"nixorium","nixpkgs":"nixpkgs"}}},"root":"root"}`)
	if err := preserveDeploymentInputNode(before, after, "nixpkgs"); err != nil {
		t.Fatalf("framework-only lock change rejected: %v", err)
	}
	changed := []byte(strings.Replace(string(after), strings.Repeat("a", 40), strings.Repeat("b", 40), 1))
	if err := preserveDeploymentInputNode(before, changed, "nixpkgs"); err == nil || !strings.Contains(err.Error(), "deployment-owned nixpkgs") {
		t.Fatalf("changed package-base node error = %v", err)
	}
	removed := []byte(`{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"2222222222222222222222222222222222222222"}}}}`)
	if err := preserveDeploymentInputNode(before, removed, "nixpkgs"); err == nil {
		t.Fatal("removed package-base node was accepted")
	}
	legacy := []byte(`{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"1111111111111111111111111111111111111111"}}}}`)
	if err := preserveDeploymentInputNode(legacy, removed, "nixpkgs"); err != nil {
		t.Fatalf("legacy lock without direct package base rejected: %v", err)
	}
}

func TestInspectUpdateInputRefusesAmbiguousComputedAndSymlinkSources(t *testing.T) {
	for _, content := range []string{
		"{ inputs.nixorium.url = target; }\n",
		"inputs.nixorium.url = \"github:one/nixorium/v1.0.0\";\ninputs.nixorium.url = \"github:two/nixorium/v1.0.0\";\n",
		"inputs.nixorium.url = \"github:one/nixorium/v1.0.0\";\n    nixorium.url = \"github:two/nixorium/v1.0.0\";\n",
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

func TestDiscoverUpdateReleasesIsBoundedAndNonInteractive(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" > "$NIXORIUM_TEST_GIT_LOG"
printf 'prompt=%s askpass=%s sshaskpass=%s interactive=%s global=%s nosystem=%s count=%s\n' "${GIT_TERMINAL_PROMPT-}" "${GIT_ASKPASS-}" "${SSH_ASKPASS-}" "${GCM_INTERACTIVE-}" "${GIT_CONFIG_GLOBAL-}" "${GIT_CONFIG_NOSYSTEM-}" "${GIT_CONFIG_COUNT-}" >> "$NIXORIUM_TEST_GIT_LOG"
printf '%s\t%s\n' 0000000000000000000000000000000000000000 refs/heads/master
printf '%s\t%s\n' 1111111111111111111111111111111111111111 refs/tags/v1.0.0
printf '%s\t%s\n' 2222222222222222222222222222222222222222 refs/tags/v1.1.0-beta.1
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NIXORIUM_TEST_GIT_LOG", logPath)
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_ASKPASS", "/tmp/unsafe-askpass")
	t.Setenv("SSH_ASKPASS", "/tmp/unsafe-ssh-askpass")
	t.Setenv("GIT_CONFIG_GLOBAL", "/tmp/unsafe-git-config")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.https://attacker.invalid/.insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/")
	refs, err := (Local{}).DiscoverUpdateReleases(context.Background(), "owner/repository")
	if err != nil || len(refs) != 3 || refs[0].Tag != "master" || refs[1].Tag != "v1.0.0" || refs[2].Tag != "v1.1.0-beta.1" {
		t.Fatalf("release refs = %+v, error = %v", refs, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"-c credential.helper= -c core.askPass= ls-remote --refs --exit-code https://github.com/owner/repository.git refs/heads/master refs/tags/v*",
		"prompt=0 askpass= sshaskpass= interactive=Never global=/dev/null nosystem=1 count=",
	} {
		if !strings.Contains(string(log), expected) {
			t.Fatalf("discovery log omits %q:\n%s", expected, log)
		}
	}
	if _, err := (Local{}).DiscoverUpdateReleases(context.Background(), "owner/repository with space"); err == nil {
		t.Fatal("unsafe upstream identity was accepted")
	}
}

func TestDiscoverUpdateReleasesRejectsMalformedAndCancelledResults(t *testing.T) {
	bin := t.TempDir()
	scriptPath := filepath.Join(bin, "git")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf 'not-a-revision refs/tags/v1.0.0\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := (Local{}).DiscoverUpdateReleases(context.Background(), "owner/repository"); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed discovery error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Local{}).DiscoverUpdateReleases(cancelled, "owner/repository"); err == nil {
		t.Fatal("cancelled discovery succeeded")
	}
	oversized := `#!/bin/sh
i=0
while [ "$i" -lt 5000 ]; do
  printf '%s\trefs/tags/v1.0.%s\n' 1111111111111111111111111111111111111111 "$i"
  i=$((i + 1))
done
`
	if err := os.WriteFile(scriptPath, []byte(oversized), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).DiscoverUpdateReleases(context.Background(), "owner/repository"); err == nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("oversized discovery error = %v", err)
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

func TestPrepareUpdateBuildsOnlyControllerForControllerMode(t *testing.T) {
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
    printf '%s\n' '{"schemaVersion":2,"deploymentMode":"controller","controller":{"name":"pc99"},"clients":{"count":0,"hosts":[]}}'
    ;;
  *"#deploymentStatus "*)
    printf '%s\n' '{"ready":false,"issues":["Client installation is not configured"],"controller":{"ready":true,"issues":[],"requiresKeys":false}}'
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
	if len(proposal.Checks) != 3 || proposal.Checks[1].Message != "candidate controller is ready" || proposal.Checks[2].ID != "controller" {
		t.Fatalf("proposal checks = %+v", proposal.Checks)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 4 || strings.Count(string(log), " build ") != 1 || !strings.Contains(string(log), "#nixosConfigurations.pc99.config.system.build.toplevel") {
		t.Fatalf("unexpected controller-only Nix invocations (%d):\n%s", len(lines), log)
	}
	for _, excluded := range []string{"nixosConfigurations.pc01", "nixosConfigurations.netboot", "#pxeFirmware", "#installerBundle"} {
		if strings.Contains(string(log), excluded) {
			t.Fatalf("controller-only update built %q:\n%s", excluded, log)
		}
	}
}

func TestUpdateCandidateChecksFailClosedAcrossDeploymentModes(t *testing.T) {
	controllerMeta := domain.LabMeta{DeploymentMode: "controller"}
	controllerMeta.Controller.Name = "pc99"
	controllerReady := domain.DeploymentStatus{Controller: &domain.ControllerReadiness{Ready: true}}

	withClient := controllerMeta
	withClient.Clients.Count = 1
	withClient.Clients.Hosts = []domain.HostMeta{{Name: "pc01"}}
	for _, test := range []struct {
		name   string
		meta   domain.LabMeta
		status domain.DeploymentStatus
		want   string
	}{
		{"controller inventory", withClient, controllerReady, "contains client inventory"},
		{"missing controller readiness", controllerMeta, domain.DeploymentStatus{Ready: true}, "does not advertise controller readiness"},
		{"unready controller", controllerMeta, domain.DeploymentStatus{Controller: &domain.ControllerReadiness{Issues: []string{"passwords are not configured"}}}, "passwords are not configured"},
		{"unknown mode", func() domain.LabMeta { meta := controllerMeta; meta.DeploymentMode = "future"; return meta }(), controllerReady, "unsupported deployment mode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := updateCandidateChecks(test.meta, test.status); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	legacy := domain.LabMeta{}
	legacy.Controller.Name = "pc99"
	legacy.Clients.Count = 1
	legacy.Clients.Hosts = []domain.HostMeta{{Name: "pc01"}}
	builds, checks, err := updateCandidateChecks(legacy, domain.DeploymentStatus{Ready: true})
	if err != nil || len(builds) != 5 || len(checks) != 2 {
		t.Fatalf("legacy laboratory builds = %+v, checks = %+v, error = %v", builds, checks, err)
	}
}

func TestApplyPreparedUpdateWritesOnlyReviewedFiles(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", "{\n  inputs.nixorium.url = \"github:owner/project/v1.0.0\";\n}\n")
	writeGitReviewFile(t, repository, "flake.lock", `{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"1111111111111111111111111111111111111111"}}}}`+"\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil {
		t.Fatal(err)
	}
	proposedFlake, err := ProposedUpdateFlake(snapshot, "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	proposal := domain.UpdateProposal{FlakeContent: proposedFlake, LockContent: []byte("{\"version\": 7}\n")}
	revision, _ := (Local{}).GitRevision(context.Background(), repository)
	partial, err := (Local{}).ApplyPreparedUpdate(context.Background(), repository, revision, snapshot, proposal)
	if err != nil || partial {
		t.Fatalf("apply partial = %t, error = %v", partial, err)
	}
	if content := readGitReviewFile(t, repository, "flake.nix"); !strings.Contains(content, "/v1.1.0") {
		t.Fatalf("flake.nix = %q", content)
	}
	if content := readGitReviewFile(t, repository, "flake.lock"); content != "{\"version\": 7}\n" {
		t.Fatalf("flake.lock = %q", content)
	}
	status, err := run(context.Background(), "git", "-C", repository, "status", "--porcelain=v1")
	if err != nil || !strings.Contains(status, " M flake.lock") || !strings.Contains(status, " M flake.nix") || len(strings.Split(strings.TrimSpace(status), "\n")) != 2 {
		t.Fatalf("status = %q, error = %v", status, err)
	}
}

func TestApplyPreparedUpdateRefusesStaleWorktree(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", "inputs.nixorium.url = \"github:owner/project/v1.0.0\";\n")
	writeGitReviewFile(t, repository, "flake.lock", `{"root":"root","nodes":{"root":{"inputs":{"nixorium":"nixorium"}},"nixorium":{"locked":{"rev":"1111111111111111111111111111111111111111"}}}}`)
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := (Local{}).InspectUpdateInput(repository)
	revision, _ := (Local{}).GitRevision(context.Background(), repository)
	writeGitReviewFile(t, repository, "unrelated", "dirty\n")
	partial, err := (Local{}).ApplyPreparedUpdate(context.Background(), repository, revision, snapshot, domain.UpdateProposal{FlakeContent: []byte("new"), LockContent: []byte("new")})
	if err == nil || partial || !strings.Contains(err.Error(), "worktree changed") {
		t.Fatalf("stale apply partial = %t, error = %v", partial, err)
	}
	if readGitReviewFile(t, repository, "flake.nix") != string(snapshot.FlakeContent) {
		t.Fatal("stale apply changed flake.nix")
	}
}
