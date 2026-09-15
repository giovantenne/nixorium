package presentation

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type DashboardActions struct {
	LoadDoctor             func() (domain.DoctorReport, error)
	Refresh                func() (domain.StatusReport, error)
	LoadSetup              func() domain.SetupReport
	LoadHosts              func() (domain.HostsReport, error)
	LoadHost               func(string) (domain.HostsReport, error)
	PlanDeployment         func(string) domain.DeploymentPlanReport
	ApplyDeployment        func(domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport
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
	CheckUpdate            func() domain.UpdateCheckReport
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
	dashboardRestore
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
	dashboardPXELeaveReview
	dashboardAdministration
	dashboardDiagnostics
	dashboardSoftware
)

type dashboardModel struct {
	updateDetails        bool
	returnAdmin          bool
	restoreMode          bool
	restoreCursor        int
	diagnosticReturn     dashboardScreen
	adminCursor          int
	hostCursor           int
	hostQuery            string
	hostSearching        bool
	hostDetail           bool
	hostTechnical        bool
	helpOpen             bool
	pageScroll           int
	setupDetails         bool
	progressDetails      bool
	doctor               domain.DoctorReport
	diagnosticCursor     int
	diagnosticDetails    bool
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
	deployProgress       domain.DeploymentProgress
	deployRecent         []string
	deployStarted        time.Time
	deployEvents         <-chan tea.Msg
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
	updateCheck          domain.UpdateCheckReport
	updateCursor         int
	updateTarget         string
	updatePrerelease     bool
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
	pilotCursor          int
	pilotName            string
	pilotPractical       bool
	pilotVerified        []string
	width                int
	height               int
	isDark               bool
	activitySpinner      spinner.Model
}

type dashboardStatusMsg struct {
	report domain.StatusReport
	err    error
}

type dashboardDoctorMsg struct {
	report domain.DoctorReport
	err    error
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

type dashboardPilotHostsMsg struct {
	report domain.HostsReport
	err    error
}

type dashboardDeploymentPlanMsg struct {
	report domain.DeploymentPlanReport
}

type dashboardDeploymentResultMsg struct {
	report domain.DeploymentExecutionReport
}

type dashboardDeploymentProgressMsg struct {
	progress domain.DeploymentProgress
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

type dashboardUpdateCheckMsg struct {
	report domain.UpdateCheckReport
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
		homeMenu: newDashboardTaskMenu(false, 80, 24), activitySpinner: newTUISpinner(false),
	}
}

func (model dashboardModel) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, model.activitySpinner.Tick)
}

func (model dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, command := model.updateState(message)
	next := updated.(dashboardModel)
	if k, ok := message.(tea.KeyPressMsg); ok && (k.String() == "esc" || k.String() == "left") && model.returnAdmin && model.screen != dashboardAdministration && next.screen == dashboardHome {
		next.screen = dashboardAdministration
	}
	if next.screen != model.screen {
		next.pageScroll = 0
	}
	if next.screen == dashboardHome {
		next.returnAdmin = false
	}
	return next, command
}

