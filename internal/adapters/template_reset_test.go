package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func resetFixture(t *testing.T) (*os.File, domain.TemplateResetPlan) {
	t.Helper()
	repo := workspaceRepository(t)
	writeGitReviewFile(t, repo, "lab-settings.json", "{\"private\":\"settings\"}\n")
	writeGitReviewFile(t, repo, ".gitignore", "secret-key\nprivate/\n")
	writeGitReviewFile(t, repo, "keys/admin-ssh.pub", "public test key\n")
	workspaceTestGit(t, repo, "add", "--all")
	workspaceTestGit(t, repo, "commit", "-qm", "deployment")
	writeGitReviewFile(t, repo, "secret-key", "do not touch this secret")
	writeGitReviewFile(t, repo, "private/config", "do not touch ignored files")
	writeGitReviewFile(t, repo, "notes.txt", "untracked notes")
	root, err := openWorkspaceRoot(repo, unix.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	source, err := inspectResetRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	source.UpstreamRevision = strings.Repeat("a", 40)
	source.Candidate = map[string]domain.TemplateFile{}
	for name, file := range source.Original {
		source.Candidate[name] = file
	}
	delete(source.Candidate, "module.nix")
	source.Candidate["new/module.nix"] = domain.TemplateFile{Mode: "100644", Data: []byte("{}\n")}
	source.Candidate["executable"] = domain.TemplateFile{Mode: "100755", Data: []byte("#!/bin/sh\nexit 0\n")}
	source.Candidate["discovery"] = domain.TemplateFile{Mode: "120000", Data: []byte("new/module.nix")}
	plan := domain.TemplateResetPlan{State: "ready", Repository: repo, Revision: source.Revision, UpstreamRevision: source.UpstreamRevision, Preset: domain.SoftwarePreset{ID: "essential"}, Proposal: source, Confirmation: "RESET DEPLOYMENT"}
	plan.Changes, plan.Preserved = resetChanges(source.Original, source.Candidate)
	plan.ReviewToken, err = domain.TemplateResetToken(plan)
	if err != nil {
		t.Fatal(err)
	}
	return root, plan
}

func TestTemplateResetTransactionPreservesPrivateFilesAndRecoverableHistory(t *testing.T) {
	root, plan := resetFixture(t)
	result := applyResetTransaction(t.Context(), root, plan, nil)
	if result.State != "saved" || result.RecoveryRequired || result.BackupRef == "" {
		t.Fatalf("reset: %+v", result)
	}
	for name, content := range map[string]string{"secret-key": "do not touch this secret", "private/config": "do not touch ignored files", "notes.txt": "untracked notes", "keys/admin-ssh.pub": "public test key\n", "new/module.nix": "{}\n"} {
		if got := readGitReviewFile(t, root.Name(), name); got != content {
			t.Fatalf("changed %s: %q", name, got)
		}
	}
	if _, err := os.Lstat(filepath.Join(root.Name(), "module.nix")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old module was not removed")
	}
	if got := strings.TrimSpace(workspaceTestGit(t, root.Name(), "rev-parse", result.BackupRef)); got != plan.Revision {
		t.Fatal("backup is not the original revision")
	}
	if got := strings.TrimSpace(workspaceTestGit(t, root.Name(), "rev-parse", "HEAD^")); got != plan.Revision {
		t.Fatal("reset lost parent history")
	}
	if err := checkTemplateResetPending(root.Name()); err != nil {
		t.Fatal(err)
	}
	workspaceTestGit(t, root.Name(), "diff", "--exit-code", "HEAD")
	tracked := workspaceTestGit(t, root.Name(), "ls-tree", "-r", "--name-only", "HEAD")
	if strings.Contains(tracked, "secret-key") || strings.Contains(tracked, "private/config") || strings.Contains(tracked, "notes.txt") {
		t.Fatal("private/untracked files were committed")
	}
	if target, err := os.Readlink(filepath.Join(root.Name(), "discovery")); err != nil || target != "new/module.nix" {
		t.Fatal("symlink was not preserved")
	}
}

func TestTemplateResetTransactionRejectsStaleOrUnsafeSources(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, string){
		"dirty":       func(t *testing.T, repo string) { writeGitReviewFile(t, repo, "module.nix", "changed") },
		"new ignored": func(t *testing.T, repo string) { writeGitReviewFile(t, repo, "private/new", "extra") },
		"collision":   func(t *testing.T, repo string) { writeGitReviewFile(t, repo, "new/module.nix", "untracked") },
		"assume unchanged": func(t *testing.T, repo string) {
			workspaceTestGit(t, repo, "update-index", "--assume-unchanged", "module.nix")
		},
		"skip worktree": func(t *testing.T, repo string) {
			workspaceTestGit(t, repo, "update-index", "--skip-worktree", "module.nix")
		},
		"hardlink": func(t *testing.T, repo string) {
			if err := os.Link(filepath.Join(repo, "module.nix"), filepath.Join(repo, "duplicate")); err != nil {
				t.Fatal(err)
			}
		},
		"tracked private key": func(t *testing.T, repo string) {
			writeGitReviewFile(t, repo, "module.nix", "-----BEGIN OPENSSH PRIVATE KEY-----")
			workspaceTestGit(t, repo, "add", "module.nix")
			workspaceTestGit(t, repo, "commit", "-qm", "unsafe")
		},
	} {
		t.Run(name, func(t *testing.T) {
			root, plan := resetFixture(t)
			mutate(t, root.Name())
			result := applyResetTransaction(t.Context(), root, plan, nil)
			if result.State != "blocked" || result.BackupRef != "" || result.RecoveryRequired {
				t.Fatalf("unsafe result: %+v", result)
			}
		})
	}
}

