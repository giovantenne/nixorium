package presentation

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) openComputerInstallation() (tea.Model, tea.Cmd) {
	return model.startComputerInstallation()
}

func (model dashboardModel) openControllerReview() (tea.Model, tea.Cmd) {
	model.screen = dashboardController
	model.busy = "Reviewing controller revision and active system"
	model.message = ""
	return model, func() tea.Msg {
		return dashboardControllerPlanMsg{report: model.actions.PlanController()}
	}
}

func (model dashboardModel) openComputerTask(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "r":
		model.screen = dashboardRestore
		model.computers.restoreCursor = 0
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
	case "h":
		model.computers.hostDetail = false
		model.computers.hostTechnical = false
		model.computers.configurationState = domain.ConfigurationStateReport{}
		model.screen = dashboardHosts
		model.busy = "Checking configured computers"
		model.message = ""
		return model, model.loadHosts()
	}
	return model, nil
}

func (model dashboardModel) openMaintenanceTask(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "s":
		model.screen = dashboardServices
		model.busy = "Checking managed controller services"
		model.message = ""
		return model, func() tea.Msg {
			return dashboardServicesMsg{report: model.actions.LoadServices()}
		}
	case "l":
		model.screen = dashboardLogs
		model.busy = "Loading private operation logs"
		model.message = ""
		return model, func() tea.Msg {
			return dashboardLogsMsg{report: model.actions.LoadLogs()}
		}
	case "g":
		model.screen = dashboardGitReview
		model.busy = "Reviewing Git changes without modifying the worktree"
		model.message = ""
		return model, func() tea.Msg {
			return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
		}
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
		return model, model.checkUpdates()
	case "b":
		return model.openPackageBase()
	case "e":
		model.screen = dashboardSettings
		model.settings.returnScreen = dashboardAdministration
		model.settings.result = domain.ConfigurationSaveReport{}
		model.settings.plan = domain.ConfigPlanReport{}
		model.settings.candidate = domain.LabSettingsFile{}
		model.busy = "Loading managed laboratory settings"
		model.message = ""
		return model, func() tea.Msg {
			settings, err := model.actions.LoadSettings()
			return dashboardSettingsMsg{settings: settings, err: err}
		}
	}
	return model, nil
}

