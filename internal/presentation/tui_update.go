package presentation

import (
	"time"

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
			return model.startComputerInstallation()
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
		if model.installationFlow {
			return model.continueComputerInstallation(message.report)
		}
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
		if model.installationFlow {
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
			return model, model.loadSetup()
		}
		if model.setupKeysReturn == dashboardSettings {
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
			model.setupKeys = message.keys
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
			return model, model.loadSetup()
		}
		return model, nil
	case dashboardSetupSaveMsg:
		model.busy = ""
		if model.installationFlow {
			if message.report.HasErrors() || (message.report.State != "saved" && message.report.State != "unchanged") {
				return model.failComputerInstallation(message.report.Message)
			}
			model.busy = "Checking saved laboratory configuration"
			return model, model.loadSetup()
		}
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
			if model.installationFlow {
				return model.failComputerInstallation(message.report.Message)
			}
			model.message = message.report.Message
			model.screen = dashboardPXE
			return model, nil
		}
		if message.report.Mode == "active" {
			model.message = "PXE installation mode is already active."
			model.screen = dashboardPXE
			model.installationFlow = false
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardPXEStartReview
		return model, nil
	case dashboardInstallationPrepareMsg:
		model.busy = ""
		model.pxePreparing = false
		if message.statusErr == nil {
			model.report = message.status
		}
		if message.report.HasErrors() {
			return model.failComputerInstallation("Client preparation failed: " + message.report.Message)
		}
		model.message = message.report.Message
		model.installationStage = 5
		if model.actions.PlanPXEStart == nil {
			return model.failComputerInstallation("PXE start validation is not available in this session.")
		}
		model.busy = "Checking PXE readiness"
		plan := func() tea.Msg {
			return dashboardPlanMsg{report: model.actions.PlanPXEStart()}
		}
		if model.actions.LoadPXEProgress != nil {
			return model, tea.Batch(model.loadPXEProgress(model.pxeProgressID), plan)
		}
		return model, plan
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
		if model.installationFlow && message.screen == dashboardPXE && model.report.PXE.Mode == "active" {
			model.installationFlow = false
			model.installationFailed = false
			model.setupMode = false
			model.areaReturn = dashboardHome
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
	case dashboardConfigurationStateMsg:
		model.busy = ""
		model.configurationState = message.report
		model.hosts = message.report.Clients
		model.hostCursor = 0
		model.message = ""
		model.screen = dashboardHosts
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
		if model.installationFlow {
			if message.report.HasErrors() {
				return model.failComputerInstallation(controllerPlanIssues(message.report))
			}
			if message.report.Current {
				model.busy = "Checking installation prerequisites"
				return model, model.loadSetup()
			}
			model.busy = "Building and activating the laboratory controller"
			model.controllerApplying = true
			model.controllerProgress = domain.OperationProgress{}
			model.controllerStarted = time.Now().UTC()
			model.controllerProgressID++
			plan := message.report
			operation := func() tea.Msg {
				report := model.actions.ApplyController(plan)
				status := domain.StatusReport{}
				var err error
				if model.actions.Refresh != nil {
					status, err = model.actions.Refresh()
				}
				return dashboardControllerResultMsg{report: report, status: status, statusErr: err}
			}
			return model, tea.Batch(operation, scheduleControllerProgressTick(model.controllerProgressID))
		}
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
		if model.installationFlow {
			model.screen = dashboardPXE
			if message.report.HasErrors() || !message.report.Applied || !message.report.Verified {
				return model.failComputerInstallation("Controller activation failed: " + message.report.Message)
			}
			model.busy = "Checking installation prerequisites"
			if model.actions.LoadControllerProgress != nil {
				return model, tea.Batch(model.loadControllerProgress(model.controllerProgressID), model.loadSetup())
			}
			return model, model.loadSetup()
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
	case dashboardPackageBaseMsg:
		model.busy = ""
		model.baseStatus = message.report
		model.baseTarget = message.report.Channel
		model.message = operationLogIssues(message.report.Issues)
		return model, nil
	case dashboardUpdatePlanProgressMsg:
		if !model.updatePlanning || model.updatePlanEvents == nil {
			return model, nil
		}
		model.updatePlanProgress = message.progress
		model.busy = message.progress.Detail
		return model, waitForUpdatePlanEvent(model.updatePlanEvents)
	case dashboardUpdatePlanMsg:
		model.updatePlanning = false
		model.updatePlanEvents = nil
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
			if model.installationFlow {
				model.installationFlow = false
				model.installationFailed = false
			}
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
			model.settings.Lab.DeploymentMode = "laboratory"
			if model.settings.Lab.PCCount == 0 {
				model.settings.Lab.PCCount = 20
			}
			model.settingsCollectPasswords = model.settings.Lab.AdminPassword == domain.DefaultPasswordHash || model.settings.Lab.TeacherPassword == domain.DefaultPasswordHash || model.settings.Lab.StudentPassword == domain.DefaultPasswordHash
			model.settingsEditor = newSettingsEditorModel(model.settings, installationSettingsFields, "Nixorium — Install computers / Laboratory settings")
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
			if model.settingsReturn == dashboardSetup || model.installationFlow {
				title := "Nixorium — First setup / Laboratory settings"
				if model.installationFlow {
					title = "Nixorium — Install computers / Laboratory settings"
				}
				fields := settingsFields
				if model.installationFlow {
					fields = installationSettingsFields
				}
				model.settingsEditor = newSettingsEditorModel(model.settingsCandidate, fields, title)
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
		if model.installationFlow {
			if model.actions.SaveSettings == nil {
				return model.failComputerInstallation("Laboratory settings cannot be saved in this session.")
			}
			model.installationStage = 1
			model.busy = "Saving the validated laboratory settings"
			model.settingsApplying = true
			model.message = ""
			candidate := model.settingsCandidate
			plan := model.settingsPlan
			return model, func() tea.Msg {
				return dashboardSettingsApplyMsg{report: model.actions.SaveSettings(candidate, plan)}
			}
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
			if (model.settingsReturn == dashboardSetup || model.installationFlow) && message.candidate.SchemaVersion != 0 {
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
		if model.installationFlow {
			if message.report.HasErrors() || (message.report.State != "saved" && message.report.State != "unchanged") {
				return model.failComputerInstallation(message.report.Message)
			}
			model.settings = model.settingsCandidate
			model.settingsReturn = dashboardHome
			model.screen = dashboardPXE
			model.busy = "Checking installation prerequisites"
			model.message = ""
			if model.actions.LoadSetup == nil {
				return model.failComputerInstallation("Installation prerequisite checks are not available in this session.")
			}
			return model, model.loadSetup()
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
		model.busy = ""
		software, result := model.software.loadCatalog(message.report)
		model.software = software
		model.message = result.message
		model.screen = dashboardSoftware
		return model, nil
	case dashboardSoftwareSearchStartMsg:
		if model.screen != dashboardSoftware {
			return model, nil
		}
		software, result, command := model.software.startSearch(message, model.actions.SearchSoftware)
		if !result.accepted {
			return model, nil
		}
		model.software = software
		return model, command
	case dashboardSoftwareSearchMsg:
		if model.screen != dashboardSoftware {
			return model, nil
		}
		software, result := model.software.finishSearch(message)
		if !result.accepted {
			return model, nil
		}
		model.software = software
		model.message = result.message
		return model, nil
	case dashboardSoftwarePlanMsg:
		model.busy = ""
		software, result := model.software.finishPlan(message.report)
		model.software = software
		model.message = result.message
		if model.software.reviewing() {
			model.confirmation = ""
		}
		return model, nil
	case dashboardSoftwareApplyMsg:
		model.busy = ""
		software, result := model.software.finishApply(message.report)
		model.software = software
		model.message = result.message
		if result.startController {
			return model.startSoftwareControllerApply()
		}
		return model, nil
	case dashboardSoftwareControllerMsg:
		model.busy = ""
		model.controllerPlan = message.plan
		model.controllerResult = message.report
		software, result := model.software.finishController(message.plan, message.report)
		model.software = software
		model.message = result.message
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
	if model.baseUpdate && model.baseEditing && model.screen == dashboardUpdate && model.busy == "" && key.String() != "ctrl+c" {
		return model.updatePackageBaseKey(key)
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
	if key.String() == "l" && (model.deploying || model.controllerApplying || model.pxePreparing) {
		model.progressDetails = !model.progressDetails
		return model, nil
	}
	if (key.String() == "ctrl+c" || key.String() == "q") && (model.deploying || model.updating || model.settingsApplying || model.software.mutating() || model.shutdownApplying) {
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
		model.software.cancelSearch()
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
		if key.String() == "c" {
			return model.openControllerReview()
		}
		if key.String() == "i" {
			model.diagnosticReturn = dashboardAdministration
			model.screen = dashboardDiagnostics
			return model, model.startDiagnostics()
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
	if key.String() == "i" && model.screen == dashboardHosts {
		model.diagnosticReturn = model.screen
		model.screen = dashboardDiagnostics
		command := model.startDiagnostics()
		return model, command
	}
	return model.updatePrimaryScreenKey(key)
}
