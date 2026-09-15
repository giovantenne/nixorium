package presentation

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type DashboardActions struct {
	Refresh                func() (domain.StatusReport, error)
	LoadSetup              func() domain.SetupReport
	LoadHosts              func() (domain.HostsReport, error)
	PlanDeployment         func(string) domain.DeploymentPlanReport
	ApplyDeployment        func(domain.DeploymentPlanReport) domain.DeploymentExecutionReport
	PlanController         func() domain.ControllerRebuildPlanReport
	ApplyController        func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport
	LoadControllerProgress func() (domain.OperationProgress, error)
	LoadServices           func() domain.ServicesReport
	RestartService         func(string) domain.ServiceActionReport
	LoadLogs               func() domain.OperationLogsReport
	LoadLog                func(string) domain.OperationLogReport
	LoadGitReview          func() domain.GitReviewReport
	PlanGitCommit          func(string) domain.GitCommitPlanReport
	ApplyGitCommit         func(domain.GitCommitPlanReport) domain.GitCommitReport
	PlanUpdate             func(string, bool, bool) domain.UpdatePlanReport
	ApplyUpdate            func(domain.UpdatePlanReport) domain.UpdateApplyReport
	LoadSettings           func() (domain.LabSettingsFile, error)
	PlanSettings           func(domain.LabSettingsFile) domain.ConfigPlanReport
	ApplySettings          func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigApplyReport
	ChangePassword         SettingsPasswordAction
	PreparePXE             func() domain.ActionReport
	LoadPXEProgress        func() (domain.OperationProgress, error)
	PlanPXEStart           func() domain.PXELifecycleReport
	StartPXE               func() domain.PXELifecycleReport
	StopPXE                func() domain.PXELifecycleReport
	RecoverPXE             func() domain.PXELifecycleReport
}

type dashboardScreen int

const (
	dashboardHome dashboardScreen = iota
	dashboardSetup
	dashboardHosts
	dashboardDeploy
	dashboardDeployReview
	dashboardController
	dashboardControllerReview
	dashboardServices
	dashboardServicesRestartReview
	dashboardLogs
	dashboardLogDetail
	dashboardGitReview
	dashboardGitCommitSelect
	dashboardGitCommitReview
	dashboardUpdate
	dashboardUpdateReview
	dashboardSettings
	dashboardSettingsEdit
	dashboardSettingsPasswords
	dashboardSettingsReview
	dashboardPXE
	dashboardPXEStartReview
)

type dashboardModel struct {
	report               domain.StatusReport
	setup                domain.SetupReport
	setupMode            bool
	homeMenu             dashboardTaskMenu
	actions              DashboardActions
	screen               dashboardScreen
	busy                 string
	message              string
	confirmation         string
	startPlan            domain.PXELifecycleReport
	hosts                domain.HostsReport
	deployCursor         int
	deployChosen         map[string]bool
	deployPlan           domain.DeploymentPlanReport
	deployResult         domain.DeploymentExecutionReport
	deploying            bool
	controllerPlan       domain.ControllerRebuildPlanReport
	controllerResult     domain.ControllerRebuildExecutionReport
	controllerApplying   bool
	controllerProgress   domain.OperationProgress
	controllerStarted    time.Time
	controllerProgressID uint64
	controllerDetails    bool
	services             domain.ServicesReport
	serviceResult        domain.ServiceActionReport
	logs                 domain.OperationLogsReport
	logDetail            domain.OperationLogReport
	logCursor            int
	logScroll            int
	gitReview            domain.GitReviewReport
	gitScroll            int
	gitCommitCursor      int
	gitCommitChosen      map[string]bool
	gitCommitPlan        domain.GitCommitPlanReport
	gitCommitResult      domain.GitCommitReport
	updateTarget         string
	updatePrerelease     bool
	updateDowngrade      bool
	updatePlan           domain.UpdatePlanReport
	updateResult         domain.UpdateApplyReport
	updateScroll         int
	updating             bool
	settings             domain.LabSettingsFile
	settingsCandidate    domain.LabSettingsFile
	settingsMenu         routineSettingsMenu
	settingsPasswordMenu routinePasswordMenu
	settingsEditor       settingsWizardModel
	settingsPlan         domain.ConfigPlanReport
	settingsResult       domain.ConfigApplyReport
	settingsApplying     bool
	pxePreparing         bool
	pxeProgress          domain.OperationProgress
	pxeProgressStarted   time.Time
	pxeProgressID        uint64
	width                int
	height               int
	isDark               bool
}

type dashboardPlanMsg struct {
	report domain.PXELifecycleReport
}

type dashboardSetupMsg struct {
	report domain.SetupReport
}

type dashboardOperationMsg struct {
	message string
	report  domain.StatusReport
	err     error
	screen  dashboardScreen
}

type dashboardPXEProgressTickMsg struct {
	id uint64
}

type dashboardPXEProgressMsg struct {
	id       uint64
	progress domain.OperationProgress
	err      error
}

type dashboardHostsMsg struct {
	report domain.HostsReport
	err    error
}

type dashboardDeploymentPlanMsg struct {
	report domain.DeploymentPlanReport
}

type dashboardDeploymentResultMsg struct {
	report domain.DeploymentExecutionReport
}

type dashboardControllerPlanMsg struct {
	report domain.ControllerRebuildPlanReport
}

type dashboardControllerResultMsg struct {
	report    domain.ControllerRebuildExecutionReport
	status    domain.StatusReport
	statusErr error
}

type dashboardControllerProgressTickMsg struct {
	id uint64
}

type dashboardControllerProgressMsg struct {
	id       uint64
	progress domain.OperationProgress
	err      error
}

type dashboardServicesMsg struct {
	report domain.ServicesReport
}

type dashboardServiceResultMsg struct {
	report domain.ServiceActionReport
}

type dashboardLogsMsg struct {
	report domain.OperationLogsReport
}

type dashboardLogMsg struct {
	report domain.OperationLogReport
}