func (model dashboardModel) updatePrimaryScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardHome:
		if model.initialError {
			switch key.String() {
			case "enter", "r":
				if model.actions.LoadInitial == nil {
					return model, nil
				}
				model.initialError = false
				model.initializing = true
				model.busy = "Opening the laboratory and checking setup progress"
				model.message = ""
				return model, model.loadInitial()
			}
			return model, nil
		}
		// Restoration context never leaks into a later intervention after a
		// completed result or a detour through logs.
		model.computers.restoreMode = false
		model.ensureHomeMenu()
		action := key.String()
		if action == "enter" {
			if selected, ok := model.homeMenu.selected(); ok {
				action = selected.shortcut
			}
		}
		switch action {
		case "a":
			model.screen = dashboardAdministration
		case "c":
			model.screen = dashboardComputersArea
			model.message = ""
		case "w":
			model.screen = dashboardSoftware
			model.software = model.software.open()
			model.controller.plan = domain.ControllerRebuildPlanReport{}
			model.controller.result = domain.ControllerRebuildExecutionReport{}
			model.busy = "Loading supported software from pinned inputs"
			model.message = ""
			if model.actions.LoadSoftware == nil {
				model.busy = ""
				model.message = "The supported software service is not available in this deployment."
				return model, nil
			}
			return model, func() tea.Msg { return dashboardSoftwareCatalogMsg{report: model.actions.LoadSoftware()} }
		case "n":
			model.screen = dashboardInstallationArea
			model.message = ""
		default:
			var command tea.Cmd
			model.homeMenu, command = model.homeMenu.update(key)
			return model, command
		}
	case dashboardComputersArea:
		action := key.String()
		if action == "enter" {
			action = computersAreaTasks[model.computers.areaCursor].shortcut
		}
		switch action {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "up", "k":
			model.computers.areaCursor = max(0, model.computers.areaCursor-1)
		case "down", "j":
			model.computers.areaCursor = min(len(computersAreaTasks)-1, model.computers.areaCursor+1)
		case "h", "d", "r", "x":
			model.areaReturn = dashboardComputersArea
			return model.openComputerTask(action)
		}
	case dashboardInstallationArea:
		action := key.String()
		if action == "enter" {
			action = installationAreaTasks[model.installationAreaCursor].shortcut
		}
		switch action {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "up", "k":
			model.installationAreaCursor = max(0, model.installationAreaCursor-1)
		case "down", "j":
			model.installationAreaCursor = min(len(installationAreaTasks)-1, model.installationAreaCursor+1)
		case "n":
			return model.openComputerInstallation()
		case "p":
			model.areaReturn = dashboardInstallationArea
			model.screen = dashboardPXE
			model.message = ""
		}
	case dashboardRestore:
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "up", "k":
			model.computers.restoreCursor = max(0, model.computers.restoreCursor-1)
		case "down", "j":
			model.computers.restoreCursor = min(1, model.computers.restoreCursor+1)
		case "enter":
			model.message = ""
			if model.computers.restoreCursor == 0 {
				model.computers.restoreMode = true
				model.screen = dashboardDeploy
				model.deployment.result = domain.DeploymentExecutionReport{}
				model.deployment.context = ""
				model.deployment.chosen = map[string]bool{}
				model.deployment.cursor = 0
			} else {
				model.computers.restoreMode = true
				model.screen = dashboardPXE
			}
		}
	case dashboardSetup:
		if key.String() == "t" {
			model.setupDetails = !model.setupDetails
			return model, nil
		}
		if key.String() == "esc" || key.String() == "left" {
			model.setupMode = false
			model.screen = dashboardHome
			model.message = "Setup paused; completed work is saved and you can resume at any time."
			return model, nil
		}
		if key.String() != "enter" {
			return model, nil
		}
		switch model.setup.CurrentStage {
		case domain.SetupStageInspectEnvironment:
			model.diagnosticReturn = dashboardSetup
			model.screen = dashboardDiagnostics
			return model, model.startDiagnostics()
		case domain.SetupStageNetwork:
			return model.openSetupSettings()
		case domain.SetupStageIdentity:
			return model.openSetupSettings()
		case domain.SetupStageCredentials:
			return model.openSetupSettings()
		case domain.SetupStageKeys:
			if model.actions.LoadSetupKeys == nil {
				model.message = "Key preparation is not available in this session."
				return model, nil
			}
			model.screen = dashboardSetupKeys
			model.setupKeysReturn = dashboardSetup
			model.busy = "Checking existing controller keys"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardSetupKeyStatusMsg{report: model.actions.LoadSetupKeys()}
			}
		case domain.SetupStageValidate:
			return model.openSetupSettings()
		case domain.SetupStageReview:
			if model.actions.SaveSetupConfiguration == nil {
				model.message = "Local configuration saving is not available in this session."
				return model, nil
			}
			model.busy = "Saving the generated configuration locally"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardSetupSaveMsg{report: model.actions.SaveSetupConfiguration()}
			}
		case domain.SetupStageApply:
			model.screen = dashboardController
			model.busy = "Reviewing controller revision and active system"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardControllerPlanMsg{report: model.actions.PlanController()}
			}
		case domain.SetupStageArtifacts:
			model.screen = dashboardPXE
			model.busy = "Preparing netboot artifacts and client closures"
			model.installation.pxePreparing = true
			model.installation.pxeProgress = domain.OperationProgress{}
			model.installation.pxeStarted = time.Now().UTC()
			model.installation.pxeProgressID++
			model.message = ""
			operation := model.runAction(func() string {
				report := model.actions.PreparePXE()
				return report.Message
			}, dashboardPXE)
			return model, tea.Batch(operation, schedulePXEProgressTick(model.installation.pxeProgressID))
		case "":
			model.screen = dashboardPXE
			model.message = ""
			return model, nil
		default:
			model.message = "This setup step is not available in the current session. Refresh setup and try again."
			return model, nil
		}
	case dashboardSetupKeys:
		if model.setupKeyImporting {
			switch key.String() {
			case "esc":
				model.setupKeyImporting = false
				model.setupKeyPath = ""
				model.message = "Import cancelled; no key was changed."
			case "backspace":
				value := []rune(model.setupKeyPath)
				if len(value) > 0 {
					model.setupKeyPath = string(value[:len(value)-1])
				}
			case "enter":
				if strings.TrimSpace(model.setupKeyPath) == "" {
					model.message = "Enter the path to an existing private key."
					return model, nil
				}
				if model.actions.ImportSetupKey == nil || model.actions.LoadSetupKeys == nil {
					model.message = "Key import is not available in this session."
					return model, nil
				}
				name := model.selectedSetupKeyName()
				path := strings.TrimSpace(model.setupKeyPath)
				model.busy = "Validating and importing the existing " + setupKeyShortLabel(name) + " key"
				model.message = ""
				return model, func() tea.Msg {
					report, err := model.actions.ImportSetupKey(name, path)
					return dashboardSetupKeyImportMsg{report: report, err: err, keys: model.actions.LoadSetupKeys()}
				}
			default:
				if key.Text != "" && len(model.setupKeyPath) < 4096 {
					for _, character := range key.Text {
						if unicode.IsPrint(character) && !unicode.In(character, unicode.Cf) {
							model.setupKeyPath += string(character)
						}
					}
				}
			}
			return model, nil
		}
		switch key.String() {
		case "esc", "left":
			model.screen = model.setupKeysReturn
			if model.screen == dashboardHome {
				model.screen = dashboardSetup
			}
			model.message = ""
		case "up", "k":
			model.setupKeyCursor = max(0, model.setupKeyCursor-1)
		case "down", "j":
			model.setupKeyCursor = min(max(0, len(model.setupKeys.Keys)-1), model.setupKeyCursor+1)
		case "i":
			state, found := model.selectedSetupKey()
			if !found || state.Ready() {
				model.message = "This key is already ready; existing keys are never replaced here."
				return model, nil
			}
			model.setupKeyImporting = true
			model.setupKeyPath = ""
			model.message = ""
		case "c":
			if !model.setupKeyActionsAvailable() {
				model.message = "Key preparation is not available in this session."
				return model, nil
			}
			model.busy = "Creating and verifying missing controller keys"
			model.message = ""
			return model, model.prepareSetupKeys()
		case "enter":
			if model.setupKeys.State != "ready" {
				model.message = "Import an existing key or create the missing keys before continuing."
				return model, nil
			}
			if !model.setupKeyActionsAvailable() {
				model.message = "Key preparation is not available in this session."
				return model, nil
			}
			model.busy = "Saving and installing verified controller keys"
			model.message = ""
			return model, model.prepareSetupKeys()
		}
	case dashboardSettings:
		if model.settings.result.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				return model.returnFromSettings()
			case "r":
				if !model.settings.result.RecoveryRequired || model.actions.SaveSettings == nil {
					return model, nil
				}
				model.busy = "Recovering the local configuration save"
				model.settings.applying = true
				model.message = ""
				candidate := model.settings.candidate
				plan := model.settings.plan
				return model, func() tea.Msg {
					report := model.actions.SaveSettings(candidate, plan)
					return dashboardSettingsApplyMsg{report: report}
				}
			case "e":
				model.settings.result = domain.ConfigurationSaveReport{}
				model.message = ""
			}
			return model, nil
		}
		switch key.String() {
		case "esc", "left":
			return model.returnFromSettings()
		case "enter":
			group, selected := model.settings.menu.selected()
			if !selected {
				model.message = "Select a settings category."
				return model, nil
			}
			model.settings.editor = newSettingsEditorModel(model.settings.current, group.fields, "Nixorium — Edit "+group.label)
			model.settings.editor.width = model.width
			model.settings.editor.height = model.height
			model.settings.editor.isDark = model.isDark
			model.settings.editor.prepareCurrentField()
			model.message = ""
			model.screen = dashboardSettingsEdit
		case "p":
			model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
			model.message = ""
			model.screen = dashboardSettingsPasswords
		case "k":
			if model.actions.LoadSetupKeys == nil {
				model.message = "Controller key management is not available in this session."
				return model, nil
			}
			model.setupKeysReturn = dashboardSettings
			model.screen = dashboardSetupKeys
			model.busy = "Checking existing controller keys"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardSetupKeyStatusMsg{report: model.actions.LoadSetupKeys()}
			}
		default:
			model.settings.menu, _ = model.settings.menu.update(key)
		}
	case dashboardSettingsEdit:
		updated, command := model.settings.editor.Update(key)
		model.settings.editor = updated.(settingsWizardModel)
		if model.settings.editor.cancelled {
			model.settings.editor = settingsWizardModel{}
			if model.installation.flow {
				model.installation.flow = false
				model.installation.failed = false
				model.settings.returnScreen = dashboardHome
				model.screen = dashboardHome
				model.message = "Computer installation cancelled; no setting was changed."
			} else {
				model.message = "Settings edit cancelled; no file changed."
				model.screen = dashboardSettings
			}
			return model, nil
		}
		if model.settings.editor.accepted {
			model.settings.candidate = model.settings.editor.settings
			if (model.settings.returnScreen == dashboardSetup || model.installation.flow) && model.settings.collectPasswords {
				if model.actions.ChangePassword == nil {
					model.message = "Password setup is not available in this deployment."
					return model, nil
				}
				model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
				model.screen = dashboardSettingsPasswords
				candidate := model.settings.candidate
				command := &settingsPasswordCommand{action: model.actions.ChangePassword, account: "all", settings: candidate}
				return model, tea.Exec(command, func(err error) tea.Msg {
					return dashboardSettingsPasswordMsg{candidate: command.candidate, err: err}
				})
			}
			model.busy = "Validating the complete settings candidate through Nix"
			model.message = ""
			candidate := model.settings.candidate
			return model, func() tea.Msg {
				return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(candidate)}
			}
		}
		return model, command
	case dashboardSettingsPasswords:
		if model.settings.returnScreen == dashboardSetup || model.installation.flow {
			switch key.String() {
			case "esc", "left":
				model.settings.editor = settingsWizardModel{}
				model.settings.candidate = domain.LabSettingsFile{}
				if model.installation.flow {
					model.installation.flow = false
					model.installation.failed = false
					model.screen = dashboardHome
				} else {
					model.screen = dashboardSetup
				}
				model.settings.returnScreen = dashboardHome
				model.settings.collectPasswords = false
				model.message = "Computer installation cancelled; no setting was changed."
			case "enter":
				if model.actions.ChangePassword == nil {
					model.message = "Password setup is not available in this deployment."
					return model, nil
				}
				candidate := model.settings.candidate
				command := &settingsPasswordCommand{action: model.actions.ChangePassword, account: "all", settings: candidate}
				model.message = ""
				return model, tea.Exec(command, func(err error) tea.Msg {
					return dashboardSettingsPasswordMsg{candidate: command.candidate, err: err}
				})
			}
			return model, nil
		}
		switch key.String() {
		case "esc", "left":
			model.message = ""
			model.screen = dashboardSettings
		case "enter":
			choice, selected := model.settings.passwordMenu.selected()
			if !selected {
				model.message = "Select an account."
				return model, nil
			}
			command := &settingsPasswordCommand{
				action:   model.actions.ChangePassword,
				account:  choice.id,
				settings: model.settings.current,
			}
			model.message = ""
			return model, tea.Exec(command, func(err error) tea.Msg {
				return dashboardSettingsPasswordMsg{candidate: command.candidate, err: err}
			})
		default:
			model.settings.passwordMenu, _ = model.settings.passwordMenu.update(key)
		}
	case dashboardSettingsReview:
		switch strings.ToLower(key.String()) {
		case "n", "esc":
			model.message = "Settings apply cancelled; no file changed."
			model.screen = dashboardSettings
		case "y", "enter":
			model.busy = "Saving the reviewed laboratory configuration"
			model.settings.applying = true
			model.message = ""
			candidate := model.settings.candidate
			plan := model.settings.plan
			return model, func() tea.Msg {
				report := model.actions.SaveSettings(candidate, plan)
				return dashboardSettingsApplyMsg{report: report}
			}
		}
	case dashboardHosts:
		switch key.String() {
		case "esc", "left":
			if model.computers.hostDetail {
				model.computers.hostDetail = false
				model.computers.hostTechnical = false
				return model, nil
			}
			if model.computers.hostQuery != "" {
				model.computers.hostQuery = ""
				model.computers.hostCursor = 0
				return model, nil
			}
			if model.computers.configurationState.Operation != "" {
				model.screen = dashboardSoftware
			} else {
				model.screen = dashboardHome
			}
			model.message = ""
		case "/":
			model.computers.hostSearching = true
			model.computers.hostDetail = false
		case "up", "k":
			model.computers.hostCursor = max(0, model.computers.hostCursor-1)
		case "down", "j":
			model.computers.hostCursor = max(0, min(len(model.filteredHosts())-1, model.computers.hostCursor+1))
		case "enter":
			model.computers.hostDetail = len(model.filteredHosts()) > 0
		case "t":
			model.computers.hostTechnical = !model.computers.hostTechnical
			model.computers.hostDetail = true
		case "d":
			hosts := model.filteredHosts()
			if len(hosts) > 0 {
				model.screen = dashboardDeploy
				model.deployment.result = domain.DeploymentExecutionReport{}
				model.deployment.context = ""
				model.deployment.chosen = map[string]bool{hosts[min(model.computers.hostCursor, len(hosts)-1)].Name: true}
				model.deployment.cursor = 0
			}
		case "r":
			model.busy = "Refreshing computer status"
			model.message = ""
			if model.computers.configurationState.Operation != "" {
				model.busy = "Refreshing desired and observed system state"
				return model, model.loadConfigurationState()
			}
			return model, model.loadHosts()
		}
	default:
		return model.updateOperationScreenKey(key)
	}
	return model, nil
}

