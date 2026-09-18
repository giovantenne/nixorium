package presentation

import (
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateState(message tea.Msg) (tea.Model, tea.Cmd) {
	model.ensureActivitySpinner()
	switch message := message.(type) {
	case dashboardInitialMsg:
		model.busy = ""
		model.initializing = false
		if message.err != nil {
			model.initialError = true
			model.message = "The laboratory could not be opened: " + message.err.Error()
			model.screen = dashboardHome
			return model, nil
		}
		model.initialError = false
		model.report = message.report
		model.setup = message.setup
		if model.setupMode || setupNeedsImmediateAttention(message.setup) {
			model.setupMode = true
			model.screen = dashboardSetup
		} else {
			model.screen = dashboardHome
		}
		model.message = ""
		return model, nil
	case dashboardStatusMsg:
		if message.err == nil {
			model.report = message.report
		} else {
			model.hosts = domain.HostsReport{}
			model.message = "Status refresh failed. Check the laboratory again."
		}
		return model, nil
	case dashboardDoctorMsg:
		model.busy = ""
		model.doctor = message.report
		model.diagnosticCursor = 0
		if message.err != nil {
			model.message = "Checks could not finish: " + message.err.Error()
		} else {
			model.message = ""
		}
		return model, nil
	case spinner.TickMsg:
		var command tea.Cmd
		model.activitySpinner, command = model.activitySpinner.Update(message)
		return model, command
	case dashboardSetupMsg:
		model.busy = ""
		model.setup = message.report
		model.screen = dashboardSetup
		return model, nil
	case dashboardSetupKeyStatusMsg:
		model.busy = ""
		model.setupKeys = message.report
		model.setupKeyCursor = min(model.setupKeyCursor, max(0, len(message.report.Keys)-1))
		model.screen = dashboardSetupKeys
		return model, nil
	case dashboardSetupKeyImportMsg:
		model.busy = ""
		model.setupKeyImporting = false
		model.setupKeyImportResult = message.report
		model.setupKeys = message.keys
		if message.err != nil {
			model.message = message.report.Message + " " + firstValidationIssue(message.report.Issues)
		} else {
			model.message = message.report.Message + " Fingerprint: " + message.report.Fingerprint
		}
		model.screen = dashboardSetupKeys
		return model, nil
	case dashboardSetupKeysMsg:
		model.busy = ""
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
			return model, model.loadSetup()
		}
		return model, nil
	case dashboardSetupSaveMsg:
		model.busy = ""
		model.message = message.report.Message
		model.screen = dashboardSetup
		if model.actions.LoadSetup != nil {
			model.busy = "Refreshing setup progress"
			return model, model.loadSetup()
		}
		return model, nil
	case dashboardPlanMsg:
		model.busy = ""
		model.startPlan = message.report
		if message.report.HasErrors() {
			model.message = message.report.Message
			model.screen = dashboardPXE
			return model, nil
		}
		if message.report.Mode == "active" {
			model.message = "PXE installation mode is already active."
			model.screen = dashboardPXE
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardPXEStartReview
		return model, nil
	case dashboardOperationMsg:
		preparationFinished := model.pxePreparing && message.screen == dashboardPXE
		model.busy = ""
		if preparationFinished {
			model.pxePreparing = false
		}
		model.message = message.message
		if message.err != nil {
			model.message += "; refresh failed: " + message.err.Error()
		} else {
			model.report = message.report
		}
		model.screen = message.screen
		if message.screen == dashboardPXE && model.guidedInstallation() && model.pilotPractical && model.report.PXE.Mode != "active" {
			model.pilotName = ""
			model.pilotPractical = false
			model.installationSummary = true
			model.hosts = domain.HostsReport{}
		}
		if preparationFinished && model.actions.LoadPXEProgress != nil {
			return model, model.loadPXEProgress(model.pxeProgressID)
		}
		return model, nil
	case dashboardPXEProgressTickMsg:
		if !model.pxePreparing || message.id != model.pxeProgressID || model.actions.LoadPXEProgress == nil {
			return model, nil
		}
		return model, model.loadPXEProgress(message.id)
	case dashboardPXEProgressMsg:
		if message.id != model.pxeProgressID {
			return model, nil
		}
		if message.err == nil && (model.pxeProgressStarted.IsZero() || !message.progress.StartedAt.Before(model.pxeProgressStarted)) {
			model.pxeProgress = message.progress
		}
		if model.pxePreparing {
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
			model.hosts = message.report
			model.hostCursor = 0
			model.message = ""
		}
		model.screen = dashboardHosts
		return model, nil
	case dashboardInstallationSessionMsg:
		model.busy = ""
		model.installationSession = message.report
		model.applyInstallationSession()
		model.message = message.report.Message
		model.screen = message.screen
		return model, nil
	case dashboardDeploymentPlanMsg:
		model.busy = ""
		model.deployPlan = message.report
		if message.report.HasErrors() {
			model.message = deploymentPlanIssues(message.report)
			model.screen = dashboardDeploy
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardDeployReview
		return model, nil
	case dashboardDeploymentResultMsg:
		model.hosts = domain.HostsReport{}
		model.busy = ""
		model.deploying = false
		model.deployEvents = nil
		model.deployResult = message.report
		model.message = message.report.Message
		model.screen = dashboardDeploy
		return model, nil
	case dashboardDeploymentProgressMsg:
		if !model.deploying || model.deployEvents == nil {
			return model, nil
		}
		model.deployProgress = message.progress
		model.deployRecent = appendBoundedActivity(model.deployRecent, message.progress.Activity, 5)
		return model, waitForDeploymentEvent(model.deployEvents)
	case dashboardControllerPlanMsg:
		model.busy = ""
		model.controllerPlan = message.report
		if message.report.HasErrors() {
			model.message = controllerPlanIssues(message.report)
			model.screen = dashboardController
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardControllerReview
		return model, nil
	case dashboardControllerResultMsg:
		model.hosts = domain.HostsReport{}
		model.busy = ""
		model.controllerApplying = false
		model.controllerResult = message.report
		model.message = message.report.Message
		model.controllerDetails = false
		if message.statusErr != nil {
			model.message += "; dashboard refresh failed: " + message.statusErr.Error()
		} else {
			model.report = message.status
		}
		model.screen = dashboardController
		if model.actions.LoadControllerProgress != nil {
			return model, model.loadControllerProgress(model.controllerProgressID)
		}
		return model, nil
	case dashboardControllerProgressTickMsg:
		if !model.controllerApplying || message.id != model.controllerProgressID || model.actions.LoadControllerProgress == nil {
			return model, nil
		}
		return model, model.loadControllerProgress(message.id)
	case dashboardControllerProgressMsg:
		if message.id != model.controllerProgressID {
			return model, nil
		}
		if message.err == nil && (model.controllerStarted.IsZero() || !message.progress.StartedAt.Before(model.controllerStarted)) {
			model.controllerProgress = message.progress
		}
		if model.controllerApplying {
			return model, scheduleControllerProgressTick(message.id)
		}
		return model, nil
	case dashboardServicesMsg:
		model.busy = ""
		model.services = message.report
		model.message = ""
		model.screen = dashboardServices
		return model, nil
	case dashboardServiceResultMsg:
		model.busy = ""
		model.serviceResult = message.report
		model.message = message.report.Message
		model.screen = dashboardServices
		return model, nil
	case dashboardLogsMsg:
		model.busy = ""
		model.logs = message.report
		if model.logCursor >= len(message.report.Logs) {
			model.logCursor = 0
		}
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardLogs
		return model, nil
	case dashboardLogMsg:
		model.busy = ""
		model.logDetail = message.report
		model.logScroll = maximumLogScroll(message.report, model.logDetailHeight())
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardLogDetail
		return model, nil
	case dashboardGitReviewMsg:
		model.busy = ""
		model.gitReview = message.report
		model.gitScroll = 0
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardGitReview
		return model, nil
	case dashboardGitCommitPlanMsg:
		model.busy = ""
		model.gitCommitPlan = message.report
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
			model.screen = dashboardGitCommitSelect
			return model, nil
		}
		model.confirmation = ""
		model.gitScroll = 0
		model.message = ""
		model.screen = dashboardGitCommitReview
		return model, nil
	case dashboardGitCommitResultMsg:
		model.hosts = domain.HostsReport{}
		model.busy = ""
		model.gitCommitResult = message.report
		model.gitReview = message.review
		model.gitScroll = 0
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardGitReview
		return model, nil
	case dashboardUpdateCheckMsg:
		model.busy = ""
		model.updateCheck = message.report
		model.updateCursor = 0
		model.updateTarget = ""
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
		} else {
			model.message = ""
		}
		model.screen = dashboardUpdate
		return model, nil
	case dashboardUpdatePlanMsg:
		model.busy = ""
		model.updatePlan = message.report
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
			model.screen = dashboardUpdate
			return model, nil
		}
		model.confirmation = ""
		model.updateScroll = 0
		model.message = ""
		model.screen = dashboardUpdateReview
		return model, nil
	case dashboardUpdateResultMsg:
		model.hosts = domain.HostsReport{}
		model.busy = ""
		model.updating = false
		model.updateResult = message.report
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardUpdate
		if !message.report.HasErrors() && message.report.Updated {
			return model.startUpdateControllerApply()
		}
		return model, nil
	case dashboardUpdateControllerMsg:
		model.busy = ""
		model.updating = false
		model.controllerPlan = message.plan
		model.controllerResult = message.report
		if message.plan.HasErrors() {
			model.message = controllerPlanIssues(message.plan)
		} else {
			model.message = message.report.Message
		}
		model.screen = dashboardUpdate
		return model, nil
	default:
		return model.updateConfigurationMessage(message)
	}
}

