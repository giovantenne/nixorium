package presentation

import (
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardPXEOverviewMsg struct {
	report domain.StatusReport
	err    error
}

// Opening PXE observes current state. Preparation and network changes remain
// explicit actions, with the existing guided setup and reviewed start boundary.
func (model dashboardModel) openNetworkInstallation() (tea.Model, tea.Cmd) {
	model.areaReturn = dashboardInstallationArea
	model.screen = dashboardPXE
	model.installation.flow = false
	model.installation.failed = false
	model.installation.stateError = false
	model.message = ""
	if model.actions.Refresh == nil {
		return model, nil
	}
	model.busy = "Checking network installation state"
	return model, func() tea.Msg {
		report, err := model.actions.Refresh()
		return dashboardPXEOverviewMsg{report: report, err: err}
	}
}

func (model dashboardModel) pxePrimaryAction() tuiAction {
	if model.installation.stateError {
		return tuiAction{key: "f", label: "Retry status"}
	}
	switch model.report.PXE.Mode {
	case "active":
		return tuiAction{key: "x", label: "Finish installation"}
	case "degraded", "recovery-required":
		return tuiAction{key: "r", label: "Recover network"}
	}
	if model.report.PXEPreparation.Ready {
		return tuiAction{key: "s", label: "Review start"}
	}
	return tuiAction{key: "p", label: "Configure and prepare"}
}
