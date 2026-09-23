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
		model.restoreCursor = 0
		model.message = ""
	case "x":
		model.screen = dashboardShutdown
		model.shutdown = newShutdownModel()
		model.message = ""
	case "d":
		model.screen = dashboardDeploy
		model.deployResult = domain.DeploymentExecutionReport{}
		model.deployContext = ""
		model.message = ""
		model.deployChosen = map[string]bool{}
		model.deployCursor = 0
	case "h":
		model.hostDetail = false
		model.hostTechnical = false
		model.configurationState = domain.ConfigurationStateReport{}
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
		model.baseUpdate = false
		model.screen = dashboardUpdate
		model.updateResult = domain.UpdateApplyReport{}
		model.controllerPlan = domain.ControllerRebuildPlanReport{}
		model.controllerResult = domain.ControllerRebuildExecutionReport{}
		model.updatePlan = domain.UpdatePlanReport{}
		model.updatePrerelease = false
		model.updateCheck = domain.UpdateCheckReport{}
		model.updateCursor = 0
		model.updateTarget = ""
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
		model.settingsReturn = dashboardAdministration
		model.settingsResult = domain.ConfigurationSaveReport{}
		model.settingsPlan = domain.ConfigPlanReport{}
		model.settingsCandidate = domain.LabSettingsFile{}
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
		model.restoreMode = false
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
			model.controllerPlan = domain.ControllerRebuildPlanReport{}
			model.controllerResult = domain.ControllerRebuildExecutionReport{}
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
			action = computersAreaTasks[model.computersAreaCursor].shortcut
		}
		switch action {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "up", "k":
			model.computersAreaCursor = max(0, model.computersAreaCursor-1)
		case "down", "j":
			model.computersAreaCursor = min(len(computersAreaTasks)-1, model.computersAreaCursor+1)
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
			model.restoreCursor = max(0, model.restoreCursor-1)
		case "down", "j":
			model.restoreCursor = min(1, model.restoreCursor+1)
		case "enter":
			model.message = ""
			if model.restoreCursor == 0 {
				model.restoreMode = true
				model.screen = dashboardDeploy
				model.deployResult = domain.DeploymentExecutionReport{}
				model.deployContext = ""
				model.deployChosen = map[string]bool{}
				model.deployCursor = 0
			} else {
				model.restoreMode = true
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
			model.pxePreparing = true
			model.pxeProgress = domain.OperationProgress{}
			model.pxeProgressStarted = time.Now().UTC()
			model.pxeProgressID++
			model.message = ""
			operation := model.runAction(func() string {
				report := model.actions.PreparePXE()
				return report.Message
			}, dashboardPXE)
			return model, tea.Batch(operation, schedulePXEProgressTick(model.pxeProgressID))
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
		if model.settingsResult.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				return model.returnFromSettings()
			case "r":
				if !model.settingsResult.RecoveryRequired || model.actions.SaveSettings == nil {
					return model, nil
				}
				model.busy = "Recovering the local configuration save"
				model.settingsApplying = true
				model.message = ""
				candidate := model.settingsCandidate
				plan := model.settingsPlan
				return model, func() tea.Msg {
					report := model.actions.SaveSettings(candidate, plan)
					return dashboardSettingsApplyMsg{report: report}
				}
			case "e":
				model.settingsResult = domain.ConfigurationSaveReport{}
				model.message = ""
			}
			return model, nil
		}
		switch key.String() {
		case "esc", "left":
			return model.returnFromSettings()
		case "enter":
			group, selected := model.settingsMenu.selected()
			if !selected {
				model.message = "Select a settings category."
				return model, nil
			}
			model.settingsEditor = newSettingsEditorModel(model.settings, group.fields, "Nixorium — Edit "+group.label)
			model.settingsEditor.width = model.width
			model.settingsEditor.height = model.height
			model.settingsEditor.isDark = model.isDark
			model.settingsEditor.prepareCurrentField()
			model.message = ""
			model.screen = dashboardSettingsEdit
		case "p":
			model.settingsPasswordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
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
			model.settingsMenu, _ = model.settingsMenu.update(key)
		}
	case dashboardSettingsEdit:
		updated, command := model.settingsEditor.Update(key)
		model.settingsEditor = updated.(settingsWizardModel)
		if model.settingsEditor.cancelled {
			model.settingsEditor = settingsWizardModel{}
			if model.installationFlow {
				model.installationFlow = false
				model.installationFailed = false
				model.settingsReturn = dashboardHome
				model.screen = dashboardHome
				model.message = "Computer installation cancelled; no setting was changed."
			} else {
				model.message = "Settings edit cancelled; no file changed."
				model.screen = dashboardSettings
			}
			return model, nil
		}
		if model.settingsEditor.accepted {
			model.settingsCandidate = model.settingsEditor.settings
			if (model.settingsReturn == dashboardSetup || model.installationFlow) && model.settingsCollectPasswords {
				if model.actions.ChangePassword == nil {
					model.message = "Password setup is not available in this deployment."
					return model, nil
				}
				model.settingsPasswordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
				model.screen = dashboardSettingsPasswords
				candidate := model.settingsCandidate
				command := &settingsPasswordCommand{action: model.actions.ChangePassword, account: "all", settings: candidate}
				return model, tea.Exec(command, func(err error) tea.Msg {
					return dashboardSettingsPasswordMsg{candidate: command.candidate, err: err}
				})
			}
			model.busy = "Validating the complete settings candidate through Nix"
			model.message = ""
			candidate := model.settingsCandidate
			return model, func() tea.Msg {
				return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(candidate)}
			}
		}
		return model, command
	case dashboardSettingsPasswords:
		if model.settingsReturn == dashboardSetup || model.installationFlow {
			switch key.String() {
			case "esc", "left":
				model.settingsEditor = settingsWizardModel{}
				model.settingsCandidate = domain.LabSettingsFile{}
				if model.installationFlow {
					model.installationFlow = false
					model.installationFailed = false
					model.screen = dashboardHome
				} else {
					model.screen = dashboardSetup
				}
				model.settingsReturn = dashboardHome
				model.settingsCollectPasswords = false
				model.message = "Computer installation cancelled; no setting was changed."
			case "enter":
				if model.actions.ChangePassword == nil {
					model.message = "Password setup is not available in this deployment."
					return model, nil
				}
				candidate := model.settingsCandidate
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
			choice, selected := model.settingsPasswordMenu.selected()
			if !selected {
				model.message = "Select an account."
				return model, nil
			}
			command := &settingsPasswordCommand{
				action:   model.actions.ChangePassword,
				account:  choice.id,
				settings: model.settings,
			}
			model.message = ""
			return model, tea.Exec(command, func(err error) tea.Msg {
				return dashboardSettingsPasswordMsg{candidate: command.candidate, err: err}
			})
		default:
			model.settingsPasswordMenu, _ = model.settingsPasswordMenu.update(key)
		}
	case dashboardSettingsReview:
		switch strings.ToLower(key.String()) {
		case "n", "esc":
			model.message = "Settings apply cancelled; no file changed."
			model.screen = dashboardSettings
		case "y", "enter":
			model.busy = "Saving the reviewed laboratory configuration"
			model.settingsApplying = true
			model.message = ""
			candidate := model.settingsCandidate
			plan := model.settingsPlan
			return model, func() tea.Msg {
				report := model.actions.SaveSettings(candidate, plan)
				return dashboardSettingsApplyMsg{report: report}
			}
		}
	case dashboardHosts:
		switch key.String() {
		case "esc", "left":
			if model.hostDetail {
				model.hostDetail = false
				model.hostTechnical = false
				return model, nil
			}
			if model.hostQuery != "" {
				model.hostQuery = ""
				model.hostCursor = 0
				return model, nil
			}
			if model.configurationState.Operation != "" {
				model.screen = dashboardSoftware
			} else {
				model.screen = dashboardHome
			}
			model.message = ""
		case "/":
			model.hostSearching = true
			model.hostDetail = false
		case "up", "k":
			model.hostCursor = max(0, model.hostCursor-1)
		case "down", "j":
			model.hostCursor = max(0, min(len(model.filteredHosts())-1, model.hostCursor+1))
		case "enter":
			model.hostDetail = len(model.filteredHosts()) > 0
		case "t":
			model.hostTechnical = !model.hostTechnical
			model.hostDetail = true
		case "d":
			hosts := model.filteredHosts()
			if len(hosts) > 0 {
				model.screen = dashboardDeploy
				model.deployResult = domain.DeploymentExecutionReport{}
				model.deployContext = ""
				model.deployChosen = map[string]bool{hosts[min(model.hostCursor, len(hosts)-1)].Name: true}
				model.deployCursor = 0
			}
		case "r":
			model.busy = "Refreshing computer status"
			model.message = ""
			if model.configurationState.Operation != "" {
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
	case dashboardDeploy:
		if model.deployResult.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				model.screen = dashboardComputersArea
				model.message = ""
			case "l":
				model.busy = "Loading operation logs"
				model.message = ""
				return model, func() tea.Msg {
					return dashboardLogsMsg{report: model.actions.LoadLogs()}
				}
			case "r":
				model.deployResult = domain.DeploymentExecutionReport{}
				model.deployProgress = domain.DeploymentProgress{}
				model.deployRecent = nil
				model.message = ""
			}
			return model, nil
		}
		hosts := model.report.Meta.Clients.Hosts
		switch key.String() {
		case "esc", "left":
			if model.restoreMode {
				model.screen = dashboardRestore
				model.restoreMode = false
			} else if model.deployContext != "" {
				model.screen = dashboardSoftware
				model.deployContext = ""
			} else {
				model.screen = dashboardHome
			}
			model.message = ""
		case "up", "k":
			if model.deployCursor > 0 {
				model.deployCursor--
			}
		case "down", "j":
			if model.deployCursor+1 < len(hosts) {
				model.deployCursor++
			}
		case "space":
			if len(hosts) > 0 {
				if model.deployChosen == nil {
					model.deployChosen = map[string]bool{}
				}
				name := hosts[model.deployCursor].Name
				model.deployChosen[name] = !model.deployChosen[name]
			}
		case "a":
			model.deployChosen = toggleAllDeploymentTargets(hosts, model.deployChosen)
		case "enter":
			requested := selectedDeploymentTargets(hosts, model.deployChosen)
			if model.deployContext != "" {
				requested = strings.Join(selectedDeploymentTargetNames(hosts, model.deployChosen), ",")
			}
			if requested == "" {
				model.message = "Select at least one computer before reviewing a deployment."
				return model, nil
			}
			model.busy = "Validating revision and selected computers"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardDeploymentPlanMsg{report: model.actions.PlanDeployment(requested)}
			}
		}
	case dashboardDeployReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardDeploy
			model.confirmation = ""
			model.message = "Deployment cancelled; no build or apply was started."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != "DEPLOY" {
				model.confirmation = ""
				model.message = "Confirmation did not match; no build or apply was started."
				return model, nil
			}
			model.busy = "Building and applying the reviewed deployment"
			model.deploying = true
			model.deployProgress = domain.DeploymentProgress{}
			model.deployRecent = nil
			model.deployStarted = time.Now()
			model.confirmation = ""
			model.message = ""
			plan := model.deployPlan
			events := make(chan tea.Msg)
			model.deployEvents = events
			return model, startDeployment(model.actions.ApplyDeployment, plan, events)
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardController:
		if key.String() == "esc" || key.String() == "left" || (key.String() == "enter" && model.controllerResult.Operation != "") {
			if model.setupMode {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			if model.controllerResult.Operation != "" && !model.controllerResult.HasErrors() {
				model.message = "Controller configuration activated and verified."
			} else {
				model.message = ""
			}
			if model.setupMode && model.actions.LoadSetup != nil {
				model.busy = "Refreshing first-run progress"
				return model, model.loadSetup()
			}
		} else if key.String() == "d" && model.controllerResult.Operation != "" {
			model.controllerDetails = !model.controllerDetails
		} else if key.String() == "l" && model.controllerResult.Operation != "" {
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
			model.controllerApplying = true
			model.controllerProgress = domain.OperationProgress{}
			model.controllerStarted = time.Now().UTC()
			model.controllerProgressID++
			model.message = ""
			plan := model.controllerPlan
			operation := func() tea.Msg {
				report := model.actions.ApplyController(plan)
				if model.actions.Refresh == nil {
					return dashboardControllerResultMsg{report: report}
				}
				status, err := model.actions.Refresh()
				return dashboardControllerResultMsg{report: report, status: status, statusErr: err}
			}
			return model, tea.Batch(operation, scheduleControllerProgressTick(model.controllerProgressID))
		}
	case dashboardServices:
		if model.serviceResult.Operation != "" {
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
				model.serviceResult = domain.ServiceActionReport{}
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
			if len(model.services.Services) == 0 || model.services.Services[0].ID != "cache" || len(model.services.Services[0].Units) == 0 || !model.services.Services[0].Units[0].Loaded {
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
			if model.logCursor > 0 {
				model.logCursor--
			}
		case "down", "j":
			if model.logCursor+1 < len(model.logs.Logs) {
				model.logCursor++
			}
		case "f":
			model.busy = "Refreshing private operation logs"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardLogsMsg{report: model.actions.LoadLogs()}
			}
		case "enter":
			if len(model.logs.Logs) == 0 || !model.logs.Logs[model.logCursor].Available {
				model.message = "The selected operation log is not available for safe reading."
				return model, nil
			}
			id := model.logs.Logs[model.logCursor].ID
			model.busy = "Reading the bounded operation log tail"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardLogMsg{report: model.actions.LoadLog(id)}
			}
		}
	case dashboardLogDetail:
		maximum := maximumLogScroll(model.logDetail, model.logDetailHeight())
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardLogs
			model.message = ""
		case "up", "k":
			if model.logScroll > 0 {
				model.logScroll--
			}
		case "down", "j":
			if model.logScroll < maximum {
				model.logScroll++
			}
		case "pgup":
			model.logScroll -= model.logDetailHeight()
			if model.logScroll < 0 {
				model.logScroll = 0
			}
		case "pgdown":
			model.logScroll += model.logDetailHeight()
			if model.logScroll > maximum {
				model.logScroll = maximum
			}
		case "home":
			model.logScroll = 0
		case "end":
			model.logScroll = maximum
		}
	default:
		return model.updateRepositoryScreenKey(key)
	}
	return model, nil
}

