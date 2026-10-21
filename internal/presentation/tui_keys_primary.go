package presentation

import (
	"context"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateHomeKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			return model.loadInitial()
		case "b":
			if model.actions.LoadRecovery != nil && !model.actions.ClassroomMode {
				return model.openRecovery()
			}
		case "d":
			if model.actions.LoadDoctor != nil && !model.actions.ClassroomMode {
				model.diagnosticReturn = dashboardHome
				model.screen = dashboardDiagnostics
				return model.startDiagnostics()
			}
		case "s":
			if model.actions.PreviewSupport != nil && !model.actions.ClassroomMode {
				return model.openSupportPreview()
			}
		case "g":
			if model.actions.LoadGitReview != nil && !model.actions.ClassroomMode {
				return model.openMaintenanceTask("g")
			}
		}
		return model, nil
	}
	if model.updateNotification.Key != "" {
		if key.String() == "x" {
			return model.dismissUpdateNotification()
		}
		if key.String() == "u" {
			return model.openMaintenanceTask("u")
		}
	}
	model.syncHomeTasks()
	if key.String() == "r" {
		return model.refreshOverview()
	}
	action := key.String()
	for _, task := range model.pendingTasks() {
		if action == task.shortcut {
			return model.openPendingTask(task.id)
		}
	}
	if action == "enter" {
		if selected, ok := model.homeMenu.selected(); ok {
			if strings.HasPrefix(selected.id, "pending-") {
				return model.openPendingTask(selected.id)
			}
			action = selected.shortcut
		}
	}
	switch action {
	case "a":
		return model.openAssistant()
	case "m":
		model.screen = dashboardAdministration
	case "c":
		return model.openComputersArea()
	case "w":
		model.screen = dashboardSoftware
		model.software = model.software.open()
		model.controller.plan = domain.ControllerRebuildPlanReport{}
		model.controller.result = domain.ControllerRebuildExecutionReport{}
		model.busy = "Loading this laboratory’s software choices"
		model.message = ""
		if model.actions.LoadSoftware == nil {
			model.busy = ""
			model.message = "The supported software service is not available in this deployment."
			return model, nil
		}
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSoftwareCatalogMsg{report: model.actions.LoadSoftware(ctx)}
		})
	case "n":
		return model.startComputerInstallation()
	default:
		var command tea.Cmd
		model.homeMenu, command = model.homeMenu.update(key)
		return model, command
	}
	return model, nil
}

func (model dashboardModel) updateAssistantScreenKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		model.screen = dashboardHome
		model.message = ""
	}
	return model, nil
}

func (model dashboardModel) updateComputersAreaKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "n" && model.computerSetup != "" && model.computerSetup != "ready" && !model.actions.ClassroomMode {
		return model.startComputerInstallation()
	}
	tasks := model.availableComputerTasks()
	action := key.String()
	if action == "enter" {
		action = tasks[model.computers.areaCursor].shortcut
	}
	switch action {
	case "esc", "left":
		if !model.actions.ClassroomMode {
			model.screen = dashboardHome
			model.message = ""
		}
	case "up", "k":
		model.computers.areaCursor = max(0, model.computers.areaCursor-1)
	case "down", "j":
		model.computers.areaCursor = min(len(tasks)-1, model.computers.areaCursor+1)
	case "d":
		if model.actions.ClassroomMode {
			return model, nil
		}
		model.areaReturn = dashboardComputersArea
		return model.openComputerTask(action)
	case "h", "x", "i", "l", "s":
		model.areaReturn = dashboardComputersArea
		return model.openComputerTask(action)
	case "u":
		if model.actions.ClassroomMode {
			return model, nil
		}
		model.areaReturn = dashboardComputersArea
		return model.openComputerTask(action)
	case "v":
		return model.openComputerTask(action)
	}
	return model, nil
}

func (model dashboardModel) updateInstallationAreaKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := key.String()
	if action == "enter" {
		action = installationAreaTasks[model.installationAreaCursor].shortcut
	}
	switch action {
	case "p":
		return model.openNetworkInstallation()
	case "u":
		return model.openUSBInstallation()
	case "esc", "left":
		model.installation.flow = false
		model.screen = dashboardHome
	case "up", "k":
		model.installationAreaCursor = max(0, model.installationAreaCursor-1)
	case "down", "j":
		model.installationAreaCursor = min(len(installationAreaTasks)-1, model.installationAreaCursor+1)
	case "r":
		if model.installation.remote.operationID != "" {
			model.screen = dashboardUSBInstall
			model.installation.method = domain.RemoteInstallUSBSSH
			model.busy = "Reattaching to the recorded remote installation operation"
			return model.remoteInstallCommand("status", domain.RemoteInstallRequest{Operation: domain.RemoteInstallStatusOperation, OperationID: model.installation.remote.operationID})
		}
	}
	return model, nil
}

func (model dashboardModel) updateSetupKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		return model.startDiagnostics()
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
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSetupKeyStatusMsg{report: model.actions.LoadSetupKeys(ctx)}
		})
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
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardControllerPlanMsg{report: model.actions.PlanController(ctx)}
		})
	case domain.SetupStageArtifacts:
		if reason := model.managedJobConflict(); reason != "" {
			model.message = reason
			return model, nil
		}
		model.screen = dashboardPXE
		model.busy = "Building the network installer and client systems"
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
}

func (model dashboardModel) updateSetupKeysKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
				ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
				defer cancel()
				return dashboardSetupKeyImportMsg{report: report, err: err, keys: model.actions.LoadSetupKeys(ctx)}
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
	return model, nil
}