type dashboardGitReviewMsg struct {
	report domain.GitReviewReport
}

type dashboardGitCommitPlanMsg struct {
	report domain.GitCommitPlanReport
}

type dashboardGitCommitResultMsg struct {
	report domain.GitCommitReport
	review domain.GitReviewReport
}

type dashboardUpdatePlanMsg struct {
	report domain.UpdatePlanReport
}

type dashboardUpdateResultMsg struct {
	report domain.UpdateApplyReport
}

type dashboardSettingsMsg struct {
	settings domain.LabSettingsFile
	err      error
}

type dashboardSettingsPlanMsg struct {
	report domain.ConfigPlanReport
}

type dashboardSettingsPasswordMsg struct {
	candidate domain.LabSettingsFile
	err       error
}

type dashboardSettingsApplyMsg struct {
	report    domain.ConfigApplyReport
	status    domain.StatusReport
	statusErr error
}

func RunDashboard(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions) error {
	_, err := tea.NewProgram(newDashboardModel(report, setup, actions, false)).Run()
	return err
}

func RunSetupDashboard(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions) error {
	_, err := tea.NewProgram(newDashboardModel(report, setup, actions, true)).Run()
	return err
}

func newDashboardModel(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions, setupMode bool) dashboardModel {
	screen := dashboardHome
	if setupMode {
		screen = dashboardSetup
	}
	return dashboardModel{
		report: report, setup: setup, setupMode: setupMode, screen: screen, actions: actions,
		homeMenu: newDashboardTaskMenu(false, 80, 24),
	}
}

