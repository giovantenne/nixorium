package presentation

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) openControllerReview() (tea.Model, tea.Cmd) {
	model.controller.fromSave, model.controller.autoApply = false, false
	if reason := model.managedJobConflict(); reason != "" {
		model.message = reason
		return model, nil
	}
	model.screen = dashboardController
	model.busy = "Reviewing controller revision and active system"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardControllerPlanMsg{report: model.actions.PlanController(ctx)}
	})
}

func (model dashboardModel) openComputerTask(action string) (tea.Model, tea.Cmd) {
	if action == "v" {
		return model.openClassroomView()
	}
	if model.report.Meta.Controller.Name == "" {
		return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) {
			return ready.openComputerTask(action)
		})
	}
	switch action {
	case "u":
		if model.actions.LoadDeferredUpdates == nil {
			return model, nil
		}
		return model.openDeferredUpdates()
	case "i":
		model.screen = dashboardInternet
		model.internet = internetModel{hostPicker: newHostPicker(), action: domain.InternetBlock}
		model.message = ""
	case "l":
		if model.actions.PlanLock == nil {
			return model, nil
		}
		model.screen = dashboardLock
		model.lock = lockModel{hostPicker: newHostPicker(), action: domain.LockOn}
		model.message = ""
	case "s":
		if model.actions.PlanShare == nil {
			return model, nil
		}
		model.screen = dashboardShare
		model.share = newShareModel()
		model.message = ""
	case "x":
		model.screen = dashboardShutdown
		model.shutdown = newShutdownModel()
		model.message = ""
	case "d":
		model.screen = dashboardDeploy
		model.deployment.result = domain.DeploymentExecutionReport{}
		model.deployment.context = ""
		model.message = ""
		model.deployment.chosen = map[string]bool{}
		model.deployment.cursor = 0
		return model, model.loadWaitingSelection()
	case "h":
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

func (model dashboardModel) openMaintenanceTask(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "a":
		return model.openTelemetry()
	case "f":
		return model.openCleanup()
	case "y":
		return model.openBackup()
	case "o":
		return model.openRestoreLab()
	case "t":
		return model.openTemplateReset()
	case "n":
		return model.openNetworkChange()
	case "s":
		model.screen = dashboardServices
		model.busy = "Checking managed controller services"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardServicesMsg{report: model.actions.LoadServices(ctx)}
		})
	case "l":
		model.screen = dashboardLogs
		model.busy = "Loading private operation logs"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardLogsMsg{report: model.actions.LoadLogs(ctx)}
		})
	case "g":
		model.maintenance.gitFromWorkspace = model.screen == dashboardWorkspace
		model.maintenance.gitFromHomeFiles = model.screen == dashboardHomeFiles
		model.maintenance.gitCommitResult = domain.GitCommitReport{}
		model.maintenance.gitUndoResult = domain.GitUndoReport{}
		model.screen = dashboardGitReview
		model.busy = "Reviewing Git changes without modifying the worktree"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardGitReviewMsg{report: model.actions.LoadGitReview(ctx)}
		})
	case "u":
		model.updates.packageBase = false
		model.screen = dashboardUpdate
		model.updates.result = domain.UpdateApplyReport{}
		model.controller.plan = domain.ControllerRebuildPlanReport{}
		model.controller.result = domain.ControllerRebuildExecutionReport{}
		model.updates.plan = domain.UpdatePlanReport{}
		model.updates.prerelease = false
		model.updates.check = domain.UpdateCheckReport{}
		model.updates.cursor = 0
		model.updates.target = ""
		model.message = ""
		if model.actions.CheckUpdate == nil {
			model.message = "Update discovery is not available in this session."
			return model, nil
		}
		model.busy = "Fetching available Nixorium updates"
		return model.checkUpdates()
	case "b":
		return model.openPackageBase()
	case "e":
		model.settings.fromAccounts = false
		model.screen = dashboardSettings
		model.settings.returnScreen = dashboardAdministration
		model.settings.result = domain.ConfigurationSaveReport{}
		model.settings.plan = domain.ConfigPlanReport{}
		model.settings.candidate = domain.LabSettingsFile{}
		model.busy = "Loading managed laboratory settings"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return model.loadSettingsMsg(ctx)
		})
	}
	return model, nil
}