func (model dashboardModel) updateOperationScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardSoftware:
		return model.updateSoftware(key)
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		return model.updateShutdown(key)
	case dashboardDeploy, dashboardDeployReview:
		return model.updateDeployment(key)
	case dashboardController:
		if key.String() == "esc" || key.String() == "left" || (key.String() == "enter" && model.controller.result.Operation != "") {
			if model.setupMode {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			if model.controller.result.Operation != "" && !model.controller.result.HasErrors() {
				model.message = "Controller configuration activated and verified."
			} else {
				model.message = ""
			}
			if model.setupMode && model.actions.LoadSetup != nil {
				model.busy = "Refreshing first-run progress"
				return model, model.loadSetup()
			}
		} else if key.String() == "d" && model.controller.result.Operation != "" {
			model.controller.details = !model.controller.details
		} else if key.String() == "l" && model.controller.result.Operation != "" {
			model.screen = dashboardLogs
			model.busy = "Loading private operation logs"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardLogsMsg{report: model.actions.LoadLogs()}
			}
		} else if key.String() == "r" {
			model.busy = "Reviewing controller revision and active system"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardControllerPlanMsg{report: model.actions.PlanController()}
			}
		}
	case dashboardControllerReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardController
			model.message = "Controller rebuild cancelled; no action was started."
		case "enter":
			model.busy = "Building and activating the reviewed controller revision"
			model.controller.applying = true
			model.controller.progress = domain.OperationProgress{}
			model.controller.started = time.Now().UTC()
			model.controller.progressID++
			model.message = ""
			plan := model.controller.plan
			operation := func() tea.Msg {
				report := model.actions.ApplyController(plan)
				if model.actions.Refresh == nil {
					return dashboardControllerResultMsg{report: report}
				}
				status, err := model.actions.Refresh()
				return dashboardControllerResultMsg{report: report, status: status, statusErr: err}
			}
			return model, tea.Batch(operation, scheduleControllerProgressTick(model.controller.progressID))
		}
	case dashboardServices:
		if model.maintenance.serviceResult.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				model.screen = dashboardHome
				model.message = ""
			case "l":
				model.busy = "Loading private operation logs"
				model.message = ""
				return model, func() tea.Msg {
					return dashboardLogsMsg{report: model.actions.LoadLogs()}
				}
			case "r":
				model.maintenance.serviceResult = domain.ServiceActionReport{}
				model.confirmation = ""
				model.message = ""
				model.screen = dashboardServicesRestartReview
			}
			return model, nil
		}
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "f":
			model.busy = "Refreshing managed controller services"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardServicesMsg{report: model.actions.LoadServices()}
			}
		case "r":
			if len(model.maintenance.services.Services) == 0 || model.maintenance.services.Services[0].ID != "cache" || len(model.maintenance.services.Services[0].Units) == 0 || !model.maintenance.services.Services[0].Units[0].Loaded {
				model.message = "Binary cache restart is unavailable because the managed unit is not installed."
				return model, nil
			}
			model.confirmation = ""
			model.message = ""
			model.screen = dashboardServicesRestartReview
		}
	case dashboardServicesRestartReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardServices
			model.confirmation = ""
			model.message = "Service restart cancelled; no action was started."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != "RESTART" {
				model.confirmation = ""
				model.message = "Confirmation did not match; the cache was not restarted."
				return model, nil
			}
			model.busy = "Restarting and verifying the binary cache"
			model.confirmation = ""
			model.message = ""
			return model, func() tea.Msg {
				return dashboardServiceResultMsg{report: model.actions.RestartService("cache")}
			}
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardLogs:
		switch key.String() {
		case "esc", "left":
			if model.setupMode {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			model.message = ""
			if model.setupMode && model.actions.LoadSetup != nil {
				model.busy = "Refreshing first-run progress"
				return model, model.loadSetup()
			}
		case "up", "k":
			if model.maintenance.logCursor > 0 {
				model.maintenance.logCursor--
			}
		case "down", "j":
			if model.maintenance.logCursor+1 < len(model.maintenance.logs.Logs) {
				model.maintenance.logCursor++
			}
		case "f":
			model.busy = "Refreshing private operation logs"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardLogsMsg{report: model.actions.LoadLogs()}
			}
		case "enter":
			if len(model.maintenance.logs.Logs) == 0 || !model.maintenance.logs.Logs[model.maintenance.logCursor].Available {
				model.message = "The selected operation log is not available for safe reading."
				return model, nil
			}
			id := model.maintenance.logs.Logs[model.maintenance.logCursor].ID
			model.busy = "Reading the bounded operation log tail"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardLogMsg{report: model.actions.LoadLog(id)}
			}
		}
	case dashboardLogDetail:
		maximum := maximumLogScroll(model.maintenance.logDetail, model.logDetailHeight())
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardLogs
			model.message = ""
		case "up", "k":
			if model.maintenance.logScroll > 0 {
				model.maintenance.logScroll--
			}
		case "down", "j":
			if model.maintenance.logScroll < maximum {
				model.maintenance.logScroll++
			}
		case "pgup":
			model.maintenance.logScroll -= model.logDetailHeight()
			if model.maintenance.logScroll < 0 {
				model.maintenance.logScroll = 0
			}
		case "pgdown":
			model.maintenance.logScroll += model.logDetailHeight()
			if model.maintenance.logScroll > maximum {
				model.maintenance.logScroll = maximum
			}
		case "home":
			model.maintenance.logScroll = 0
		case "end":
			model.maintenance.logScroll = maximum
		}
	default:
		return model.updateRepositoryScreenKey(key)
	}
	return model, nil
}