func (dashboardModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (model dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardSetupMsg:
		model.busy = ""
		model.setup = message.report
		model.screen = dashboardSetup
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
	case dashboardHostsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Computer status refresh failed: " + message.err.Error()
		} else {
			model.hosts = message.report
			model.message = ""
		}
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
		model.busy = ""
		model.deploying = false
		model.deployResult = message.report
		model.message = message.report.Message
		model.screen = dashboardDeploy
		return model, nil
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
		model.busy = ""
		model.gitCommitResult = message.report
		model.gitReview = message.review
		model.gitScroll = 0
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardGitReview
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
		model.busy = ""
		model.updating = false
		model.updateResult = message.report
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardUpdate
		return model, nil
	case dashboardSettingsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Settings could not be loaded: " + message.err.Error()
			model.screen = dashboardHome
			return model, nil
		}
		model.settings = message.settings
		model.settingsMenu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		model.message = ""
		model.screen = dashboardSettings
		return model, nil
	case dashboardSettingsPlanMsg:
		model.busy = ""
		model.settingsEditor = settingsWizardModel{}
		model.settingsPlan = message.report
		if message.report.HasErrors() {
			model.message = "Candidate validation failed: " + settingsIssueMessage(message.report.Issues)
			model.screen = dashboardSettings
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
			model.screen = dashboardSettingsPasswords
			return model, nil
		}
		model.settingsCandidate = message.candidate
		model.busy = "Validating the password change through Nix"
		model.message = ""
		candidate := model.settingsCandidate
		return model, func() tea.Msg {
			return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(candidate)}
		}
	case dashboardSettingsApplyMsg:
		model.busy = ""
		model.settingsApplying = false
		model.settingsResult = message.report
		if !message.report.HasErrors() && message.report.State == "applied" {
			model.settings = model.settingsCandidate
			model.message = "Settings applied. Review and commit lab-settings.json, then rebuild or deploy affected machines."
		} else if message.report.State == "unchanged" {
			model.message = "No managed settings changed."
		} else {
			model.message = "Settings apply failed: " + settingsIssueMessage(message.report.Issues)
		}
		if message.statusErr != nil {
			model.message += " Status refresh failed: " + message.statusErr.Error()
		} else {
			model.report = message.status
		}
		model.screen = dashboardSettings
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
	}

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
	if (key.String() == "ctrl+c" || key.String() == "q") && (model.deploying || model.updating || model.settingsApplying) {
		model.message = "A mutating operation is running; wait for its result before closing Nixorium."
		return model, nil
	}
	if key.String() == "ctrl+c" || (key.String() == "q" && model.screen != dashboardUpdate && model.screen != dashboardUpdateReview && model.screen != dashboardSettingsEdit) {
		return model, tea.Quit
	}
	if model.busy != "" {
		return model, nil
	}

	switch model.screen {
	case dashboardHome:
		model.ensureHomeMenu()
		action := key.String()
		if action == "enter" {
			if model.setup.State != "" && model.setup.State != "ready" {
				model.setupMode = true
				model.screen = dashboardSetup
				model.message = ""
				return model, nil
			}
			if selected, ok := model.homeMenu.selected(); ok {
				action = selected.shortcut
			}
		}
		switch action {
		case "d":
			model.screen = dashboardDeploy
			model.message = ""
			model.deployChosen = map[string]bool{}
			model.deployCursor = 0
		case "c":
			model.screen = dashboardController
			model.busy = "Reviewing controller revision and active system"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardControllerPlanMsg{report: model.actions.PlanController()}
			}
		case "h":
			model.screen = dashboardHosts
			model.busy = "Checking configured computers"
			model.message = ""
			return model, model.loadHosts()
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
			model.screen = dashboardUpdate
			model.updateTarget = ""
			model.updatePrerelease = false
			model.updateDowngrade = false
			model.message = ""
		case "e":
			model.screen = dashboardSettings
			model.busy = "Loading managed laboratory settings"
			model.message = ""
			return model, func() tea.Msg {
				settings, err := model.actions.LoadSettings()
				return dashboardSettingsMsg{settings: settings, err: err}
			}
		case "p":
			model.screen = dashboardPXE
			model.message = ""
		default:
			var command tea.Cmd
			model.homeMenu, command = model.homeMenu.update(key)
			return model, command
		}
	case dashboardSetup:
		if key.String() == "esc" || key.String() == "left" {
			model.setupMode = false
			model.screen = dashboardHome
			model.message = ""
			return model, nil
		}
		if key.String() != "enter" {
			return model, nil
		}
		switch model.setup.CurrentStage {
		case domain.SetupStageReview:
			model.screen = dashboardGitReview
			model.busy = "Reviewing generated setup changes"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
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
		case domain.SetupStageReadiness, domain.SetupStageInstall:
			model.screen = dashboardPXE
			model.message = "Continue by starting installation mode and booting the first computer from the network."
			return model, nil
		case "":
			model.screen = dashboardPXE
			model.message = "Start installation mode, then boot the first computer from the network."
			return model, nil
		default:
			model.message = "Configuration input is still required; exit and run `nixorium setup` again."
			return model, nil
		}
	case dashboardSettings:
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
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
		default:
			model.settingsMenu, _ = model.settingsMenu.update(key)
		}
	case dashboardSettingsEdit:
		updated, command := model.settingsEditor.Update(key)
		model.settingsEditor = updated.(settingsWizardModel)
		if model.settingsEditor.cancelled {
			model.settingsEditor = settingsWizardModel{}
			model.message = "Settings edit cancelled; no file changed."
			model.screen = dashboardSettings
			return model, nil
		}
		if model.settingsEditor.accepted {
			model.settingsCandidate = model.settingsEditor.settings
			model.busy = "Validating the complete settings candidate through Nix"
			model.message = ""
			candidate := model.settingsCandidate
			return model, func() tea.Msg {
				return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(candidate)}
			}
		}
		return model, command
	case dashboardSettingsPasswords:
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
		case "y":
			model.busy = "Applying the reviewed managed settings"
			model.settingsApplying = true
			model.message = ""
			candidate := model.settingsCandidate
			plan := model.settingsPlan
			return model, func() tea.Msg {
				report := model.actions.ApplySettings(candidate, plan)
				status, err := model.actions.Refresh()
				return dashboardSettingsApplyMsg{report: report, status: status, statusErr: err}
			}
		}
	case dashboardHosts:
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "r":
			model.busy = "Refreshing computer status"
			model.message = ""
			return model, model.loadHosts()
		}
	case dashboardDeploy:
		hosts := model.report.Meta.Clients.Hosts
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
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
			expected := "DEPLOY " + model.deployPlan.ColmenaSelector
			if model.confirmation != expected {
				model.confirmation = ""
				model.message = "Confirmation did not match; no build or apply was started."
				return model, nil
			}
			model.busy = "Building and applying the reviewed deployment"
			model.deploying = true
			model.confirmation = ""
			model.message = ""
			plan := model.deployPlan
			return model, func() tea.Msg {
				return dashboardDeploymentResultMsg{report: model.actions.ApplyDeployment(plan)}
			}
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
			model.confirmation = ""
			model.message = "Controller rebuild cancelled; no action was started."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != model.controllerPlan.Confirmation {
				model.confirmation = ""
				model.message = "Confirmation did not match; no controller action was started."
				return model, nil
			}
			model.busy = "Building and activating the reviewed controller revision"
			model.controllerApplying = true
			model.controllerProgress = domain.OperationProgress{}
			model.controllerStarted = time.Now().UTC()
			model.controllerProgressID++
			model.confirmation = ""
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
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardServices:
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
			if model.confirmation != "RESTART CACHE" {
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
	case dashboardGitReview:
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
		switch key.String() {
		case "esc":
			model.screen = dashboardHome
			model.message = ""
		case "f2":
			model.updatePrerelease = !model.updatePrerelease
			model.message = ""
		case "f3":
			model.updateDowngrade = !model.updateDowngrade
			model.message = ""
		case "backspace":
			value := []rune(model.updateTarget)
			if len(value) > 0 {
				model.updateTarget = string(value[:len(value)-1])
			}
		case "enter":
			target := strings.TrimSpace(model.updateTarget)
			if target == "" {
				model.message = "Enter an explicit release tag before creating an update plan."
				return model, nil
			}
			model.busy = "Validating the candidate release and representative builds"
			model.message = ""
			allowPrerelease := model.updatePrerelease
			allowDowngrade := model.updateDowngrade
			return model, func() tea.Msg {
				return dashboardUpdatePlanMsg{report: model.actions.PlanUpdate(target, allowPrerelease, allowDowngrade)}
			}
		default:
			if key.Text != "" && len([]rune(model.updateTarget))+len([]rune(key.Text)) <= 128 {
				model.updateTarget += key.Text
			}
		}
	case dashboardUpdateReview:
		maximum := maximumUpdateScroll(model.updatePlan, model.updateReviewHeight())
		switch key.String() {
		case "esc":
			model.screen = dashboardUpdate
			model.confirmation = ""
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
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != model.updatePlan.Confirmation {
				model.confirmation = ""
				model.message = "Confirmation did not match; flake.nix and flake.lock were not changed."
				return model, nil
			}
			model.busy = "Writing the validated Nixorium release update"
			model.updating = true
			model.confirmation = ""
			model.message = ""
			plan := model.updatePlan
			return model, func() tea.Msg {
				return dashboardUpdateResultMsg{report: model.actions.ApplyUpdate(plan)}
			}
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardPXE:
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
			model.busy = "Recovering normal controller networking"
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.RecoverPXE().Message
			}, dashboardPXE)
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
			if model.confirmation != "START PXE" {
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
	}
	return model, nil
}

func (model dashboardModel) runAction(operation func() string, screen dashboardScreen) tea.Cmd {
	return func() tea.Msg {
		message := operation()
		report, err := model.actions.Refresh()
		return dashboardOperationMsg{message: message, report: report, err: err, screen: screen}
	}
}

func (model dashboardModel) loadHosts() tea.Cmd {
	return func() tea.Msg {
		report, err := model.actions.LoadHosts()
		return dashboardHostsMsg{report: report, err: err}
	}
}