func (model dashboardModel) updatePrimaryScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.screen == dashboardSettings && model.installation.savedSummary {
		return model.updateInstallationSummary(key)
	}
	if model.screen == dashboardSettings && model.settings.result.Operation == "" {
		if model.settings.menu.filtering() {
			var command tea.Cmd
			model.settings.menu, command = model.settings.menu.update(key)
			return model, command
		}
		for index, group := range routineSettingsGroups {
			if key.String() == group.shortcut {
				model.settings.menu.list.ResetFilter()
				model.settings.menu.list.Select(index)
				key = tea.KeyPressMsg{Code: tea.KeyEnter}
				break
			}
		}
	}
	if model.screen == dashboardSettingsAccounts && model.settings.accountsMenu.initialized {
		for index, choice := range routineAccountsChoices {
			if key.String() == choice.shortcut {
				model.settings.accountsMenu.list.Select(index)
				key = tea.KeyPressMsg{Code: tea.KeyEnter}
				break
			}
		}
	}
	if model.screen == dashboardSettingsPasswords && model.settings.returnScreen != dashboardSetup && !model.installation.flow && model.settings.passwordMenu.initialized {
		for index, choice := range routinePasswordChoices {
			if key.String() == choice.id[:1] {
				model.settings.passwordMenu.list.Select(index)
				key = tea.KeyPressMsg{Code: tea.KeyEnter}
				break
			}
		}
	}
	switch model.screen {
	case dashboardHome:
		return model.updateHomeKey(key)
	case dashboardAssistant:
		return model.updateAssistantScreenKey(key)
	case dashboardComputersArea:
		return model.updateComputersAreaKey(key)
	case dashboardInstallationArea:
		return model.updateInstallationAreaKey(key)
	case dashboardUSBInstall:
		return model.updateRemoteInstallKey(key)
	case dashboardSetup:
		return model.updateSetupKey(key)
	case dashboardSetupKeys:
		return model.updateSetupKeysKey(key)
	case dashboardSettings:
		return model.updateSettingsKey(key)
	case dashboardSettingsEdit:
		return model.updateSettingsEditKey(key)
	case dashboardSettingsPasswords:
		return model.updateSettingsPasswordsKey(key)
	case dashboardSettingsAccounts:
		return model.updateSettingsAccountsKey(key)
	case dashboardSettingsReview:
		return model.updateSettingsReviewKey(key)
	case dashboardHosts:
		return model.updateHostsKey(key)
	default:
		return model.updateOperationScreenKey(key)
	}
}

func (model dashboardModel) updateOperationScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardInternet:
		return model.updateInternet(key)
	case dashboardLock:
		return model.updateLock(key)
	case dashboardShare:
		return model.updateShare(key)
	case dashboardDeferredUpdates:
		return model.updateDeferredUpdates(key)
	case dashboardNetworkChange:
		return model.updateNetworkChange(key)
	case dashboardCleanup:
		return model.updateCleanup(key)
	case dashboardRecovery:
		return model.updateRecovery(key)
	case dashboardRecoveryReview:
		return model.updateRecoveryReview(key)
	case dashboardBackup:
		return model.updateBackup(key)
	case dashboardSoftware:
		return model.updateSoftware(key)
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		return model.updateShutdown(key)
	case dashboardDeploy, dashboardDeployReview:
		return model.updateDeployment(key)
	case dashboardController:
		return model.updateControllerKey(key)
	case dashboardControllerReview:
		return model.updateControllerReviewKey(key)
	case dashboardServices:
		return model.updateServicesKey(key)
	case dashboardServicesRestartReview:
		return model.updateServicesRestartReviewKey(key)
	case dashboardLogs:
		return model.updateLogsKey(key)
	case dashboardLogDetail:
		return model.updateLogDetailKey(key)
	default:
		return model.updateRepositoryScreenKey(key)
	}
}

func (model dashboardModel) updateRepositoryScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardGitReview:
		return model.updateGitReviewKey(key)
	case dashboardGitCommitSelect:
		return model.updateGitCommitSelectKey(key)
	case dashboardGitCommitReview:
		return model.updateGitCommitReviewKey(key)
	case dashboardGitUndoReview:
		return model.updateGitUndoReviewKey(key)
	case dashboardUpdate:
		return model.updateUpdateKey(key)
	case dashboardUpdateReview:
		return model.updateUpdateReviewKey(key)
	default:
		return model.updatePXEScreenKey(key)
	}
}

func (model dashboardModel) updatePXEScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardPXE:
		return model.updatePXEKey(key)
	case dashboardDiagnostics:
		return model.updateDiagnosticsKey(key)
	case dashboardPXEStartReview:
		return model.updatePXEStartReviewKey(key)
	case dashboardPXELeaveReview:
		return model.updatePXELeaveReviewKey(key)
	}
	return model, nil
}

type dashboardClassroomViewMsg struct {
	text string
	err  error
}

// openClassroomView asks the classroom service for the page, which the
// command layer opens in the browser; the dashboard only shows the outcome.
func (model dashboardModel) openClassroomView() (tea.Model, tea.Cmd) {
	if model.actions.OpenClassroomView == nil {
		return model, nil
	}
	model.busy = "Opening the classroom view"
	model.message = ""
	open := model.actions.OpenClassroomView
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		text, err := open(ctx)
		return dashboardClassroomViewMsg{text: text, err: err}
	}
}

// startReviewedControllerApply applies the controller plan already held in the
// model, exactly as Enter on the controller review does.
func (model dashboardModel) startReviewedControllerApply() (tea.Model, tea.Cmd) {
	model.busy = "Building and activating the reviewed controller revision"
	model.controller.applying = true
	model.controller.progress = domain.OperationProgress{}
	model.controller.started = time.Now().UTC()
	model.controller.progressID++
	model.message = ""
	plan := model.controller.plan
	refresh, id := model.actions.Refresh, model.controller.progressID
	operation := func() tea.Msg {
		report := model.actions.ApplyController(plan)
		recovery := model.recoveryAfterControllerApply()
		result := dashboardControllerResultMsg{report: report, recovery: recovery, refreshLater: refresh != nil}
		if refresh == nil {
			return result
		}
		// The result is shown at once; the full status, which evaluates the
		// configuration again, follows in the background.
		return tea.BatchMsg{
			func() tea.Msg { return result },
			func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
				defer cancel()
				status, err := refresh(ctx)
				return controllerStatusRefreshMsg{status: status, err: err, id: id}
			},
		}
	}
	return model, tea.Batch(operation, scheduleControllerProgressTick(model.controller.progressID))
}