func (model dashboardModel) updateRepositoryScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardGitReview:
		if model.maintenance.gitCommitResult.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				if model.setupMode {
					model.screen = dashboardSetup
					model.message = ""
					if model.actions.LoadSetup != nil {
						model.busy = "Refreshing first-run progress"
						return model, model.loadSetup()
					}
				} else {
					model.screen = dashboardHome
					model.message = ""
				}
			case "f":
				model.maintenance.gitCommitResult = domain.GitCommitReport{}
				model.busy = "Refreshing the read-only Git review"
				model.message = ""
				return model, func() tea.Msg {
					return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
				}
			}
			return model, nil
		}
		maximum := maximumGitReviewScroll(model.maintenance.gitReview, model.gitReviewHeight())
		switch key.String() {
		case "esc", "left":
			if model.setupMode {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			model.message = ""
			if model.setupMode && model.actions.LoadSetup != nil {
				model.busy = "Refreshing first-run progress"
				return model, model.loadSetup()
			}
		case "up", "k":
			if model.maintenance.gitScroll > 0 {
				model.maintenance.gitScroll--
			}
		case "down", "j":
			if model.maintenance.gitScroll < maximum {
				model.maintenance.gitScroll++
			}
		case "pgup":
			model.maintenance.gitScroll -= model.gitReviewHeight()
			if model.maintenance.gitScroll < 0 {
				model.maintenance.gitScroll = 0
			}
		case "pgdown":
			model.maintenance.gitScroll += model.gitReviewHeight()
			if model.maintenance.gitScroll > maximum {
				model.maintenance.gitScroll = maximum
			}
		case "home":
			model.maintenance.gitScroll = 0
		case "end":
			model.maintenance.gitScroll = maximum
		case "f":
			model.busy = "Refreshing the read-only Git review"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
			}
		case "c":
			if len(model.maintenance.gitReview.Changes) == 0 || model.maintenance.gitReview.HasErrors() {
				model.message = "A clean, unblocked change review is required before selecting commit paths."
				return model, nil
			}
			model.maintenance.gitCommitChosen = map[string]bool{}
			model.maintenance.gitCommitCursor = 0
			model.message = ""
			model.screen = dashboardGitCommitSelect
		}
	case dashboardGitCommitSelect:
		changes := model.maintenance.gitReview.Changes
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardGitReview
			model.message = ""
		case "up", "k":
			if model.maintenance.gitCommitCursor > 0 {
				model.maintenance.gitCommitCursor--
			}
		case "down", "j":
			if model.maintenance.gitCommitCursor+1 < len(changes) {
				model.maintenance.gitCommitCursor++
			}
		case "space":
			if len(changes) > 0 && !changes[model.maintenance.gitCommitCursor].Private {
				path := changes[model.maintenance.gitCommitCursor].Path
				model.maintenance.gitCommitChosen[path] = !model.maintenance.gitCommitChosen[path]
			}
		case "a":
			model.maintenance.gitCommitChosen = toggleAllGitCommitPaths(changes, model.maintenance.gitCommitChosen)
		case "enter":
			paths := selectedGitCommitPaths(changes, model.maintenance.gitCommitChosen)
			if paths == "" {
				model.message = "Select at least one changed path before creating a commit plan."
				return model, nil
			}
			model.busy = "Building an isolated commit proposal from HEAD"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardGitCommitPlanMsg{report: model.actions.PlanGitCommit(paths)}
			}
		}
	case dashboardGitCommitReview:
		maximum := maximumGitCommitPlanScroll(model.maintenance.gitCommitPlan, model.gitReviewHeight())
		switch key.String() {
		case "esc":
			model.screen = dashboardGitCommitSelect
			model.confirmation = ""
			model.message = "Git commit cancelled; no repository state changed."
		case "up":
			if model.maintenance.gitScroll > 0 {
				model.maintenance.gitScroll--
			}
		case "down":
			if model.maintenance.gitScroll < maximum {
				model.maintenance.gitScroll++
			}
		case "pgup":
			model.maintenance.gitScroll -= model.gitReviewHeight()
			if model.maintenance.gitScroll < 0 {
				model.maintenance.gitScroll = 0
			}
		case "pgdown":
			model.maintenance.gitScroll += model.gitReviewHeight()
			if model.maintenance.gitScroll > maximum {
				model.maintenance.gitScroll = maximum
			}
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != model.maintenance.gitCommitPlan.Confirmation {
				model.confirmation = ""
				model.message = "Confirmation did not match; no Git commit was created."
				return model, nil
			}
			model.busy = "Revalidating and creating the reviewed local commit"
			model.confirmation = ""
			model.message = ""
			plan := model.maintenance.gitCommitPlan
			return model, func() tea.Msg {
				result := model.actions.ApplyGitCommit(plan)
				return dashboardGitCommitResultMsg{report: result, review: model.actions.LoadGitReview()}
			}
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardUpdate:
		if model.updates.result.Operation != "" {
			switch key.String() {
			case "enter", "esc":
				model.screen = dashboardHome
				model.message = ""
			case "r":
				if model.updates.result.RecoveryRequired {
					model.busy = "Completing the local update save"
					model.updates.applying = true
					model.message = ""
					plan := model.updates.plan
					return model, func() tea.Msg {
						return dashboardUpdateResultMsg{report: model.saveReviewedUpdate(plan)}
					}
				}
				model.updates.result = domain.UpdateApplyReport{}
				model.updates.check = domain.UpdateCheckReport{}
				model.updates.target = ""
				model.message = ""
				if model.updates.packageBase {
					return model.openPackageBase()
				}
				if model.actions.CheckUpdate != nil {
					model.busy = "Fetching available Nixorium updates"
					return model, model.checkUpdates()
				}
			case "a":
				if model.updates.result.Updated && !model.updates.result.HasErrors() && !model.updates.result.RecoveryRequired {
					return model.startUpdateControllerApply()
				}
			}
			return model, nil
		}
		if model.updates.packageBase {
			return model.updatePackageBaseKey(key)
		}
		switch key.String() {
		case "esc":
			model.screen = dashboardHome
			model.message = ""
		case "p", "f2":
			model.updates.prerelease = !model.updates.prerelease
			model.updates.cursor = 0
			model.message = ""
		case "r":
			if model.actions.CheckUpdate == nil {
				model.message = "Update discovery is not available in this session."
				return model, nil
			}
			model.updates.check = domain.UpdateCheckReport{}
			model.updates.target = ""
			model.message = ""
			model.busy = "Fetching available Nixorium updates"
			return model, model.checkUpdates()
		case "up", "k":
			model.updates.cursor = max(0, model.updates.cursor-1)
		case "down", "j":
			releases := model.availableUpdateReleases()
			if model.updates.cursor+1 < len(releases) {
				model.updates.cursor++
			}
		case "enter":
			releases := model.availableUpdateReleases()
			if model.updates.check.HasErrors() || len(releases) == 0 {
				model.message = "Fetch available updates before selecting a target."
				return model, nil
			}
			selected := releases[min(model.updates.cursor, len(releases)-1)]
			target := selected.Tag
			if updateReleaseAlreadyCurrent(model.updates.check, selected) {
				if selected.Channel == domain.UpdateChannelMoving {
					model.message = target + " already points to the current upstream revision."
				} else {
					model.message = target + " is already the configured Nixorium release."
				}
				return model, nil
			}
			model.updates.target = target
			model.busy = "Starting candidate validation"
			model.message = ""
			allowPrerelease := selected.Channel == domain.UpdateChannelPrerelease
			if model.actions.PlanUpdateWithProgress != nil {
				model.updates.planning = true
				model.updates.planProgress = domain.UpdatePlanProgress{}
				model.updates.planStarted = time.Now().UTC()
				events := make(chan tea.Msg)
				model.updates.planEvents = events
				return model, startUpdatePlan(model.actions.PlanUpdateWithProgress, target, allowPrerelease, false, events)
			}
			return model, func() tea.Msg {
				return dashboardUpdatePlanMsg{report: model.actions.PlanUpdate(target, allowPrerelease, false)}
			}
		}
	case dashboardUpdateReview:
		maximum := maximumUpdateScroll(model.updates.plan, model.updateReviewHeight())
		switch key.String() {
		case "f4":
			model.updateDetails = !model.updateDetails
		case "esc":
			model.screen = dashboardUpdate
			model.message = "Update cancelled; flake.nix and flake.lock were not changed."
		case "up":
			if model.updates.scroll > 0 {
				model.updates.scroll--
			}
		case "down":
			if model.updates.scroll < maximum {
				model.updates.scroll++
			}
		case "pgup":
			model.updates.scroll -= model.updateReviewHeight()
			if model.updates.scroll < 0 {
				model.updates.scroll = 0
			}
		case "pgdown":
			model.updates.scroll += model.updateReviewHeight()
			if model.updates.scroll > maximum {
				model.updates.scroll = maximum
			}
		case "enter":
			model.busy = "Saving the validated update"
			model.updates.applying = true
			model.message = ""
			plan := model.updates.plan
			return model, func() tea.Msg {
				return dashboardUpdateResultMsg{report: model.saveReviewedUpdate(plan)}
			}
		}
	default:
		return model.updatePXEScreenKey(key)
	}
	return model, nil
}

