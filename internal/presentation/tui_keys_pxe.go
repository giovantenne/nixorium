package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updatePXEKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := key.String()
	if action == "enter" {
		action = model.pxePrimaryAction().key
	}
	if model.installation.stateError && action != "r" && action != "esc" && action != "left" {
		return model, nil
	}
	switch action {
	case "esc", "left":
		if model.installation.flow && model.installation.failed {
			model.installation.flow = false
			model.installation.failed = false
			model.screen = dashboardHome
			model.message = ""
			return model, nil
		}
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
	case "r":
		return model.openNetworkInstallation()
	case "p":
		if model.report.PXE.Mode == "active" || model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required" {
			return model, nil
		}
		return model.beginComputerInstallation("pxe")
	case "s":
		if !model.report.PXEPreparation.Ready || model.report.PXE.Mode == "active" || model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required" || model.actions.PlanPXEStart == nil {
			return model, nil
		}
		model.busy = "Checking PXE readiness"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardPlanMsg{report: model.actions.PlanPXEStart(ctx)}
		})
	case "x":
		if model.actions.StopPXE == nil || model.actions.Refresh == nil || (model.report.PXE.Mode != "active" && model.report.PXE.Mode != "degraded" && model.report.PXE.Mode != "recovery-required") {
			return model, nil
		}
		model.busy = "Stopping network installation and checking the controller network"
		model.message = ""
		model.installation.pxeResult = domain.PXELifecycleReport{}
		return model, model.runPXELifecycle(model.actions.StopPXE)
	case "n":
		if !pxeRecoveryMode(model.report.PXE.Mode) || model.actions.RecoverPXE == nil || model.actions.Refresh == nil {
			return model, nil
		}
		// The review on this screen says what Enter does; recovery
		// changes only the controller's network (ADR 0026).
		model.busy = "Recovering the controller network"
		model.message = ""
		model.installation.pxeResult = domain.PXELifecycleReport{}
		return model, model.runPXELifecycle(model.actions.RecoverPXE)
	case "d":
		if model.installation.pxeResult.HasErrors() {
			model.noticeDetails = !model.noticeDetails
		}
	case "h":
		if !model.pxeJustFinished() || model.actions.LoadHosts == nil {
			return model, nil
		}
		model.computers.hostDetail = false
		model.computers.hostTechnical = false
		model.computers.configurationState = domain.ConfigurationStateReport{}
		model.screen = dashboardHosts
		model.busy = "Checking configured computers"
		model.message = ""
		return model.loadHosts()
	}
	return model, nil
}

func (model dashboardModel) updateDiagnosticsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "e":
		return model.openSupportPreview()
	case "esc", "left":
		model.screen = model.diagnosticReturn
		model.message = ""
	case "up", "k":
		model.diagnosticCursor = max(0, model.diagnosticCursor-1)
	case "down", "j":
		model.diagnosticCursor = max(0, min(len(model.doctor.Findings)-1, model.diagnosticCursor+1))
	case "enter":
		model.diagnosticDetails = !model.diagnosticDetails
	case "r":
		return model.startDiagnostics()
	}
	return model, nil
}

func (model dashboardModel) updatePXEStartReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.screen = dashboardPXE
		model.confirmation = ""
		model.message = "PXE start cancelled; networking was not changed."
	case "enter":
		model.busy = "Starting managed PXE installation mode"
		model.confirmation = ""
		model.message = ""
		return model, model.runAction(func() string {
			return model.actions.StartPXE().Message
		}, dashboardPXE)
	}
	return model, nil
}

func (model dashboardModel) updatePXELeaveReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.screen = dashboardPXE
		model.confirmation = ""
		model.message = "Continue the installation or stop PXE before leaving."
	case "enter":
		return model, tea.Quit
	case "x":
		if model.actions.StopPXE == nil || model.actions.Refresh == nil {
			model.message = "Installation mode cannot be stopped from this session. Return and use the installation controls."
			return model, nil
		}
		model.busy = "Stopping installation mode before exit"
		model.confirmation = ""
		model.message = ""
		return model, func() tea.Msg {
			lifecycle := model.actions.StopPXE()
			ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
			defer cancel()
			status, err := model.actions.Refresh(ctx)
			return dashboardPXEExitMsg{lifecycle: lifecycle, status: status, statusErr: err}
		}
	}
	return model, nil
}
