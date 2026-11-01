package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateControllerKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" || key.String() == "left" || (key.String() == "enter" && model.controller.result.Operation != "") {
		if model.setupMode {
			model.screen = dashboardSetup
		} else {
			model.screen = dashboardHome
		}
		if model.controllerObservation == controllerOverviewCurrent && controllerVerifiedForSave(model.controller.result.Revision, model.controller.result) {
			model.message = "Controller configuration activated and verified."
		} else {
			model.message = ""
		}
		if model.setupMode && model.actions.LoadSetup != nil {
			model.busy = "Refreshing first-run progress"
			return model.loadSetup()
		}
	} else if key.String() == "d" && model.controller.result.Operation != "" {
		model.controller.details = !model.controller.details
	} else if key.String() == "l" && model.controller.result.Operation != "" {
		model.screen = dashboardLogs
		model.busy = "Loading private operation logs"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardLogsMsg{report: model.actions.LoadLogs(ctx)}
		})
	} else if key.String() == "n" {
		model.busy = "Reviewing controller revision and active system"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardControllerPlanMsg{report: model.actions.PlanController(ctx)}
		})
	}
	return model, nil
}

func (model dashboardModel) updateControllerReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.screen = dashboardController
		if model.controller.fromSave {
			model.screen = model.controller.saveOrigin
			model.controller.fromSave = false
		}
		model.message = "Apply to controller cancelled; nothing was started."
		if controllerPlanCurrent(model.controller.plan) {
			model.message = ""
		}
	case "enter":
		if controllerPlanCurrent(model.controller.plan) {
			model.message = "This controller is already up to date; nothing was started."
			return model, nil
		}
		return model.startReviewedControllerApply()
	}
	return model, nil
}

func (model dashboardModel) updateServicesKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.maintenance.serviceResult.Operation != "" {
		switch key.String() {
		case "enter", "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "l":
			model.busy = "Loading private operation logs"
			model.message = ""
			return model.startRead(func(ctx context.Context) tea.Msg {
				return dashboardLogsMsg{report: model.actions.LoadLogs(ctx)}
			})
		case "c":
			model.maintenance.serviceResult = domain.ServiceActionReport{}
			model.confirmation = ""
			model.message = ""
			model.screen = dashboardServicesRestartReview
		}
		return model, nil
	}
	switch key.String() {
	case "esc", "left":
		model.screen = dashboardHome
		model.message = ""
	case "r":
		model.busy = "Refreshing managed controller services"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardServicesMsg{report: model.actions.LoadServices(ctx)}
		})
	case "c":
		if len(model.maintenance.services.Services) == 0 || model.maintenance.services.Services[0].ID != "cache" || len(model.maintenance.services.Services[0].Units) == 0 || !model.maintenance.services.Services[0].Units[0].Loaded {
			model.message = "Binary cache restart is unavailable because the managed unit is not installed."
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardServicesRestartReview
	}
	return model, nil
}

func (model dashboardModel) updateServicesRestartReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.screen = dashboardServices
		model.confirmation = ""
		model.message = "Service restart cancelled; no action was started."
	case "enter":
		model.busy = "Restarting and verifying the binary cache"
		model.confirmation = ""
		model.message = ""
		return model, func() tea.Msg {
			return dashboardServiceResultMsg{report: model.actions.RestartService("cache")}
		}
	}
	return model, nil
}

func (model dashboardModel) updateLogsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left":
		if model.setupMode {
			model.screen = dashboardSetup
		} else {
			model.screen = dashboardHome
		}
		model.message = ""
		if model.setupMode && model.actions.LoadSetup != nil {
			model.busy = "Refreshing first-run progress"
			return model.loadSetup()
		}
	case "up", "k":
		if model.maintenance.logCursor > 0 {
			model.maintenance.logCursor--
		}
	case "down", "j":
		if model.maintenance.logCursor+1 < len(model.maintenance.logs.Logs) {
			model.maintenance.logCursor++
		}
	case "r":
		model.busy = "Refreshing private operation logs"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardLogsMsg{report: model.actions.LoadLogs(ctx)}
		})
	case "enter":
		if len(model.maintenance.logs.Logs) == 0 || !model.maintenance.logs.Logs[model.maintenance.logCursor].Available {
			model.message = "The selected operation log is not available for safe reading."
			return model, nil
		}
		id := model.maintenance.logs.Logs[model.maintenance.logCursor].ID
		model.busy = "Reading the bounded operation log tail"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardLogMsg{report: model.actions.LoadLog(ctx, id)}
		})
	}
	return model, nil
}

func (model dashboardModel) updateLogDetailKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	maximum := maximumLogScroll(model.maintenance.logDetail, model.logDetailHeight())
	switch key.String() {
	case "esc", "left":
		model.screen = dashboardLogs
		model.message = ""
	case "up", "k":
		if model.maintenance.logScroll > 0 {
			model.maintenance.logScroll--
		}
	case "down", "j":
		if model.maintenance.logScroll < maximum {
			model.maintenance.logScroll++
		}
	case "pgup":
		model.maintenance.logScroll -= model.logDetailHeight()
		if model.maintenance.logScroll < 0 {
			model.maintenance.logScroll = 0
		}
	case "pgdown":
		model.maintenance.logScroll += model.logDetailHeight()
		if model.maintenance.logScroll > maximum {
			model.maintenance.logScroll = maximum
		}
	case "home":
		model.maintenance.logScroll = 0
	case "end":
		model.maintenance.logScroll = maximum
	}
	return model, nil
}