func (model dashboardModel) updatePXEScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardPXE:
		switch key.String() {
		case "esc", "left":
			if model.installation.flow && model.installation.failed {
				model.installation.flow = false
				model.installation.failed = false
				model.screen = dashboardHome
				model.message = ""
				return model, nil
			}
			if model.computers.restoreMode {
				model.screen = dashboardRestore
				model.computers.restoreMode = false
			} else if model.setupMode {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			model.message = ""
			if model.setupMode && model.actions.LoadSetup != nil {
				model.busy = "Refreshing first-run progress"
				return model, model.loadSetup()
			}
		case "p":
			model.busy = "Preparing netboot artifacts and client closures"
			model.installation.pxePreparing = true
			model.installation.pxeProgress = domain.OperationProgress{}
			model.installation.pxeStarted = time.Now().UTC()
			model.installation.pxeProgressID++
			model.message = ""
			operation := model.runAction(func() string {
				report := model.actions.PreparePXE()
				return report.Message
			}, dashboardPXE)
			return model, tea.Batch(operation, schedulePXEProgressTick(model.installation.pxeProgressID))
		case "s":
			model.busy = "Checking PXE readiness"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardPlanMsg{report: model.actions.PlanPXEStart()}
			}
		case "x":
			model.busy = "Stopping installation mode and restoring networking"
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.StopPXE().Message
			}, dashboardPXE)
		case "r":
			if model.report.PXE.Mode != "degraded" && model.report.PXE.Mode != "recovery-required" {
				return model, nil
			}
			model.busy = "Recovering normal controller networking"
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.RecoverPXE().Message
			}, dashboardPXE)
		}
	case dashboardDiagnostics:
		switch key.String() {
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
			command := model.startDiagnostics()
			return model, command
		}
	case dashboardPXEStartReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardPXE
			model.confirmation = ""
			model.message = "PXE start cancelled; networking was not changed."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != "START" {
				model.confirmation = ""
				model.message = "Confirmation did not match; networking was not changed."
				return model, nil
			}
			model.busy = "Starting managed PXE installation mode"
			model.confirmation = ""
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.StartPXE().Message
			}, dashboardPXE)
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardPXELeaveReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardPXE
			model.confirmation = ""
			model.message = "Continue the installation or stop PXE before leaving."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != "LEAVE" {
				model.confirmation = ""
				model.message = "Confirmation did not match; Nixorium remains open."
				return model, nil
			}
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
				status, err := model.actions.Refresh()
				return dashboardPXEExitMsg{lifecycle: lifecycle, status: status, statusErr: err}
			}
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	}
	return model, nil
}
