package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func mixedDeploymentPlan() domain.DeploymentPlanReport {
	return domain.DeploymentPlanReport{Operation: "deploy-plan", State: "ready", Requested: "pc01,pc02", ColmenaSelector: "pc01,pc02", Revision: strings.Repeat("a", 40), ReachableRequested: "pc01", Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}}, Availability: []domain.DeploymentTargetAvailability{{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}, {Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown}}}
}

func TestDeploymentReachableOnlyCreatesFreshConfirmationAndExactSelection(t *testing.T) {
	original := mixedDeploymentPlan()
	subset := original
	subset.Requested = "pc01"
	subset.ColmenaSelector = "pc01"
	subset.Targets = original.Targets[:1]
	subset.Availability = original.Availability[:1]
	subset.ReachableRequested = ""
	subset.Revision = strings.Repeat("b", 40)
	calls := 0
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{PlanReachableDeployment: func(ctx context.Context, plan domain.DeploymentPlanReport) domain.DeploymentPlanReport {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded subset planning")
		}
		if plan.ColmenaSelector != original.ColmenaSelector {
			t.Fatal("presentation edited the old selector")
		}
		return subset
	}, ApplyDeployment: func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
		t.Fatal("subset dispatched a deployment")
		return domain.DeploymentExecutionReport{}
	}}, false)
	model.screen = dashboardDeployReview
	model.deployment.plan = original
	model.deployment.confirmation = "DEPLOY"
	model.deployment.chosen = map[string]bool{"pc01": true, "pc02": true}
	updated, command := model.Update(demoCode(tea.KeyF2))
	model = updated.(dashboardModel)
	if command == nil || model.deployment.confirmation != "" || model.deployment.applying {
		t.Fatal("old authorization survived subset request")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if calls != 1 || model.screen != dashboardDeployReview || model.deployment.plan.Revision != subset.Revision || model.deployment.chosen["pc02"] || !model.deployment.chosen["pc01"] || model.deployment.confirmation != "" {
		t.Fatalf("subset not independently reviewed: %+v", model.deployment)
	}
	updated, command = model.Update(demoCode(tea.KeyEnter))
	if command != nil || updated.(dashboardModel).deployment.applying {
		t.Fatal("subset skipped exact confirmation")
	}
}

func TestDeploymentSubsetCancellationKeepsOldReviewAndIgnoresLateReply(t *testing.T) {
	original := mixedDeploymentPlan()
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{PlanReachableDeployment: func(context.Context, domain.DeploymentPlanReport) domain.DeploymentPlanReport {
		return domain.DeploymentPlanReport{State: "ready", Revision: "late"}
	}}, false)
	model.screen = dashboardDeployReview
	model.deployment.plan = original
	model.deployment.confirmation = "DEPLOY"
	updated, command := model.Update(demoCode(tea.KeyF2))
	model = updated.(dashboardModel)
	reply := command()
	updated, _ = model.Update(demoCode(tea.KeyEscape))
	model = updated.(dashboardModel)
	updated, command = model.Update(reply)
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardDeployReview || model.deployment.plan.Revision != original.Revision || model.deployment.confirmation != "" {
		t.Fatal("cancelled subset replaced or authorized the original review")
	}
}

func TestDeploymentPerComputerResultsAndRecoveryRemainScrollable(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
		model.width, model.height = size[0], size[1]
		model.screen = dashboardDeploy
		model.deployment.result = domain.DeploymentExecutionReport{Operation: "deploy-apply", State: "partial", Phase: domain.DeploymentPhaseVerify, BuildCompleted: true, ApplyCompleted: true, Targets: mixedDeploymentPlan().Targets, Verification: domain.DeploymentVerificationSummary{Attempted: 2, Verified: 1, Recorded: 1, Targets: []domain.DeploymentTargetVerification{{Name: "pc01", State: "verified"}, {Name: "pc02", State: "unverified", Reachability: domain.ReachabilityUnreachable}}}, LogPath: "/private/deploy.log"}
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatal("result layout overflow")
		}
		combined := view
		for range 25 {
			updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
			model = updated.(dashboardModel)
			combined += model.View().Content
		}
		for _, want := range []string{"Some computers were not reached", "pc01 — Updated", "pc02 — Not reached", "Check power and networking", "Logs"} {
			if !strings.Contains(combined, want) {
				t.Fatalf("%v missing %q", size, want)
			}
		}
		model.pageScroll = 0
		model.deployment.result.RecoveryRequired = true
		model.deployment.result.ApplyCompleted = false
		model.deployment.result.State = "failed"
		model.deployment.result.Phase = domain.DeploymentPhaseApply
		view = model.View().Content
		if strings.Contains(view, "New review") || strings.Contains(view, "pc01 — Updated") || !strings.Contains(view, "Recovery required") {
			t.Fatalf("recovery weakened: %s", view)
		}
		updated, command := model.Update(demoText("r"))
		if command != nil || !updated.(dashboardModel).deployment.result.RecoveryRequired {
			t.Fatal("recovery allowed retry")
		}
	}
}