func (model *dashboardModel) ensureHomeMenu() {
	if len(model.homeMenu.list.Items()) == 0 {
		model.homeMenu = newDashboardTaskMenu(model.isDark, model.width, model.height)
	}
}

func (model dashboardModel) loadSetup() tea.Cmd {
	return func() tea.Msg {
		return dashboardSetupMsg{report: model.actions.LoadSetup()}
	}
}

func schedulePXEProgressTick(id uint64) tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return dashboardPXEProgressTickMsg{id: id}
	})
}

func (model dashboardModel) loadPXEProgress(id uint64) tea.Cmd {
	return func() tea.Msg {
		progress, err := model.actions.LoadPXEProgress()
		return dashboardPXEProgressMsg{id: id, progress: progress, err: err}
	}
}

func scheduleControllerProgressTick(id uint64) tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return dashboardControllerProgressTickMsg{id: id}
	})
}

func (model dashboardModel) loadControllerProgress(id uint64) tea.Cmd {
	return func() tea.Msg {
		progress, err := model.actions.LoadControllerProgress()
		return dashboardControllerProgressMsg{id: id, progress: progress, err: err}
	}
}

func (model dashboardModel) View() tea.View {
	content := ""
	switch model.screen {
	case dashboardSetup:
		content = model.setupView()
	case dashboardHosts:
		content = model.hostsView()
	case dashboardDeploy, dashboardDeployReview:
		content = model.deployView()
	case dashboardController, dashboardControllerReview:
		content = model.controllerView()
	case dashboardServices, dashboardServicesRestartReview:
		content = model.servicesView()
	case dashboardLogs:
		content = model.logsView()
	case dashboardLogDetail:
		content = model.logDetailView()
	case dashboardGitReview, dashboardGitCommitSelect, dashboardGitCommitReview:
		content = model.gitReviewView()
	case dashboardUpdate, dashboardUpdateReview:
		content = model.updateView()
	case dashboardSettings, dashboardSettingsEdit, dashboardSettingsPasswords, dashboardSettingsReview:
		content = model.settingsView()
	case dashboardPXE, dashboardPXEStartReview:
		content = model.pxeView()
	default:
		content = model.homeView()
	}
	return tea.NewView(content)
}

func (model dashboardModel) setupView() string {
	completed := 0
	for _, stage := range model.setup.Stages {
		if stage.State == domain.SetupStageComplete {
			completed++
		}
	}
	lines := []string{
		tuiTitle("Nixorium — First setup", model.isDark),
		"",
		fmt.Sprintf("Laboratory setup: %d/%d steps complete", completed, len(model.setup.Stages)),
		"You can quit safely and resume later with `nixorium setup`.",
		"",
	}
	for _, stage := range model.setup.Stages {
		marker := "[ ]"
		if stage.State == domain.SetupStageComplete {
			marker = "[x]"
		} else if stage.State == domain.SetupStageCurrent {
			marker = "[>]"
		}
		lines = append(lines, fmt.Sprintf("  %s %s", marker, stage.Title))
	}
	lines = append(lines, "")
	if model.setup.State == "ready" {
		lines = append(lines,
			tuiResult("Laboratory setup is ready", true, model.isDark),
			"Enter opens network installation for the first computer.",
		)
	} else {
		lines = append(lines,
			tuiResult("Next step", false, model.isDark),
			"  "+setupCurrentTitle(model.setup),
			"  "+setupCurrentAction(model.setup),
		)
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"enter"}, "enter", "continue"),
		tuiHelpBinding([]string{"esc"}, "esc", "dashboard"),
		tuiHelpBinding([]string{"q"}, "q", "quit"),
	))
	return strings.Join(lines, "\n") + "\n"
}

func setupCurrentTitle(report domain.SetupReport) string {
	for _, stage := range report.Stages {
		if stage.State == domain.SetupStageCurrent {
			return stage.Title
		}
	}
	return "Setup is complete"
}

func setupCurrentAction(report domain.SetupReport) string {
	switch report.CurrentStage {
	case domain.SetupStageReview:
		return "Review and commit the generated configuration"
	case domain.SetupStageApply:
		return "Review and activate the controller configuration"
	case domain.SetupStageArtifacts:
		return "Prepare installation files and client systems"
	case domain.SetupStageReadiness, domain.SetupStageInstall:
		return "Open network installation and install the first computer"
	default:
		return "Continue the guided laboratory configuration"
	}
}