func (model dashboardModel) updateState(message tea.Msg) (tea.Model, tea.Cmd) {
	model.ensureActivitySpinner()
	switch message := message.(type) {
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
	case dashboardPilotHostsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "The selected computer could not be checked: " + message.err.Error()
		} else {
			model.hosts = message.report
			model.message = "Selected-computer observation refreshed."
		}
		model.screen = dashboardPXE
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
		model.hosts = domain.HostsReport{}
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
	if key.String() == "l" && (model.deploying || model.controllerApplying || model.pxePreparing) {
		model.progressDetails = !model.progressDetails
		return model, nil
	}
	if (key.String() == "ctrl+c" || key.String() == "q") && (model.deploying || model.updating || model.settingsApplying) {
		model.message = "A mutating operation is running; wait for its result before closing Nixorium."
		return model, nil
	}
	if key.String() == "q" && model.screen == dashboardPXE && model.guidedInstallation() && model.report.PXE.Mode == "active" {
		model.screen = dashboardPXELeaveReview
		model.confirmation = ""
		model.message = ""
		return model, nil
	}
	if key.String() == "ctrl+c" || (key.String() == "q" && !model.textEntry()) {
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

	switch model.screen {
	case dashboardHome:
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
		case "r":
			model.screen = dashboardRestore
			model.restoreCursor = 0
			model.pilotName = ""
			model.pilotPractical = false
			model.pilotVerified = nil
			model.message = ""
		case "w":
			model.screen = dashboardSoftware
		case "d":
			model.screen = dashboardDeploy
			model.deployResult = domain.DeploymentExecutionReport{}
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
			model.hostDetail = false
			model.hostTechnical = false
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
			model.updatePrerelease = false
			model.updateCheck = domain.UpdateCheckReport{}
			model.updateCursor = 0
			model.updateTarget = ""
			model.message = ""
			if model.actions.CheckUpdate == nil {
				model.message = "Release discovery is not available in this session."
				return model, nil
			}
			model.busy = "Fetching available Nixorium releases"
			return model, model.checkUpdates()
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
				model.deployChosen = map[string]bool{}
				model.deployCursor = 0
			} else {
				model.restoreMode = true
				model.pilotName = ""
				model.pilotPractical = false
				model.pilotVerified = nil
				model.hosts = domain.HostsReport{}
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
		if model.settingsResult.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				model.screen = dashboardHome
				model.message = ""
			case "g":
				model.busy = "Reviewing Git changes without modifying the worktree"
				model.message = ""
				return model, func() tea.Msg {
					return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
				}
			case "e":
				model.settingsResult = domain.ConfigApplyReport{}
				model.message = ""
			}
			return model, nil
		}
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
			model.screen = dashboardHome
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
				model.deployChosen = map[string]bool{hosts[min(model.hostCursor, len(hosts)-1)].Name: true}
				model.deployCursor = 0
			}
		case "r":
			model.busy = "Refreshing computer status"
			model.message = ""
			return model, model.loadHosts()
		}
	case dashboardDeploy:
		if model.deployResult.Operation != "" {
			switch key.String() {
			case "enter", "esc", "left":
				model.screen = dashboardHome
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
			case "g":
				model.busy = "Reviewing Git changes without modifying the worktree"
				model.message = ""
				return model, func() tea.Msg {
					return dashboardGitReviewMsg{report: model.actions.LoadGitReview()}
				}
			case "r":
				model.updateResult = domain.UpdateApplyReport{}
				model.updateCheck = domain.UpdateCheckReport{}
				model.updateTarget = ""
				model.message = ""
				if model.actions.CheckUpdate != nil {
					model.busy = "Fetching available Nixorium releases"
					return model, model.checkUpdates()
				}
			}
			return model, nil
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
				model.message = "Release discovery is not available in this session."
				return model, nil
			}
			model.updateCheck = domain.UpdateCheckReport{}
			model.updateTarget = ""
			model.message = ""
			model.busy = "Fetching available Nixorium releases"
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
				model.message = "Fetch available releases before selecting an update."
				return model, nil
			}
			target := releases[min(model.updateCursor, len(releases)-1)].Tag
			if target == model.updateCheck.CurrentRef {
				model.message = target + " is already the configured Nixorium release."
				return model, nil
			}
			model.updateTarget = target
			model.busy = "Validating the candidate release and representative builds"
			model.message = ""
			allowPrerelease := releases[min(model.updateCursor, len(releases)-1)].Channel == domain.UpdateChannelPrerelease
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
			if model.guidedInstallation() && model.pilotName != "" {
				model.pilotName = ""
				model.pilotPractical = false
				model.hosts = domain.HostsReport{}
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
		case "up", "k":
			if model.guidedInstallation() && model.pilotName == "" {
				model.pilotCursor = max(0, model.pilotCursor-1)
			}
		case "down", "j":
			if model.guidedInstallation() && model.pilotName == "" {
				model.pilotCursor = min(max(0, len(model.report.Meta.Clients.Hosts)-1), model.pilotCursor+1)
			}
		case "enter":
			if model.guidedInstallation() {
				if model.pilotName == "" {
					if len(model.report.Meta.Clients.Hosts) == 0 {
						model.message = "No client identity is configured. Return to laboratory settings and add one first."
						return model, nil
					}
					pilot := model.report.Meta.Clients.Hosts[min(model.pilotCursor, len(model.report.Meta.Clients.Hosts)-1)]
					model.pilotName = pilot.Name
					model.pilotPractical = false
					model.hosts = domain.HostsReport{}
					model.message = ""
					return model, nil
				}
				if model.pilotTechnicallyVerified() && !model.pilotPractical {
					model.pilotPractical = true
					model.pilotVerified = appendUnique(model.pilotVerified, model.pilotName)
					model.message = model.pilotName + " was verified in this session."
					return model, nil
				}
				if model.pilotPractical {
					model.pilotName = ""
					model.pilotPractical = false
					model.hosts = domain.HostsReport{}
					model.message = "Choose another configured computer, or stop installation mode to finish."
					return model, nil
				}
			}
		case "v":
			if model.guidedInstallation() && model.pilotName != "" {
				if model.actions.LoadHost == nil {
					model.message = "Pilot verification is not available in this session."
					return model, nil
				}
				model.busy = "Checking authenticated system state on " + model.pilotName
				model.message = ""
				return model, model.loadPilotHosts()
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
	case dashboardSoftware:
		if key.String() == "esc" || key.String() == "left" {
			model.screen = dashboardAdministration
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
			if model.confirmation != "LEAVE PXE ACTIVE" {
				model.confirmation = ""
				model.message = "Confirmation did not match; Nixorium remains open."
				return model, nil
			}
			return model, tea.Quit
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

func startDeployment(action func(domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport, plan domain.DeploymentPlanReport, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			report := action(plan, func(progress domain.DeploymentProgress) {
				events <- dashboardDeploymentProgressMsg{progress: progress}
			})
			events <- dashboardDeploymentResultMsg{report: report}
			close(events)
		}()
		return <-events
	}
}

func waitForDeploymentEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return nil
		}
		return message
	}
}

func appendBoundedActivity(recent []string, activity string, maximum int) []string {
	if activity == "" || maximum < 1 {
		return recent
	}
	recent = append(recent, activity)
	if len(recent) > maximum {
		recent = append([]string(nil), recent[len(recent)-maximum:]...)
	}
	return recent
}

func (model dashboardModel) loadHosts() tea.Cmd {
	return func() tea.Msg {
		report, err := model.actions.LoadHosts()
		return dashboardHostsMsg{report: report, err: err}
	}
}

func (model dashboardModel) loadPilotHosts() tea.Cmd {
	return func() tea.Msg {
		report, err := model.actions.LoadHost(model.pilotName)
		return dashboardPilotHostsMsg{report: report, err: err}
	}
}

func (model *dashboardModel) ensureHomeMenu() {
	if len(model.homeMenu.list.Items()) == 0 {
		model.homeMenu = newDashboardTaskMenu(model.isDark, model.width, model.height)
	}
}

func (model *dashboardModel) ensureActivitySpinner() {
	if len(model.activitySpinner.Spinner.Frames) == 0 {
		model.activitySpinner = newTUISpinner(model.isDark)
	}
}

func (model dashboardModel) busyView() string {
	model.ensureActivitySpinner()
	return model.activitySpinner.View() + " " + model.busy
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
	if model.helpOpen {
		view := tea.NewView(model.frame(model.helpView()))
		view.AltScreen = true
		return view
	}
	content := ""
	switch model.screen {
	case dashboardSetup:
		content = model.setupView()
	case dashboardRestore:
		content = model.restoreView()
	case dashboardHosts:
		content = model.computersView()
	case dashboardAdministration:
		content = model.administrationView()
	case dashboardDiagnostics:
		content = model.diagnosticsView()
	case dashboardSoftware:
		content = model.softwareView()
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
	case dashboardPXE, dashboardPXEStartReview, dashboardPXELeaveReview:
		content = model.pxeView()
	default:
		content = model.homeView()
	}
	view := tea.NewView(model.frame(content))
	view.AltScreen = true
	return view
}