func (model dashboardModel) updateConfigurationMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardSettingsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Settings could not be loaded: " + message.err.Error()
			if model.settingsReturn == dashboardSetup {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			return model, nil
		}
		model.settings = message.settings
		model.settingsMenu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		if model.startingLabSetup {
			model.startingLabSetup = false
			if model.settings.Lab.DeploymentMode != "controller" {
				model.screen = dashboardSetup
				model.message = "Continuing the existing computer installation setup."
				return model, nil
			}
			model.settings.Lab.DeploymentMode = "laboratory"
			if model.settings.Lab.PCCount == 0 {
				model.settings.Lab.PCCount = 20
			}
			model.settingsEditor = newSettingsEditorModel(model.settings, clientSetupFields, "Nixorium — Install new computers / Laboratory network")
			model.settingsEditor.width = model.width
			model.settingsEditor.height = model.height
			model.settingsEditor.isDark = model.isDark
			model.settingsEditor.prepareCurrentField()
			model.message = ""
			model.screen = dashboardSettingsEdit
			return model, nil
		}
		if model.settingsReturn == dashboardSetup {
			model.settingsEditor = newSettingsEditorModel(model.settings, settingsFields, "Nixorium — First setup / Laboratory settings")
			model.settingsEditor.width = model.width
			model.settingsEditor.height = model.height
			model.settingsEditor.isDark = model.isDark
			model.settingsEditor.prepareCurrentField()
			model.message = ""
			model.screen = dashboardSettingsEdit
			return model, nil
		}
		model.message = ""
		model.screen = dashboardSettings
		return model, nil
	case dashboardSettingsPlanMsg:
		model.busy = ""
		model.settingsEditor = settingsWizardModel{}
		model.settingsPlan = message.report
		if message.report.HasErrors() {
			model.message = "Candidate validation failed: " + settingsIssueMessage(message.report.Issues)
			if model.settingsReturn == dashboardSetup {
				model.settingsEditor = newSettingsEditorModel(model.settingsCandidate, settingsFields, "Nixorium — First setup / Laboratory settings")
				model.settingsEditor.index = len(model.settingsEditor.fields) - 1
				model.settingsEditor.accepted = false
				model.settingsEditor.err = model.message
				model.settingsEditor.prepareCurrentField()
				model.screen = dashboardSettingsEdit
			} else {
				model.screen = dashboardSettings
			}
			return model, nil
		}
		if len(message.report.Changes) == 0 {
			model.message = "No managed settings changed."
			model.screen = dashboardSettings
			return model, nil
		}
		model.message = ""
		model.screen = dashboardSettingsReview
		return model, nil
	case dashboardSettingsPasswordMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Password change failed: " + message.err.Error()
			if model.settingsReturn == dashboardSetup && message.candidate.SchemaVersion != 0 {
				model.settingsCandidate = message.candidate
			}
			model.settingsPasswordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
			model.screen = dashboardSettingsPasswords
			return model, nil
		}
		model.settingsCandidate = message.candidate
		model.settingsCollectPasswords = false
		model.busy = "Validating the password change through Nix"
		model.message = ""
		candidate := model.settingsCandidate
		return model, func() tea.Msg {
			return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(candidate)}
		}
	case dashboardSettingsApplyMsg:
		model.hosts = domain.HostsReport{}
		model.busy = ""
		model.settingsApplying = false
		model.settingsResult = message.report
		if !message.report.HasErrors() && message.report.State == "saved" {
			model.settings = model.settingsCandidate
			model.message = message.report.Message
		} else if message.report.State == "unchanged" {
			model.message = message.report.Message
		} else {
			model.message = message.report.Message
			if model.message == "" {
				model.message = "Configuration save failed: " + settingsIssueMessage(message.report.Issues)
			}
		}
		if model.settingsReturn == dashboardSetup && !message.report.HasErrors() && (message.report.State == "saved" || message.report.State == "unchanged") {
			model.settingsReturn = dashboardHome
			model.settingsCollectPasswords = false
			model.screen = dashboardSetup
			model.busy = "Continuing first setup"
			model.message = ""
			if model.actions.LoadSetup != nil {
				return model, model.loadSetup()
			}
			model.busy = ""
			return model, nil
		}
		model.screen = dashboardSettings
		return model, nil
	case dashboardSoftwareCatalogMsg:
		if model.softwareSearchCancel != nil {
			model.softwareSearchCancel()
			model.softwareSearchCancel = nil
		}
		model.busy = ""
		model.softwareCatalog = message.report
		model.softwareCursor = 0
		model.softwareQuery = ""
		model.softwareSearching = false
		model.softwareSearch = domain.SoftwareSearchReport{}
		model.softwareSearchBusy = false
		model.softwareSearchID++
		model.softwareMode = softwareSuggested
		if len(message.report.Packages) > 0 {
			model.softwareMode = softwareConfigured
		}
		model.message = message.report.Message
		model.screen = dashboardSoftware
		return model, nil
	case dashboardSoftwareSearchStartMsg:
		if model.screen != dashboardSoftware || model.softwareMode != softwareSearch || message.id != model.softwareSearchID || message.query != strings.TrimSpace(model.softwareQuery) || model.actions.SearchSoftware == nil {
			return model, nil
		}
		model.softwareSearchBusy = true
		query := message.query
		id := message.id
		searchContext := message.ctx
		return model, func() tea.Msg {
			return dashboardSoftwareSearchMsg{id: id, report: model.actions.SearchSoftware(searchContext, query)}
		}
	case dashboardSoftwareSearchMsg:
		if model.screen != dashboardSoftware || model.softwareMode != softwareSearch || message.id != model.softwareSearchID {
			return model, nil
		}
		model.softwareSearchBusy = false
		if model.softwareSearchCancel != nil {
			model.softwareSearchCancel()
			model.softwareSearchCancel = nil
		}
		model.softwareSearch = message.report
		model.softwareCursor = 0
		if message.report.HasErrors() {
			model.message = message.report.Message
		} else {
			model.message = ""
		}
		return model, nil
	case dashboardSoftwarePlanMsg:
		model.busy = ""
		model.softwarePlan = message.report
		model.message = message.report.Message
		if message.report.HasErrors() || message.report.State == "unchanged" {
			if message.report.Request.Present {
				model.screen = dashboardSoftwareScope
			} else {
				model.screen = dashboardSoftware
			}
		} else {
			model.confirmation = ""
			model.screen = dashboardSoftwareReview
		}
		return model, nil
	case dashboardSoftwareApplyMsg:
		model.busy = ""
		model.softwareApplying = false
		model.softwareResult = message.report
		model.message = message.report.Message
		model.screen = dashboardSoftwareResult
		if !message.report.HasErrors() && message.report.State == "saved" && message.report.AffectedController != "" {
			return model.startSoftwareControllerApply()
		}
		return model, nil
	case dashboardSoftwareControllerMsg:
		model.busy = ""
		model.softwareApplying = false
		model.controllerPlan = message.plan
		model.controllerResult = message.report
		if message.plan.HasErrors() {
			model.message = controllerPlanIssues(message.plan)
		} else {
			model.message = message.report.Message
		}
		model.screen = dashboardSoftwareResult
		return model, nil
	case dashboardShutdownPlanMsg:
		model.busy = ""
		model.shutdownPlan = message.report
		model.message = message.report.Message
		if len(message.report.Targets) == 0 {
			model.screen = dashboardShutdown
			return model, nil
		}
		model.confirmation = ""
		model.screen = dashboardShutdownReview
		return model, nil
	case dashboardShutdownApplyMsg:
		model.busy = ""
		model.shutdownApplying = false
		model.shutdownResult = message.report
		model.shutdownTechnical = false
		model.message = message.report.Message
		model.screen = dashboardShutdownResult
		return model, nil
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		model.ensureHomeMenu()
		model.homeMenu.setSize(model.width, model.height)
		if model.screen == dashboardLogDetail && model.logScroll > maximumLogScroll(model.logDetail, model.logDetailHeight()) {
			model.logScroll = maximumLogScroll(model.logDetail, model.logDetailHeight())
		}
		if model.screen == dashboardUpdateReview && model.updateScroll > maximumUpdateScroll(model.updatePlan, model.updateReviewHeight()) {
			model.updateScroll = maximumUpdateScroll(model.updatePlan, model.updateReviewHeight())
		}
		if model.screen == dashboardSettings {
			model.settingsMenu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settingsPasswordMenu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsEdit {
			updated, _ := model.settingsEditor.Update(message)
			model.settingsEditor = updated.(settingsWizardModel)
		}
		return model, nil
	case tea.BackgroundColorMsg:
		model.isDark = message.IsDark()
		model.activitySpinner.Style = newTUISpinner(model.isDark).Style
		selected := model.homeMenu.list.Index()
		model.homeMenu = newDashboardTaskMenu(model.isDark, model.width, model.height)
		model.homeMenu.list.Select(selected)
		if model.screen == dashboardSettings {
			model.settingsMenu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settingsPasswordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
		}
		if model.screen == dashboardSettingsEdit {
			updated, _ := model.settingsEditor.Update(message)
			model.settingsEditor = updated.(settingsWizardModel)
		}
		return model, nil
	default:
		return model.updateKeyState(message)
	}
}

