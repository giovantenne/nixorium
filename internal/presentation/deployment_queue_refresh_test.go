package presentation

import (
	"context"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDeploymentResultRefreshesQueuedUpdatesOnOverview(t *testing.T) {
	queued := domain.RecoveryReport{Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryDeferredUpdates, Title: "2 computers will update when switched on"}}}
	model := dashboardModel{screen: dashboardDeploy, width: 80, height: 24, deployment: deploymentModel{applying: true},
		actions: DashboardActions{LoadRecovery: func(context.Context) domain.RecoveryReport { return queued }}}
	updated, cmd := model.Update(dashboardDeploymentResultMsg{report: domain.DeploymentExecutionReport{Operation: "deploy-apply", State: "completed"}})
	model = updated.(dashboardModel)
	if cmd == nil || model.busy != "" {
		t.Fatal("deployment result did not observe the overview conditions")
	}
	updated, _ = model.Update(cmd())
	model = updated.(dashboardModel)
	for _, task := range model.pendingTasks() {
		if task.id == "pending-deferred" && task.title == "2 computers will update when switched on" {
			return
		}
	}
	t.Fatal("queued updates missing from the overview", model.pendingTasks())
}
