package presentation

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestUpdateComputersSelectsComputersWithAnObsoleteQueuedUpdate(t *testing.T) {
	status := domain.DeferredUpdateStatus{Updates: []domain.DeferredUpdateEntry{
		{DeferredUpdate: domain.DeferredUpdate{Host: "pc02"}, Stale: true},
		{DeferredUpdate: domain.DeferredUpdate{Host: "pc03"}},
		{DeferredUpdate: domain.DeferredUpdate{Host: "pc09"}, Stale: true},
	}}
	model := dashboardModel{screen: dashboardComputersArea, width: 100, height: 30,
		actions: DashboardActions{LoadDeferredUpdates: func(context.Context) domain.DeferredUpdateStatus { return status }}}
	model.report.Meta.Controller.Name = "pc99"
	model.report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}, {Name: "pc03"}}
	updated, command := model.openComputerTask("d")
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || command == nil {
		t.Fatalf("Update computers did not load the queue: screen %v", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if !model.deployment.chosen["pc02"] || model.deployment.chosen["pc03"] || model.deployment.chosen["pc09"] || !strings.Contains(model.message, "pc02") {
		t.Fatalf("selection %v, message %q", model.deployment.chosen, model.message)
	}

	// A selection the administrator already made is never replaced.
	model.deployment.chosen = map[string]bool{"pc01": true}
	model = model.applyWaitingSelection(status)
	if !model.deployment.chosen["pc01"] || model.deployment.chosen["pc02"] {
		t.Fatalf("existing selection changed: %v", model.deployment.chosen)
	}
}
