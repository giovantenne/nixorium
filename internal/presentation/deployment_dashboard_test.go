package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDashboardReviewsAndRunsAllClientDeployment(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc02", IP: "10.0.0.2"},
	}
	planned := ""
	applied := 0
	actions := DashboardActions{
		PlanDeployment: func(ctx context.Context, requested string) domain.DeploymentPlanReport {
			planned = requested
			return domain.DeploymentPlanReport{
				Operation:       "deploy-plan",
				State:           "ready",
				Requested:       requested,
				Revision:        "0123456789abcdef",
				ColmenaSelector: "@lab",
				Targets:         []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc02"}},
			}
		},
		ApplyDeployment: func(_ context.Context, plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			applied++
			observe(domain.DeploymentProgress{Phase: domain.DeploymentPhaseBuild, Total: 4, Activity: "Building configurations for 2 selected computer(s)"})
			observe(domain.DeploymentProgress{Phase: domain.DeploymentPhaseVerify, Completed: 3, Total: 4, TargetCurrent: 2, TargetTotal: 2, Activity: "Checked authenticated state for 2/2 computer(s)"})
			return domain.DeploymentExecutionReport{
				Operation:       "deploy-apply",
				State:           "completed",
				Phase:           domain.DeploymentPhaseComplete,
				ColmenaSelector: plan.ColmenaSelector,
				BuildCompleted:  true,
				ApplyCompleted:  true,
				Verification:    domain.DeploymentVerificationSummary{Attempted: 2, Verified: 2, Recorded: 2},
				LogPath:         "/state/deploy.log",
				Message:         "all selected targets were built and applied successfully",
			}
		},
	}
	model := dashboardModel{report: report, actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "[ ] pc01") {
		t.Fatalf("deployment selection not shown:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != "@lab" || model.screen != dashboardDeployReview || !strings.Contains(model.View().Content, "Reviewed revision  0123456789abcdef") {
		t.Fatalf("planned = %q, screen = %d:\n%s", planned, model.screen, model.View().Content)
	}

	if view := model.View().Content; !strings.Contains(view, "Enter updates these computers.") || strings.Contains(view, "Type DEPLOY") || !strings.Contains(view, "Update computers") {
		t.Fatalf("the review does not say what Enter does:\n%s", view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.deployment.applying || !strings.Contains(model.View().Content, "Closing is disabled") {
		t.Fatalf("confirmed deployment did not enter protected busy state:\n%s", model.View().Content)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "Wait for the result") {
		t.Fatal("dashboard allowed quit while deployment was running")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Building configurations") || !strings.Contains(model.View().Content, "Progress details") || !strings.Contains(model.View().Content, "private log") {
		t.Fatalf("deployment progress missing:\n%s", model.View().Content)
	}
	for model.deployment.applying && command != nil {
		updated, command = model.Update(command())
		model = updated.(dashboardModel)
	}
	if applied != 1 || model.deployment.applying || model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "Deployment completed and verified") || strings.Contains(model.View().Content, "Authenticated: 2/2") || strings.Contains(model.View().Content, "/state/deploy.log") || strings.Contains(model.View().Content, "Build complete:") || !strings.Contains(model.View().Content, "New review") {
		t.Fatalf("deployment result missing: applied=%d\n%s", applied, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardComputersArea {
		t.Fatalf("deployment result did not return to Computers: screen=%d", model.screen)
	}
}

func TestDeploymentProgressKeepsOnlyFiveAuthoredActivities(t *testing.T) {
	recent := []string{}
	for _, activity := range []string{"one", "two", "three", "four", "five", "six"} {
		recent = appendBoundedActivity(recent, activity, 5)
	}
	if got := strings.Join(recent, ","); got != "two,three,four,five,six" {
		t.Fatalf("bounded deployment activity = %q", got)
	}

	model := dashboardModel{
		deployment: deploymentModel{
			progress: domain.DeploymentProgress{
				Phase: domain.DeploymentPhaseVerify, Completed: 3, Total: 4,
				TargetCurrent: 2, TargetTotal: 3,
			},
			recent: recent,
		},
		progressDetails: true,
		width:           80,
	}
	view := strings.Join(model.deploymentProgressView(), "\n")
	for _, expected := range []string{"Verifying computers", "3/4", "Computers checked: 2/3", "six"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("deployment progress omits %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "one") {
		t.Fatalf("deployment progress retained more than five activities:\n%s", view)
	}
}

func TestDeploymentProgressDoesNotRegressAfterBuild(t *testing.T) {
	model := dashboardModel{deployment: deploymentModel{progress: domain.DeploymentProgress{Phase: domain.DeploymentPhasePreflight}}}
	view := strings.Join(model.deploymentProgressView(), "\n")
	if !strings.Contains(view, "✓ Building configurations") || !strings.Contains(view, "● Revalidating reviewed configuration · Running") {
		t.Fatalf("post-build revalidation regressed to an initial step:\n%s", view)
	}
}

func TestDashboardDeploymentRejectsEmptySelectionAndBlockedPlan(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	actions := DashboardActions{
		PlanDeployment: func(ctx context.Context, _ string) domain.DeploymentPlanReport {
			return domain.DeploymentPlanReport{State: "blocked", Issues: []domain.ValidationIssue{{Field: "git", Message: "worktree is dirty"}}}
		},
	}
	model := dashboardModel{report: report, actions: actions, screen: dashboardDeploy, deployment: deploymentModel{chosen: map[string]bool{}}}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.View().Content, "Select at least one") {
		t.Fatalf("empty selection was planned:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "git: worktree is dirty") {
		t.Fatalf("blocked plan was not surfaced:\n%s", model.View().Content)
	}
}

func TestSelectedDeploymentTargetsPreservesInventoryOrder(t *testing.T) {
	hosts := []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}, {Name: "pc03"}}
	if got := selectedDeploymentTargets(hosts, map[string]bool{"pc03": true, "pc01": true}); got != "pc01,pc03" {
		t.Fatalf("selected targets = %q, want pc01,pc03", got)
	}
	if got := selectedDeploymentTargets(hosts, map[string]bool{"pc01": true, "pc02": true, "pc03": true}); got != "@lab" {
		t.Fatalf("all targets = %q, want @lab", got)
	}
	if got := strings.Join(selectedDeploymentTargetNames(hosts, map[string]bool{"pc01": true, "pc02": true, "pc03": true}), ","); got != "pc01,pc02,pc03" {
		t.Fatalf("explicit targets = %q, want pc01,pc02,pc03", got)
	}
}
