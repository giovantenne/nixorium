package presentation

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDeploymentReviewPlanUsesTheBuildReadLimit(t *testing.T) {
	plan := mixedDeploymentPlan()
	actions := DashboardActions{
		PlanDeployment:          func(context.Context, string) domain.DeploymentPlanReport { return plan },
		PlanReachableDeployment: func(context.Context, domain.DeploymentPlanReport) domain.DeploymentPlanReport { return plan },
	}
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, actions, false)
	opened, command := model.planSelectedDeployment("pc01,pc02")
	if command == nil || opened.(dashboardModel).read.limit != dashboardBuildTimeout {
		t.Fatalf("review plan limit is %v, want %v", opened.(dashboardModel).read.limit, dashboardBuildTimeout)
	}

	model = newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, actions, false)
	model.screen = dashboardDeployReview
	model.deployment.plan = plan
	updated, command := model.Update(demoCode(tea.KeyF2))
	if command == nil || updated.(dashboardModel).read.limit != dashboardBuildTimeout {
		t.Fatalf("reachable-only plan limit is %v, want %v", updated.(dashboardModel).read.limit, dashboardBuildTimeout)
	}
}
