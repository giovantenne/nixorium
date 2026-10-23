package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDashboardShowsScrollableReadOnlyGitReview(t *testing.T) {
	loads := 0
	actions := DashboardActions{
		LoadGitReview: func(ctx context.Context) domain.GitReviewReport {
			loads++
			return domain.GitReviewReport{
				Operation: "git-review",
				State:     "changes",
				Summary:   domain.GitChangeSummary{Staged: 1, Untracked: 1, Managed: 1, Unexpected: 1},
				Changes: []domain.GitChange{
					{Path: "lab-settings.json", Staged: "modified", Managed: true},
					{Path: "notes", Untracked: true},
				},
				Diffs: []domain.GitDiff{{Scope: "staged", Content: "line-01\nline-02\nline-03\nline-04\nline-05\nline-06\nline-07\n"}},
			}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("Git review did not load: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View().Content, "Repository changes") || !strings.Contains(model.View().Content, "lab-settings.json") {
		t.Fatalf("Git review missing: loads=%d\n%s", loads, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "line-07") || strings.Contains(model.View().Content, "lab-settings.json") {
		t.Fatalf("Git review did not scroll:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.maintenance.gitScroll != 0 {
		t.Fatalf("Git review refresh = loads %d, scroll %d", loads, model.maintenance.gitScroll)
	}
}

func TestDashboardPlansAndCreatesExactLocalGitCommit(t *testing.T) {
	revision := strings.Repeat("a", 40)
	token := "sha256:" + strings.Repeat("b", 64)
	applied := 0
	review := domain.GitReviewReport{
		Operation: "git-review",
		State:     "changes",
		Summary:   domain.GitChangeSummary{Unstaged: 2, Managed: 1, Unexpected: 1},
		Changes: []domain.GitChange{
			{Path: "lab-settings.json", Unstaged: "modified", Managed: true},
			{Path: "modules/site.nix", Unstaged: "modified"},
		},
	}
	actions := DashboardActions{
		LoadGitReview: func(ctx context.Context) domain.GitReviewReport { return review },
		PlanGitCommit: func(ctx context.Context, paths string) domain.GitCommitPlanReport {
			if paths != "lab-settings.json" {
				t.Fatalf("planned paths = %q", paths)
			}
			return domain.GitCommitPlanReport{Operation: "git-commit-plan", State: "ready", Revision: revision, Paths: []string{"lab-settings.json"}, ReviewToken: token, CommitMessage: "chore: update laboratory settings", Diff: domain.GitDiff{Content: "+settings\n"}}
		},
		ApplyGitCommit: func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
			applied++
			if plan.ReviewToken != token {
				t.Fatalf("applied token = %q", plan.ReviewToken)
			}
			return domain.GitCommitReport{Operation: "git-commit", State: "completed", Paths: plan.Paths, PreviousRevision: revision, Revision: strings.Repeat("c", 40), Committed: true, Message: "no remote push was attempted"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if model.screen != dashboardGitCommitSelect || !strings.Contains(model.View().Content, "Select paths for the local commit") {
		t.Fatalf("commit selection missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardGitCommitReview || !strings.Contains(model.View().Content, "Enter saves these changes as one local commit.") || strings.Contains(model.View().Content, "Type COMMIT") || !strings.Contains(model.View().Content, "No hooks, signing actions, remote operations, or push") {
		t.Fatalf("commit review missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("commit apply did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View().Content, "Git changes committed locally") || !strings.Contains(model.View().Content, "no remote push was attempted") || !strings.Contains(model.View().Content, "Refresh review") {
		t.Fatalf("commit result missing: applied=%d\n%s", applied, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("Git result did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestFirstSetupCommitResultReturnsToObservedChecklist(t *testing.T) {
	loads := 0
	model := dashboardModel{
		setupMode: true,
		screen:    dashboardGitReview,
		maintenance: maintenanceModel{
			gitCommitResult: domain.GitCommitReport{
				Operation: "git-commit", State: "completed", Committed: true,
			},
		},
		actions: DashboardActions{LoadSetup: func(ctx context.Context) domain.SetupReport {
			loads++
			return testSetupReport(true, false, false, false)
		}},
	}
	if !strings.Contains(model.View().Content, "Enter") || !strings.Contains(model.View().Content, "Setup") {
		t.Fatalf("setup return action missing:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSetup {
		t.Fatalf("commit result did not begin setup refresh: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.screen != dashboardSetup || model.setup.CurrentStage != domain.SetupStageApply {
		t.Fatalf("setup result was not reconciled: loads=%d model=%+v", loads, model.setup)
	}
}

func TestGitCommitSelectionPreservesReviewOrderAndSkipsPrivatePaths(t *testing.T) {
	changes := []domain.GitChange{{Path: "z"}, {Path: "secret-key", Private: true}, {Path: "a"}}
	chosen := toggleAllGitCommitPaths(changes, nil)
	if chosen["secret-key"] || selectedGitCommitPaths(changes, chosen) != "z,a" {
		t.Fatalf("unsafe or reordered selection = %v / %q", chosen, selectedGitCommitPaths(changes, chosen))
	}
}

func TestSuccessfulCommitShowsItsNoteAsInformation(t *testing.T) {
	model := dashboardModel{
		screen:  dashboardGitReview,
		message: "reviewed paths committed locally; no remote push was attempted",
		maintenance: maintenanceModel{
			gitCommitResult: domain.GitCommitReport{Operation: "git-commit", State: "completed", Committed: true},
		},
	}
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	if !strings.Contains(view, "○ reviewed paths committed locally") || strings.Contains(view, "! reviewed paths") {
		t.Fatalf("success note shown as a warning:\n%s", view)
	}
}