func TestTemplateResetInterruptedCheckoutKeepsBackupAndBlocksOperations(t *testing.T) {
	root, plan := resetFixture(t)
	result := applyResetTransaction(t.Context(), root, plan, func() error { return errors.New("injected interruption") })
	if result.State != "recovery-required" || !result.RecoveryRequired || result.BackupRef == "" {
		t.Fatalf("interruption: %+v", result)
	}
	if _, err := (Local{}).GitState(t.Context(), root.Name()); err == nil {
		t.Fatal("operation preflight accepted an interrupted reset")
	}
	if _, err := inspectResetRepository(t.Context(), root); err == nil {
		t.Fatal("second reset bypassed recovery")
	}
	data, err := os.ReadFile(filepath.Join(root.Name(), ".git", resetPendingName))
	if err != nil {
		t.Fatal(err)
	}
	var journal resetJournal
	if json.Unmarshal(data, &journal) != nil || journal.BackupRef != result.BackupRef || journal.OriginalRevision != plan.Revision || !fullGitObjectIDPattern.MatchString(journal.CandidateRevision) {
		t.Fatal("invalid recovery evidence")
	}
	// Recovery can complete the prepared checkout without touching private files.
	workspaceTestGit(t, root.Name(), "diff", "--exit-code", journal.CandidateRevision)
	workspaceTestGit(t, root.Name(), "update-ref", journal.Branch, journal.CandidateRevision, journal.OriginalRevision)
	if err := clearResetJournal(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).GitState(t.Context(), root.Name()); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateResetRefusesConcurrentIndexChangeAfterCheckout(t *testing.T) {
	root, plan := resetFixture(t)
	result := applyResetTransaction(t.Context(), root, plan, func() error {
		workspaceTestGit(t, root.Name(), "update-index", "--force-remove", "new/module.nix")
		return nil
	})
	if !result.RecoveryRequired || result.State != "recovery-required" {
		t.Fatalf("concurrent staged deletion was ignored: %+v", result)
	}
	if strings.TrimSpace(workspaceTestGit(t, root.Name(), "rev-parse", "HEAD")) != plan.Revision {
		t.Fatal("advanced HEAD over a changed index")
	}
}

func TestTemplateResetCandidatePreservesPinAndEnablesGuidedHome(t *testing.T) {
	files, err := readResetTemplate("../../templates/site")
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]domain.TemplateFile{}
	for name, file := range files {
		before[name] = file
	}
	before["flake.lock"] = domain.TemplateFile{Mode: "100644", Data: []byte("exact lock")}
	before["keys/admin-ssh.pub"] = domain.TemplateFile{Mode: "100644", Data: []byte("existing key")}
	flake := strings.ReplaceAll(string(files["flake.nix"].Data), "github:giovantenne/nixorium/v2.0.0", "github:giovantenne/nixorium/master")
	snapshot := domain.UpdateInputSnapshot{FlakeContent: []byte(flake), SourceURL: "github:giovantenne/nixorium/master"}
	catalog, err := domain.DecodeSoftwarePresetCatalog(files["software-presets.json"].Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range catalog.Presets {
		candidate, selected, err := prepareResetFiles(before, files, snapshot, preset.ID)
		if err != nil || selected.ID != preset.ID {
			t.Fatalf("%s: %v", preset.ID, err)
		}
		if strings.Contains(string(candidate["flake.nix"].Data), "workspaceRuntimeEnabled") || !strings.Contains(string(candidate["flake.nix"].Data), snapshot.SourceURL) {
			t.Fatal("pin/runtime not preserved")
		}
		if _, issues := domain.DecodeWorkspaceProfile(candidate[domain.WorkspaceFileName].Data); len(issues) != 0 {
			t.Fatalf("profile: %v", issues)
		}
		if string(candidate["flake.lock"].Data) != "exact lock" || string(candidate["keys/admin-ssh.pub"].Data) != "existing key" {
			t.Fatal("lost pin/key")
		}
		software, err := domain.DecodeLabSoftware(candidate["lab-software.json"].Data)
		if err != nil || len(software.Packages) != len(preset.Packages) {
			t.Fatal("preset was not substituted")
		}
	}
}

func TestTemplateResetCancelledBeforeWriteIsReadOnly(t *testing.T) {
	root, plan := resetFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := applyResetTransaction(ctx, root, plan, nil)
	if result.State != "blocked" || result.BackupRef != "" {
		t.Fatalf("cancelled: %+v", result)
	}
	if strings.TrimSpace(workspaceTestGit(t, root.Name(), "rev-parse", "HEAD")) != plan.Revision {
		t.Fatal("cancellation changed HEAD")
	}
}

func TestTemplateResetManagedOperationGate(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_TEMPLATE_RESET_GATE") != "1" {
		t.Skip("requires the isolated management VM coordination directory")
	}
	root, plan := resetFixture(t)
	root.Close()
	gate, err := acquireManagedOperationGate()
	if err != nil {
		t.Fatal(err)
	}
	blocked := (TemplateReset{}).ApplyTemplateReset(t.Context(), plan)
	gate.Close()
	if blocked.State != "blocked" || blocked.BackupRef != "" {
		t.Fatalf("active gate bypassed: %+v", blocked)
	}
	result := (TemplateReset{}).ApplyTemplateReset(t.Context(), plan)
	if result.State != "saved" {
		t.Fatalf("managed reset: %+v", result)
	}
}