func (model dashboardModel) setupView() string {
	groups, current := setupJourney(model.setup)
	lines := []string{
		tuiTitle("Nixorium — First setup", model.isDark),
		"",
		fmt.Sprintf("Step %d of %d", current+1, len(groups)),
		"You can leave safely and resume later with `nixorium setup`.",
		"",
	}
	for index, group := range groups {
		label := "○ " + group.title + " · " + group.pending
		switch group.state {
		case domain.SetupStageComplete:
			label = "✓ " + group.title + " · Complete"
		case domain.SetupStageCurrent:
			label = "● " + group.title + " · In progress"
		}
		if index == current {
			label = "› " + label
		} else {
			label = "  " + label
		}
		lines = append(lines, label)
	}
	if model.setupDetails {
		lines = append(lines, "", tuiSection("Technical steps", model.isDark))
		for _, stage := range model.setup.Stages {
			marker := "○"
			if stage.State == domain.SetupStageComplete {
				marker = "✓"
			} else if stage.State == domain.SetupStageCurrent {
				marker = "●"
			}
			lines = append(lines, fmt.Sprintf("  %s %s", marker, stage.Title))
		}
	}
	lines = append(lines, "")
	if model.setup.State == "ready" {
		lines = append(lines,
			tuiResult("Controller and client system are ready", true, model.isDark),
			"Next, install and verify a pilot computer. The other computers can remain powered off.",
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
		tuiHelpBinding([]string{"t"}, "t", "technical steps"),
		tuiHelpBinding([]string{"esc"}, "esc", "interventions"),
		tuiHelpBinding([]string{"q"}, "q", "quit"),
	))
	return strings.Join(lines, "\n") + "\n"
}

type setupJourneyGroup struct {
	title   string
	pending string
	ids     []string
	state   domain.SetupStageState
}

func setupJourney(report domain.SetupReport) ([]setupJourneyGroup, int) {
	groups := []setupJourneyGroup{
		{title: "Laboratory settings", pending: "To configure", ids: []string{domain.SetupStageInspectEnvironment, domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials, domain.SetupStageKeys, domain.SetupStageValidate, domain.SetupStageReview}},
		{title: "Controller", pending: "To configure", ids: []string{domain.SetupStageApply}},
		{title: "Client system", pending: "To prepare", ids: []string{domain.SetupStageArtifacts}},
		{title: "First computer", pending: "To install", ids: []string{domain.SetupStageReadiness, domain.SetupStageInstall}},
		{title: "Other computers", pending: "Whenever you are ready"},
	}
	states := map[string]domain.SetupStageState{}
	for _, stage := range report.Stages {
		states[stage.ID] = stage.State
	}
	current := 0
	for index := range groups {
		group := &groups[index]
		group.state = domain.SetupStageComplete
		if len(group.ids) == 0 {
			group.state = domain.SetupStagePending
			continue
		}
		for _, id := range group.ids {
			state, found := states[id]
			if !found && report.State != "ready" {
				group.state = domain.SetupStagePending
			}
			if state == domain.SetupStagePending {
				group.state = domain.SetupStagePending
			}
			if state == domain.SetupStageCurrent {
				group.state = domain.SetupStageCurrent
				current = index
				break
			}
		}
	}
	if report.State == "ready" {
		groups[3].state = domain.SetupStageCurrent
		current = 3
	}
	return groups, current
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
	lines := []string{tuiTitle("Nixorium — Git change review", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return strings.Join(lines, "\n") + "\n"
	}
	if model.gitCommitResult.Operation != "" {
		success := !model.gitCommitResult.HasErrors() && model.gitCommitResult.Committed
		title := "Git commit needs attention"
		if success {
			title = "Git changes committed locally"
		}
		returnLabel := "dashboard"
		if model.setupMode {
			returnLabel = "setup"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Committed: %t", model.gitCommitResult.State, model.gitCommitResult.Committed),
			"HEAD: "+model.gitCommitResult.Revision,
		)
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"enter"}, "enter", returnLabel),
			tuiHelpBinding([]string{"f"}, "f", "refresh review"),
		))
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
		fmt.Sprintf("State: %s   paths: %d   staged/unstaged/untracked: %d/%d/%d", tuiStatus(model.gitReview.State, gitReviewStatusKind(model.gitReview), model.isDark), len(model.gitReview.Changes), model.gitReview.Summary.Staged, model.gitReview.Summary.Unstaged, model.gitReview.Summary.Untracked),
		fmt.Sprintf("Managed/unexpected/private: %d/%d/%d", model.gitReview.Summary.Managed, model.gitReview.Summary.Unexpected, model.gitReview.Summary.Private),
		fmt.Sprintf("Showing lines %d-%d of %d", displayedLineStart(model.gitScroll, len(content)), end, len(content)),
		"",
	)
	lines = append(lines, content[model.gitScroll:end]...)
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"c"}, "c", "commit paths"),
		tuiHelpBinding([]string{"up", "down", "pgup", "pgdown"}, "↑/↓/pg", "scroll"),
		tuiHelpBinding([]string{"f"}, "f", "refresh"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
	))
	if model.message != "" {
		lines = append(lines, "", "Warning: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) gitCommitSelectView() string {
	lines := []string{tuiTitle("Nixorium — Select Git commit paths", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return strings.Join(lines, "\n") + "\n"
	}
	start, end := listWindow(len(model.gitReview.Changes), model.gitCommitCursor, model.rowCapacity())
	for index := start; index < end; index++ {
		change := model.gitReview.Changes[index]
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
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"space"}, "space", "select"),
		tuiHelpBinding([]string{"a"}, "a", "all safe"),
		tuiHelpBinding([]string{"enter"}, "enter", "review"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
	))
	if model.message != "" {
		lines = append(lines, "", model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) gitCommitReviewView() string {
	lines := []string{tuiTitle("Nixorium — Local Git commit review", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
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
	lines = append(lines, "", "Type "+model.gitCommitPlan.Confirmation+" to continue:", "> "+model.confirmation+"█", "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down", "pgup", "pgdown"}, "↑/↓/pg", "scroll"),
		tuiHelpBinding([]string{"esc"}, "esc", "cancel"),
	))
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
	lines := []string{tuiTitle("Nixorium — Update Nixorium", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		if model.updating {
			lines = append(lines, "", "Wait for the atomic two-file result before closing Nixorium.")
		} else {
			lines = append(lines, "", "Candidate evaluation and builds do not modify the deployment.")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	if model.updateResult.Operation != "" {
		success := !model.updateResult.HasErrors() && model.updateResult.Updated
		title := "Nixorium update needs attention"
		if success {
			title = "Nixorium files updated"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Files updated: %t   Retry safe: %t", model.updateResult.State, model.updateResult.Updated, model.updateResult.RetrySafe),
			"Target: "+model.updateResult.Target,
		)
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"g"}, "g", "review Git changes"),
			tuiHelpBinding([]string{"r"}, "r", "new update"),
			tuiHelpBinding([]string{"enter"}, "enter", "dashboard"),
		))
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardUpdateReview {
		return model.releaseReviewView()
	}
	if model.updateCheck.HasErrors() {
		lines = append(lines,
			tuiResult("Releases could not be fetched", false, model.isDark),
			"",
			"Nixorium could not obtain a usable release list from the configured upstream.",
			"No candidate can be selected and no file changed.",
		)
		if model.message != "" {
			lines = append(lines, "", "Detail: "+model.message)
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"r"}, "r", "try again"),
			tuiHelpBinding([]string{"esc"}, "esc", "back"),
		))
		return strings.Join(lines, "\n") + "\n"
	}

	releases := model.availableUpdateReleases()
	lines = append(lines,
		fmt.Sprintf("Current release  %s", model.updateCheck.CurrentRef),
		tuiMuted("Source  "+model.updateCheck.Upstream, model.isDark),
		"",
		tuiSection("Available releases", model.isDark),
	)
	start, end := listWindow(len(releases), model.updateCursor, max(4, model.height-15))
	for index := start; index < end; index++ {
		release := releases[index]
		marker := "  "
		if index == model.updateCursor {
			marker = "› "
		}
		note := ""
		if release.Tag == model.updateCheck.CurrentRef {
			note = "  Current"
		} else if index == 0 && release.Channel == domain.UpdateChannelStable {
			note = "  Latest stable"
		}
		if release.Channel == domain.UpdateChannelPrerelease {
			note += "  Prerelease"
		}
		lines = append(lines, marker+release.Tag+tuiMuted(note, model.isDark))
	}
	if len(releases) == 0 {
		lines = append(lines, "No releases are available in the selected channel.")
	}
	if model.updateCheck.Truncated {
		lines = append(lines, "", tuiMuted("The upstream result was safely limited to the newest releases.", model.isDark))
	}
	prereleaseLabel := "show prereleases"
	if model.updatePrerelease {
		prereleaseLabel = "hide prereleases"
	}
	lines = append(lines,
		"",
		"Selecting a release starts validation; it does not change files.",
		"Controller activation and client distribution remain separate operations.",
		"",
		tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"up", "down"}, "↑/↓", "select"),
			tuiHelpBinding([]string{"enter"}, "enter", "validate"),
			tuiHelpBinding([]string{"p"}, "p", prereleaseLabel),
			tuiHelpBinding([]string{"r"}, "r", "fetch again"),
			tuiHelpBinding([]string{"esc"}, "esc", "back"),
		),
	)
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) availableUpdateReleases() []domain.UpdateRelease {
	releases := append([]domain.UpdateRelease{}, model.updateCheck.Stable...)
	if model.updatePrerelease {
		releases = append(releases, model.updateCheck.Prerelease...)
	}
	return releases
}

