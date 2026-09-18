package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type DashboardActions struct {
	RunningVersion            string
	LoadInitial               func() (domain.StatusReport, domain.SetupReport, error)
	LoadDoctor                func() (domain.DoctorReport, error)
	Refresh                   func() (domain.StatusReport, error)
	LoadSetup                 func() domain.SetupReport
	LoadSetupKeys             func() domain.KeyReconcileReport
	ReconcileSetupKeys        func() (domain.KeyReconcileReport, error)
	ImportSetupKey            func(string, string) (domain.KeyImportReport, error)
	SaveSetupConfiguration    func() domain.ConfigurationSaveReport
	InstallSetupSecrets       func() domain.ActionReport
	LoadHosts                 func() (domain.HostsReport, error)
	LoadInstallationSession   func() domain.InstallationSessionReport
	SelectInstallationTarget  func(string) domain.InstallationSessionReport
	VerifyInstallationTarget  func(string) domain.InstallationSessionReport
	ConfirmInstallationTarget func(string) domain.InstallationSessionReport
	LoadSoftware              func() domain.SoftwareCatalogReport
	SearchSoftware            func(context.Context, string) domain.SoftwareSearchReport
	PlanSoftware              func(domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport
	SaveSoftware              func(domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport
	PlanShutdown              func(string, domain.ShutdownSessionPolicy) domain.ShutdownPlanReport
	ApplyShutdown             func(domain.ShutdownPlanReport) domain.ShutdownApplyReport
	PlanDeployment            func(string) domain.DeploymentPlanReport
	ApplyDeployment           func(domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport
	PlanController            func() domain.ControllerRebuildPlanReport
	ApplyController           func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport
	LoadControllerProgress    func() (domain.OperationProgress, error)
	LoadServices              func() domain.ServicesReport
	RestartService            func(string) domain.ServiceActionReport
	LoadLogs                  func() domain.OperationLogsReport
	LoadLog                   func(string) domain.OperationLogReport
	LoadGitReview             func() domain.GitReviewReport
	PlanGitCommit             func(string) domain.GitCommitPlanReport
	ApplyGitCommit            func(domain.GitCommitPlanReport) domain.GitCommitReport
	CheckUpdate               func() domain.UpdateCheckReport
	PlanUpdate                func(string, bool, bool) domain.UpdatePlanReport
	SaveUpdate                func(domain.UpdatePlanReport) domain.UpdateApplyReport
	LoadSettings              func() (domain.LabSettingsFile, error)
	PlanSettings              func(domain.LabSettingsFile) domain.ConfigPlanReport
	SaveSettings              func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport
	ChangePassword            SettingsPasswordAction
	PreparePXE                func() domain.ActionReport
	LoadPXEProgress           func() (domain.OperationProgress, error)
	PlanPXEStart              func() domain.PXELifecycleReport
	StartPXE                  func() domain.PXELifecycleReport
	StopPXE                   func() domain.PXELifecycleReport
	RecoverPXE                func() domain.PXELifecycleReport
}

type dashboardScreen int

const (
	dashboardHome dashboardScreen = iota
	dashboardComputersArea
	dashboardInstallationArea
	dashboardSetup
	dashboardSetupKeys
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
	dashboardSoftwareScope
	dashboardSoftwareReview
	dashboardSoftwareResult
	dashboardShutdown
	dashboardShutdownReview
	dashboardShutdownResult
)

type dashboardModel struct {
	updateDetails          bool
	returnAdmin            bool
	restoreMode            bool
	restoreCursor          int
	computersAreaCursor    int
	installationAreaCursor int
	areaReturn             dashboardScreen
	diagnosticReturn       dashboardScreen
	adminCursor            int
	hostCursor             int
	hostQuery              string
	hostSearching          bool
	hostDetail             bool
	hostTechnical          bool
	helpOpen               bool
	pageScroll             int
	setupDetails           bool
	setupKeys              domain.KeyReconcileReport
	setupKeyCursor         int
	setupKeyImporting      bool
	setupKeyPath           string
	setupKeyImportResult   domain.KeyImportReport
	progressDetails        bool
	doctor                 domain.DoctorReport
	diagnosticCursor       int
	diagnosticDetails      bool
	report                 domain.StatusReport
	setup                  domain.SetupReport
	setupMode              bool
	homeMenu               dashboardTaskMenu
	actions                DashboardActions
	screen                 dashboardScreen
	busy                   string
	message                string
	confirmation           string
	startPlan              domain.PXELifecycleReport
	hosts                  domain.HostsReport
	deployCursor           int
	deployChosen           map[string]bool
	deployPlan             domain.DeploymentPlanReport
	deployResult           domain.DeploymentExecutionReport
	deploying              bool
	deployProgress         domain.DeploymentProgress
	deployRecent           []string
	deployStarted          time.Time
	deployEvents           <-chan tea.Msg
	controllerPlan         domain.ControllerRebuildPlanReport
	controllerResult       domain.ControllerRebuildExecutionReport
	controllerApplying     bool
	controllerProgress     domain.OperationProgress
	controllerStarted      time.Time
	controllerProgressID   uint64
	controllerDetails      bool
	services               domain.ServicesReport
	serviceResult          domain.ServiceActionReport
	logs                   domain.OperationLogsReport
	logDetail              domain.OperationLogReport
	logCursor              int
	logScroll              int
	gitReview              domain.GitReviewReport
	gitScroll              int
	gitCommitCursor        int
	gitCommitChosen        map[string]bool
	gitCommitPlan          domain.GitCommitPlanReport
	gitCommitResult        domain.GitCommitReport
	updateCheck            domain.UpdateCheckReport
	updateCursor           int
	updateTarget           string
	updatePrerelease       bool
	updatePlan             domain.UpdatePlanReport
	updateResult           domain.UpdateApplyReport
	updateScroll           int
	updating               bool
	settings               domain.LabSettingsFile
	settingsCandidate      domain.LabSettingsFile
	settingsMenu           routineSettingsMenu
	settingsPasswordMenu   routinePasswordMenu
	settingsEditor         settingsWizardModel
	settingsPlan           domain.ConfigPlanReport
	settingsResult         domain.ConfigurationSaveReport
	settingsApplying       bool

	settingsReturn           dashboardScreen
	settingsCollectPasswords bool
	startingLabSetup         bool

	pxePreparing         bool
	pxeProgress          domain.OperationProgress
	pxeProgressStarted   time.Time
	pxeProgressID        uint64
	pilotCursor          int
	pilotName            string
	pilotPractical       bool
	pilotVerified        []string
	installationSummary  bool
	installationSession  domain.InstallationSessionReport
	softwareCatalog      domain.SoftwareCatalogReport
	softwareMode         softwareListMode
	softwareCursor       int
	softwareQuery        string
	softwareSearching    bool
	softwareSearch       domain.SoftwareSearchReport
	softwareSearchID     uint64
	softwareSearchBusy   bool
	softwareSearchCancel context.CancelFunc
	softwareSelected     string
	softwareScopeCursor  int
	softwareClientCursor int
	softwareClients      map[string]bool
	softwarePlan         domain.SoftwareChangePlanReport
	softwareResult       domain.SoftwareChangeApplyReport
	softwareApplying     bool
	shutdownCursor       int
	shutdownChosen       map[string]bool
	shutdownPolicy       domain.ShutdownSessionPolicy
	shutdownPlan         domain.ShutdownPlanReport
	shutdownResult       domain.ShutdownApplyReport
	shutdownApplying     bool
	shutdownTechnical    bool
	width                int
	height               int
	isDark               bool
	activitySpinner      spinner.Model

	initializing bool
	initialError bool
}

type dashboardStatusMsg struct {
	report domain.StatusReport
	err    error
}

type dashboardInitialMsg struct {
	report domain.StatusReport
	setup  domain.SetupReport
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

type dashboardSetupKeysMsg struct {
	keys    domain.KeyReconcileReport
	keyErr  error
	save    domain.ConfigurationSaveReport
	install domain.ActionReport
}

type dashboardSetupSaveMsg struct {
	report domain.ConfigurationSaveReport
}

type dashboardSetupKeyStatusMsg struct {
	report domain.KeyReconcileReport
}

type dashboardSetupKeyImportMsg struct {
	report domain.KeyImportReport
	err    error
	keys   domain.KeyReconcileReport
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

type dashboardPXEExitMsg struct {
	lifecycle domain.PXELifecycleReport
	status    domain.StatusReport
	statusErr error
}

type dashboardHostsMsg struct {
	report domain.HostsReport
	err    error
}

type dashboardInstallationSessionMsg struct {
	report domain.InstallationSessionReport
	screen dashboardScreen
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
type dashboardUpdateControllerMsg struct {
	plan   domain.ControllerRebuildPlanReport
	report domain.ControllerRebuildExecutionReport
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
	report domain.ConfigurationSaveReport
}

type dashboardSoftwareCatalogMsg struct{ report domain.SoftwareCatalogReport }
type dashboardSoftwareSearchStartMsg struct {
	id    uint64
	query string
	ctx   context.Context
}
type dashboardSoftwareSearchMsg struct {
	id     uint64
	report domain.SoftwareSearchReport
}
type dashboardSoftwarePlanMsg struct {
	report domain.SoftwareChangePlanReport
}
type dashboardSoftwareApplyMsg struct {
	report domain.SoftwareChangeApplyReport
}
type dashboardSoftwareControllerMsg struct {
	plan   domain.ControllerRebuildPlanReport
	report domain.ControllerRebuildExecutionReport
}
type dashboardShutdownPlanMsg struct{ report domain.ShutdownPlanReport }
type dashboardShutdownApplyMsg struct{ report domain.ShutdownApplyReport }

func RunDashboard(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions) error {
	_, err := tea.NewProgram(newDashboardModel(report, setup, actions, false)).Run()
	return err
}

func RunSetupDashboard(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions) error {
	_, err := tea.NewProgram(newDashboardModel(report, setup, actions, true)).Run()
	return err
}

func RunLoadingDashboard(actions DashboardActions, setupMode bool) error {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, actions, false)
	model.setupMode = setupMode
	model.initializing = true
	model.busy = "Opening the laboratory and checking setup progress"
	_, err := tea.NewProgram(model).Run()
	return err
}

func newDashboardModel(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions, setupMode bool) dashboardModel {
	screen := dashboardHome
	if setupMode || setupNeedsImmediateAttention(setup) {
		screen = dashboardSetup
		setupMode = true
	}
	return dashboardModel{
		report: report, setup: setup, setupMode: setupMode, screen: screen, actions: actions,
		homeMenu: newDashboardTaskMenu(false, 80, 24), activitySpinner: newTUISpinner(false),
	}
}

func (model dashboardModel) Init() tea.Cmd {
	commands := []tea.Cmd{tea.RequestBackgroundColor, model.activitySpinner.Tick}
	if model.initializing && model.actions.LoadInitial != nil {
		commands = append(commands, model.loadInitial())
	}
	return tea.Batch(commands...)
}

func (model dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, command := model.updateState(message)
	next := updated.(dashboardModel)
	if model.areaReturn != dashboardHome && model.screen != model.areaReturn && next.screen == dashboardHome {
		next.screen = model.areaReturn
	}
	if k, ok := message.(tea.KeyPressMsg); ok && (k.String() == "esc" || k.String() == "left") && model.returnAdmin && model.screen != dashboardAdministration && next.screen == dashboardHome {
		next.screen = dashboardAdministration
	}
	if next.screen != model.screen {
		next.pageScroll = 0
	}
	if next.screen == dashboardHome {
		next.returnAdmin = false
		next.areaReturn = dashboardHome
	}
	return next, command
}

func (model dashboardModel) runAction(operation func() string, screen dashboardScreen) tea.Cmd {
	return func() tea.Msg {
		message := operation()
		report, err := model.actions.Refresh()
		return dashboardOperationMsg{message: message, report: report, err: err, screen: screen}
	}
}

func setupNeedsImmediateAttention(report domain.SetupReport) bool {
	switch report.CurrentStage {
	case domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials:
		return true
	default:
		return false
	}
}

func (model dashboardModel) openSetupSettings() (tea.Model, tea.Cmd) {
	if model.actions.LoadSettings == nil {
		model.message = "Laboratory settings are not available in this session."
		return model, nil
	}
	model.screen = dashboardSettings
	model.settingsReturn = dashboardSetup
	model.settingsCollectPasswords = model.setup.CurrentStage != domain.SetupStageValidate
	model.settingsResult = domain.ConfigurationSaveReport{}
	model.settingsPlan = domain.ConfigPlanReport{}
	model.settingsCandidate = domain.LabSettingsFile{}
	model.busy = "Loading laboratory settings"
	model.message = ""
	return model, func() tea.Msg {
		settings, err := model.actions.LoadSettings()
		return dashboardSettingsMsg{settings: settings, err: err}
	}
}

func (model dashboardModel) returnFromSettings() (tea.Model, tea.Cmd) {
	returnTo := model.settingsReturn
	model.settingsReturn = dashboardHome
	model.settingsCollectPasswords = false
	model.message = ""
	if returnTo == dashboardSetup {
		model.screen = dashboardSetup
		if model.actions.LoadSetup != nil && (model.settingsResult.State == "saved" || model.settingsResult.State == "unchanged") {
			model.busy = "Refreshing setup progress"
			return model, model.loadSetup()
		}
		return model, nil
	}
	model.screen = dashboardHome
	return model, nil
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

func (model dashboardModel) loadInstallationSession(screen dashboardScreen) tea.Cmd {
	return func() tea.Msg {
		return dashboardInstallationSessionMsg{report: model.actions.LoadInstallationSession(), screen: screen}
	}
}

func (model dashboardModel) selectInstallationTarget(name string) tea.Cmd {
	return func() tea.Msg {
		return dashboardInstallationSessionMsg{report: model.actions.SelectInstallationTarget(name), screen: dashboardPXE}
	}
}

func (model dashboardModel) verifyInstallationTarget(name string) tea.Cmd {
	return func() tea.Msg {
		return dashboardInstallationSessionMsg{report: model.actions.VerifyInstallationTarget(name), screen: dashboardPXE}
	}
}

func (model dashboardModel) confirmInstallationTarget(name string) tea.Cmd {
	return func() tea.Msg {
		return dashboardInstallationSessionMsg{report: model.actions.ConfirmInstallationTarget(name), screen: dashboardPXE}
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

func (model dashboardModel) loadInitial() tea.Cmd {
	return func() tea.Msg {
		report, setup, err := model.actions.LoadInitial()
		return dashboardInitialMsg{report: report, setup: setup, err: err}
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
	case dashboardComputersArea:
		content = model.computersAreaView()
	case dashboardInstallationArea:
		content = model.installationAreaView()
	case dashboardSetup:
		content = model.setupView()
	case dashboardSetupKeys:
		content = model.setupKeysView()
	case dashboardRestore:
		content = model.restoreView()
	case dashboardHosts:
		content = model.computersView()
	case dashboardAdministration:
		content = model.administrationView()
	case dashboardDiagnostics:
		content = model.diagnosticsView()
	case dashboardSoftware, dashboardSoftwareScope, dashboardSoftwareReview, dashboardSoftwareResult:
		content = model.softwareView()
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		content = model.shutdownView()
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
		tuiTitle("Laboratory setup", model.isDark),
		fmt.Sprintf("Step %d of %d", current+1, len(groups)),
		"You can leave safely and resume this setup later.",
		"",
	}
	if model.busy != "" {
		lines = append(lines, model.busyView(), "")
	}
	for _, group := range groups {
		label := "○ " + group.title + " · " + group.pending
		switch group.state {
		case domain.SetupStageComplete:
			label = "✓ " + group.title + " · Complete"
		case domain.SetupStageCurrent:
			label = "● " + group.title + " · In progress"
		}
		lines = append(lines, "  "+label)
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
			tuiSection("Continue setup", model.isDark),
			"  "+setupCurrentTitle(model.setup),
			"  "+setupCurrentAction(model.setup),
			"  Press Enter to continue.",
		)
	}
	backLabel := "Overview"
	if model.areaReturn == dashboardInstallationArea {
		backLabel = "Installation"
	}
	shell := tuiShell{
		path:    []string{"Installation", "Setup"},
		body:    strings.Join(lines, "\n"),
		actions: []tuiAction{{key: "Enter", label: "Continue"}, {key: "t", label: "Technical steps"}, {key: "Esc", label: backLabel}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}},
	}
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusNeutral, title: model.message}}
	}
	return renderTUIShell(shell, model.width, model.isDark)
}

func (model dashboardModel) setupKeysView() string {
	lines := []string{
		tuiTitle("Controller keys", model.isDark),
		"These keys authenticate the controller. Private keys stay local and are never added to configuration history.",
		"Existing valid keys are reused and never replaced by this workflow.",
		"",
	}
	if model.busy != "" {
		return renderTUIShell(tuiShell{
			path:    []string{"Installation", "Setup", "Controller keys"},
			body:    strings.Join(append(lines, model.busyView()), "\n"),
			actions: []tuiAction{{key: "F1", label: "Help"}},
		}, model.width, model.isDark)
	}
	definitions := []struct {
		name, label, purpose string
	}{
		{name: "cache", label: "Binary cache signing", purpose: "Lets client computers verify software served by this controller."},
		{name: "ssh", label: "Administrator SSH", purpose: "Lets the controller manage enrolled computers without a password prompt."},
		{name: "veyon", label: "Veyon classroom control", purpose: "Authenticates classroom viewing and control from the teacher station."},
	}
	states := map[string]domain.KeyMaterialState{}
	for _, state := range model.setupKeys.Keys {
		states[state.Name] = state
	}
	for index, definition := range definitions {
		marker := "  "
		if index == model.setupKeyCursor {
			marker = "› "
		}
		state := states[definition.name]
		status := "Action required"
		if state.Ready() {
			status = "Existing key — ready"
		} else if state.Problem != "" {
			status += " — " + state.Problem
		}
		lines = append(lines, marker+definition.label+"  ·  "+status, "    "+definition.purpose)
	}
	if model.setupKeyImporting {
		name := model.selectedSetupKeyName()
		lines = append(lines,
			"",
			tuiSection("Import existing "+setupKeyShortLabel(name)+" private key", model.isDark),
			"Enter the path to a regular private-key file with mode 0600.",
			"Nixorium validates it, derives the public key, and leaves the source unchanged.",
			"Encrypted or hardware-backed SSH keys are not supported for unattended controller operations.",
			"",
			"> "+model.setupKeyPath+"_",
		)
	} else {
		lines = append(lines, "")
		if model.setupKeys.State == "ready" {
			lines = append(lines, tuiStatus("All controller keys are ready", tuiStatusSuccess, model.isDark))
		} else {
			lines = append(lines,
				"Choose a missing key, then import an existing private key; or create all missing keys.",
			)
		}
	}
	actions := []tuiAction{}
	if model.setupKeyImporting {
		actions = []tuiAction{{key: "Enter", label: "Import"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else if model.setupKeys.State == "ready" {
		actions = []tuiAction{{key: "Enter", label: "Save and continue"}, {key: "Esc", label: "Setup"}, {key: "F1", label: "Help"}}
	} else {
		actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "i", label: "Import"}, {key: "c", label: "Create missing"}, {key: "Esc", label: "Setup"}, {key: "F1", label: "Help"}}
	}
	shell := tuiShell{
		path:    []string{"Installation", "Setup", "Controller keys"},
		body:    strings.Join(lines, "\n"),
		actions: actions,
	}
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusNeutral, title: model.message}}
	}
	return renderTUIShell(shell, model.width, model.isDark)
}

func (model dashboardModel) selectedSetupKey() (domain.KeyMaterialState, bool) {
	name := model.selectedSetupKeyName()
	for _, state := range model.setupKeys.Keys {
		if state.Name == name {
			return state, true
		}
	}
	return domain.KeyMaterialState{Name: name}, name != ""
}

func (model dashboardModel) selectedSetupKeyName() string {
	names := []string{"cache", "ssh", "veyon"}
	if model.setupKeyCursor < 0 || model.setupKeyCursor >= len(names) {
		return ""
	}
	return names[model.setupKeyCursor]
}

func setupKeyShortLabel(name string) string {
	switch name {
	case "cache":
		return "cache-signing"
	case "ssh":
		return "administrator SSH"
	case "veyon":
		return "Veyon"
	default:
		return "controller"
	}
}

func (model dashboardModel) setupKeyActionsAvailable() bool {
	return model.actions.ReconcileSetupKeys != nil && model.actions.SaveSetupConfiguration != nil && model.actions.InstallSetupSecrets != nil
}

func (model dashboardModel) prepareSetupKeys() tea.Cmd {
	return func() tea.Msg {
		keys, err := model.actions.ReconcileSetupKeys()
		if err != nil || keys.State != "ready" {
			return dashboardSetupKeysMsg{keys: keys, keyErr: err}
		}
		save := model.actions.SaveSetupConfiguration()
		if save.HasErrors() {
			return dashboardSetupKeysMsg{keys: keys, save: save}
		}
		return dashboardSetupKeysMsg{keys: keys, save: save, install: model.actions.InstallSetupSecrets()}
	}
}

func firstValidationIssue(issues []domain.ValidationIssue) string {
	if len(issues) == 0 {
		return "Technical detail unavailable."
	}
	return issues[0].Message
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
		return "Save the generated configuration locally"
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
		success := !model.updateResult.HasErrors() && model.updateResult.Updated && model.controllerResult.Operation != "" && !model.controllerResult.HasErrors() && model.controllerResult.Applied && model.controllerResult.Verified
		title := "Nixorium update needs attention"
		if success {
			title = "Nixorium and this controller are updated"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Configuration updated: %t", model.updateResult.State, model.updateResult.Updated),
			"Configured target: "+model.updateResult.Target,
			"Running interface: "+displayRunningVersion(model.actions.RunningVersion),
			fmt.Sprintf("Controller activated and verified: %t", success),
			"Client computers are unchanged until you distribute the prepared system.",
		)
		if success {
			lines = append(lines, "Reopen Nixorium to use the updated interface.")
		} else if model.updateResult.Updated {
			lines = append(lines, "The update is saved safely. Retry controller activation after resolving the detail below.")
		}
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		retryLabel := "new update"
		if model.updateResult.RecoveryRequired {
			retryLabel = "complete save"
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"a"}, "a", "retry controller apply"),
			tuiHelpBinding([]string{"r"}, "r", retryLabel),
			tuiHelpBinding([]string{"enter"}, "enter", "dashboard"),
		))
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardUpdateReview {
		return model.releaseReviewView()
	}
	if model.updateCheck.HasErrors() {
		lines = append(lines,
			tuiResult("Updates could not be fetched", false, model.isDark),
			"",
			"Nixorium could not obtain a usable update list from the configured upstream.",
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
		fmt.Sprintf("Configured target   %s", model.updateCheck.CurrentRef),
		"Running interface   "+displayRunningVersion(model.actions.RunningVersion),
		tuiMuted("Source  "+model.updateCheck.Upstream, model.isDark),
		"",
		tuiSection("Available updates", model.isDark),
	)
	start, end := listWindow(len(releases), model.updateCursor, max(4, model.height-15))
	latestStable := ""
	if len(model.updateCheck.Stable) > 0 {
		latestStable = model.updateCheck.Stable[0].Tag
	}
	for index := start; index < end; index++ {
		release := releases[index]
		marker := "  "
		if index == model.updateCursor {
			marker = "› "
		}
		note := ""
		if release.Tag == model.updateCheck.CurrentRef {
			note = "  Current"
		} else if release.Tag == latestStable {
			note = "  Latest stable"
		}
		if release.Channel == domain.UpdateChannelPrerelease {
			note += "  Prerelease"
		} else if release.Channel == domain.UpdateChannelMoving {
			note += "  Development branch"
		}
		lines = append(lines, marker+release.Tag+tuiMuted(note, model.isDark))
	}
	if len(releases) == 0 {
		lines = append(lines, "No updates are available in the selected channel.")
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
		"Select master for the latest development revision, or choose a tagged release.",
		"Selection starts validation; it does not change files.",
		"A confirmed update activates this controller; client distribution remains separate.",
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

func displayRunningVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

func (model dashboardModel) availableUpdateReleases() []domain.UpdateRelease {
	releases := append([]domain.UpdateRelease{}, model.updateCheck.Development...)
	releases = append(releases, model.updateCheck.Stable...)
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

func (model dashboardModel) startUpdateControllerApply() (tea.Model, tea.Cmd) {
	if model.actions.PlanController == nil || model.actions.ApplyController == nil {
		model.message = "Controller activation is not available. The validated update remains saved."
		return model, nil
	}
	model.busy = "Building, activating, and verifying the updated controller"
	model.updating = true
	model.controllerPlan = domain.ControllerRebuildPlanReport{}
	model.controllerResult = domain.ControllerRebuildExecutionReport{}
	return model, func() tea.Msg {
		plan := model.actions.PlanController()
		if plan.HasErrors() {
			return dashboardUpdateControllerMsg{plan: plan}
		}
		return dashboardUpdateControllerMsg{plan: plan, report: model.actions.ApplyController(plan)}
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
	path := []string{"Maintenance", "Controller"}
	lines := []string{tuiTitle("Controller configuration", model.isDark)}
	notices := []tuiNotice{}
	if model.controllerApplying {
		elapsed := time.Since(model.controllerStarted).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines = append(lines, "", fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		lines = append(lines, model.operationProgressView(model.controllerProgress, "Current progress")...)
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Controller update is running", detail: "Wait for the verified result before closing Nixorium."})
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "l", label: "Progress details"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}}, model.width, model.isDark)
	}
	if model.screen == dashboardControllerReview {
		body := strings.Join([]string{
			tuiTitle("Update this controller?", model.isDark),
			"",
			"Affects   " + model.controllerPlan.Controller + " (this controller only)",
			"Revision  " + model.controllerPlan.Revision,
			"",
			tuiSection("Type "+model.controllerPlan.Confirmation+" to continue:", model.isDark),
			"> " + model.confirmation + "_",
		}, "\n")
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Services and networking may restart", detail: "This connection may be interrupted. Nixorium builds, activates and verifies the reviewed configuration; a reboot is not normally required."})
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return renderTUIShell(tuiShell{path: append(path, "Review"), body: body, notices: notices, actions: []tuiAction{{key: "Enter", label: "Update controller"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
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
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		detailsLabel := "Show details"
		if model.controllerDetails {
			detailsLabel = "Hide details"
		}
		returnLabel := "Maintenance"
		if model.setupMode {
			returnLabel = "Setup"
		}
		actions := []tuiAction{{key: "Enter", label: returnLabel}, {key: "d", label: detailsLabel}, {key: "l", label: "Logs"}, {key: "r", label: "New review"}, {key: "F1", label: "Help"}}
		return renderTUIShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: actions}, model.width, model.isDark)
	}
	lines = append(lines,
		"",
		"Review the current committed controller configuration before rebuilding.",
	)
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "r", label: "Create review"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
}

func (model dashboardModel) servicesView() string {
	path := []string{"Maintenance", "Services"}
	lines := []string{tuiTitle("Managed services", model.isDark)}
	notices := []tuiNotice{}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}}, model.width, model.isDark)
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
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return renderTUIShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Enter", label: "Maintenance"}, {key: "l", label: "Logs"}, {key: "r", label: "Restart again"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
	}
	if model.screen == dashboardServicesRestartReview {
		body := strings.Join([]string{
			tuiTitle("Restart the software cache?", model.isDark),
			"",
			"Affects  Controller cache; active installations may be affected",
			"",
			tuiSection("Type RESTART CACHE to continue:", model.isDark),
			"> " + model.confirmation + "_",
		}, "\n")
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "The signed cache will be briefly unavailable", detail: "Active installers may retry downloads. PXE networking and listeners are unchanged; the cache is verified afterward."})
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return renderTUIShell(tuiShell{path: append(path, "Review"), body: body, notices: notices, actions: []tuiAction{{key: "Enter", label: "Restart cache"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
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
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if model.serviceRestartAvailable() {
		actions = append(actions, tuiAction{key: "r", label: "Review cache restart"})
	}
	actions = append(actions, tuiAction{key: "f", label: "Refresh"}, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
	return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions}, model.width, model.isDark)
}

func (model dashboardModel) serviceRestartAvailable() bool {
	return len(model.services.Services) > 0 && model.services.Services[0].ID == "cache" && len(model.services.Services[0].Units) > 0 && model.services.Services[0].Units[0].Loaded
}

func (model dashboardModel) logsView() string {
	path := []string{"Maintenance", "History"}
	lines := []string{tuiTitle("Operation history", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}}, model.width, model.isDark)
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
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if len(model.logs.Logs) > 0 {
		actions = append(actions, tuiAction{key: "↑/↓", label: "Select"}, tuiAction{key: "Enter", label: "View tail"})
	}
	back := "Maintenance"
	if model.setupMode {
		back = "Setup"
	}
	actions = append(actions, tuiAction{key: "f", label: "Refresh"}, tuiAction{key: "Esc", label: back}, tuiAction{key: "F1", label: "Help"})
	return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions}, model.width, model.isDark)
}

