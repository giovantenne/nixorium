package presentation

import (
	tea "charm.land/bubbletea/v2"
)

// handlePxeMessage handles the pxe message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handlePxeMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardRemoteFingerprintMsg:
		return model.handleRemoteFingerprintMessage(message)
	case dashboardRemoteInterfaceSavedMsg:
		return model.handleRemoteInterfaceSaved(message)
	case dashboardRemoteInstallMsg:
		next, cmd := model.handleRemoteInstallMessage(message)
		return model.withRemoteInstallWatch(next, cmd)
	case remoteInstallTickMsg:
		return model.handleRemoteInstallTick(message)
	case dashboardPXEOverviewMsg:
		model.busy = ""
		model.installation.stateError = message.err != nil
		if message.err != nil {
			model.message = "Network installation state could not be checked: " + message.err.Error()
		} else {
			model.report = message.report
			model.message = ""
		}
		return model, nil
	case dashboardPXELifecycleMsg:
		return model.handlePXELifecycle(message)
	case dashboardOperationMsg:
		preparationFinished := model.installation.pxePreparing && message.screen == dashboardPXE
		model.busy = ""
		if preparationFinished {
			model.installation.pxePreparing = false
		}
		model.message = message.message
		if message.err != nil {
			model.message += "; refresh failed: " + message.err.Error()
		} else {
			model.report = message.report
		}
		model.screen = message.screen
		if model.installation.flow && message.screen == dashboardPXE && model.report.PXE.Mode == "active" {
			model.installation.flow = false
			model.installation.failed = false
			model.setupMode = false
			model.areaReturn = dashboardInstallationArea
		}
		if preparationFinished && model.actions.LoadPXEProgress != nil {
			return model, model.loadPXEProgress(model.installation.pxeProgressID)
		}
		return model, nil
	case dashboardPXEProgressTickMsg:
		if !model.installation.pxePreparing || message.id != model.installation.pxeProgressID || model.actions.LoadPXEProgress == nil {
			return model, nil
		}
		return model, model.loadPXEProgress(message.id)
	case dashboardPXEProgressMsg:
		if message.id != model.installation.pxeProgressID {
			return model, nil
		}
		if message.err == nil && (model.installation.pxeStarted.IsZero() || !message.progress.StartedAt.Before(model.installation.pxeStarted)) {
			model.installation.pxeProgress = message.progress
		}
		if model.installation.pxePreparing {
			return model, schedulePXEProgressTick(message.id)
		}
		return model, nil
	case dashboardPXEExitMsg:
		model.busy = ""
		if message.lifecycle.HasErrors() {
			model.message = "Installation mode could not be stopped: " + message.lifecycle.Message
			model.screen = dashboardPXELeaveReview
			return model, nil
		}
		if message.statusErr != nil {
			model.message = "Installation mode stopped, but its final state could not be verified: " + message.statusErr.Error()
			model.screen = dashboardPXELeaveReview
			return model, nil
		}
		model.report = message.status
		if message.status.PXE.Mode == "active" {
			model.message = "Installation mode still reports as active; Nixorium remains open."
			model.screen = dashboardPXELeaveReview
			return model, nil
		}
		return model, tea.Quit
	case dashboardHostsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Computer status refresh failed: " + message.err.Error()
		} else {
			model.computers.hosts = message.report
			model.message = ""
		}
		if model.computers.refreshReturn != 0 {
			model.screen = model.computers.refreshReturn
			model.computers.refreshReturn = 0
			return model, nil
		}
		model.computers.hostCursor = 0
		model.screen = dashboardHosts
		return model, nil
	default:
		return model.handleBackupMessage(message)
	}
}