func (model dashboardModel) gitReviewView() string {
	if model.screen == dashboardGitCommitSelect {
		return model.gitCommitSelectView()
	}
	if model.screen == dashboardGitCommitReview {
		return model.gitCommitReviewView()
	}
	lines := []string{"Nixorium — Git change review", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		return strings.Join(lines, "\n") + "\n"
	}
	content := gitReviewContentLines(model.gitReview)
	height := model.gitReviewHeight()
	maximum := maximumGitReviewScroll(model.gitReview, height)
	if model.gitScroll > maximum {
		model.gitScroll = maximum
	}
	end := model.gitScroll + height
	if end > len(content) {
		end = len(content)
	}
	lines = append(lines,
		fmt.Sprintf("State: %s   paths: %d   staged/unstaged/untracked: %d/%d/%d", model.gitReview.State, len(model.gitReview.Changes), model.gitReview.Summary.Staged, model.gitReview.Summary.Unstaged, model.gitReview.Summary.Untracked),
		fmt.Sprintf("Managed/unexpected/private: %d/%d/%d", model.gitReview.Summary.Managed, model.gitReview.Summary.Unexpected, model.gitReview.Summary.Private),
		fmt.Sprintf("Showing lines %d-%d of %d", displayedLineStart(model.gitScroll, len(content)), end, len(content)),
		"",
	)
	lines = append(lines, content[model.gitScroll:end]...)
	lines = append(lines, "", "c: select paths to commit   Up/Down/PgUp/PgDn/Home/End: scroll", "f: refresh   Esc: back   q: quit")
	if model.gitCommitResult.Operation != "" {
		lines = append(lines, "", fmt.Sprintf("Last commit: %s; committed=%t; HEAD=%s", model.gitCommitResult.State, model.gitCommitResult.Committed, model.gitCommitResult.Revision))
	}
	if model.message != "" {
		lines = append(lines, "", "Warning: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) gitCommitSelectView() string {
	lines := []string{"Nixorium — Select Git commit paths", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		return strings.Join(lines, "\n") + "\n"
	}
	for index, change := range model.gitReview.Changes {
		cursor := " "
		if index == model.gitCommitCursor {
			cursor = ">"
		}
		chosen := "[ ]"
		if model.gitCommitChosen[change.Path] {
			chosen = "[x]"
		}
		if change.Private {
			chosen = "[!]"
		}
		lines = append(lines, fmt.Sprintf("%s %s %-10s %-10s %-9s %s", cursor, chosen, gitChangeOwnership(change), gitChangeIndex(change), gitChangeWorktree(change), change.Path))
	}
	lines = append(lines, "", "Space: select   a: toggle all safe paths   Enter: create plan", "Esc: back   q: quit")
	if model.message != "" {
		lines = append(lines, "", model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) gitCommitReviewView() string {
	lines := []string{"Nixorium — Local Git commit review", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		return strings.Join(lines, "\n") + "\n"
	}
	diffLines := strings.Split(strings.TrimSuffix(model.gitCommitPlan.Diff.Content, "\n"), "\n")
	height := model.gitReviewHeight()
	maximum := maximumGitCommitPlanScroll(model.gitCommitPlan, height)
	if model.gitScroll > maximum {
		model.gitScroll = maximum
	}
	end := model.gitScroll + height
	if end > len(diffLines) {
		end = len(diffLines)
	}
	lines = append(lines,
		"Paths: "+strings.Join(model.gitCommitPlan.Paths, ", "),
		"Message: "+model.gitCommitPlan.CommitMessage,
		"No hooks, signing actions, remote operations, or push will run.",
		fmt.Sprintf("Diff lines %d-%d of %d", displayedLineStart(model.gitScroll, len(diffLines)), end, len(diffLines)),
		"",
	)
	lines = append(lines, diffLines[model.gitScroll:end]...)
	lines = append(lines, "", "Type "+model.gitCommitPlan.Confirmation+" to continue:", "> "+model.confirmation+"█", "", "Up/Down/PgUp/PgDn: scroll   Esc: cancel")
	if model.message != "" {
		lines = append(lines, "", model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func selectedGitCommitPaths(changes []domain.GitChange, chosen map[string]bool) string {
	paths := []string{}
	for _, change := range changes {
		if chosen[change.Path] && !change.Private {
			paths = append(paths, change.Path)
		}
	}
	return strings.Join(paths, ",")
}

func toggleAllGitCommitPaths(changes []domain.GitChange, chosen map[string]bool) map[string]bool {
	all := true
	for _, change := range changes {
		if !change.Private && !chosen[change.Path] {
			all = false
		}
	}
	result := map[string]bool{}
	if !all {
		for _, change := range changes {
			if !change.Private {
				result[change.Path] = true
			}
		}
	}
	return result
}

func maximumGitCommitPlanScroll(report domain.GitCommitPlanReport, height int) int {
	maximum := len(strings.Split(strings.TrimSuffix(report.Diff.Content, "\n"), "\n")) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func (model dashboardModel) updateView() string {
	lines := []string{"Nixorium — Update Nixorium", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		if model.updating {
			lines = append(lines, "", "Wait for the atomic two-file result before closing Nixorium.")
		} else {
			lines = append(lines, "", "Candidate evaluation and builds do not modify the deployment.")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardUpdateReview {
		diffLines := strings.Split(strings.TrimSuffix(model.updatePlan.Diff.Content, "\n"), "\n")
		height := model.updateReviewHeight()
		maximum := maximumUpdateScroll(model.updatePlan, height)
		if model.updateScroll > maximum {
			model.updateScroll = maximum
		}
		end := model.updateScroll + height
		if end > len(diffLines) {
			end = len(diffLines)
		}
		lines = append(lines,
			"Validated release review",
			fmt.Sprintf("  Current: %s (%s)", model.updatePlan.CurrentRef, model.updatePlan.CurrentChannel),
			fmt.Sprintf("  Target:  %s (%s)", model.updatePlan.Target, model.updatePlan.TargetChannel),
			fmt.Sprintf("  Deployment revision: %s", model.updatePlan.Revision),
			fmt.Sprintf("  Downgrade: %t", model.updatePlan.Downgrade),
			"  Scope: flake.nix and flake.lock only",
			"  No commit, push, activation, PXE action, or client deployment is implicit",
			"",
			"Candidate checks:",
		)
		for _, check := range model.updatePlan.Checks {
			lines = append(lines, fmt.Sprintf("  %-18s %-8s %s", check.ID, check.State, check.Message))
		}
		lines = append(lines,
			"",
			fmt.Sprintf("Diff lines %d-%d of %d", displayedLineStart(model.updateScroll, len(diffLines)), end, len(diffLines)),
			"",
		)
		lines = append(lines, diffLines[model.updateScroll:end]...)
		lines = append(lines,
			"",
			"Type "+model.updatePlan.Confirmation+" to continue:",
			"> "+model.confirmation+"█",
			"",
			"Up/Down/PgUp/PgDn: scroll   Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}

	lines = append(lines,
		"Enter one explicit v-prefixed Semantic Version release tag.",
		"Target: > "+model.updateTarget+"█",
		"",
		fmt.Sprintf("F2  Allow prerelease: %s", toggleLabel(model.updatePrerelease)),
		fmt.Sprintf("F3  Allow downgrade:  %s", toggleLabel(model.updateDowngrade)),
		"",
		"Enter: validate release and build representative outputs",
		"Esc: back",
		"No file changes occur until the reviewed confirmation succeeds.",
	)
	if model.updateResult.Operation != "" {
		lines = append(lines,
			"",
			fmt.Sprintf("Last result: %s; files updated=%t; retry safe=%t", model.updateResult.State, model.updateResult.Updated, model.updateResult.RetrySafe),
		)
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) updateReviewHeight() int {
	if model.height <= 0 {
		return 10
	}
	height := model.height - 26
	if height < 4 {
		return 4
	}
	return height
}

func maximumUpdateScroll(report domain.UpdatePlanReport, height int) int {
	maximum := len(strings.Split(strings.TrimSuffix(report.Diff.Content, "\n"), "\n")) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func toggleLabel(enabled bool) string {
	if enabled {
		return "[x]"
	}
	return "[ ]"
}

func gitReviewContentLines(report domain.GitReviewReport) []string {
	lines := []string{}
	if len(report.Changes) == 0 {
		lines = append(lines, "The deployment worktree is clean.")
	}
	for _, change := range report.Changes {
		lines = append(lines, fmt.Sprintf("%-10s %-10s %-9s %s", gitChangeOwnership(change), gitChangeIndex(change), gitChangeWorktree(change), change.Path))
		if change.OriginalPath != "" {
			lines = append(lines, "  from "+change.OriginalPath)
		}
	}
	for _, diff := range report.Diffs {
		lines = append(lines, "", gitScopeTitle(diff.Scope)+" diff"+truncatedGitDiffLabel(diff.Truncated)+":")
		lines = append(lines, strings.Split(strings.TrimSuffix(diff.Content, "\n"), "\n")...)
	}
	if report.Summary.Untracked > 0 {
		lines = append(lines, "", "Untracked file contents are not opened automatically.")
	}
	for _, issue := range report.Issues {
		lines = append(lines, "", "BLOCKED: "+issue.Field+": "+issue.Message)
	}
	return lines
}

func (model dashboardModel) gitReviewHeight() int {
	if model.height <= 0 {
		return 14
	}
	height := model.height - 9
	if height < 4 {
		return 4
	}
	return height
}

func maximumGitReviewScroll(report domain.GitReviewReport, height int) int {
	maximum := len(gitReviewContentLines(report)) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func (model dashboardModel) controllerView() string {
	lines := []string{tuiTitle("Nixorium — Rebuild controller", model.isDark), ""}
	if model.controllerApplying {
		elapsed := time.Since(model.controllerStarted).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines = append(lines, fmt.Sprintf("%s…  elapsed %s", model.busy, elapsed))
		lines = append(lines, model.operationProgressView(model.controllerProgress, "Current progress")...)
		lines = append(lines, "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.busy != "" {
		lines = append(lines, model.busy+"…", "", "This systemd-owned action continues if the dashboard closes.")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardControllerReview {
		lines = append(lines,
			"Controller rebuild review",
			fmt.Sprintf("  Machine:  %s", model.controllerPlan.Controller),
			fmt.Sprintf("  Revision: %s", model.controllerPlan.Revision),
			fmt.Sprintf("  Already current: %t", model.controllerPlan.Current),
			"  Build as the deployment owner; activate only the resulting closure",
			"  Services and networking may restart",
			"",
			fmt.Sprintf("Type %s to continue:", model.controllerPlan.Confirmation),
			"> "+model.confirmation+"█",
			"",
			"Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	if model.controllerResult.Operation != "" {
		resultTitle := "Controller action needs attention"
		if !model.controllerResult.HasErrors() && model.controllerResult.Applied && model.controllerResult.Verified {
			resultTitle = "Controller updated and verified"
		}
		lines = append(lines,
			tuiResult(resultTitle, !model.controllerResult.HasErrors(), model.isDark),
			"",
			fmt.Sprintf("Last result: %s at phase %s", model.controllerResult.State, model.controllerResult.Phase),
			fmt.Sprintf("Applied: %t   Verified: %t", model.controllerResult.Applied, model.controllerResult.Verified),
		)
		if model.controllerDetails && model.controllerProgress.Operation != "" {
			lines = append(lines, "")
			lines = append(lines, model.operationProgressView(model.controllerProgress, "Last controller apply")...)
		}
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		detailsLabel := "show details"
		if model.controllerDetails {
			detailsLabel = "hide details"
		}
		returnLabel := "dashboard"
		if model.setupMode {
			returnLabel = "setup"
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"enter"}, "enter", returnLabel),
			tuiHelpBinding([]string{"d"}, "d", detailsLabel),
			tuiHelpBinding([]string{"l"}, "l", "logs"),
			tuiHelpBinding([]string{"r"}, "r", "new review"),
		))
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines,
		"Review the current committed controller configuration before rebuilding.",
		"",
		tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"r"}, "r", "create review"),
			tuiHelpBinding([]string{"esc"}, "esc", "back"),
			tuiHelpBinding([]string{"q"}, "q", "quit"),
		),
	)
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) servicesView() string {
	lines := []string{"Nixorium — Managed services", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardServicesRestartReview {
		lines = append(lines,
			"Binary cache restart review",
			"  The signed cache will be briefly unavailable",
			"  Active PXE clients may retry downloads",
			"  PXE networking and listeners are not controlled by this action",
			"",
			"Type RESTART CACHE to continue:",
			"> "+model.confirmation+"█",
			"",
			"Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	for _, service := range model.services.Services {
		lines = append(lines,
			fmt.Sprintf("%s — %s", service.Name, service.State),
			"  "+service.Detail,
		)
		for _, unit := range service.Units {
			lines = append(lines, fmt.Sprintf("  %-32s %s", unit.Name, unit.State))
		}
		if service.ID == "pxe" {
			lines = append(lines, "  Managed through the Install computers workflow")
		}
		lines = append(lines, "")
	}
	lines = append(lines, "r: restart cache   f: refresh   Esc: back   q: quit")
	if model.serviceResult.Operation != "" {
		lines = append(lines, "", fmt.Sprintf("Last action: %s; verified=%t", model.serviceResult.State, model.serviceResult.Verified))
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) logsView() string {
	lines := []string{"Nixorium — Operation logs", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, "Recent actions")
	recordLimit := len(model.logs.Records)
	if recordLimit > 10 {
		recordLimit = 10
	}
	if recordLimit == 0 {
		lines = append(lines, "  No recorded operation outcomes.")
	}
	for _, record := range model.logs.Records[:recordLimit] {
		lines = append(lines, fmt.Sprintf("  %s  %-18s %-10s %s", record.RecordedAt.UTC().Format("2006-01-02 15:04Z"), record.Operation, record.State, record.Subject))
	}
	lines = append(lines, "", "Deployment logs")
	if len(model.logs.Logs) == 0 {
		lines = append(lines, "No deployment operation logs are available.")
	}
	for index, entry := range model.logs.Logs {
		cursor := " "
		if index == model.logCursor {
			cursor = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s  %-10s %-11s %d bytes", cursor, entry.StartedAt.UTC().Format("2006-01-02 15:04Z"), entry.Kind, entry.State, entry.SizeBytes))
		lines = append(lines, "    "+entry.ID)
	}
	lines = append(lines, "", "Up/Down: select   Enter: view tail   f: refresh   Esc: back   q: quit")
	if model.message != "" {
		lines = append(lines, "", "Warning: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) logDetailView() string {
	lines := []string{"Nixorium — Operation log detail", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.logDetail.Log == nil {
		lines = append(lines, "The selected operation log could not be read safely.")
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		lines = append(lines, "", "Esc: back   q: quit")
		return strings.Join(lines, "\n") + "\n"
	}
	entry := model.logDetail.Log
	contentLines := operationLogContentLines(model.logDetail.Content)
	end := model.logScroll + model.logDetailHeight()
	if end > len(contentLines) {
		end = len(contentLines)
	}
	lines = append(lines,
		fmt.Sprintf("%s — %s", entry.StartedAt.UTC().Format("2006-01-02 15:04:05Z"), entry.State),
		entry.ID,
		fmt.Sprintf("Showing lines %d-%d of %d%s", displayedLineStart(model.logScroll, len(contentLines)), end, len(contentLines), truncatedLogLabel(model.logDetail.Truncated)),
		"",
	)
	lines = append(lines, contentLines[model.logScroll:end]...)
	lines = append(lines, "", "Up/Down/PgUp/PgDn/Home/End: scroll   Esc: back   q: quit")
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) logDetailHeight() int {
	if model.height <= 0 {
		return 12
	}
	height := model.height - 9
	if height < 4 {
		return 4
	}
	return height
}

func maximumLogScroll(report domain.OperationLogReport, height int) int {
	maximum := len(operationLogContentLines(report.Content)) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func operationLogContentLines(content string) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []string{"(empty log)"}
	}
	return strings.Split(content, "\n")
}

func operationLogIssues(issues []domain.ValidationIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Field+": "+issue.Message)
	}
	return strings.Join(parts, "; ")
}

func displayedLineStart(offset, total int) int {
	if total == 0 {
		return 0
	}
	return offset + 1
}

func truncatedLogLabel(truncated bool) string {
	if truncated {
		return " (bounded tail; earlier bytes omitted)"
	}
	return ""
}

func controllerPlanIssues(report domain.ControllerRebuildPlanReport) string {
	parts := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		parts = append(parts, issue.Field+": "+issue.Message)
	}
	return strings.Join(parts, "; ")
}

func (model dashboardModel) deployView() string {
	lines := []string{"Nixorium — Deploy updates", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		if model.deploying {
			lines = append(lines, "", "Nixorium will show the durable log and result when Colmena exits.", "Closing is disabled while this deployment is running.")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardDeployReview {
		lines = append(lines,
			"Deployment review",
			fmt.Sprintf("  Revision: %s", model.deployPlan.Revision),
			fmt.Sprintf("  Targets:  %s (%d computer(s))", model.deployPlan.ColmenaSelector, len(model.deployPlan.Targets)),
			"  Build every selected configuration before applying it",
			"  Target services may restart; offline computers will fail explicitly",
			"  A failed apply may leave mixed target state; a fresh full retry is safe",
			"",
			fmt.Sprintf("Type DEPLOY %s to continue:", model.deployPlan.ColmenaSelector),
			"> "+model.confirmation+"█",
			"",
			"Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}

	hosts := model.report.Meta.Clients.Hosts
	lines = append(lines, "Select computers (Space toggles; a selects all):", "")
	for index, host := range hosts {
		cursor := " "
		if index == model.deployCursor {
			cursor = ">"
		}
		checked := " "
		if model.deployChosen[host.Name] {
			checked = "x"
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %-10s %s", cursor, checked, host.Name, host.IP))
	}
	if len(hosts) == 0 {
		lines = append(lines, "No configured client computers.")
	}
	lines = append(lines, "", "Enter: review selected targets   Esc: back   q: quit")
	if model.deployResult.Operation != "" {
		lines = append(lines,
			"",
			fmt.Sprintf("Last result: %s at phase %s", model.deployResult.State, model.deployResult.Phase),
			fmt.Sprintf("Build complete: %t   Apply complete: %t", model.deployResult.BuildCompleted, model.deployResult.ApplyCompleted),
		)
		if model.deployResult.Verification.Attempted > 0 {
			lines = append(lines, fmt.Sprintf("Authenticated: %d/%d   Recorded: %d", model.deployResult.Verification.Verified, model.deployResult.Verification.Attempted, model.deployResult.Verification.Recorded))
		}
		if model.deployResult.LogPath != "" {
			lines = append(lines, "Detailed log: "+model.deployResult.LogPath)
		}
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) hostsView() string {
	available, total := hostAvailability(model.hosts.Hosts)
	lines := []string{
		"Nixorium — Computers",
		"",
		fmt.Sprintf("SSH available: %d/%d", available, total),
		fmt.Sprintf("Deployment: %d current, %d outdated, %d unknown", model.hosts.Deployment.Current, model.hosts.Deployment.Outdated, model.hosts.Deployment.Unknown),
		"",
		fmt.Sprintf("  %-10s %-15s %-12s %-11s %-9s %-17s", "NAME", "ADDRESS", "NETWORK", "SSH", "DEPLOY", "LAST VERIFIED"),
	}
	for _, host := range model.hosts.Hosts {
		lastVerified := "never"
		if host.LastSuccessfulDeploy != nil {
			lastVerified = host.LastSuccessfulDeploy.VerifiedAt.UTC().Format("2006-01-02 15:04Z")
		}
		lines = append(lines, fmt.Sprintf("  %-10s %-15s %-12s %-11s %-9s %-17s", host.Name, host.IP, host.Reachability, host.SSH, host.Deployment, lastVerified))
	}
	if model.hosts.HistoryDetail != "" {
		lines = append(lines, "", "History warning: "+model.hosts.HistoryDetail)
	}
	if model.busy != "" {
		lines = append(lines, "", model.busy+"…")
	} else {
		lines = append(lines, "", "r: refresh   Esc: back   q: quit")
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) pxeView() string {
	preparation := "missing"
	if model.report.PXEPreparation.Present {
		preparation = "stale"
	}
	if model.report.PXEPreparation.Ready {
		preparation = "ready"
	}
	lines := []string{
		"Nixorium — Install computers over network",
		"",
		fmt.Sprintf("Installation mode:  %s", model.report.PXE.Mode),
		fmt.Sprintf("Prepared artifacts: %s", preparation),
		fmt.Sprintf("Interface:          %s", model.report.Meta.Network.Interface),
		fmt.Sprintf("Service address:    %s", model.report.Meta.Controller.DHCPIP),
	}
	if model.pxePreparing {
		elapsed := time.Since(model.pxeProgressStarted).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines = append(lines, "", fmt.Sprintf("%s…  elapsed %s", model.busy, elapsed))
		lines = append(lines, model.operationProgressView(model.pxeProgress, "Current progress")...)
		lines = append(lines, "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.busy != "" {
		lines = append(lines, "", model.busy+"…", "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardPXEStartReview {
		lines = append(lines,
			"",
			"Start review",
			fmt.Sprintf("  Temporarily remove %s", model.startPlan.StaticCIDR),
			fmt.Sprintf("  Serve ProxyDHCP, TFTP, HTTP, and cache via %s", model.startPlan.DHCPAddress),
			"  Institutional DHCP remains authoritative",
			"  `nixorium pxe stop` or reboot recovery restores normal addressing",
			"",
			"Type START PXE to continue:",
			"> "+model.confirmation+"█",
			"",
			"Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, "", "Actions")
	if model.report.PXE.Mode != "active" {
		lines = append(lines, "  p   Prepare artifacts and client closures", "  s   Start installation mode")
	}
	if model.report.PXE.Mode == "active" || model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required" {
		lines = append(lines, "  x   Stop and restore normal networking")
	}
	lines = append(lines, "  r   Recover normal networking")
	if model.pxeProgress.Operation != "" {
		lines = append(lines, "")
		lines = append(lines, model.operationProgressView(model.pxeProgress, "Last preparation")...)
	}
	lines = append(lines, "", "  Esc Back", "  q   Quit (active services keep running)")
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) operationProgressView(operation domain.OperationProgress, title string) []string {
	if operation.Operation == "" {
		return []string{"  Waiting for managed progress…"}
	}
	phaseLabels := map[string]string{
		"starting":  "Starting",
		"validate":  "Validating configuration",
		"network":   "Checking network and cache",
		"artifacts": "Building netboot artifacts",
		"clients":   "Building client systems",
		"publish":   "Publishing preparation",
		"complete":  "Complete",
		"build":     "Building system",
		"activate":  "Activating system",
		"verify":    "Verifying activation",
	}
	lines := []string{title, "  Phase: " + phaseLabels[operation.Phase]}
	if operation.Total > 0 {
		barWidth := model.width - 8
		if barWidth < 24 {
			barWidth = 24
		}
		if barWidth > 64 {
			barWidth = 64
		}
		bar := progress.New(progress.WithDefaultBlend(), progress.WithWidth(barWidth))
		percentage := float64(operation.Current) / float64(operation.Total)
		lines = append(lines,
			fmt.Sprintf("  %d/%d", operation.Current, operation.Total),
			"  "+bar.ViewAs(percentage),
		)
	}
	if len(operation.Recent) > 0 {
		lines = append(lines, "  Recent activity:")
		for _, activity := range operation.Recent {
			lines = append(lines, "    • "+activity)
		}
	}
	return lines
}

func serviceLabel(services []domain.ServiceState, name string) string {
	for _, service := range services {
		if service.Name == name {
			return service.State
		}
	}
	return "unknown"
}

func hostAvailability(hosts []domain.HostStatus) (int, int) {
	available := 0
	for _, host := range hosts {
		if host.SSH == domain.SSHAvailable {
			available++
		}
	}
	return available, len(hosts)
}

func selectedDeploymentTargets(hosts []domain.HostMeta, chosen map[string]bool) string {
	selected := []string{}
	for _, host := range hosts {
		if chosen[host.Name] {
			selected = append(selected, host.Name)
		}
	}
	if len(selected) == len(hosts) && len(hosts) > 0 {
		return "@lab"
	}
	return strings.Join(selected, ",")
}

func toggleAllDeploymentTargets(hosts []domain.HostMeta, chosen map[string]bool) map[string]bool {
	allSelected := len(hosts) > 0
	for _, host := range hosts {
		if !chosen[host.Name] {
			allSelected = false
			break
		}
	}
	result := map[string]bool{}
	if !allSelected {
		for _, host := range hosts {
			result[host.Name] = true
		}
	}
	return result
}

func deploymentPlanIssues(report domain.DeploymentPlanReport) string {
	issues := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		issues = append(issues, issue.Field+": "+issue.Message)
	}
	return strings.Join(issues, "; ")
}