func (model dashboardModel) logDetailView() string {
	path := []string{"Maintenance", "History", "Log"}
	lines := []string{tuiTitle("Operation log detail", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}}, model.width, model.isDark)
	}
	if model.logDetail.Log == nil {
		lines = append(lines, "The selected operation log could not be read safely.")
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Esc", label: "History"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
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
	return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "↑/↓/Pg/Home/End", label: "Scroll"}, {key: "Esc", label: "History"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
}

func (model dashboardModel) logDetailHeight() int {
	if model.height <= 0 {
		return 12
	}
	height := model.height - 15
	if height < 1 {
		return 1
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
	path := []string{"Computers", "Distribute"}
	if model.restoreMode {
		path = []string{"Computers", "Restore", "Reapply"}
	}
	shell := tuiShell{path: path}
	if model.deploying {
		elapsed := time.Since(model.deployStarted).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines := []string{
			tuiTitle("Distributing the prepared system", model.isDark),
			"",
			fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed),
		}
		lines = append(lines, model.deploymentProgressView()...)
		shell.body = strings.Join(lines, "\n")
		shell.notices = []tuiNotice{{
			kind:   tuiStatusAttention,
			title:  "Deployment is running",
			detail: "Detailed output is saved in the private log. Closing is disabled until this foreground operation returns.",
		}}
		shell.actions = []tuiAction{{key: "l", label: "Progress details"}, {key: "F1", label: "Help"}}
		return renderTUIShell(shell, model.width, model.isDark)
	}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return renderTUIShell(shell, model.width, model.isDark)
	}
	if model.screen == dashboardDeployReview {
		lines := []string{
			tuiTitle("Distribute the system?", model.isDark),
			"",
			fmt.Sprintf("Affects  %s · %d computer(s)", model.deployPlan.ColmenaSelector, len(model.deployPlan.Targets)),
			"",
			tuiMuted("Reviewed revision  "+model.deployPlan.Revision, model.isDark),
			"",
			tuiSection("Type DEPLOY "+model.deployPlan.ColmenaSelector+" to continue:", model.isDark),
			"> " + model.confirmation + "_",
		}
		shell.body = strings.Join(lines, "\n")
		shell.notices = []tuiNotice{{
			kind:   tuiStatusAttention,
			title:  "Target services may restart; unreachable computers may remain unchanged",
			detail: "Every selected configuration is built first. A failed apply may leave mixed target state; a fresh full retry is safe.",
		}}
		if model.message != "" {
			shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Deploy"}, {key: "Esc", label: "Selection"}, {key: "F1", label: "Help"}}
		return renderTUIShell(shell, model.width, model.isDark)
	}

	if model.deployResult.Operation != "" {
		success := !model.deployResult.HasErrors()
		resultTitle := "Deployment needs attention"
		if success {
			resultTitle = "Deployment completed and verified"
		}
		lines := []string{
			tuiResult(resultTitle, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Phase: %s", model.deployResult.State, model.deployResult.Phase),
			fmt.Sprintf("Build complete: %t   Apply complete: %t", model.deployResult.BuildCompleted, model.deployResult.ApplyCompleted),
		}
		if model.deployResult.Verification.Attempted > 0 {
			lines = append(lines, fmt.Sprintf("Authenticated: %d/%d   Recorded: %d", model.deployResult.Verification.Verified, model.deployResult.Verification.Attempted, model.deployResult.Verification.Recorded))
		}
		if model.deployResult.LogPath != "" {
			lines = append(lines, "Detailed log: "+model.deployResult.LogPath)
		}
		if model.message != "" {
			shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
		}
		shell.body = strings.Join(lines, "\n")
		shell.actions = []tuiAction{{key: "r", label: "New review"}, {key: "l", label: "Logs"}, {key: "Enter", label: "Computers"}, {key: "?", label: "Help"}}
		return renderTUIShell(shell, model.width, model.isDark)
	}

	lines := []string{
		tuiTitle("Distribute the prepared system", model.isDark),
		tuiMuted("Select → Review → Deploy → Verify", model.isDark),
		"",
	}
	hosts := model.report.Meta.Clients.Hosts
	selected := 0
	for _, host := range hosts {
		if model.deployChosen[host.Name] {
			selected++
		}
	}
	lines = append(lines, "Choose where to apply the saved configuration.", "", fmt.Sprintf("%d of %d computers selected", selected, len(hosts)), "")
	start, end := listWindow(len(hosts), model.deployCursor, max(3, model.height-20))
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
	if model.message != "" {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	shell.body = strings.Join(lines, "\n")
	shell.actions = []tuiAction{{key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Computers"}, {key: "?", label: "Help"}}
	return renderTUIShell(shell, model.width, model.isDark)
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
		return lines
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
	path := []string{"Installation", "Network installation"}
	title := "Network installation"
	if model.setupMode {
		path = []string{"Installation", "First computer"}
		title = "Install the first computer"
	} else if model.restoreMode {
		path = []string{"Computers", "Restore", "Reinstall"}
		title = "Reinstall a computer"
	}
	lines := []string{
		tuiTitle(title, model.isDark),
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
		return renderTUIShell(tuiShell{
			path: path,
			body: strings.Join(lines, "\n"),
			notices: []tuiNotice{{
				kind:   tuiStatusAttention,
				title:  "Preparation continues if this view closes",
				detail: "The systemd-owned operation is recorded and can be inspected again later.",
			}},
			actions: model.pxeActions(),
		}, model.width, model.isDark)
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return renderTUIShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			actions: model.pxeActions(),
		}, model.width, model.isDark)
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
		body := strings.Join([]string{
			tuiTitle("Start network installation?", model.isDark),
			"",
			"Affects  " + scope,
			"",
			tuiSection("Type START PXE to continue:", model.isDark),
			"> " + model.confirmation + "_",
		}, "\n")
		notices := []tuiNotice{{
			kind:   tuiStatusAttention,
			title:  "Temporarily remove " + model.startPlan.StaticCIDR + "; remote connections may be interrupted",
			detail: "Serve ProxyDHCP, TFTP, HTTP and cache via " + model.startPlan.DHCPAddress + ". Institutional DHCP remains authoritative; stop or reboot recovery restores normal addressing.",
		}}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return renderTUIShell(tuiShell{path: path, body: body, notices: notices, actions: model.pxeActions()}, model.width, model.isDark)
	}
	if model.screen == dashboardPXELeaveReview {
		leaveLines := []string{
			tuiTitle("Exit while installation mode is active?", model.isDark),
			"Closing Nixorium will not stop installation mode.",
			"It changes controller networking and can continue after Nixorium closes.",
			"Configured computers may continue to network-boot into the installer.",
			"",
			tuiSection("Recommended", model.isDark),
			"  x  Stop installation mode, verify normal networking, and exit",
			"",
			tuiSection("Keep it active", model.isDark),
			"  Type LEAVE PXE ACTIVE to continue:",
			"> " + model.confirmation + "_",
		}
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return renderTUIShell(tuiShell{path: path, body: strings.Join(leaveLines, "\n"), notices: notices, actions: model.pxeActions()}, model.width, model.isDark)
	}
	if model.guidedInstallation() {
		lines = append(lines, "")
		lines = append(lines, model.pilotInstallationView()...)
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
		}
		return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: model.pxeActions()}, model.width, model.isDark)
	}
	lines = append(lines, "")
	lines = append(lines, model.pxeNextStepView()...)
	if model.pxeProgress.Operation != "" {
		lines = append(lines, "")
		lines = append(lines, model.operationProgressView(model.pxeProgress, "Last preparation")...)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: model.pxeActions()}, model.width, model.isDark)
}

