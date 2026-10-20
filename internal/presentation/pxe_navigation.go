package presentation

import (
	"context"

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
	model.installation.pxeResult = domain.PXELifecycleReport{}
	model.noticeDetails = false
	model.message = ""
	if model.actions.Refresh == nil {
		return model, nil
	}
	model.busy = "Checking network installation state"
	return model.startRead(func(ctx context.Context) tea.Msg {
		report, err := model.actions.Refresh(ctx)
		return dashboardPXEOverviewMsg{report: report, err: err}
	})
}

func (model dashboardModel) pxePrimaryAction() tuiAction {
	if model.installation.stateError {
		return tuiAction{key: "r", label: "Retry status"}
	}
	switch model.report.PXE.Mode {
	case "active":
		return tuiAction{key: "x", label: "Finish installation"}
	case "degraded", "recovery-required":
		return tuiAction{key: "n", label: "Recover the network"}
	}
	if model.pxeJustFinished() && model.actions.LoadHosts != nil {
		// After network installation the next step is checking the
		// installed computers, as the screen says.
		return tuiAction{key: "h", label: "Check computers"}
	}
	if model.report.PXEPreparation.Ready {
		return tuiAction{key: "s", label: "Review start"}
	}
	return tuiAction{key: "p", label: "Configure and prepare"}
}
