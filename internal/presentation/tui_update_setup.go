package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// handleSetupMessage handles the setup message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleSetupMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case computerSetupMsg:
		return model.finishComputerSetup(message)
	case disclaimerMsg:
		return model.finishDisclaimer(message)
	case dashboardBeginInitialMsg:
		if !model.initializing || model.actions.LoadInitial == nil {
			return model, nil
		}
		return model.loadInitial()
	case hostTrustPlanMsg:
		if model.screen != dashboardHostTrust || model.hostTrust.cancel == nil || model.hostTrust.id != message.id {
			return model, nil
		}
		model.hostTrust.cancel()
		model.hostTrust.cancel = nil
		model.hostTrust.plan = message.plan
		model.busy = ""
		return model, nil
	case hostTrustResultMsg:
		if model.screen != dashboardHostTrust || !model.hostTrust.applying || message.id != model.hostTrust.id {
			return model, nil
		}
		model.hostTrust.applying = false
		model.busy = ""
		model.hostTrust.result = message.result
		if message.result.State == "saved" && model.actions.LoadHosts != nil {
			// Check the computers again at once, so its details and the
			// update selection show the reviewed key instead of the warning.
			model.screen = dashboardHosts
			next, read := model.checkComputersThen()
			updated := next.(dashboardModel)
			updated.message = "The reviewed SSH key of " + message.result.Host + " is saved; checking the computers again."
			return updated, read
		}
		return model, nil
	case dashboardInitialMsg:
		model.jobs.items, model.jobs.err, model.jobs.id = message.jobs, message.jobsErr, message.jobsID
		model.usbReserved = message.usbReserved
		model.recovery.report = message.recovery
		poll := model.scheduleManagedJobsTick()
		model.busy = ""
		model.initializing = false
		if message.err != nil {
			model.initialError = true
			model.message = "The laboratory could not be opened: " + message.err.Error()
			model.screen = dashboardHome
			return model, poll
		}
		model.initialError = false
		model.report = message.report
		model.setup = message.setup
		model.observeSetupController(message.setup)
		model.observeRecoveryController(message.recovery)
		if (model.setupMode || (model.actions.LoadDisclaimer == nil && setupNeedsImmediateAttention(message.setup))) && model.managedJobConflict() == "" {
			next, command := model.beginComputerInstallation("")
			return next, tea.Batch(command, poll)
		} else if model.actions.ClassroomMode {
			model.screen = dashboardComputersArea
		} else {
			model.screen = dashboardHome
		}
		// The outcome of the update that reopened this interface, if any.
		model.message, model.reopenNotice = model.reopenNotice, ""
		if model.actions.LoadDisclaimer != nil && !model.actions.ClassroomMode {
			next, invitation := model.beginTelemetryInvitation()
			return next, tea.Batch(poll, invitation, model.checkUpdateNotification())
		}
		return model, tea.Batch(poll, model.checkTelemetryOffer(), model.checkUpdateNotification())
	case dashboardSetupMsg:
		model.busy = ""
		model.setup = message.report
		model.observeSetupController(message.report)
		if model.installation.flow {
			return model.continueComputerInstallation(message.report)
		}
		model.screen = dashboardSetup
		return model, nil
	case dashboardSetupKeyStatusMsg:
		model.busy = ""
		model.keys.report = message.report
		model.keys.cursor = min(model.keys.cursor, max(0, len(message.report.Keys)-1))
		model.screen = dashboardSetupKeys
		return model, nil
	case dashboardSetupKeyImportMsg:
		model.busy = ""
		model.keys.importing = false
		model.keys.report = message.keys
		if message.err != nil {
			model.message = message.report.Message + " " + firstValidationIssue(message.report.Issues)
		} else {
			model.message = message.report.Message + " Fingerprint: " + message.report.Fingerprint
		}
		model.screen = dashboardSetupKeys
		return model, nil
	case dashboardSetupKeysMsg:
		model.busy = ""
		if !message.save.HasErrors() && message.save.State == "saved" {
			model.noteControllerSave(message.save.Revision)
		}
		if model.installation.flow {
			switch {
			case message.keyErr != nil:
				return model.failComputerInstallation("Controller key preparation failed: " + message.keyErr.Error())
			case message.keys.State != "ready":
				return model.failComputerInstallation("Controller key preparation did not complete.")
			case message.save.HasErrors():
				return model.failComputerInstallation(message.save.Message)
			case message.install.HasErrors():
				return model.failComputerInstallation("Controller keys are ready, but protected installation failed: " + message.install.Message)
			}
			model.busy = "Checking saved laboratory configuration"
			return model.loadSetup()
		}
		if model.keys.back == dashboardSettings {
			switch {
			case message.keyErr != nil:
				model.message = "Key preparation needs attention: " + message.keyErr.Error()
			case message.keys.State != "ready":
				model.message = "Key preparation did not complete."
			case message.save.HasErrors():
				model.message = message.save.Message
			case message.install.HasErrors():
				model.message = "Keys are ready, but protected installation failed: " + message.install.Message
			default:
				model.message = "Controller keys are ready and installed."
			}
			model.keys.report = message.keys
			model.screen = dashboardSettings
			return model, nil
		}
		switch {
		case message.keyErr != nil:
			model.message = "Key preparation needs attention: " + message.keyErr.Error()
		case message.keys.State != "ready":
			model.message = "Key preparation did not complete. Open technical steps for details."
		case message.save.HasErrors():
			model.message = message.save.Message
		case message.install.HasErrors():
			model.message = "Keys are ready, but protected controller installation failed: " + message.install.Message
		default:
			model.message = "Controller keys are ready and protected material was installed."
		}
		model.screen = dashboardSetup
		if model.actions.LoadSetup != nil {
			model.busy = "Refreshing setup progress"
			return model.loadSetup()
		}
		return model, nil
	case dashboardSetupSaveMsg:
		model.busy = ""
		if !message.report.HasErrors() && message.report.State == "saved" {
			model.noteControllerSave(message.report.Revision)
		}
		if model.installation.flow {
			if message.report.HasErrors() || (message.report.State != "saved" && message.report.State != "unchanged") {
				return model.failComputerInstallation(message.report.Message)
			}
			model.busy = "Checking saved laboratory configuration"
			return model.loadSetup()
		}
		model.message = message.report.Message
		model.screen = dashboardSetup
		if model.actions.LoadSetup != nil {
			model.busy = "Refreshing setup progress"
			return model.loadSetup()
		}
		return model, nil
	case dashboardPlanMsg:
		model.busy = ""
		model.installation.startPlan = message.report
		if message.report.HasErrors() {
			if model.installation.flow {
				return model.failComputerInstallation(message.report.Message)
			}
			model.message = message.report.Message
			model.screen = dashboardPXE
			return model, nil
		}
		if message.report.Mode == "active" {
			model.message = "PXE installation mode is already active."
			model.screen = dashboardPXE
			model.installation.flow = false
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardPXEStartReview
		return model, nil
	case dashboardInstallationPrepareMsg:
		model.busy = ""
		model.installation.pxePreparing = false
		if message.statusErr == nil {
			model.report = message.status
		}
		if message.report.HasErrors() {
			return model.failComputerInstallation("Client preparation failed: " + message.report.Message)
		}
		model.message = message.report.Message
		model.installation.stage = 5
		if model.actions.PlanPXEStart == nil {
			return model.failComputerInstallation("PXE start validation is not available in this session.")
		}
		model.busy = "Checking PXE readiness"
		next, plan := model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardPlanMsg{report: model.actions.PlanPXEStart(ctx)}
		})
		if model.actions.LoadPXEProgress != nil {
			return next, tea.Batch(model.loadPXEProgress(model.installation.pxeProgressID), plan)
		}
		return next, plan
	default:
		return model.handlePxeMessage(message)
	}
}