func (model dashboardModel) updateSettingsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.settings.result.Operation != "" {
		switch key.String() {
		case "enter":
			return model.continueSavedConfiguration()
		case "esc", "left":
			return model.returnFromSettings()
		case "s":
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
		if group.id == "workspace" {
			return model.openWorkspace()
		}
		if group.id == "accounts" {
			model.settings.accountsMenu = newRoutineAccountsMenu(model.isDark, model.width, model.height)
			model.settings.fromAccounts = false
			model.message = ""
			model.screen = dashboardSettingsAccounts
			return model, nil
		}
		model.settings.fromAccounts = false
		model = model.openSettingsEditor(group.fields, "Maintenance / Settings / "+group.label)
	case "y":
		if model.actions.LoadSetupKeys == nil {
			model.message = "Controller key management is not available in this session."
			return model, nil
		}
		model.setupKeysReturn = dashboardSettings
		model.screen = dashboardSetupKeys
		model.busy = "Checking existing controller keys"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSetupKeyStatusMsg{report: model.actions.LoadSetupKeys(ctx)}
		})
	default:
		model.settings.menu, _ = model.settings.menu.update(key)
	}
	return model, nil
}

func (model dashboardModel) updateSettingsEditKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			if model.settings.fromAccounts {
				model.screen = dashboardSettingsAccounts
			}
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
		model.busy = "Checking that the proposed settings can build a valid system"
		model.message = ""
		candidate := model.settings.candidate
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(ctx, candidate)}
		})
	}
	return model, command
}

func (model dashboardModel) updateSettingsPasswordsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.settings.returnScreen == dashboardSetup || model.installation.flow {
		switch key.String() {
		case "esc", "left":
			model.settings.editor = settingsWizardModel{}
			model.settings.candidate = domain.LabSettingsFile{}
			if model.installation.flow {
				model.installation.flow = false
				model.installation.failed = false
				model.screen = dashboardHome
				model.message = "Computer installation cancelled; no setting was changed."
			} else {
				model.screen = dashboardSetup
				model.message = "Setup cancelled; no setting was changed."
			}
			model.settings.returnScreen = dashboardHome
			model.settings.collectPasswords = false
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
		if model.settings.fromAccounts {
			model.screen = dashboardSettingsAccounts
		}
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
	return model, nil
}

func (model dashboardModel) updateSettingsAccountsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left":
		model.message = ""
		model.settings.fromAccounts = false
		model.screen = dashboardSettings
	case "enter":
		choice, selected := model.settings.accountsMenu.selected()
		if !selected {
			model.message = "Select what to change."
			return model, nil
		}
		model.settings.fromAccounts = true
		model.message = ""
		if choice.id == "passwords" {
			model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
			model.screen = dashboardSettingsPasswords
			return model, nil
		}
		model = model.openSettingsEditor(accountsSettingsGroup().fields, "Maintenance / Settings / Accounts / Names")
	default:
		model.settings.accountsMenu, _ = model.settings.accountsMenu.update(key)
	}
	return model, nil
}

func (model dashboardModel) updateSettingsReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(key.String()) {
	case "n", "esc":
		model.message = "Settings apply cancelled; no file changed."
		model.screen = dashboardSettings
	case "c":
		if len(model.settings.plan.NetworkPreflight.Missing()) > 0 {
			model.settings.saveAnyway = !model.settings.saveAnyway
			model.message = ""
		}
	case "y", "enter":
		plan := model.reviewedSettingsPlan()
		if refusal := plan.NetworkPreflight.SaveRefusal(); refusal != "" {
			model.message = "Not saved: " + refusal
			return model, nil
		}
		model.busy = "Saving the reviewed laboratory configuration"
		model.settings.applying = true
		model.message = ""
		candidate := model.settings.candidate
		model.settings.plan = plan
		return model, func() tea.Msg {
			report := model.actions.SaveSettings(candidate, plan)
			return dashboardSettingsApplyMsg{report: report}
		}
	}
	return model, nil
}

func (model dashboardModel) updateHostsKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "h":
		hosts := model.filteredHosts()
		if !model.actions.ClassroomMode && model.computers.hostDetail && len(hosts) > 0 {
			host := hosts[min(model.computers.hostCursor, len(hosts)-1)]
			if host.HostKeyCondition == domain.HostKeyChanged {
				return model.openHostTrust(host.Name)
			}
		}
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
		} else if model.actions.ClassroomMode {
			model.screen = dashboardComputersArea
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
	case "u":
		if model.actions.ClassroomMode || model.computers.hostDetail {
			return model, nil
		}
		return model.updateComputersThatNeedIt()
	case "d":
		if model.actions.ClassroomMode {
			return model, nil
		}
		hosts := model.filteredHosts()
		if len(hosts) > 0 {
			host := hosts[min(model.computers.hostCursor, len(hosts)-1)]
			if host.HostKeyCondition == domain.HostKeyChanged {
				model.message = host.Name + " offers a changed SSH key. Review it first (h); updating it would not be safe."
				return model, nil
			}
			return model.openHostDeployment(host.Name)
		}
	case "r":
		model.busy = "Refreshing computer status"
		model.message = ""
		if model.computers.configurationState.Operation != "" {
			model.busy = "Refreshing desired and observed system state"
			return model.loadConfigurationState()
		}
		return model.loadHosts()
	}
	return model, nil
}
