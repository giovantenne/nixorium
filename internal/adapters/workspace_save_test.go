package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWorkspaceRecordBindsCandidateAndPreservesIndex(t *testing.T) {
	for _, mode := range []string{"success", "content", "revision", "mode", "attributes", "staged profile", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			repository := workspaceRepository(t)
			revision := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
			candidate := domain.WorkspaceProfile{SchemaVersion: 1}
			data, _ := domain.MarshalWorkspaceProfile(candidate)
			writeGitReviewFile(t, repository, domain.WorkspaceFileName, string(data))
			if err := os.Chmod(filepath.Join(repository, domain.WorkspaceFileName), 0600); err != nil {
				t.Fatal(err)
			}
			writeGitReviewFile(t, repository, "module.nix", "# unrelated staged edit\n")
			workspaceTestGit(t, repository, "add", "module.nix")
			before := workspaceTestGit(t, repository, "diff", "--cached", "--", "module.nix")
			writeGitReviewFile(t, repository, ".git/hooks/pre-commit", "#!/bin/sh\nexit 99\n")
			if err := os.Chmod(filepath.Join(repository, ".git/hooks/pre-commit"), 0700); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "content":
				candidate.Desktop = &domain.WorkspaceDesktop{}
			case "revision":
				revision = strings.Repeat("e", 40)
			case "mode":
				if err := os.Chmod(filepath.Join(repository, domain.WorkspaceFileName), 0755); err != nil {
					t.Fatal(err)
				}
			case "attributes":
				writeGitReviewFile(t, repository, ".gitattributes", "workspace-profile.json filter=external\n")
			case "staged profile":
				workspaceTestGit(t, repository, "add", domain.WorkspaceFileName)
			case "symlink":
				if err := os.Rename(filepath.Join(repository, domain.WorkspaceFileName), filepath.Join(repository, "profile-copy")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("profile-copy", filepath.Join(repository, domain.WorkspaceFileName)); err != nil {
					t.Fatal(err)
				}
			}
			commit, err := (Local{}).RecordWorkspaceCandidate(t.Context(), repository, revision, candidate)
			if mode == "success" {
				if err != nil || commit == "" {
					t.Fatalf("record: %s %v", commit, err)
				}
				if got := strings.TrimSpace(workspaceTestGit(t, repository, "show", "--format=", "--name-only", "HEAD")); got != domain.WorkspaceFileName {
					t.Fatalf("committed %s", got)
				}
			} else if err == nil || commit != "" {
				t.Fatal("unsafe record accepted")
			}
			if mode == "staged profile" && !strings.Contains(workspaceTestGit(t, repository, "diff", "--cached", "--name-only"), domain.WorkspaceFileName) {
				t.Fatal("concurrent workspace staging was removed")
			}
			if after := workspaceTestGit(t, repository, "diff", "--cached", "--", "module.nix"); after != before {
				t.Fatal("unrelated index changed")
			}
		})
	}
}

func TestWorkspaceRealNixRecordedSave(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKSPACE_NIX") != "1" {
		t.Skip("requires real Nix")
	}
	repository := workspaceRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", workspaceIntegrationFlake)
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; version = "1.0"; }`)
	workspaceTestGit(t, repository, "add", "flake.nix", "policy.nix")
	workspaceTestGit(t, repository, "commit", "-qm", "workspace save fixture")
	writeGitReviewFile(t, repository, "module.nix", "# unrelated staged edit\n")
	workspaceTestGit(t, repository, "add", "module.nix")
	local := Local{}
	workspace := app.NewWorkspaceManager(local)
	plan := workspace.Plan(t.Context(), repository, []byte(`{"schemaVersion":1,"desktop":{"colorScheme":"dark"}}`))
	if plan.HasErrors() {
		t.Fatalf("plan: %+v", plan)
	}
	manager := app.NewWorkspaceSaveManager(local, app.NewGitReviewManager(local))
	result := manager.Save(t.Context(), plan)
	if result.HasErrors() || !result.Recorded || result.Revision == "" {
		t.Fatalf("save: %+v", result)
	}
	if got := strings.TrimSpace(workspaceTestGit(t, repository, "show", "--format=", "--name-only", "HEAD")); got != domain.WorkspaceFileName {
		t.Fatalf("committed %s", got)
	}
	if status := workspaceTestGit(t, repository, "status", "--porcelain=v1"); !strings.Contains(status, "M  module.nix") || strings.Contains(status, domain.WorkspaceFileName) {
		t.Fatalf("status: %s", status)
	}
	if again := manager.Save(t.Context(), plan); !again.HasErrors() || again.Recorded {
		t.Fatalf("consumed token reused: %+v", again)
	}
}
