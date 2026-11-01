package presentation

import (
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func (model dashboardModel) updateState(message tea.Msg) (tea.Model, tea.Cmd) {
	model.ensureActivitySpinner()
	if updated, command, handled := model.handleNetworkChangeMessage(message); handled {
		return updated, command
	}
	return model.handleAssistantMessage(message)
}

func (model dashboardModel) updateKeyState(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if ok && model.screen == dashboardDisclaimer {
		return model.updateDisclaimer(key)
	}
	if !ok {
		if model.screen == dashboardAssistant && !model.navigation.helpOpen && model.assistant.state == "chat" {
			var command tea.Cmd
			model.assistant.input, command = model.assistant.input.Update(message)
			return model, command
		}
		if paste, pasted := message.(tea.PasteMsg); pasted && model.screen == dashboardAssistant &&
			!model.navigation.helpOpen && model.assistant.state == "name" {
			var command tea.Cmd
			model.assistant.labName, command = model.assistant.labName.Update(tea.PasteMsg{Content: strings.Join(strings.Fields(paste.Content), " ")})
			return model, command
		}
		// Activation codes are usually pasted from the email: accept the paste,
		// without surrounding spaces or line breaks.
		if paste, pasted := message.(tea.PasteMsg); pasted && model.screen == dashboardAssistant &&
			!model.navigation.helpOpen && model.assistant.state == "activate" {
			var command tea.Cmd
			model.assistant.codeStep = true
			focus := model.assistant.code.Focus()
			model.assistant.code, command = model.assistant.code.Update(tea.PasteMsg{Content: strings.TrimSpace(paste.Content)})
			return model, tea.Batch(focus, command)
		}
		if model.busy != "" {
			if _, pasted := message.(tea.PasteMsg); pasted {
				model.message = "Text cannot be changed while work is running. Use the available actions below."
			}
			return model, nil
		}
		if paste, pasted := message.(tea.PasteMsg); pasted && !model.navigation.helpOpen {
			if next, command, handled := model.pasteIntoInput(paste); handled {
				return next, command
			}
		}
		if model.screen == dashboardHome {
			model.ensureHomeMenu()
			model.homeMenu, _ = model.homeMenu.update(message)
		}
		if model.screen == dashboardSettings {
			model.settings.menu, _ = model.settings.menu.update(message)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settings.passwordMenu, _ = model.settings.passwordMenu.update(message)
		}
		if model.screen == dashboardSettingsAccounts {
			model.settings.accountsMenu, _ = model.settings.accountsMenu.update(message)
		}
		if model.screen == dashboardSettingsEdit {
			updated, command := model.settings.editor.Update(message)
			model.settings.editor = updated.(settingsWizardModel)
			return model, command
		}
		return model, nil
	}
	if model.busy == "" && !model.navigation.helpOpen && model.screen == dashboardSettings && key.String() != "f1" && (model.settings.menu.list.FilterState() == list.Filtering || (model.settings.menu.list.FilterState() == list.FilterApplied && key.String() == "esc")) {
		var command tea.Cmd
		model.settings.menu, command = model.settings.menu.update(key)
		return model, command
	}
	if model.navigation.helpOpen {
		if model.screen == dashboardAssistant && (model.assistant.state == "chat" || model.assistant.state == "running") {
			switch key.String() {
			case "down":
				model.navigation.pageScroll++
			case "up":
				model.navigation.pageScroll = max(0, model.navigation.pageScroll-1)
			case "pgdown":
				model.navigation.pageScroll += max(1, model.height-5)
			case "pgup":
				model.navigation.pageScroll = max(0, model.navigation.pageScroll-max(1, model.height-5))
			case "home":
				model.navigation.pageScroll = 0
			case "end":
				model.navigation.pageScroll = len(strings.Split(assistantHelpText, "\n"))
			}
		}
		if key.String() == "?" || key.String() == "f1" || key.String() == "esc" {
			model.navigation.helpOpen = false
			model.navigation.pageScroll = 0
		}
		if key.String() == "shift+down" {
			model.navigation.pageScroll++
		}
		if key.String() == "shift+up" {
			model.navigation.pageScroll = max(0, model.navigation.pageScroll-1)
		}
		return model, nil
	}
	if key.String() == "f1" || (key.String() == "?" && !model.textEntry()) {
		model.navigation.helpOpen = true
		model.navigation.pageScroll = 0
		return model, nil
	}
	if next, command, handled := model.updateInterfaceReloadKey(key); handled {
		return next, command
	}
	// Ctrl+C closes Nixorium here as on every other screen, except where the
	// Assistant uses it to stop its own running request or step.
	if model.screen == dashboardAssistant && (key.String() != "ctrl+c" || model.assistantUsesCtrlC()) {
		return model.updateAssistantKey(key)
	}
	if model.screen == dashboardAssistantReview && model.read.cancel == nil && key.String() != "ctrl+c" {
		return model.updateAssistantReviewKey(key)
	}
	if model.read.cancel == nil && model.screen == dashboardWorkspace && key.String() != "ctrl+c" && (key.String() != "q" || model.textEntry()) {
		return model.updateWorkspaceKey(key)
	}
	if model.read.cancel == nil && model.screen == dashboardHomeFiles && key.String() != "ctrl+c" && (key.String() != "q" || model.textEntry()) {
		return model.updateHomeFilesKey(key)
	}
	if model.read.cancel == nil && model.screen == dashboardTemplateReset && key.String() != "ctrl+c" && (key.String() != "q" || model.textEntry()) {
		return model.updateTemplateResetKey(key)
	}
	if key.String() == "shift+down" {
		model.navigation.pageScroll++
		return model, nil
	}
	if key.String() == "shift+up" {
		model.navigation.pageScroll = max(0, model.navigation.pageScroll-1)
		return model, nil
	}
	if model.read.cancel != nil {
		switch key.String() {
		case "esc":
			return model.cancelActivity(), nil
		case "ctrl+c":
			model = model.cancelActivity()
			// Keep the ordinary active-PXE exit review below.
		case "l":
			if model.updates.planning {
				if model.updatePlanDetailsAvailable() {
					model.progressDetails = !model.progressDetails
				}
				return model, nil
			}
			fallthrough
		default:
			model.message = "Loading information. Press Esc to cancel, or F1 for help."
			return model, nil
		}
	}
	if next, command, handled := model.updateResultSituation(key.String()); handled {
		return next, command
	}
	if model.screen == dashboardTelemetry && (model.telemetry.applying || (key.String() != "ctrl+c" && key.String() != "q")) {
		return model.updateTelemetryKey(key)
	}
	if model.screen == dashboardSupport && key.String() != "ctrl+c" && key.String() != "q" {
		return model.updateSupportKey(key)
	}
	if model.updates.packageBase && model.updates.baseEditing && model.screen == dashboardUpdate && model.busy == "" && key.String() != "ctrl+c" {
		return model.updatePackageBaseKey(key)
	}
	if model.screen == dashboardHosts && model.computers.hostSearching {
		switch key.String() {
		case "esc":
			model.computers.hostSearching = false
			model.computers.hostQuery = ""
		case "enter":
			model.computers.hostSearching = false
		case "backspace":
			model.computers.hostQuery = dropLastRune(model.computers.hostQuery)
		default:
			model.computers.hostQuery = appendInputText(model.computers.hostQuery, key.Text, hostSearchLimit)
		}
		model.computers.hostCursor = 0
		return model, nil
	}
	if model.screen == dashboardSoftware {
		software, input := model.software.updateSearchInput(key, model.actions.SearchSoftware)
		model.software = software
		if input.handled {
			if input.clearMessage {
				model.message = ""
			}
			if input.delegate {
				return model.updateSoftware(key)
			}
			return model, input.command
		}
	}
	if key.String() == "l" && model.progressOperationRunning() {
		// With nothing to expand the key is inert: no toggle, no empty view.
		if model.progressDetailsAvailable() {
			model.progressDetails = !model.progressDetails
			model.navigation.pageScroll = 0
		}
		return model, nil
	}
	if model.deployment.applying && model.deployment.stopReview {
		return model.updateDeploymentStop(key)
	}
	if key.String() == "s" && model.deployment.applying && !model.deployment.stopRequested && model.deployment.cancel != nil {
		model.deployment.stopReview = true
		model.deployment.confirmation = ""
		return model, nil
	}
	if (key.String() == "ctrl+c" || key.String() == "q") && ((model.busy != "" && !model.hasCancellableRead() && !model.installation.pxePreparing) || model.hostTrust.applying || model.deployment.applying || model.controller.applying || model.updates.applying || model.settings.applying || model.workspace.saving || model.homeFiles.saving || model.support.saving || model.templateReset.saving || model.software.mutating() || model.shutdown.applying || model.internet.applying) {
		model.message = "Changes are being applied. Wait for the result before closing Nixorium."
		return model, nil
	}
	exitKey := key.String() == "ctrl+c" || (key.String() == "q" && !model.textEntry())
	if model.inventory.cancel != nil && (exitKey || key.String() == "esc") {
		model.inventory.cancelRead()
		model.busy = ""
		if !exitKey {
			model.message = ""
			return model, nil
		}
	}
	if exitKey && model.screen == dashboardUSBInstall {
		model.installation.remote.password = ""
	}
	if exitKey && model.report.PXE.Mode == "active" {
		if model.screen != dashboardPXELeaveReview {
			model.screen = dashboardPXELeaveReview
			model.confirmation = ""
			model.message = ""
		}
		return model, nil
	}
	if exitKey {
		model.backup.clearSecrets()
		if model.hostTrust.cancel != nil {
			model.hostTrust.cancel()
			model.hostTrust.cancel = nil
			model.hostTrust.id++
		}
		model.templateReset.cancelRead()
		model.workspace.cancelRead()
		model.support.cancelRead()
		model.software.cancelSearch()
		return model, tea.Quit
	}
	if model.screen == dashboardHostTrust {
		return model.updateHostTrustKey(key)
	}
	if model.busy != "" {
		model.message = "This action is unavailable while work is running. Wait for its result; F1 opens help."
		return model, nil
	}
	if model.screen == dashboardManagedJobs {
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
		case "tab", "down":
			model.jobs.cursor = (model.jobs.cursor + 1) % max(1, len(model.jobs.items))
		case "up":
			count := max(1, len(model.jobs.items))
			model.jobs.cursor = (model.jobs.cursor + count - 1) % count
		}
		return model, nil
	}
	if key.String() == "v" && model.actions.LoadManagedJobs != nil && !model.actions.ClassroomMode && model.screen == dashboardHome {
		model.screen = dashboardManagedJobs
		model.message = ""
		for index, job := range model.jobs.items {
			if job.State == "running" || job.State == "interrupted" {
				model.jobs.cursor = index
				break
			}
		}
		return model, nil
	}
	if model.screen == dashboardAdministration {
		model.returnAdmin = true
		switch key.String() {
		case "esc", "left":
			model.returnAdmin = false
			model.screen = dashboardHome
			return model, nil
		case "up", "k":
			model.navigation.adminCursor = max(0, model.navigation.adminCursor-1)
			return model, nil
		case "down", "j":
			model.navigation.adminCursor = min(len(administrationTasks)-1, model.navigation.adminCursor+1)
			return model, nil
		case "enter":
			key = tea.KeyPressMsg{Code: []rune(administrationTasks[model.navigation.adminCursor].shortcut)[0], Text: administrationTasks[model.navigation.adminCursor].shortcut}
		}
		if key.String() == "c" {
			return model.openControllerReview()
		}
		if key.String() == "i" {
			model.diagnostics.back = dashboardAdministration
			model.screen = dashboardDiagnostics
			return model.startDiagnostics()
		}
		known := false
		for _, task := range administrationTasks {
			if key.String() == task.shortcut {
				known = true
				break
			}
		}
		if !known {
			return model, nil
		}
		return model.openMaintenanceTask(key.String())
	}
	if key.String() == "i" && model.screen == dashboardHosts && !model.actions.ClassroomMode {
		model.diagnostics.back = model.screen
		model.screen = dashboardDiagnostics
		return model.startDiagnostics()
	}
	return model.updatePrimaryScreenKey(key)
}
