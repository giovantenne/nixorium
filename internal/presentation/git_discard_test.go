package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestGitDiscardNeedsTheWordAndShowsTheBackup(t *testing.T) {
	applied := 0
	review := domain.GitReviewReport{State: "ready", Changes: []domain.GitChange{{Path: "lab-settings.json", Unstaged: "modified", Managed: true}}}
	actions := DashboardActions{
		LoadGitReview: func(context.Context) domain.GitReviewReport { return review },
		PlanGitCommit: func(context.Context, string) domain.GitCommitPlanReport {
			t.Fatal("commit planned instead of discard")
			return domain.GitCommitPlanReport{}
		},
		PlanGitDiscard: func(_ context.Context, paths string) domain.GitCommitPlanReport {
			return domain.GitCommitPlanReport{State: "ready", Paths: []string{paths}, Confirmation: "DISCARD", ReviewToken: "sha256:x", Diff: domain.GitDiff{Content: "-\"saved\"\n+\"mistake\"\n"}}
		},
		ApplyGitDiscard: func(domain.GitCommitPlanReport) domain.GitCommitReport {
			applied++
			return domain.GitCommitReport{Operation: "git-discard", State: "completed", BackupRef: "refs/nixorium/discard-backups/x", Message: "The selected files are back to the last saved version."}
		},
	}
	model := dashboardModel{actions: actions, screen: dashboardGitReview, width: 120, height: 30}
	model.maintenance.gitReview = review
	if !strings.Contains(model.View().Content, "Discard changes") {
		t.Fatalf("review view:\n%s", model.View().Content)
	}
	model = cleanupPress(t, model, tea.KeyPressMsg{Code: 'x', Text: "x"})
	model = cleanupPress(t, model, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Type DISCARD to continue") {
		t.Fatalf("discard review:\n%s", model.View().Content)
	}
	for _, key := range "DISCARD" {
		model = cleanupPress(t, model, tea.KeyPressMsg{Code: key, Text: string(key)})
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || !strings.Contains(model.View().Content, "Changes discarded") {
		t.Fatalf("result (applied %d):\n%s", applied, model.View().Content)
	}
}