func (model dashboardModel) updateKeyState(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		if model.screen == dashboardHome {
			model.ensureHomeMenu()
			model.homeMenu, _ = model.homeMenu.update(message)
		}
		if model.screen == dashboardSettings {
			model.settingsMenu, _ = model.settingsMenu.update(message)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settingsPasswordMenu, _ = model.settingsPasswordMenu.update(message)
		}
		if model.screen == dashboardSettingsEdit {
			updated, command := model.settingsEditor.Update(message)
			model.settingsEditor = updated.(settingsWizardModel)
			return model, command
		}
		return model, nil
	}
	if !model.helpOpen && model.screen == dashboardSettings && key.String() != "f1" && (model.settingsMenu.list.FilterState() == list.Filtering || (model.settingsMenu.list.FilterState() == list.FilterApplied && key.String() == "esc")) {
		var command tea.Cmd
		model.settingsMenu, command = model.settingsMenu.update(key)
		return model, command
	}
	if model.helpOpen {
		if key.String() == "?" || key.String() == "f1" || key.String() == "esc" {
			model.helpOpen = false
			model.pageScroll = 0
		}
		if key.String() == "shift+down" {
			model.pageScroll++
		}
		if key.String() == "shift+up" {
			model.pageScroll = max(0, model.pageScroll-1)
		}
		return model, nil
	}
	if key.String() == "f1" || (key.String() == "?" && !model.textEntry()) {
		model.helpOpen = true
		model.pageScroll = 0
		return model, nil
	}
	if key.String() == "shift+down" {
		model.pageScroll++
		return model, nil
	}
	if key.String() == "shift+up" {
		model.pageScroll = max(0, model.pageScroll-1)
		return model, nil
	}
	if model.screen == dashboardHosts && model.hostSearching {
		switch key.String() {
		case "esc":
			model.hostSearching = false
			model.hostQuery = ""
		case "enter":
			model.hostSearching = false
		case "backspace":
			value := []rune(model.hostQuery)
			if len(value) > 0 {
				model.hostQuery = string(value[:len(value)-1])
			}
		default:
			if key.Text != "" && len(model.hostQuery) < 128 {
				model.hostQuery += key.Text
			}
		}
		model.hostCursor = 0
		return model, nil
	}
	if model.screen == dashboardSoftware && model.softwareSearching {
		changed := false
		switch key.String() {
		case "tab":
			model = model.changeSoftwareMode(1)
			return model, nil
		case "shift+tab":
			model = model.changeSoftwareMode(-1)
			return model, nil
		case "esc":
			model.softwareSearching = false
		case "up", "down", "enter":
			model.softwareSearching = false
			return model.updateSoftware(key)
		case "backspace":
			value := []rune(model.softwareQuery)
			if len(value) > 0 {
				model.softwareQuery = string(value[:len(value)-1])
				changed = true
			}
		default:
			if key.Text != "" && len(model.softwareQuery) < 80 {
				model.softwareQuery += key.Text
				changed = true
			}
		}
		model.softwareCursor = 0
		if changed {
			return model, model.scheduleSoftwareSearch()
		}
		return model, nil
	}
	if key.String() == "l" && (model.deploying || model.controllerApplying || model.pxePreparing) {
		model.progressDetails = !model.progressDetails
		return model, nil
	}
	if (key.String() == "ctrl+c" || key.String() == "q") && (model.deploying || model.updating || model.settingsApplying || model.softwareApplying || model.shutdownApplying) {
		model.message = "A mutating operation is running; wait for its result before closing Nixorium."
		return model, nil
	}
	exitKey := key.String() == "ctrl+c" || (key.String() == "q" && !model.textEntry())
	if exitKey && model.report.PXE.Mode == "active" {
		if model.screen != dashboardPXELeaveReview {
			model.screen = dashboardPXELeaveReview
			model.confirmation = ""
			model.message = ""
		}
		return model, nil
	}
	if exitKey {
		if model.softwareSearchCancel != nil {
			model.softwareSearchCancel()
		}
		return model, tea.Quit
	}
	if model.busy != "" {
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
			model.adminCursor = max(0, model.adminCursor-1)
			return model, nil
		case "down", "j":
			model.adminCursor = min(len(administrationTasks)-1, model.adminCursor+1)
			return model, nil
		case "enter":
			key = tea.KeyPressMsg{Code: []rune(administrationTasks[model.adminCursor].shortcut)[0], Text: administrationTasks[model.adminCursor].shortcut}
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
		model.screen = dashboardHome
	}
	if key.String() == "i" && (model.screen == dashboardHome || model.screen == dashboardHosts) {
		model.diagnosticReturn = model.screen
		model.screen = dashboardDiagnostics
		command := model.startDiagnostics()
		return model, command
	}
	return model.updatePrimaryScreenKey(key)
}