func (model dashboardModel) checkUpdates() tea.Cmd {
	return func() tea.Msg {
		return dashboardUpdateCheckMsg{report: model.actions.CheckUpdate()}
	}
}

func (model dashboardModel) updateReviewHeight() int {
	if model.height <= 0 {
		return 10
	}
	height := model.height - 21
	if model.updateDetails {
		height -= len(model.updatePlan.Checks) + 2
	}
	return max(1, height)
}

func maximumUpdateScroll(report domain.UpdatePlanReport, height int) int {
	maximum := len(strings.Split(strings.TrimSuffix(report.Diff.Content, "\n"), "\n")) - height
	if maximum < 0 {
		return 0
	}
	return maximum
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
	height := model.height - 13
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
		lines = append(lines, fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		lines = append(lines, model.operationProgressView(model.controllerProgress, "Current progress")...)
		lines = append(lines, "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.busy != "" {
		lines = append(lines, model.busyView(), "", "This systemd-owned action continues if the dashboard closes.")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardControllerReview {
		return model.confirmationView("Update this controller?", model.controllerPlan.Controller+" (this controller only)", "Services and networking may restart; this connection may be interrupted.", "Build, activate and verify the reviewed configuration. A reboot is not normally required.", model.controllerPlan.Revision, model.controllerPlan.Confirmation)
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
	lines := []string{tuiTitle("Nixorium — Managed services", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return strings.Join(lines, "\n") + "\n"
	}
	if model.serviceResult.Operation != "" {
		success := !model.serviceResult.HasErrors() && model.serviceResult.Verified
		title := "Service action needs attention"
		if success {
			title = "Binary cache restarted and verified"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Verified: %t   Retry safe: %t", model.serviceResult.State, model.serviceResult.Verified, model.serviceResult.RetrySafe),
		)
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"enter"}, "enter", "dashboard"),
			tuiHelpBinding([]string{"l"}, "l", "logs"),
			tuiHelpBinding([]string{"r"}, "r", "restart again"),
		))
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardServicesRestartReview {
		return model.confirmationView("Restart the software cache?", "Controller cache; active installations may be affected", "The signed cache will be briefly unavailable. Active PXE clients may retry downloads.", "PXE networking and listeners are not controlled by this action. The cache is verified afterward.", "", "RESTART CACHE")
	}
	for _, service := range model.services.Services {
		kind := tuiStatusAttention
		if service.State == "healthy" || service.State == "active" || service.State == "standby" {
			kind = tuiStatusSuccess
		} else if service.State == "failed" || service.State == "degraded" {
			kind = tuiStatusFailure
		}
		lines = append(lines,
			fmt.Sprintf("%s — %s", tuiSection(service.Name, model.isDark), tuiStatus(service.State, kind, model.isDark)),
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
	lines = append(lines, tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"r"}, "r", "restart cache"),
		tuiHelpBinding([]string{"f"}, "f", "refresh"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
		tuiHelpBinding([]string{"q"}, "q", "quit"),
	))
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) logsView() string {
	lines := []string{tuiTitle("Nixorium — Operation logs", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, tuiSection("Recent actions", model.isDark))
	recordLimit := len(model.logs.Records)
	if recordLimit > 3 {
		recordLimit = 3
	}
	if recordLimit == 0 {
		lines = append(lines, "  No recorded operation outcomes.")
	}
	for _, record := range model.logs.Records[:recordLimit] {
		lines = append(lines, fmt.Sprintf("  %s  %-18s %-10s %s", record.RecordedAt.UTC().Format("2006-01-02 15:04Z"), record.Operation, record.State, record.Subject))
	}
	lines = append(lines, "", tuiSection("Deployment logs", model.isDark))
	if len(model.logs.Logs) == 0 {
		lines = append(lines, "No deployment operation logs are available.")
	}
	start, end := listWindow(len(model.logs.Logs), model.logCursor, max(1, (model.height-16)/2))
	for index := start; index < end; index++ {
		entry := model.logs.Logs[index]
		cursor := " "
		if index == model.logCursor {
			cursor = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s  %-10s %-11s %d bytes", cursor, entry.StartedAt.UTC().Format("2006-01-02 15:04Z"), entry.Kind, entry.State, entry.SizeBytes))
		lines = append(lines, "    "+entry.ID)
	}
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down"}, "↑/↓", "select"),
		tuiHelpBinding([]string{"enter"}, "enter", "view tail"),
		tuiHelpBinding([]string{"f"}, "f", "refresh"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
	))
	if model.message != "" {
		lines = append(lines, "", "Warning: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) logDetailView() string {
	lines := []string{tuiTitle("Nixorium — Operation log detail", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return strings.Join(lines, "\n") + "\n"
	}
	if model.logDetail.Log == nil {
		lines = append(lines, "The selected operation log could not be read safely.")
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"esc"}, "esc", "back"),
			tuiHelpBinding([]string{"q"}, "q", "quit"),
		))
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
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down", "pgup", "pgdown", "home", "end"}, "↑/↓/pg/home/end", "scroll"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
		tuiHelpBinding([]string{"q"}, "q", "quit"),
	))
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) logDetailHeight() int {
	if model.height <= 0 {
		return 12
	}
	height := model.height - 13
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

func gitReviewStatusKind(report domain.GitReviewReport) tuiStatusKind {
	if report.HasErrors() {
		return tuiStatusFailure
	}
	if len(report.Changes) > 0 {
		return tuiStatusAttention
	}
	return tuiStatusSuccess
}

func (model dashboardModel) deployView() string {
	lines := []string{tuiTitle("Nixorium — Distribute the prepared system", model.isDark), ""}
	if model.deploying {
		elapsed := time.Since(model.deployStarted).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines = append(lines, fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		lines = append(lines, model.deploymentProgressView()...)
		lines = append(lines,
			"",
			tuiMuted("Detailed Colmena output is being saved in the private deployment log.", model.isDark),
			"Closing is disabled while this foreground deployment is running.",
		)
		return strings.Join(lines, "\n") + "\n"
	}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardDeployReview {
		return model.confirmationView("Distribute the system?", fmt.Sprintf("%s · %d computer(s)", model.deployPlan.ColmenaSelector, len(model.deployPlan.Targets)), "Target services may restart; unreachable computers may remain unchanged.", "Build every selected configuration before applying it. A failed apply may leave mixed target state; a fresh full retry is safe.", model.deployPlan.Revision, "DEPLOY "+model.deployPlan.ColmenaSelector)
	}

	if model.deployResult.Operation != "" {
		success := !model.deployResult.HasErrors()
		resultTitle := "Deployment needs attention"
		if success {
			resultTitle = "Deployment completed and verified"
		}
		lines = append(lines,
			tuiResult(resultTitle, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Phase: %s", model.deployResult.State, model.deployResult.Phase),
			fmt.Sprintf("Build complete: %t   Apply complete: %t", model.deployResult.BuildCompleted, model.deployResult.ApplyCompleted),
		)
		if model.deployResult.Verification.Attempted > 0 {
			lines = append(lines, fmt.Sprintf("Authenticated: %d/%d   Recorded: %d", model.deployResult.Verification.Verified, model.deployResult.Verification.Attempted, model.deployResult.Verification.Recorded))
		}
		if model.deployResult.LogPath != "" {
			lines = append(lines, "Detailed log: "+model.deployResult.LogPath)
		}
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"enter"}, "enter", "dashboard"),
			tuiHelpBinding([]string{"l"}, "l", "logs"),
			tuiHelpBinding([]string{"r"}, "r", "new review"),
		))
		return strings.Join(lines, "\n") + "\n"
	}

	hosts := model.report.Meta.Clients.Hosts
	selected := 0
	for _, host := range hosts {
		if model.deployChosen[host.Name] {
			selected++
		}
	}
	lines = append(lines, "Choose where to apply the saved configuration.", tuiMuted("Select → Review → Deploy → Verify", model.isDark), "", fmt.Sprintf("%d of %d computers selected", selected, len(hosts)), "")
	start, end := listWindow(len(hosts), model.deployCursor, model.rowCapacity())
	for index := start; index < end; index++ {
		host := hosts[index]
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
	if len(hosts) > end || start > 0 {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d", start+1, end, len(hosts)), model.isDark))
	}
	if len(hosts) == 0 {
		lines = append(lines, "No configured client computers.")
	}
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"space"}, "space", "select"),
		tuiHelpBinding([]string{"a"}, "a", "all"),
		tuiHelpBinding([]string{"enter"}, "enter", "review"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
	))
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) deploymentProgressView() []string {
	progressState := model.deployProgress
	if progressState.Phase == "" {
		return []string{"", "  Waiting for deployment progress…"}
	}
	phaseLabels := map[domain.DeploymentPhase]string{
		domain.DeploymentPhasePreflight: "Revalidating review",
		domain.DeploymentPhaseBuild:     "Building configurations",
		domain.DeploymentPhaseApply:     "Applying configurations",
		domain.DeploymentPhaseVerify:    "Verifying computers",
		domain.DeploymentPhaseComplete:  "Complete",
	}
	index := 0
	switch progressState.Phase {
	case domain.DeploymentPhasePreflight:
		index = 1
	case domain.DeploymentPhaseBuild:
		index = 0
	case domain.DeploymentPhaseApply:
		index = 2
	case domain.DeploymentPhaseVerify:
		index = 3
	case domain.DeploymentPhaseComplete:
		index = 4
	}
	lines := phaseSteps([]string{"Building configurations", "Revalidating reviewed configuration", "Updating computers", "Verifying computers"}, index, progressState.Phase == domain.DeploymentPhaseComplete, model.isDark)
	if !model.progressDetails {
		if progressState.TargetTotal > 0 {
			lines = append(lines, "", fmt.Sprintf("Computers checked: %d/%d", progressState.TargetCurrent, progressState.TargetTotal))
		}
		return append(lines, "", "l progress details   F1 help")
	}
	lines = append(lines, "", tuiSection("Current progress", model.isDark), "  Phase: "+phaseLabels[progressState.Phase])
	if progressState.Total > 0 {
		barWidth := model.width - 8
		if barWidth < 24 {
			barWidth = 24
		}
		if barWidth > 64 {
			barWidth = 64
		}
		bar := tuiProgress(barWidth, model.isDark)
		percentage := float64(progressState.Completed) / float64(progressState.Total)
		lines = append(lines,
			fmt.Sprintf("  %d/%d", progressState.Completed, progressState.Total),
			"  "+bar.ViewAs(percentage),
		)
	}
	if progressState.TargetTotal > 0 {
		lines = append(lines, fmt.Sprintf("  Computers checked: %d/%d", progressState.TargetCurrent, progressState.TargetTotal))
	}
	if len(model.deployRecent) > 0 {
		lines = append(lines, "  Recent activity:")
		for _, activity := range model.deployRecent {
			lines = append(lines, "    • "+activity)
		}
	}
	return lines
}