func (model dashboardModel) pxeActions() []tuiAction {
	if model.screen == dashboardPXEStartReview {
		return []tuiAction{{key: "Enter", label: "Start PXE"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	if model.screen == dashboardPXELeaveReview {
		return []tuiAction{{key: "x", label: "Stop and exit"}, {key: "Enter", label: "Leave active"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	if model.pxePreparing {
		return []tuiAction{{key: "l", label: "Progress details"}, {key: "q", label: "Close view"}, {key: "F1", label: "Help"}}
	}
	if model.busy != "" {
		return []tuiAction{{key: "q", label: "Close view"}, {key: "F1", label: "Help"}}
	}
	if model.guidedInstallation() {
		recovery := model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required"
		if recovery {
			return []tuiAction{{key: "r", label: "Recover"}, {key: "Esc", label: "Back"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}}
		}
		if model.pilotName == "" {
			if model.installationSummary {
				return []tuiAction{{key: "Enter", label: model.installationChangeAnotherLabel()}, {key: "Esc", label: "Setup summary"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}}
			}
			if len(model.report.Meta.Clients.Hosts) == 0 {
				return []tuiAction{{key: "Esc", label: "Back"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}}
			}
			return []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Choose"}, {key: "Esc", label: "Back"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}}
		}
		if model.report.PXE.Mode != "active" {
			primary := tuiAction{key: "p", label: "Prepare"}
			if model.report.PXEPreparation.Ready {
				primary = tuiAction{key: "s", label: "Review start"}
			}
			return []tuiAction{primary, {key: "Esc", label: model.installationChangeLabel()}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}}
		}
		if !model.pilotTechnicallyVerified() {
			return []tuiAction{{key: "v", label: "Check computer"}, {key: "x", label: "Stop PXE"}, {key: "Esc", label: model.installationChangeLabel()}, {key: "q", label: "Leave active"}, {key: "F1", label: "Help"}}
		}
		if !model.pilotPractical {
			return []tuiAction{{key: "Enter", label: "Practical check passed"}, {key: "v", label: "Check again"}, {key: "x", label: "Stop PXE"}, {key: "q", label: "Leave active"}, {key: "F1", label: "Help"}}
		}
		return []tuiAction{{key: "Enter", label: model.installationChangeAnotherLabel()}, {key: "x", label: "Stop and finish"}, {key: "q", label: "Leave active"}, {key: "F1", label: "Help"}}
	}
	actions := []tuiAction{}
	recovery := model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required"
	if model.report.PXE.Mode != "active" && !recovery {
		actions = append(actions, tuiAction{key: "p", label: "Prepare"})
		if model.report.PXEPreparation.Ready {
			actions = append(actions, tuiAction{key: "s", label: "Start PXE"})
		}
	}
	if model.report.PXE.Mode == "active" || recovery {
		actions = append(actions, tuiAction{key: "x", label: "Stop PXE"})
	}
	backLabel := "Installation"
	if model.restoreMode {
		backLabel = "Computers"
	}
	return append(actions,
		tuiAction{key: "r", label: "Recover"},
		tuiAction{key: "Esc", label: backLabel},
		tuiAction{key: "q", label: "Quit"},
		tuiAction{key: "F1", label: "Help"},
	)
}

func (model dashboardModel) guidedInstallation() bool {
	return model.setupMode || model.restoreMode
}

func (model *dashboardModel) applyInstallationSession() {
	model.hosts = domain.HostsReport{}
	model.pilotVerified = nil
	model.pilotPractical = false
	model.installationSummary = false
	if model.installationSession.Observation != nil {
		model.hosts = domain.HostsReport{Hosts: []domain.HostStatus{*model.installationSession.Observation}}
	}
	if model.installationSession.State == "none" || model.installationSession.State == "stale" || model.installationSession.HasErrors() {
		model.pilotName = ""
		return
	}
	model.pilotName = model.installationSession.Selected
	for _, evidence := range model.installationSession.Evidence {
		if evidence.PracticalConfirmedAt != nil {
			model.pilotVerified = append(model.pilotVerified, evidence.Name)
		}
		if evidence.Name == model.pilotName && evidence.Revision == model.installationSession.Revision && evidence.PracticalConfirmedAt != nil {
			model.pilotPractical = true
		}
	}
	if model.report.PXE.Mode != "active" && model.pilotPractical {
		model.pilotName = ""
		model.pilotPractical = false
		model.installationSummary = true
		model.hosts = domain.HostsReport{}
	}
}

func (model dashboardModel) pilotInstallationView() []string {
	recovery := model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required"
	if recovery {
		return []string{
			tuiResult("Controller networking needs recovery", false, model.isDark),
			"Nixorium cannot safely continue the installation until normal addressing is reconciled.",
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
		return lines
	}

	host, observed := model.pilotHostStatus()
	technicallyVerified := model.pilotTechnicallyVerified()
	if !observed && !technicallyVerified {
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
		)
		return lines
	}

	if observed {
		level, label, guidance := domain.ComputerCondition(host)
		lines = append(lines, "", tuiStatus(label, statusLevel(level), model.isDark))
		if !technicallyVerified {
			lines = append(lines,
				guidance,
				"This does not prove that installation completed. Finish the local steps, boot from disk, then check again.",
			)
			return lines
		}
	}

	if !model.pilotPractical {
		verificationCopy := "Authenticated management reports the saved revision as active."
		if !observed {
			verificationCopy = "Authenticated technical evidence was restored from this session. Press v to check current state again."
		}
		lines = append(lines,
			tuiResult("Technical verification succeeded", true, model.isDark),
			verificationCopy,
			"",
			tuiSection("Check at the computer", model.isDark),
			"  • Log in and open the expected desktop session.",
			"  • Check required software, network and classroom peripherals.",
			"  • Confirm that the computer started from its installed disk.",
		)
		return lines
	}

	lines = append(lines,
		"",
		tuiResult(model.installationVerifiedTitle(), true, model.isDark),
		model.installationRemainingGuidance(),
		"Powered-off computers are not errors.",
	)
	return lines
}

func (model dashboardModel) pilotSelectionView() []string {
	lines := []string{}
	if model.installationSummary {
		lines = append(lines,
			tuiResult(model.installationSessionTitle(), true, model.isDark),
			"Verified in this session: "+strings.Join(model.pilotVerified, ", "),
			fmt.Sprintf("%d configured identities were not verified in this session.", max(0, len(model.report.Meta.Clients.Hosts)-len(model.pilotVerified))),
			"",
			"You can return later to install the remaining computers.",
		)
		return lines
	}

	selectionTitle := "Choose a pilot computer"
	if model.restoreMode {
		selectionTitle = "Choose a computer to reinstall"
	}
	lines = append(lines, tuiSection(selectionTitle, model.isDark))
	if model.installationSession.State == "stale" {
		lines = append(lines,
			tuiStatus("Saved session is out of date", tuiStatusAttention, model.isDark),
			model.installationSession.Message,
			"",
		)
	} else if model.installationSession.HasErrors() {
		lines = append(lines,
			tuiStatus("Saved session could not be restored", tuiStatusFailure, model.isDark),
			model.installationSession.Message,
			"Select a computer to retry after correcting the state-file problem.",
			"",
		)
	}
	if len(model.report.Meta.Clients.Hosts) == 0 {
		return append(lines,
			"No client identity is configured.",
			"Return to laboratory settings and add at least one computer.",
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

func (model dashboardModel) installationChangeAnotherLabel() string {
	if model.restoreMode {
		return "reinstall another"
	}
	return "install another"
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
	if found {
		return host.SSH == domain.SSHAvailable &&
			host.Deployment == domain.DeploymentCurrent &&
			host.CurrentRevision == model.installationSession.Revision &&
			host.CurrentSystem != ""
	}
	for _, evidence := range model.installationSession.Evidence {
		if evidence.Name == model.pilotName && evidence.Revision == model.installationSession.Revision {
			return true
		}
	}
	return false
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