func (model dashboardModel) updateRepositoryScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardGitReview:
		if model.gitCommitResult.Operation != "" {
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
				model.gitCommitResult = domain.GitCommitReport{}
				model.busy = "Refreshing the read-only Git review"
				model.message = ""
				return model, func() tea.Msg {
					return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
				}
			}
			return model, nil
		}
		maximum := maximumGitReviewScroll(model.gitReview, model.gitReviewHeight())
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
			if model.gitScroll > 0 {
				model.gitScroll--
			}
		case "down", "j":
			if model.gitScroll < maximum {
				model.gitScroll++
			}
		case "pgup":
			model.gitScroll -= model.gitReviewHeight()
			if model.gitScroll < 0 {
				model.gitScroll = 0
			}
		case "pgdown":
			model.gitScroll += model.gitReviewHeight()
			if model.gitScroll > maximum {
				model.gitScroll = maximum
			}
		case "home":
			model.gitScroll = 0
		case "end":
			model.gitScroll = maximum
		case "f":
			model.busy = "Refreshing the read-only Git review"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
			}
		case "c":
			if len(model.gitReview.Changes) == 0 || model.gitReview.HasErrors() {
				model.message = "A clean, unblocked change review is required before selecting commit paths."
				return model, nil
			}
			model.gitCommitChosen = map[string]bool{}
			model.gitCommitCursor = 0
			model.message = ""
			model.screen = dashboardGitCommitSelect
		}
	case dashboardGitCommitSelect:
		changes := model.gitReview.Changes
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardGitReview
			model.message = ""
		case "up", "k":
			if model.gitCommitCursor > 0 {
				model.gitCommitCursor--
			}
		case "down", "j":
			if model.gitCommitCursor+1 < len(changes) {
				model.gitCommitCursor++
			}
		case "space":
			if len(changes) > 0 && !changes[model.gitCommitCursor].Private {
				path := changes[model.gitCommitCursor].Path
				model.gitCommitChosen[path] = !model.gitCommitChosen[path]
			}
		case "a":
			model.gitCommitChosen = toggleAllGitCommitPaths(changes, model.gitCommitChosen)
		case "enter":
			paths := selectedGitCommitPaths(changes, model.gitCommitChosen)
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
		maximum := maximumGitCommitPlanScroll(model.gitCommitPlan, model.gitReviewHeight())
		switch key.String() {
		case "esc":
			model.screen = dashboardGitCommitSelect
			model.confirmation = ""
			model.message = "Git commit cancelled; no repository state changed."
		case "up":
			if model.gitScroll > 0 {
				model.gitScroll--
			}
		case "down":
			if model.gitScroll < maximum {
				model.gitScroll++
			}
		case "pgup":
			model.gitScroll -= model.gitReviewHeight()
			if model.gitScroll < 0 {
				model.gitScroll = 0
			}
		case "pgdown":
			model.gitScroll += model.gitReviewHeight()
			if model.gitScroll > maximum {
				model.gitScroll = maximum
			}
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != model.gitCommitPlan.Confirmation {
				model.confirmation = ""
				model.message = "Confirmation did not match; no Git commit was created."
				return model, nil
			}
			model.busy = "Revalidating and creating the reviewed local commit"
			model.confirmation = ""
			model.message = ""
			plan := model.gitCommitPlan
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
		if model.updateResult.Operation != "" {
			switch key.String() {
			case "enter", "esc":
				model.screen = dashboardHome
				model.message = ""
			case "r":
				if model.updateResult.RecoveryRequired {
					model.busy = "Completing the local update save"
					model.updating = true
					model.message = ""
					plan := model.updatePlan
					return model, func() tea.Msg {
						return dashboardUpdateResultMsg{report: model.saveReviewedUpdate(plan)}
					}
				}
				model.updateResult = domain.UpdateApplyReport{}
				model.updateCheck = domain.UpdateCheckReport{}
				model.updateTarget = ""
				model.message = ""
				if model.baseUpdate {
					return model.openPackageBase()
				}
				if model.actions.CheckUpdate != nil {
					model.busy = "Fetching available Nixorium updates"
					return model, model.checkUpdates()
				}
			case "a":
				if model.updateResult.Updated && !model.updateResult.HasErrors() && !model.updateResult.RecoveryRequired {
					return model.startUpdateControllerApply()
				}
			}
			return model, nil
		}
		if model.baseUpdate {
			return model.updatePackageBaseKey(key)
		}
		switch key.String() {
		case "esc":
			model.screen = dashboardHome
			model.message = ""
		case "p", "f2":
			model.updatePrerelease = !model.updatePrerelease
			model.updateCursor = 0
			model.message = ""
		case "r":
			if model.actions.CheckUpdate == nil {
				model.message = "Update discovery is not available in this session."
				return model, nil
			}
			model.updateCheck = domain.UpdateCheckReport{}
			model.updateTarget = ""
			model.message = ""
			model.busy = "Fetching available Nixorium updates"
			return model, model.checkUpdates()
		case "up", "k":
			model.updateCursor = max(0, model.updateCursor-1)
		case "down", "j":
			releases := model.availableUpdateReleases()
			if model.updateCursor+1 < len(releases) {
				model.updateCursor++
			}
		case "enter":
			releases := model.availableUpdateReleases()
			if model.updateCheck.HasErrors() || len(releases) == 0 {
				model.message = "Fetch available updates before selecting a target."
				return model, nil
			}
			selected := releases[min(model.updateCursor, len(releases)-1)]
			target := selected.Tag
			if updateReleaseAlreadyCurrent(model.updateCheck, selected) {
				if selected.Channel == domain.UpdateChannelMoving {
					model.message = target + " already points to the current upstream revision."
				} else {
					model.message = target + " is already the configured Nixorium release."
				}
				return model, nil
			}
			model.updateTarget = target
			model.busy = "Starting candidate validation"
			model.message = ""
			allowPrerelease := selected.Channel == domain.UpdateChannelPrerelease
			if model.actions.PlanUpdateWithProgress != nil {
				model.updatePlanning = true
				model.updatePlanProgress = domain.UpdatePlanProgress{}
				model.updatePlanStarted = time.Now().UTC()
				events := make(chan tea.Msg)
				model.updatePlanEvents = events
				return model, startUpdatePlan(model.actions.PlanUpdateWithProgress, target, allowPrerelease, false, events)
			}
			return model, func() tea.Msg {
				return dashboardUpdatePlanMsg{report: model.actions.PlanUpdate(target, allowPrerelease, false)}
			}
		}
	case dashboardUpdateReview:
		maximum := maximumUpdateScroll(model.updatePlan, model.updateReviewHeight())
		switch key.String() {
		case "f4":
			model.updateDetails = !model.updateDetails
		case "esc":
			model.screen = dashboardUpdate
			model.message = "Update cancelled; flake.nix and flake.lock were not changed."
		case "up":
			if model.updateScroll > 0 {
				model.updateScroll--
			}
		case "down":
			if model.updateScroll < maximum {
				model.updateScroll++
			}
		case "pgup":
			model.updateScroll -= model.updateReviewHeight()
			if model.updateScroll < 0 {
				model.updateScroll = 0
			}
		case "pgdown":
			model.updateScroll += model.updateReviewHeight()
			if model.updateScroll > maximum {
				model.updateScroll = maximum
			}
		case "enter":
			model.busy = "Saving the validated update"
			model.updating = true
			model.message = ""
			plan := model.updatePlan
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
			if model.installationFlow && model.installationFailed {
				model.installationFlow = false
				model.installationFailed = false
				model.screen = dashboardHome
				model.message = ""
				return model, nil
			}
			if model.restoreMode {
				model.screen = dashboardRestore
				model.restoreMode = false
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
			model.pxePreparing = true
			model.pxeProgress = domain.OperationProgress{}
			model.pxeProgressStarted = time.Now().UTC()
			model.pxeProgressID++
			model.message = ""
			operation := model.runAction(func() string {
				report := model.actions.PreparePXE()
				return report.Message
			}, dashboardPXE)
			return model, tea.Batch(operation, schedulePXEProgressTick(model.pxeProgressID))
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
