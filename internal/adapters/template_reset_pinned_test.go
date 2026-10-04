package adapters

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Explicit, network-enabled qualification against a published full revision.
// It uses the real v2.0.0 site tree, not a freshly generated modern deployment.
// No installed machine or system profile is modified.
func TestTemplateResetPinnedLegacyMigration(t *testing.T) {
	upstream := os.Getenv("NIXORIUM_TEST_PINNED_RESET_REPO")
	if upstream == "" {
		t.Skip("requires explicit public upstream checkout and network-enabled Nix")
	}
	revision, err := resetGit(t.Context(), upstream, nil, 256, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	revision = strings.TrimSpace(revision)
	manifest, err := resetGit(t.Context(), upstream, nil, 1024*1024, "ls-tree", "-rz", "v2.0.0", "--", "templates/site")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]domain.TemplateFile{}
	for _, entry := range strings.Split(strings.TrimSuffix(manifest, "\x00"), "\x00") {
		header, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			t.Fatal("unexpected legacy tree")
		}
		data, err := resetGit(t.Context(), upstream, nil, 16*1024*1024, "cat-file", "blob", fields[2])
		if err != nil {
			t.Fatal(err)
		}
		files[strings.TrimPrefix(name, "templates/site/")] = domain.TemplateFile{Mode: fields[0], Data: []byte(data)}
	}
	var settings domain.LabSettingsFile
	if err := json.Unmarshal(files["lab-settings.json"].Data, &settings); err != nil {
		t.Fatal(err)
	}
	settings.Lab.MasterDHCPIP = "192.0.2.10"
	settings.Lab.PCCount = 1
	settings.Lab.AdminPassword, settings.Lab.StudentPassword, settings.Lab.TeacherPassword = "$6$fixture$admin", "$6$fixture$student", "$6$fixture$teacher"
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	files["lab-settings.json"] = domain.TemplateFile{Mode: "100644", Data: data}
	old := files["flake.nix"]
	old.Data = []byte(strings.Replace(string(old.Data), "github:giovantenne/nixorium/v2.0.0", "github:giovantenne/nixorium/"+revision, 1))
	// Veyon is gone from the current upstream, together with its key.
	old.Data = []byte(strings.Replace(string(old.Data), "            veyon = ./keys/veyon-public-key.pem;\n", "", 1))
	files["flake.nix"] = old
	for destination, fixture := range map[string]string{"keys/admin-ssh.pub": "remote-vm-admin.pub", "keys/cache-public-key": "remote-vm-cache-public-key"} {
		content, err := os.ReadFile(filepath.Join(upstream, "tests/fixtures", fixture))
		if err != nil {
			t.Fatal(err)
		}
		files[destination] = domain.TemplateFile{Mode: "100644", Data: content}
	}
	repository, err := isolatedResetCandidate(t.Context(), files)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(repository)
	flake, _ := deploymentFlakeReference(repository)
	command := exec.CommandContext(t.Context(), "nix", "--extra-experimental-features", "nix-command flakes", "flake", "lock", flake, "--option", "accept-flake-config", "false")
	command.Env = workspaceEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture lock: %v\n%s", err, output)
	}
	workspaceTestGit(t, repository, "add", "flake.lock")
	workspaceTestGit(t, repository, "commit", "-qm", "pin upgraded framework without migrating the old template")
	writeGitReviewFile(t, repository, "secret-key", "private-preservation-sentinel")
	legacy := app.NewWorkspaceManager(Local{}).Load(t.Context(), repository)
	if !legacy.HasErrors() {
		t.Fatal("legacy template unexpectedly supports initial workspace loading")
	}
	manager := app.NewTemplateResetManager(TemplateReset{})
	catalog := manager.Catalog(t.Context(), repository)
	if catalog.Error != "" || catalog.UpstreamRevision != revision {
		t.Fatalf("catalog: %+v", catalog)
	}
	plan := manager.Plan(t.Context(), repository, "essential", func(message string) { t.Log(message) })
	if plan.HasErrors() {
		if len(plan.Proposal.Candidate) != 0 {
			candidate, err := isolatedResetCandidate(t.Context(), plan.Proposal.Candidate)
			if err == nil {
				defer os.RemoveAll(candidate)
				for _, expression := range []string{resetIdentityExpression, resetValidationExpression} {
					flake, _ := deploymentFlakeReference(candidate)
					command := exec.CommandContext(t.Context(), "nix", "--extra-experimental-features", "nix-command flakes", "eval", "--impure", "--json", "--show-trace", "--no-write-lock-file", "--no-update-lock-file", "--option", "allow-import-from-derivation", "false", "--expr", `let f = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE"); in `+expression)
					command.Env = append(workspaceEnvironment(), "NIXORIUM_DEPLOYMENT_FLAKE="+flake)
					if output, err := command.CombinedOutput(); err != nil {
						t.Logf("Synthetic pinned candidate only: %s", output)
					}
				}
			}
		}
		t.Fatalf("plan: %s", plan.Message)
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	result := applyResetTransaction(t.Context(), root, plan, nil)
	root.Close()
	if result.State != "saved" {
		t.Fatalf("reset: %+v", result)
	}
	resolved := app.NewWorkspaceManager(Local{}).Load(t.Context(), repository)
	if resolved.HasErrors() || resolved.Inspection == nil || !resolved.Inspection.Resolution.RuntimeEnabled {
		t.Fatalf("workspace after reset: %+v", resolved.Issues)
	}
	if readGitReviewFile(t, repository, "secret-key") != "private-preservation-sentinel" {
		t.Fatal("private key was changed")
	}
	t.Log("Real v2.0.0 template migration passed: locked revision retained, guided workspace loads, no system built/applied")
}