func (model dashboardModel) hostsView() string { return model.computersView() }

func (model dashboardModel) pxeView() string {
	preparation := "missing"
	if model.report.PXEPreparation.Present {
		preparation = "stale"
	}
	if model.report.PXEPreparation.Ready {
		preparation = "ready"
	}
	title := "Nixorium — Install or reinstall computers"
	if model.setupMode {
		title = "Nixorium — First setup / First computer"
	} else if model.restoreMode {
		title = "Nixorium — Restore / Reinstall from scratch"
	}
	lines := []string{
		tuiTitle(title, model.isDark),
		"",
		fmt.Sprintf("Installation mode:  %s", tuiStatus(model.report.PXE.Mode, pxeStatusKind(model.report.PXE.Mode), model.isDark)),
		fmt.Sprintf("Prepared artifacts: %s", preparation),
		fmt.Sprintf("Interface:          %s", model.report.Meta.Network.Interface),
		fmt.Sprintf("Service address:    %s", model.report.Meta.Controller.DHCPIP),
	}
	if model.pxePreparing {
		elapsed := time.Since(model.pxeProgressStarted).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines = append(lines, "", fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		lines = append(lines, model.operationProgressView(model.pxeProgress, "Current progress")...)
		lines = append(lines, "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView(), "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardPXEStartReview {
		scope := model.startPlan.Interface + " · controller network"
		if model.guidedInstallation() && model.pilotName != "" {
			role := "pilot "
			if model.restoreMode {
				role = "selected computer "
			}
			scope += " · " + role + model.pilotName
		}
		return model.confirmationView("Start network installation?", scope, "Temporarily remove "+model.startPlan.StaticCIDR+"; remote connections may be interrupted.", "Serve ProxyDHCP, TFTP, HTTP and cache via "+model.startPlan.DHCPAddress+". Institutional DHCP remains authoritative. `nixorium pxe stop` or reboot recovery restores normal addressing.", "", "START PXE")
	}
	if model.screen == dashboardPXELeaveReview {
		return model.confirmationView("Leave installation mode active?", "Controller PXE services and laboratory installation network", "Closing Nixorium will not stop installation mode.", "Configured computers may continue to network-boot into the installer. Run `nixorium pxe stop` later to restore normal controller networking.", "", "LEAVE PXE ACTIVE")
	}
	if model.guidedInstallation() {
		lines = append(lines, "")
		lines = append(lines, model.pilotInstallationView()...)
		if model.message != "" {
			lines = append(lines, "", tuiSection("Last action", model.isDark), "  "+model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, "")
	lines = append(lines, model.pxeNextStepView()...)
	if model.pxeProgress.Operation != "" {
		lines = append(lines, "")
		lines = append(lines, model.operationProgressView(model.pxeProgress, "Last preparation")...)
	}
	lines = append(lines, "", tuiSection("Available actions", model.isDark), model.pxeActionHelp())
	if model.message != "" {
		lines = append(lines, "", tuiSection("Last action", model.isDark), "  "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) guidedInstallation() bool {
	return model.setupMode || model.restoreMode
}

func (model dashboardModel) pilotInstallationView() []string {
	recovery := model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required"
	if recovery {
		return []string{
			tuiResult("Controller networking needs recovery", false, model.isDark),
			"Nixorium cannot safely continue the installation until normal addressing is reconciled.",
			"",
			tuiHelp(model.width, model.isDark,
				tuiHelpBinding([]string{"r"}, "r", "recover"),
				tuiHelpBinding([]string{"esc"}, "esc", "back"),
				tuiHelpBinding([]string{"q"}, "q", "quit"),
			),
		}
	}

	if model.pilotName == "" {
		return model.pilotSelectionView()
	}

	computerLabel := "Pilot computer"
	if model.restoreMode {
		computerLabel = "Computer to reinstall"
	}
	lines := []string{tuiSection(computerLabel, model.isDark), "  " + model.pilotName}
	if model.report.PXE.Mode != "active" {
		if model.restoreMode {
			lines = append(lines, "", tuiStatus("Reinstallation erases the disk confirmed locally on "+model.pilotName+".", tuiStatusAttention, model.isDark))
		}
		lines = append(lines, "")
		lines = append(lines, model.pxeNextStepView()...)
		bindings := []key.Binding{}
		if !model.report.PXEPreparation.Ready {
			bindings = append(bindings, tuiHelpBinding([]string{"p"}, "p", "prepare"))
		} else {
			bindings = append(bindings, tuiHelpBinding([]string{"s"}, "s", "review and start"))
		}
		bindings = append(bindings,
			tuiHelpBinding([]string{"esc"}, "esc", model.installationChangeLabel()),
			tuiHelpBinding([]string{"q"}, "q", "quit"),
		)
		return append(lines, "", tuiHelp(model.width, model.isDark, bindings...))
	}

	host, observed := model.pilotHostStatus()
	if !observed {
		checkLabel := "check pilot"
		if model.restoreMode {
			checkLabel = "check computer"
		}
		lines = append(lines,
			"",
			tuiResult("Continue at "+model.pilotName, false, model.isDark),
			"  1. Power it on and choose UEFI network boot.",
			"  2. In the downloaded installer, run /installer/setup.sh.",
			"  3. Choose "+model.pilotName+" and inspect the target disk.",
			"  4. Confirm installation locally, then boot from the installed disk.",
			"",
			tuiStatus("The disk selected on the computer will be erased.", tuiStatusAttention, model.isDark),
			"Nixorium has not yet verified an authenticated installed system.",
			"No remote progress is shown because the installer does not provide telemetry.",
			"",
			tuiHelp(model.width, model.isDark,
				tuiHelpBinding([]string{"v"}, "v", checkLabel),
				tuiHelpBinding([]string{"x"}, "x", "stop installation"),
				tuiHelpBinding([]string{"esc"}, "esc", model.installationChangeLabel()),
				tuiHelpBinding([]string{"q"}, "q", "leave PXE active"),
			),
		)
		return lines
	}

	level, label, guidance := domain.ComputerCondition(host)
	lines = append(lines, "", tuiStatus(label, statusLevel(level), model.isDark))
	if !model.pilotTechnicallyVerified() {
		lines = append(lines,
			guidance,
			"This does not prove that installation completed. Finish the local steps, boot from disk, then check again.",
			"",
			tuiHelp(model.width, model.isDark,
				tuiHelpBinding([]string{"v"}, "v", "check again"),
				tuiHelpBinding([]string{"x"}, "x", "stop installation"),
				tuiHelpBinding([]string{"esc"}, "esc", model.installationChangeLabel()),
				tuiHelpBinding([]string{"q"}, "q", "leave PXE active"),
			),
		)
		return lines
	}

	if !model.pilotPractical {
		lines = append(lines,
			tuiResult("Technical verification succeeded", true, model.isDark),
			"Authenticated management reports the saved revision as active.",
			"",
			tuiSection("Check at the computer", model.isDark),
			"  • Log in and open the expected desktop session.",
			"  • Check required software, network and classroom peripherals.",
			"  • Confirm that the computer started from its installed disk.",
			"",
			tuiHelp(model.width, model.isDark,
				tuiHelpBinding([]string{"enter"}, "enter", "practical check passed"),
				tuiHelpBinding([]string{"v"}, "v", "check again"),
				tuiHelpBinding([]string{"x"}, "x", "stop installation"),
				tuiHelpBinding([]string{"q"}, "q", "leave PXE active"),
			),
		)
		return lines
	}

	nextLabel := "install another"
	if model.restoreMode {
		nextLabel = "reinstall another"
	}
	lines = append(lines,
		"",
		tuiResult(model.installationVerifiedTitle(), true, model.isDark),
		model.installationRemainingGuidance(),
		"Powered-off computers are not errors.",
		"",
		tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"enter"}, "enter", nextLabel),
			tuiHelpBinding([]string{"x"}, "x", "stop and finish"),
			tuiHelpBinding([]string{"q"}, "q", "leave PXE active"),
		),
	)
	return lines
}

func (model dashboardModel) pilotSelectionView() []string {
	lines := []string{}
	if len(model.pilotVerified) > 0 && model.report.PXE.Mode != "active" {
		lines = append(lines,
			tuiResult(model.installationSessionTitle(), true, model.isDark),
			"Verified in this session: "+strings.Join(model.pilotVerified, ", "),
			fmt.Sprintf("%d configured identities were not verified in this session.", max(0, len(model.report.Meta.Clients.Hosts)-len(model.pilotVerified))),
			"",
			"You can return later to install the remaining computers.",
			"",
			tuiHelp(model.width, model.isDark,
				tuiHelpBinding([]string{"esc"}, "esc", "setup summary"),
				tuiHelpBinding([]string{"q"}, "q", "quit"),
			),
		)
		return lines
	}

	selectionTitle := "Choose a pilot computer"
	if model.restoreMode {
		selectionTitle = "Choose a computer to reinstall"
	}
	lines = append(lines, tuiSection(selectionTitle, model.isDark))
	if len(model.report.Meta.Clients.Hosts) == 0 {
		return append(lines,
			"No client identity is configured.",
			"Return to laboratory settings and add at least one computer.",
			"",
			tuiHelp(model.width, model.isDark,
				tuiHelpBinding([]string{"esc"}, "esc", "back"),
				tuiHelpBinding([]string{"q"}, "q", "quit"),
			),
		)
	}
	start, end := listWindow(len(model.report.Meta.Clients.Hosts), model.pilotCursor, max(3, model.height-18))
	for index := start; index < end; index++ {
		host := model.report.Meta.Clients.Hosts[index]
		marker := "  "
		if index == model.pilotCursor {
			marker = "› "
		}
		verified := ""
		if containsString(model.pilotVerified, host.Name) {
			verified = "  ✓ verified this session"
		}
		lines = append(lines, fmt.Sprintf("%s%-12s %s%s", marker, host.Name, host.IP, verified))
	}
	lines = append(lines,
		"",
		"The identity comes from the saved inventory. Disk selection and erasure are confirmed locally.",
		"",
		tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"up", "down"}, "↑/↓", "move"),
			tuiHelpBinding([]string{"enter"}, "enter", "select"),
			tuiHelpBinding([]string{"esc"}, "esc", "back"),
			tuiHelpBinding([]string{"q"}, "q", "quit"),
		),
	)
	return lines
}

func (model dashboardModel) installationVerifiedTitle() string {
	if model.restoreMode {
		return model.pilotName + " reinstalled and verified in this session"
	}
	return model.pilotName + " verified in this session"
}

func (model dashboardModel) installationRemainingGuidance() string {
	if model.restoreMode {
		return "Other configured computers can remain unchanged or be reinstalled one at a time."
	}
	return "The other configured computers may be installed now or later."
}

func (model dashboardModel) installationSessionTitle() string {
	if model.restoreMode {
		return "Reinstallation session complete"
	}
	return "Installation session complete"
}

func (model dashboardModel) installationChangeLabel() string {
	if model.restoreMode {
		return "change computer"
	}
	return "change pilot"
}

func (model dashboardModel) pilotHostStatus() (domain.HostStatus, bool) {
	for _, host := range model.hosts.Hosts {
		if host.Name == model.pilotName {
			return host, true
		}
	}
	return domain.HostStatus{}, false
}

func (model dashboardModel) pilotTechnicallyVerified() bool {
	host, found := model.pilotHostStatus()
	return found && host.SSH == domain.SSHAvailable && host.Deployment == domain.DeploymentCurrent
}

func appendUnique(values []string, value string) []string {
	if containsString(values, value) {
		return values
	}
	return append(values, value)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (model dashboardModel) pxeNextStepView() []string {
	switch model.report.PXE.Mode {
	case "active":
		return []string{
			tuiResult("Next: install a computer", true, model.isDark),
			"  1. Boot one configured computer using UEFI network boot.",
			"  2. In the downloaded installer, run /installer/setup.sh.",
			"  3. When installations are finished, press x here to stop PXE.",
		}
	case "degraded", "recovery-required":
		return []string{
			tuiResult("Next: recover normal controller networking", false, model.isDark),
			"  Press r to reconcile the recorded address and managed PXE state.",
		}
	}
	if !model.report.PXEPreparation.Ready {
		return []string{
			tuiResult("Next: prepare installation files", false, model.isDark),
			"  Press p to build netboot artifacts and every configured client system.",
		}
	}
	return []string{
		tuiResult("Next: start network installation", false, model.isDark),
		"  Press s to review the temporary address change and start PXE.",
	}
}

func (model dashboardModel) pxeActionHelp() string {
	bindings := []key.Binding{}
	recovery := model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required"
	if model.report.PXE.Mode != "active" && !recovery {
		bindings = append(bindings, tuiHelpBinding([]string{"p"}, "p", "prepare"))
		if model.report.PXEPreparation.Ready {
			bindings = append(bindings, tuiHelpBinding([]string{"s"}, "s", "start PXE"))
		}
	}
	if model.report.PXE.Mode == "active" || recovery {
		bindings = append(bindings, tuiHelpBinding([]string{"x"}, "x", "stop PXE"))
	}
	bindings = append(bindings,
		tuiHelpBinding([]string{"r"}, "r", "recover"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
		tuiHelpBinding([]string{"q"}, "q", "quit; services continue"),
	)
	return tuiHelp(model.width, model.isDark, bindings...)
}

func pxeStatusKind(mode string) tuiStatusKind {
	switch mode {
	case "ready", "stopped":
		return tuiStatusSuccess
	case "active", "preparing":
		return tuiStatusAttention
	case "degraded", "recovery-required":
		return tuiStatusFailure
	default:
		return tuiStatusNeutral
	}
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
	if !model.progressDetails && !model.controllerDetails {
		kind := tuiStatusNeutral
		label := "● " + phaseLabels[operation.Phase] + " · Running"
		if operation.State == "completed" {
			kind = tuiStatusSuccess
			label = "Preparation completed"
			if operation.Operation == "controller-apply" {
				label = "Controller operation completed"
			}
		}
		if operation.State == "failed" {
			kind = tuiStatusFailure
			label = phaseLabels[operation.Phase] + " failed"
		}
		if kind == tuiStatusNeutral {
			lines = []string{tuiTitle(label, model.isDark)}
		} else {
			lines = []string{tuiStatus(label, kind, model.isDark)}
		}
		if operation.Total > 0 {
			lines = append(lines, fmt.Sprintf("%d/%d steps complete", operation.Current, operation.Total))
		}
		if len(operation.Recent) > 0 {
			lines = append(lines, tuiMuted(operation.Recent[len(operation.Recent)-1], model.isDark))
		}
		if model.pxePreparing || model.controllerApplying {
			lines = append(lines, "", "l progress details   F1 help")
		}
		return lines
	}
	if operation.Total > 0 {
		barWidth := model.width - 8
		if barWidth < 24 {
			barWidth = 24
		}
		if barWidth > 64 {
			barWidth = 64
		}
		bar := tuiProgress(barWidth, model.isDark)
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
