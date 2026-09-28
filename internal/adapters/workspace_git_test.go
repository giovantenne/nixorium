package adapters

import (
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWorkspaceGitCommitPreservesUnrelatedStagedContent(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, domain.WorkspaceFileName, `{"schemaVersion":1}`)
	writeGitReviewFile(t, repository, "module.nix", "# unrelated staged change\n")
	workspaceTestGit(t, repository, "add", "module.nix")
	local := Local{}
	review := app.NewGitReviewManager(local).Review(t.Context(), repository)
	found := false
	for _, change := range review.Changes {
		if change.Path == domain.WorkspaceFileName {
			found = change.Managed && change.Untracked
		}
	}
	if review.HasErrors() || !found {
		t.Fatalf("workspace classification: %+v", review)
	}
	manager := app.NewGitCommitManager(local)
	plan := manager.Plan(t.Context(), repository, domain.WorkspaceFileName)
	if plan.HasErrors() {
		t.Fatalf("workspace commit plan: %+v", plan)
	}
	result := manager.Apply(t.Context(), repository, domain.WorkspaceFileName, plan.ReviewToken)
	if result.HasErrors() || !result.Committed {
		t.Fatalf("workspace commit: %+v", result)
	}
	if got := workspaceTestGit(t, repository, "show", "--format=", "--name-only", "HEAD"); strings.TrimSpace(got) != domain.WorkspaceFileName {
		t.Fatalf("unexpected committed paths: %q", got)
	}
	if status := workspaceTestGit(t, repository, "status", "--porcelain=v1"); !strings.Contains(status, "M  module.nix") {
		t.Fatalf("unrelated staged change lost: %q", status)
	}
}
