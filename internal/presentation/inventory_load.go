package presentation

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type inventoryLoad struct {
	id     uint64
	cancel context.CancelFunc
	resume func(dashboardModel) (tea.Model, tea.Cmd)
}

type dashboardInventoryMsg struct {
	id     uint64
	report domain.StatusReport
	err    error
}

func (load *inventoryLoad) cancelRead() {
	if load.cancel != nil {
		load.cancel()
	}
	load.cancel = nil
	load.resume = nil
	load.id++
}

// Keep evaluated identities off the startup path, but require them before any
// client selection. Retain the originating screen on cancellation or failure.
func (model dashboardModel) loadInventoryThen(resume func(dashboardModel) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	if model.actions.LoadInventory == nil {
		model.message = "Computer inventory is unavailable in this session."
		return model, nil
	}
	model.inventory.cancelRead()
	ctx, activityID := model.beginRead(dashboardReadTimeout)
	model.inventory.cancel = model.read.cancel
	model.inventory.resume = resume
	model.busy = "Loading configured computers from the saved configuration"
	model.message = ""
	id := model.inventory.id
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg {
		report, err := model.actions.LoadInventory(ctx)
		return dashboardInventoryMsg{id: id, report: report, err: err}
	})
}

func (model dashboardModel) finishInventory(message dashboardInventoryMsg) (tea.Model, tea.Cmd) {
	if model.inventory.cancel == nil || message.id != model.inventory.id {
		return model, nil
	}
	resume := model.inventory.resume
	model.inventory.cancelRead()
	model.busy = ""
	if message.err == nil && message.report.Meta.Controller.Name == "" {
		message.err = errors.New("the evaluated inventory has no controller identity")
	}
	if message.err != nil {
		model.message = "Computer inventory could not be loaded: " + message.err.Error()
		return model, nil
	}
	model.report = message.report
	model.message = ""
	return resume(model)
}

func (model dashboardModel) openHostDeployment(name string) (tea.Model, tea.Cmd) {
	if model.report.Meta.Controller.Name == "" {
		return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) {
			return ready.openHostDeployment(name)
		})
	}
	for _, host := range model.report.Meta.Clients.Hosts {
		if host.Name != name {
			continue
		}
		model.screen = dashboardDeploy
		model.deployment.result = domain.DeploymentExecutionReport{}
		model.deployment.context = ""
		model.deployment.chosen = map[string]bool{name: true}
		model.deployment.cursor = 0
		return model, nil
	}
	model.message = "The selected computer is no longer in the configured inventory."
	return model, nil
}
