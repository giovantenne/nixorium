package presentation

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateState(message tea.Msg) (tea.Model, tea.Cmd) {
	model.ensureActivitySpinner()
	switch message := message.(type) {
	case dashboardRemoteFingerprintMsg:
		return model.handleRemoteFingerprintMessage(message)
	case dashboardRemoteInstallMsg:
		return model.handleRemoteInstallMessage(message)
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
			return model.beginComputerInstallation("")
		} else {
			model.screen = dashboardHome
		}
		model.message = ""
		return model, nil
	case dashboardStatusMsg:
		if message.err == nil {
			model.report = message.report
		} else {
			model.computers.hosts = domain.HostsReport{}
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
		if model.installation.flow {
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
		if model.installation.flow {
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
		plan := func() tea.Msg {
			return dashboardPlanMsg{report: model.actions.PlanPXEStart()}
		}
		if model.actions.LoadPXEProgress != nil {
			return model, tea.Batch(model.loadPXEProgress(model.installation.pxeProgressID), plan)
		}
		return model, plan
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
			model.areaReturn = dashboardHome
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
			model.computers.hostCursor = 0
			model.message = ""
		}
		model.screen = dashboardHosts
		return model, nil
	case dashboardConfigurationStateMsg:
		model.busy = ""
		model.computers.configurationState = message.report
		model.computers.hosts = message.report.Clients
		model.computers.hostCursor = 0
		model.message = ""
		model.screen = dashboardHosts
		return model, nil
	case dashboardDeploymentUSBMsg:
		return model.handleDeploymentUSBMessage(message)
	case dashboardDeploymentPlanMsg:
		model.busy = ""
		model.deployment.plan = message.report
		if message.report.HasErrors() {
			model.message = deploymentPlanIssues(message.report)
			model.screen = dashboardDeploy
			return model, nil
		}
		model.deployment.confirmation = ""
		model.message = ""
		model.screen = dashboardDeployReview
		return model, nil
	case dashboardDeploymentResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.deployment.applying = false
		model.deployment.events = nil
		model.deployment.result = message.report
		model.message = message.report.Message
		model.screen = dashboardDeploy
		if message.report.HasErrors() && message.report.Phase == domain.DeploymentPhasePreflight &&
			!message.report.BuildCompleted && !message.report.ApplyCompleted && model.actions.LoadRemoteInstall != nil {
			names := make([]string, 0, len(message.report.Targets))
			for _, target := range message.report.Targets {
				names = append(names, target.Name)
			}
			if len(names) > 0 {
				return model.checkDeploymentUSB(strings.Join(names, ","), true)
			}
		}
		return model, nil
	case dashboardDeploymentProgressMsg:
		if !model.deployment.applying || model.deployment.events == nil {
			return model, nil
		}
		model.deployment.progress = message.progress
		model.deployment.recent = appendBoundedActivity(model.deployment.recent, message.progress.Activity, 5)
		return model, waitForDeploymentEvent(model.deployment.events)
	case dashboardControllerPlanMsg:
		model.busy = ""
		model.controller.plan = message.report
		if model.installation.flow {
			if message.report.HasErrors() {
				return model.failComputerInstallation(controllerPlanIssues(message.report))
			}
			if message.report.Current {
				model.busy = "Checking installation prerequisites"
				return model, model.loadSetup()
			}
			model.busy = "Building and activating the laboratory controller"
			model.controller.applying = true
			model.controller.progress = domain.OperationProgress{}
			model.controller.started = time.Now().UTC()
			model.controller.progressID++
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
			return model, tea.Batch(operation, scheduleControllerProgressTick(model.controller.progressID))
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
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.controller.applying = false
		model.controller.result = message.report
		model.message = message.report.Message
		model.controller.details = false
		if message.statusErr != nil {
			model.message += "; dashboard refresh failed: " + message.statusErr.Error()
		} else {
			model.report = message.status
		}
		if model.installation.flow {
			model.screen = dashboardPXE
			if message.report.HasErrors() || !message.report.Applied || !message.report.Verified {
				return model.failComputerInstallation("Controller activation failed: " + message.report.Message)
			}
			model.busy = "Checking installation prerequisites"
			if model.actions.LoadControllerProgress != nil {
				return model, tea.Batch(model.loadControllerProgress(model.controller.progressID), model.loadSetup())
			}
			return model, model.loadSetup()
		}
		model.screen = dashboardController
		if model.actions.LoadControllerProgress != nil {
			return model, model.loadControllerProgress(model.controller.progressID)
		}
		return model, nil
	case dashboardControllerProgressTickMsg:
		if !model.controller.applying || message.id != model.controller.progressID || model.actions.LoadControllerProgress == nil {
			return model, nil
		}
		return model, model.loadControllerProgress(message.id)
	case dashboardControllerProgressMsg:
		if message.id != model.controller.progressID {
			return model, nil
		}
		if message.err == nil && (model.controller.started.IsZero() || !message.progress.StartedAt.Before(model.controller.started)) {
			model.controller.progress = message.progress
		}
		if model.controller.applying {
			return model, scheduleControllerProgressTick(message.id)
		}
		return model, nil
	case dashboardServicesMsg:
		model.busy = ""
		model.maintenance.services = message.report
		model.message = ""
		model.screen = dashboardServices
		return model, nil
	case dashboardServiceResultMsg:
		model.busy = ""
		model.maintenance.serviceResult = message.report
		model.message = message.report.Message
		model.screen = dashboardServices
		return model, nil
	case dashboardLogsMsg:
		model.busy = ""
		model.maintenance.logs = message.report
		if model.maintenance.logCursor >= len(message.report.Logs) {
			model.maintenance.logCursor = 0
		}
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardLogs
		return model, nil
	case dashboardLogMsg:
		model.busy = ""
		model.maintenance.logDetail = message.report
		model.maintenance.logScroll = maximumLogScroll(message.report, model.logDetailHeight())
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardLogDetail
		return model, nil
	case dashboardGitReviewMsg:
		model.busy = ""
		model.maintenance.gitReview = message.report
		model.maintenance.gitScroll = 0
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardGitReview
		return model, nil
	case dashboardGitCommitPlanMsg:
		model.busy = ""
		model.maintenance.gitCommitPlan = message.report
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
			model.screen = dashboardGitCommitSelect
			return model, nil
		}
		model.confirmation = ""
		model.maintenance.gitScroll = 0
		model.message = ""
		model.screen = dashboardGitCommitReview
		return model, nil
	case dashboardGitCommitResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.maintenance.gitCommitResult = message.report
		model.maintenance.gitReview = message.review
		model.maintenance.gitScroll = 0
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardGitReview
		return model, nil
	case dashboardUpdateCheckMsg:
		model.busy = ""
		model.updates.check = message.report
		model.updates.cursor = 0
		model.updates.target = ""
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
		} else {
			model.message = ""
		}
		model.screen = dashboardUpdate
		return model, nil
	case dashboardPackageBaseMsg:
		model.busy = ""
		model.updates.baseStatus = message.report
		model.updates.baseTarget = message.report.Channel
		model.message = operationLogIssues(message.report.Issues)
		return model, nil
	case dashboardUpdatePlanProgressMsg:
		if !model.updates.planning || model.updates.planEvents == nil {
			return model, nil
		}
		model.updates.planProgress = message.progress
		model.busy = message.progress.Detail
		return model, waitForUpdatePlanEvent(model.updates.planEvents)
	case dashboardUpdatePlanMsg:
		model.updates.planning = false
		model.updates.planEvents = nil
		model.busy = ""
		model.updates.plan = message.report
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
			model.screen = dashboardUpdate
			return model, nil
		}
		model.confirmation = ""
		model.updates.scroll = 0
		model.message = ""
		model.screen = dashboardUpdateReview
		return model, nil
	case dashboardUpdateResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.updates.applying = false
		model.updates.result = message.report
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardUpdate
		if !message.report.HasErrors() && message.report.Updated {
			return model.startUpdateControllerApply()
		}
		return model, nil
	case dashboardUpdateControllerMsg:
		model.busy = ""
		model.updates.applying = false
		model.controller.plan = message.plan
		model.controller.result = message.report
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
			if model.installation.flow {
				model.installation.flow = false
				model.installation.failed = false
			}
			if model.settings.returnScreen == dashboardSetup {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			return model, nil
		}
		model.settings.current = message.settings
		model.settings.menu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		if model.installation.startingLabSetup {
			model.installation.startingLabSetup = false
			model.settings.current.Lab.DeploymentMode = "laboratory"
			if model.settings.current.Lab.PCCount == 0 {
				model.settings.current.Lab.PCCount = 20
			}
			model.settings.collectPasswords = model.settings.current.Lab.AdminPassword == domain.DefaultPasswordHash || model.settings.current.Lab.TeacherPassword == domain.DefaultPasswordHash || model.settings.current.Lab.StudentPassword == domain.DefaultPasswordHash
			model.settings.editor = newSettingsEditorModel(model.settings.current, installationSettingsFields, "Nixorium — Install computers / Laboratory settings")
			model.settings.editor.width = model.width
			model.settings.editor.height = model.height
			model.settings.editor.isDark = model.isDark
			model.settings.editor.prepareCurrentField()
			model.message = ""
			model.screen = dashboardSettingsEdit
			return model, nil
		}
		if model.settings.returnScreen == dashboardSetup {
			model.settings.editor = newSettingsEditorModel(model.settings.current, settingsFields, "Nixorium — First setup / Laboratory settings")
			model.settings.editor.width = model.width
			model.settings.editor.height = model.height
			model.settings.editor.isDark = model.isDark
			model.settings.editor.prepareCurrentField()
			model.message = ""
			model.screen = dashboardSettingsEdit
			return model, nil
		}
		model.message = ""
		model.screen = dashboardSettings
		return model, nil
	case dashboardSettingsPlanMsg:
		model.busy = ""
		model.settings.editor = settingsWizardModel{}
		model.settings.plan = message.report
		if message.report.HasErrors() {
			model.message = "Candidate validation failed: " + settingsIssueMessage(message.report.Issues)
			if model.settings.returnScreen == dashboardSetup || model.installation.flow {
				title := "Nixorium — First setup / Laboratory settings"
				if model.installation.flow {
					title = "Nixorium — Install computers / Laboratory settings"
				}
				fields := settingsFields
				if model.installation.flow {
					fields = installationSettingsFields
				}
				model.settings.editor = newSettingsEditorModel(model.settings.candidate, fields, title)
				model.settings.editor.index = len(model.settings.editor.fields) - 1
				model.settings.editor.accepted = false
				model.settings.editor.err = model.message
				model.settings.editor.prepareCurrentField()
				model.screen = dashboardSettingsEdit
			} else {
				model.screen = dashboardSettings
			}
			return model, nil
		}
		if model.installation.flow {
			if model.actions.SaveSettings == nil {
				return model.failComputerInstallation("Laboratory settings cannot be saved in this session.")
			}
			model.installation.stage = 1
			model.busy = "Saving the validated laboratory settings"
			model.settings.applying = true
			model.message = ""
			candidate := model.settings.candidate
			plan := model.settings.plan
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
			if (model.settings.returnScreen == dashboardSetup || model.installation.flow) && message.candidate.SchemaVersion != 0 {
				model.settings.candidate = message.candidate
			}
			model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
			model.screen = dashboardSettingsPasswords
			return model, nil
		}
		model.settings.candidate = message.candidate
		model.settings.collectPasswords = false
		model.busy = "Validating the password change through Nix"
		model.message = ""
		candidate := model.settings.candidate
		return model, func() tea.Msg {
			return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(candidate)}
		}
	case dashboardSettingsApplyMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.settings.applying = false
		model.settings.result = message.report
		if !message.report.HasErrors() && message.report.State == "saved" {
			model.settings.current = model.settings.candidate
			model.message = message.report.Message
		} else if message.report.State == "unchanged" {
			model.message = message.report.Message
		} else {
			model.message = message.report.Message
			if model.message == "" {
				model.message = "Configuration save failed: " + settingsIssueMessage(message.report.Issues)
			}
		}
		if model.installation.flow {
			if message.report.HasErrors() || (message.report.State != "saved" && message.report.State != "unchanged") {
				return model.failComputerInstallation(message.report.Message)
			}
			model.settings.current = model.settings.candidate
			model.settings.returnScreen = dashboardHome
			model.screen = dashboardPXE
			model.busy = "Checking installation prerequisites"
			model.message = ""
			if model.actions.LoadSetup == nil {
				return model.failComputerInstallation("Installation prerequisite checks are not available in this session.")
			}
			return model, model.loadSetup()
		}
		if model.settings.returnScreen == dashboardSetup && !message.report.HasErrors() && (message.report.State == "saved" || message.report.State == "unchanged") {
			model.settings.returnScreen = dashboardHome
			model.settings.collectPasswords = false
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
	case dashboardSoftwarePresetCatalogMsg:
		model.busy = ""
		software, result := model.software.loadProfiles(message.report)
		model.software = software
		model.message = result.message
		return model, nil
	case dashboardSoftwarePresetPlanMsg:
		model.busy = ""
		software, result := model.software.finishProfilePlan(message.report)
		model.software = software
		model.message = result.message
		return model, nil
	case dashboardSoftwarePresetApplyMsg:
		model.busy = ""
		software, result := model.software.finishProfileApply(message.report)
		model.software = software
		model.message = result.message
		if result.startController {
			return model.startSoftwareControllerApply()
		}
		return model, nil
	case dashboardSoftwareControllerMsg:
		model.busy = ""
		model.controller.plan = message.plan
		model.controller.result = message.report
		software, result := model.software.finishController(message.plan, message.report)
		model.software = software
		model.message = result.message
		return model, nil
	case internetPlanMsg:
		model.busy = ""
		model.internet.plan = message.plan
		model.internet.stage = 1
		model.message = ""
		return model, nil
	case internetApplyMsg:
		model.busy = ""
		model.internet.applying = false
		model.internet.result = message.report
		model.internet.stage = 2
		model.message = ""
		return model, nil
	case dashboardShutdownPlanMsg:
		model.busy = ""
		model.shutdown.plan = message.report
		model.message = message.report.Message
		if len(message.report.Targets) == 0 {
			model.screen = dashboardShutdown
			return model, nil
		}
		model.shutdown.confirmation = ""
		model.screen = dashboardShutdownReview
		return model, nil
	case dashboardShutdownApplyMsg:
		model.busy = ""
		model.shutdown.applying = false
		model.shutdown.result = message.report
		model.shutdown.technical = false
		model.message = message.report.Message
		model.screen = dashboardShutdownResult
		return model, nil
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		model.ensureHomeMenu()
		model.homeMenu.setSize(model.width, model.height)
		if model.screen == dashboardLogDetail && model.maintenance.logScroll > maximumLogScroll(model.maintenance.logDetail, model.logDetailHeight()) {
			model.maintenance.logScroll = maximumLogScroll(model.maintenance.logDetail, model.logDetailHeight())
		}
		if model.screen == dashboardUpdateReview && model.updates.scroll > maximumUpdateScroll(model.updates.plan, model.updateReviewHeight()) {
			model.updates.scroll = maximumUpdateScroll(model.updates.plan, model.updateReviewHeight())
		}
		if model.screen == dashboardSettings {
			model.settings.menu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settings.passwordMenu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsEdit {
			updated, _ := model.settings.editor.Update(message)
			model.settings.editor = updated.(settingsWizardModel)
		}
		return model, nil
	case tea.BackgroundColorMsg:
		model.isDark = message.IsDark()
		model.activitySpinner.Style = newTUISpinner(model.isDark).Style
		selected := model.homeMenu.list.Index()
		model.homeMenu = newDashboardTaskMenu(model.isDark, model.width, model.height)
		model.homeMenu.list.Select(selected)
		if model.screen == dashboardSettings {
			model.settings.menu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
		}
		if model.screen == dashboardSettingsEdit {
			updated, _ := model.settings.editor.Update(message)
			model.settings.editor = updated.(settingsWizardModel)
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
			model.settings.menu, _ = model.settings.menu.update(message)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settings.passwordMenu, _ = model.settings.passwordMenu.update(message)
		}
		if model.screen == dashboardSettingsEdit {
			updated, command := model.settings.editor.Update(message)
			model.settings.editor = updated.(settingsWizardModel)
			return model, command
		}
		return model, nil
	}
	if !model.helpOpen && model.screen == dashboardSettings && key.String() != "f1" && (model.settings.menu.list.FilterState() == list.Filtering || (model.settings.menu.list.FilterState() == list.FilterApplied && key.String() == "esc")) {
		var command tea.Cmd
		model.settings.menu, command = model.settings.menu.update(key)
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
			value := []rune(model.computers.hostQuery)
			if len(value) > 0 {
				model.computers.hostQuery = string(value[:len(value)-1])
			}
		default:
			if key.Text != "" && len(model.computers.hostQuery) < 128 {
				model.computers.hostQuery += key.Text
			}
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
	if key.String() == "l" && (model.deployment.applying || model.controller.applying || model.installation.pxePreparing) {
		model.progressDetails = !model.progressDetails
		return model, nil
	}
	if (key.String() == "ctrl+c" || key.String() == "q") && (model.deployment.applying || model.updates.applying || model.settings.applying || model.software.mutating() || model.shutdown.applying || model.internet.applying) {
		model.message = "A mutating operation is running; wait for its result before closing Nixorium."
		return model, nil
	}
	exitKey := key.String() == "ctrl+c" || (key.String() == "q" && !model.textEntry())
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
