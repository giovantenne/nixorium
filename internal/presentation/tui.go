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
	RunningVersion         string
	LoadInitial            func() (domain.StatusReport, domain.SetupReport, error)
	LoadDoctor             func() (domain.DoctorReport, error)
	Refresh                func() (domain.StatusReport, error)
	LoadSetup              func() domain.SetupReport
	LoadSetupKeys          func() domain.KeyReconcileReport
	ReconcileSetupKeys     func() (domain.KeyReconcileReport, error)
	ImportSetupKey         func(string, string) (domain.KeyImportReport, error)
	SaveSetupConfiguration func() domain.ConfigurationSaveReport
	InstallSetupSecrets    func() domain.ActionReport
	LoadHosts              func() (domain.HostsReport, error)
	LoadConfigurationState func() domain.ConfigurationStateReport
	LoadSoftware           func() domain.SoftwareCatalogReport
	SearchSoftware         func(context.Context, string) domain.SoftwareSearchReport
	PlanSoftware           func(domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport
	SaveSoftware           func(domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport
	LoadSoftwarePresets    func() domain.SoftwarePresetCatalogReport
	PlanSoftwarePreset     func(domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport
	SaveSoftwarePreset     func(domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport
	PlanShutdown           func(string, domain.ShutdownSessionPolicy) domain.ShutdownPlanReport
	ApplyShutdown          func(domain.ShutdownPlanReport) domain.ShutdownApplyReport
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
	PlanUpdateWithProgress func(string, bool, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport
	SaveUpdate             func(domain.UpdatePlanReport) domain.UpdateApplyReport
	LoadPackageBase        func() domain.PackageBaseStatus
	PlanPackageBase        func(string, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport
	SavePackageBase        func(domain.UpdatePlanReport) domain.UpdateApplyReport
	LoadSettings           func() (domain.LabSettingsFile, error)
	PlanSettings           func(domain.LabSettingsFile) domain.ConfigPlanReport
	SaveSettings           func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport
	ChangePassword         SettingsPasswordAction
	PreparePXE             func() domain.ActionReport
	LoadPXEProgress        func() (domain.OperationProgress, error)
	PlanPXEStart           func() domain.PXELifecycleReport
	StartPXE               func() domain.PXELifecycleReport
	StopPXE                func() domain.PXELifecycleReport
	RecoverPXE             func() domain.PXELifecycleReport
	PrepareRemoteInstall   func(string) (domain.RemoteInstallResponse, error)
	ObserveRemoteInstall   func(string) (string, error)
	BootstrapRemoteInstall func(string, string, string, []byte) (domain.RemoteInstallResponse, error)
	RemoteInstallRequest   func(domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error)
	LoadRemoteInstall      func() (domain.RemoteInstallResponse, error)
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
	dashboardInstallMethod
	dashboardUSBInstall
	dashboardAdministration
	dashboardDiagnostics
	dashboardSoftware
	dashboardShutdown
	dashboardShutdownReview
	dashboardShutdownResult
)

// deploymentModel owns target selection and the lifecycle of one reviewed
// client deployment. The dashboard root only routes its messages and effects.
type deploymentModel struct {
	cursor       int
	chosen       map[string]bool
	plan         domain.DeploymentPlanReport
	result       domain.DeploymentExecutionReport
	context      string
	applying     bool
	progress     domain.DeploymentProgress
	recent       []string
	started      time.Time
	events       <-chan tea.Msg
	confirmation string
	usbRecovery  *deploymentUSBRecovery
	usbRequestID uint64
}

// controllerModel is the shared activation boundary used after settings,
// software and update saves. It owns job identity so stale progress messages
// cannot be mistaken for the current activation.
type controllerModel struct {
	plan       domain.ControllerRebuildPlanReport
	result     domain.ControllerRebuildExecutionReport
	applying   bool
	progress   domain.OperationProgress
	started    time.Time
	progressID uint64
	details    bool
}

// settingsModel owns the settings editor and its reviewed local save. The
// controller activation that may follow remains a separate shared model.
type settingsModel struct {
	current          domain.LabSettingsFile
	candidate        domain.LabSettingsFile
	menu             routineSettingsMenu
	passwordMenu     routinePasswordMenu
	editor           settingsWizardModel
	plan             domain.ConfigPlanReport
	result           domain.ConfigurationSaveReport
	applying         bool
	returnScreen     dashboardScreen
	collectPasswords bool
}

// updateModel owns both upstream and package-base update state because both
// paths share the same reviewed plan, save result and controller follow-up.
type updateModel struct {
	check           domain.UpdateCheckReport
	packageBase     bool
	baseStatus      domain.PackageBaseStatus
	baseEditing     bool
	baseTarget      string
	allowUnverified bool
	cursor          int
	target          string
	prerelease      bool
	plan            domain.UpdatePlanReport
	result          domain.UpdateApplyReport
	scroll          int
	planning        bool
	planProgress    domain.UpdatePlanProgress
	planStarted     time.Time
	planEvents      <-chan tea.Msg
	applying        bool
}

// maintenanceModel owns the read-only service/log/repository views and the
// optional reviewed local commit state.
type maintenanceModel struct {
	services        domain.ServicesReport
	serviceResult   domain.ServiceActionReport
	logs            domain.OperationLogsReport
	logDetail       domain.OperationLogReport
	logCursor       int
	logScroll       int
	gitReview       domain.GitReviewReport
	gitScroll       int
	gitCommitCursor int
	gitCommitChosen map[string]bool
	gitCommitPlan   domain.GitCommitPlanReport
	gitCommitResult domain.GitCommitReport
}

// installationModel owns guided installation and PXE lifecycle state,
// including progress identity for delayed service messages.
type installationModel struct {
	startPlan        domain.PXELifecycleReport
	startingLabSetup bool
	flow             bool
	stage            int
	failed           bool
	pxePreparing     bool
	pxeProgress      domain.OperationProgress
	pxeStarted       time.Time
	pxeProgressID    uint64
	method           domain.RemoteInstallMethod
	methodCursor     int
	remote           remoteInstallationModel
}

type remoteInstallationStage int

const (
	remoteInstallSelectHost remoteInstallationStage = iota
	remoteInstallPreparing
	remoteInstallConsole
	remoteInstallFingerprint
	remoteInstallPassword
	remoteInstallBootstrap
	remoteInstallSelectDisk
	remoteInstallRotateHostKey
	remoteInstallReview
	remoteInstallApplying
	remoteInstallResult
	remoteInstallConfirmReboot
	remoteInstallConfirmClose
)

type remoteInstallationModel struct {
	returnToDeployment bool
	stage              remoteInstallationStage
	hostCursor         int
	host               string
	operationID        string
	address            string
	fingerprint        string
	password           string
	formField          int
	diskCursor         int
	confirmation       string
	recovery           bool
	bootstrapError     string
	response           domain.RemoteInstallResponse
	plan               domain.RemoteInstallPlanReport
}

// computersModel owns inventory, filtering, detail and restore navigation.
// Deployment and shutdown remain separate because they have independent jobs.
type computersModel struct {
	areaCursor         int
	restoreMode        bool
	restoreCursor      int
	hostCursor         int
	hostQuery          string
	hostSearching      bool
	hostDetail         bool
	hostTechnical      bool
	configurationState domain.ConfigurationStateReport
	hosts              domain.HostsReport
}

type dashboardModel struct {
	updateDetails          bool
	returnAdmin            bool
	computers              computersModel
	installationAreaCursor int
	areaReturn             dashboardScreen
	diagnosticReturn       dashboardScreen
	adminCursor            int
	helpOpen               bool
	pageScroll             int
	setupDetails           bool
	setupKeys              domain.KeyReconcileReport
	setupKeysReturn        dashboardScreen
	setupKeyCursor         int
	setupKeyImporting      bool
	setupKeyPath           string
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
	installation           installationModel
	deployment             deploymentModel
	controller             controllerModel
	maintenance            maintenanceModel
	updates                updateModel
	settings               settingsModel
	software               softwareModel
	shutdown               shutdownModel
	width                  int
	height                 int
	isDark                 bool
	activitySpinner        spinner.Model

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

type dashboardInstallationPrepareMsg struct {
	report    domain.ActionReport
	status    domain.StatusReport
	statusErr error
}

type dashboardUpdatePlanProgressMsg struct {
	progress domain.UpdatePlanProgress
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

type dashboardRemoteInstallMsg struct {
	action   string
	response domain.RemoteInstallResponse
	err      error
}

type dashboardRemoteFingerprintMsg struct {
	fingerprint string
	err         error
}

type dashboardHostsMsg struct {
	report domain.HostsReport
	err    error
}

type dashboardConfigurationStateMsg struct {
	report domain.ConfigurationStateReport
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
type dashboardSoftwarePresetCatalogMsg struct {
	report domain.SoftwarePresetCatalogReport
}
type dashboardSoftwarePresetPlanMsg struct {
	report domain.SoftwarePresetPlanReport
}
type dashboardSoftwarePresetApplyMsg struct {
	report domain.SoftwarePresetApplyReport
}
type dashboardSoftwareControllerMsg struct {
	plan   domain.ControllerRebuildPlanReport
	report domain.ControllerRebuildExecutionReport
}
type dashboardShutdownPlanMsg struct{ report domain.ShutdownPlanReport }
type dashboardShutdownApplyMsg struct{ report domain.ShutdownApplyReport }

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
		homeMenu: newDashboardTaskMenu(false, 80, 24), activitySpinner: newTUISpinner(false), shutdown: newShutdownModel(),
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
	if model.returnAdmin && model.screen != dashboardAdministration && next.screen == dashboardHome {
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
	model.settings.returnScreen = dashboardSetup
	model.settings.collectPasswords = model.setup.CurrentStage != domain.SetupStageValidate
	model.settings.result = domain.ConfigurationSaveReport{}
	model.settings.plan = domain.ConfigPlanReport{}
	model.settings.candidate = domain.LabSettingsFile{}
	model.busy = "Loading laboratory settings"
	model.message = ""
	return model, func() tea.Msg {
		settings, err := model.actions.LoadSettings()
		return dashboardSettingsMsg{settings: settings, err: err}
	}
}

func (model dashboardModel) startComputerInstallation() (tea.Model, tea.Cmd) {
	remote := model.installation.remote
	model.installation = installationModel{flow: true, remote: remote}
	model.screen = dashboardInstallMethod
	model.message = ""
	model.busy = ""
	if model.actions.LoadRemoteInstall == nil {
		return model, nil
	}
	model.busy = "Checking for an existing USB installation operation"
	return model, func() tea.Msg {
		response, err := model.actions.LoadRemoteInstall()
		return dashboardRemoteInstallMsg{action: "probe", response: response, err: err}
	}
}

func (model dashboardModel) beginComputerInstallation(method domain.RemoteInstallMethod) (tea.Model, tea.Cmd) {
	if model.actions.LoadSettings == nil {
		model.message = "Laboratory settings are not available in this session."
		return model, nil
	}
	model.installation.flow = true
	model.installation.method = method
	model.installation.stage = 0
	model.installation.failed = false
	model.setupMode = false
	model.installation.startingLabSetup = true
	model.settings.returnScreen = dashboardHome
	model.settings.collectPasswords = false
	model.settings.result = domain.ConfigurationSaveReport{}
	model.settings.plan = domain.ConfigPlanReport{}
	model.settings.candidate = domain.LabSettingsFile{}
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
	model.installation.pxeProgress = domain.OperationProgress{}
	model.areaReturn = dashboardHome
	model.screen = dashboardSettings
	model.busy = "Loading laboratory settings"
	model.message = ""
	return model, func() tea.Msg {
		settings, err := model.actions.LoadSettings()
		return dashboardSettingsMsg{settings: settings, err: err}
	}
}

func (model dashboardModel) failComputerInstallation(message string) (tea.Model, tea.Cmd) {
	model.busy = ""
	model.installation.failed = true
	model.controller.applying = false
	model.installation.pxePreparing = false
	model.message = message
	model.screen = dashboardPXE
	if model.installation.method == domain.RemoteInstallUSBSSH {
		model.screen = dashboardUSBInstall
		model.installation.remote.stage = remoteInstallResult
		model.installation.remote.response = domain.RemoteInstallResponse{
			OperationID: model.installation.remote.operationID, State: "failed", Message: message,
		}
	}
	return model, nil
}

func (model dashboardModel) continueComputerInstallation(report domain.SetupReport) (tea.Model, tea.Cmd) {
	model.setup = report
	model.screen = dashboardPXE
	model.message = ""
	switch report.CurrentStage {
	case domain.SetupStageKeys:
		if !model.setupKeyActionsAvailable() {
			return model.failComputerInstallation("Controller key preparation is not available in this session.")
		}
		model.installation.stage = 2
		model.busy = "Preparing and installing controller keys"
		return model, model.prepareSetupKeys()
	case domain.SetupStageReview:
		if model.actions.SaveSetupConfiguration == nil {
			return model.failComputerInstallation("Local configuration saving is not available in this session.")
		}
		model.installation.stage = 1
		model.busy = "Saving generated laboratory files locally"
		return model, func() tea.Msg {
			return dashboardSetupSaveMsg{report: model.actions.SaveSetupConfiguration()}
		}
	case domain.SetupStageApply:
		if model.actions.PlanController == nil || model.actions.ApplyController == nil {
			return model.failComputerInstallation("Controller activation is not available in this session.")
		}
		model.installation.stage = 3
		model.busy = "Checking the controller configuration"
		return model, func() tea.Msg {
			return dashboardControllerPlanMsg{report: model.actions.PlanController()}
		}
	case domain.SetupStageArtifacts:
		if model.installation.method == "" {
			return model.startComputerInstallation()
		}
		if model.installation.method == domain.RemoteInstallUSBSSH {
			return model.openRemoteInstallHostSelection()
		}
		return model.startComputerInstallationPreparation()
	case "":
		if model.installation.method == "" {
			return model.startComputerInstallation()
		}
		if model.installation.method == domain.RemoteInstallUSBSSH {
			return model.openRemoteInstallHostSelection()
		}
		if model.actions.PlanPXEStart == nil {
			return model.failComputerInstallation("PXE start validation is not available in this session.")
		}
		model.installation.stage = 4
		model.busy = "Checking PXE readiness"
		return model, func() tea.Msg {
			return dashboardPlanMsg{report: model.actions.PlanPXEStart()}
		}
	default:
		return model.failComputerInstallation("Laboratory configuration is still incomplete: " + setupCurrentAction(report) + ".")
	}
}

func (model dashboardModel) startComputerInstallationPreparation() (tea.Model, tea.Cmd) {
	if model.actions.PreparePXE == nil {
		return model.failComputerInstallation("Client preparation is not available in this session.")
	}
	model.installation.stage = 4
	model.busy = "Preparing client systems and network installation files"
	model.installation.pxePreparing = true
	model.installation.pxeProgress = domain.OperationProgress{}
	model.installation.pxeStarted = time.Now().UTC()
	model.installation.pxeProgressID++
	model.message = ""
	operation := func() tea.Msg {
		report := model.actions.PreparePXE()
		status := domain.StatusReport{}
		var err error
		if model.actions.Refresh != nil {
			status, err = model.actions.Refresh()
		}
		return dashboardInstallationPrepareMsg{report: report, status: status, statusErr: err}
	}
	return model, tea.Batch(operation, schedulePXEProgressTick(model.installation.pxeProgressID))
}

func (model dashboardModel) returnFromSettings() (tea.Model, tea.Cmd) {
	returnTo := model.settings.returnScreen
	model.settings.returnScreen = dashboardHome
	model.settings.collectPasswords = false
	model.message = ""
	if returnTo == dashboardSetup {
		model.screen = dashboardSetup
		if model.actions.LoadSetup != nil && (model.settings.result.State == "saved" || model.settings.result.State == "unchanged") {
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

func startUpdatePlan(action func(string, bool, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport, target string, allowPrerelease, allowDowngrade bool, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			report := action(target, allowPrerelease, allowDowngrade, func(progress domain.UpdatePlanProgress) {
				events <- dashboardUpdatePlanProgressMsg{progress: progress}
			})
			events <- dashboardUpdatePlanMsg{report: report}
			close(events)
		}()
		return <-events
	}
}

func waitForUpdatePlanEvent(events <-chan tea.Msg) tea.Cmd {
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

func (model dashboardModel) loadConfigurationState() tea.Cmd {
	return func() tea.Msg {
		return dashboardConfigurationStateMsg{report: model.actions.LoadConfigurationState()}
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
	case dashboardSoftware:
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
	case dashboardInstallMethod:
		content = model.installMethodView()
	case dashboardUSBInstall:
		content = model.remoteInstallView()
	default:
		content = model.homeView()
	}
	view := tea.NewView(content)
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
			tuiResult("Controller and client systems are ready", true, model.isDark),
			"Enter opens network installation. Any configured computer can boot the installer.",
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
	return model.renderShell(shell)
}

func (model dashboardModel) setupKeysView() string {
	path := []string{"Installation", "Setup", "Controller keys"}
	backLabel := "Setup"
	readyLabel := "Save and continue"
	if model.setupKeysReturn == dashboardSettings {
		path = []string{"Maintenance", "Settings", "Advanced", "Controller keys"}
		backLabel = "Settings"
		readyLabel = "Install verified keys"
	}
	lines := []string{
		tuiTitle("Controller keys", model.isDark),
		"These keys authenticate the controller. Private keys stay local and are never added to configuration history.",
		"Existing valid keys are reused and never replaced by this workflow.",
		"",
	}
	if model.busy != "" {
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(append(lines, model.busyView()), "\n"),
			actions: []tuiAction{{key: "F1", label: "Help"}},
		})
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
		state := states[definition.name]
		status := "Action required"
		if state.Ready() {
			status = "Existing key — ready"
		} else if state.Problem != "" {
			status += " — " + state.Problem
		}
		lines = append(lines, tuiSelection(definition.label+"  ·  "+status, index == model.setupKeyCursor, model.isDark), "    "+definition.purpose)
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
	var actions []tuiAction
	if model.setupKeyImporting {
		actions = []tuiAction{{key: "Enter", label: "Import"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else if model.setupKeys.State == "ready" {
		actions = []tuiAction{{key: "Enter", label: readyLabel}, {key: "Esc", label: backLabel}, {key: "F1", label: "Help"}}
	} else {
		actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "i", label: "Import"}, {key: "c", label: "Create missing"}, {key: "Esc", label: backLabel}, {key: "F1", label: "Help"}}
	}
	shell := tuiShell{
		path:    path,
		body:    strings.Join(lines, "\n"),
		actions: actions,
	}
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusNeutral, title: model.message}}
	}
	return model.renderShell(shell)
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
		current = len(groups) - 1
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
	path := []string{"Maintenance", "Changes"}
	lines := []string{tuiTitle("Repository changes", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.maintenance.gitCommitResult.Operation != "" {
		success := !model.maintenance.gitCommitResult.HasErrors() && model.maintenance.gitCommitResult.Committed
		title := "Git commit needs attention"
		if success {
			title = "Git changes committed locally"
		}
		returnLabel := "Maintenance"
		if model.setupMode {
			returnLabel = "Setup"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Committed: %t", model.maintenance.gitCommitResult.State, model.maintenance.gitCommitResult.Committed),
			"HEAD: "+model.maintenance.gitCommitResult.Revision,
		)
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Enter", label: returnLabel}, {key: "f", label: "Refresh review"}, {key: "F1", label: "Help"}}})
	}
	content := gitReviewContentLines(model.maintenance.gitReview)
	height := model.gitReviewHeight()
	maximum := maximumGitReviewScroll(model.maintenance.gitReview, height)
	if model.maintenance.gitScroll > maximum {
		model.maintenance.gitScroll = maximum
	}
	end := model.maintenance.gitScroll + height
	if end > len(content) {
		end = len(content)
	}
	lines = append(lines,
		fmt.Sprintf("State: %s   paths: %d   staged/unstaged/untracked: %d/%d/%d", tuiStatus(model.maintenance.gitReview.State, gitReviewStatusKind(model.maintenance.gitReview), model.isDark), len(model.maintenance.gitReview.Changes), model.maintenance.gitReview.Summary.Staged, model.maintenance.gitReview.Summary.Unstaged, model.maintenance.gitReview.Summary.Untracked),
		fmt.Sprintf("Managed/unexpected/private: %d/%d/%d", model.maintenance.gitReview.Summary.Managed, model.maintenance.gitReview.Summary.Unexpected, model.maintenance.gitReview.Summary.Private),
		fmt.Sprintf("Showing lines %d-%d of %d", displayedLineStart(model.maintenance.gitScroll, len(content)), end, len(content)),
		"",
	)
	lines = append(lines, content[model.maintenance.gitScroll:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{{key: "↑/↓/Pg", label: "Scroll"}}
	if len(model.maintenance.gitReview.Changes) > 0 && !model.maintenance.gitReview.HasErrors() {
		actions = append(actions, tuiAction{key: "c", label: "Select commit paths"})
	}
	actions = append(actions, tuiAction{key: "f", label: "Refresh"}, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) gitCommitSelectView() string {
	path := []string{"Maintenance", "Changes", "Select paths"}
	lines := []string{tuiTitle("Select paths for the local commit", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	start, end := listWindow(len(model.maintenance.gitReview.Changes), model.maintenance.gitCommitCursor, model.rowCapacity())
	for index := start; index < end; index++ {
		change := model.maintenance.gitReview.Changes[index]
		chosen := "[ ]"
		if model.maintenance.gitCommitChosen[change.Path] {
			chosen = "[x]"
		}
		if change.Private {
			chosen = "[!]"
		}
		row := fmt.Sprintf("%s %-10s %-10s %-9s %s", chosen, gitChangeOwnership(change), gitChangeIndex(change), gitChangeWorktree(change), change.Path)
		lines = append(lines, tuiSelection(row, index == model.maintenance.gitCommitCursor, model.isDark))
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Space", label: "Toggle"}, {key: "a", label: "All safe"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Changes"}, {key: "F1", label: "Help"}}})
}

func (model dashboardModel) gitCommitReviewView() string {
	path := []string{"Maintenance", "Changes", "Commit review"}
	lines := []string{tuiTitle("Review local commit", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	diffLines := strings.Split(strings.TrimSuffix(model.maintenance.gitCommitPlan.Diff.Content, "\n"), "\n")
	height := model.gitReviewHeight()
	maximum := maximumGitCommitPlanScroll(model.maintenance.gitCommitPlan, height)
	if model.maintenance.gitScroll > maximum {
		model.maintenance.gitScroll = maximum
	}
	end := model.maintenance.gitScroll + height
	if end > len(diffLines) {
		end = len(diffLines)
	}
	lines = append(lines,
		"Paths: "+strings.Join(model.maintenance.gitCommitPlan.Paths, ", "),
		"Message: "+model.maintenance.gitCommitPlan.CommitMessage,
		"No hooks, signing actions, remote operations, or push will run.",
		fmt.Sprintf("Diff lines %d-%d of %d", displayedLineStart(model.maintenance.gitScroll, len(diffLines)), end, len(diffLines)),
		"",
	)
	lines = append(lines, diffLines[model.maintenance.gitScroll:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{
		path:      path,
		body:      strings.Join(lines, "\n"),
		fixedBody: tuiSection("Type "+model.maintenance.gitCommitPlan.Confirmation+" to continue:", model.isDark) + "\n> " + model.confirmation + "_",
		notices:   notices,
		actions:   []tuiAction{{key: "↑/↓/Pg", label: "Scroll diff"}, {key: "Enter", label: "Create commit"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}},
	})
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
	path := []string{"Maintenance", model.updateTitle()}
	lines := []string{tuiTitle(model.updateTitle(), model.isDark), ""}
	if model.busy != "" {
		if model.updates.planning {
			lines = append(lines,
				"Target: "+model.updates.target,
				tuiMuted("Checking whether this version can replace the current one. Nothing is saved or activated yet.", model.isDark),
			)
			phaseLabels := updatePlanPhaseLabels()
			phaseIndex := updatePlanPhaseIndex(model.updates.planProgress.Phase)
			if model.height > 0 && model.height < 28 {
				lines = append(lines, "", fmt.Sprintf("Phase %d/%d · %s", phaseIndex+1, len(phaseLabels), phaseLabels[phaseIndex]))
			} else {
				lines = append(lines, phaseSteps(phaseLabels, phaseIndex, false, model.isDark)...)
			}
			elapsed := time.Since(model.updates.planStarted).Truncate(time.Second)
			if elapsed < 0 {
				elapsed = 0
			}
			model.busy = updatePlanProgressDescription(model.updates.planProgress)
			if model.updates.packageBase && model.updates.planProgress.Phase == domain.UpdatePlanPhaseLock {
				model.busy = "Preparing the selected system and package base"
			}
			lines = append(lines, "", fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
			if model.updates.planProgress.Total > 0 {
				lines = append(lines, fmt.Sprintf("Safety check %d/%d", model.updates.planProgress.Current, model.updates.planProgress.Total))
			}
		} else {
			lines = append(lines, model.busyView())
		}
		notices := []tuiNotice{}
		if model.updates.applying {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Update save is running", detail: "Wait for the atomic two-file result before closing Nixorium."})
		} else {
			notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: "The current deployment remains unchanged", detail: "No deployment files or running systems change during these checks. The controller may download or build software locally, so this can take several minutes."})
		}
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.updates.result.Operation != "" {
		success := !model.updates.result.HasErrors() && model.updates.result.Updated && model.controller.result.Operation != "" && !model.controller.result.HasErrors() && model.controller.result.Applied && model.controller.result.Verified
		title := "Nixorium update needs attention"
		if success {
			title = "Nixorium and this controller are updated"
		}
		if model.updates.packageBase {
			title = "System update needs attention"
			if success {
				title = "Controller configuration activated and verified"
			}
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Configuration updated: %t", model.updates.result.State, model.updates.result.Updated),
			"Configured target: "+model.updates.result.Target,
			"Running interface: "+displayRunningVersion(model.actions.RunningVersion),
			fmt.Sprintf("Controller activated and verified: %t", success),
			"Client computers are unchanged until you distribute the prepared system.",
		)
		if success && model.updates.packageBase {
			lines = append(lines, "Check boot, networking, desktop and services before client distribution; a reboot may be needed.")
		} else if success {
			lines = append(lines, "Reopen Nixorium to use the updated interface.")
		} else if model.updates.result.RecoveryRequired {
			lines = append(lines, "Files were written but saving needs recovery. Complete the save before controller activation.")
		} else if model.updates.result.Updated && !model.updates.result.HasErrors() {
			lines = append(lines, "The update is saved safely. Retry controller activation after resolving the detail below.")
		}
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		retryLabel := "New update"
		if model.updates.result.RecoveryRequired {
			retryLabel = "Complete save"
		}
		actions := []tuiAction{}
		if model.updates.result.Updated && !model.updates.result.HasErrors() && !model.updates.result.RecoveryRequired && !success {
			actions = append(actions, tuiAction{key: "a", label: "Retry controller"})
		}
		actions = append(actions, tuiAction{key: "r", label: retryLabel}, tuiAction{key: "Enter", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: actions})
	}
	if model.screen == dashboardUpdateReview {
		return model.releaseReviewView()
	}
	if model.updates.packageBase {
		return model.packageBaseView()
	}
	if model.updates.check.HasErrors() {
		failureTitle, failureDetail := updateCheckFailureSummary(model.updates.check)
		lines = append(lines,
			tuiResult(failureTitle, false, model.isDark),
			"",
			failureDetail,
			"No candidate can be selected and no file changed.",
		)
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "r", label: "Try again"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}})
	}

	releases := model.availableUpdateReleases()
	lines = append(lines,
		fmt.Sprintf("Configured target   %s", model.updates.check.CurrentRef),
		"Running interface   "+displayRunningVersion(model.actions.RunningVersion),
		tuiMuted("Source  "+model.updates.check.Upstream, model.isDark),
		"",
		tuiSection("Available updates", model.isDark),
	)
	start, end := listWindow(len(releases), model.updates.cursor, max(4, model.height-15))
	latestStable := ""
	if len(model.updates.check.Stable) > 0 {
		latestStable = model.updates.check.Stable[0].Tag
	}
	for index := start; index < end; index++ {
		release := releases[index]
		note := updateReleaseStatus(model.updates.check, release)
		if note == "" && release.Tag == latestStable {
			note = "  Latest stable"
		}
		if release.Channel == domain.UpdateChannelPrerelease {
			note += "  Prerelease"
		} else if release.Channel == domain.UpdateChannelMoving {
			note += "  Development branch"
		}
		lines = append(lines, tuiSelection(release.Tag, index == model.updates.cursor, model.isDark)+tuiMuted(note, model.isDark))
	}
	if len(releases) == 0 {
		lines = append(lines, "No updates are available in the selected channel.")
	}
	if model.updates.check.Truncated {
		lines = append(lines, "", tuiMuted("The upstream result was safely limited to the newest releases.", model.isDark))
	}
	prereleaseLabel := "Show prereleases"
	if model.updates.prerelease {
		prereleaseLabel = "Hide prereleases"
	}
	lines = append(lines,
		"",
		"Select master for the latest development revision, or choose a tagged release.",
		"Selection starts validation; it does not change files.",
		"A confirmed update activates this controller; client distribution remains separate.",
	)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if len(releases) > 0 {
		actions = append(actions, tuiAction{key: "↑/↓", label: "Select"}, tuiAction{key: "Enter", label: "Validate"})
	}
	actions = append(actions, tuiAction{key: "p", label: prereleaseLabel}, tuiAction{key: "r", label: "Fetch again"}, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func updatePlanPhaseLabels() []string {
	return []string{
		"Check current deployment",
		"Prepare selected version",
		"Check laboratory configuration",
		"Test systems before saving",
		"Prepare review",
		"Confirm files remain unchanged",
	}
}

func updatePlanProgressDescription(progress domain.UpdatePlanProgress) string {
	switch progress.Phase {
	case domain.UpdatePlanPhaseInspect:
		return "Checking this deployment and the selected version"
	case domain.UpdatePlanPhaseLock:
		return "Preparing the selected Nixorium version"
	case domain.UpdatePlanPhaseEvaluate:
		return "Checking the laboratory configuration"
	case domain.UpdatePlanPhaseBuild:
		detail := strings.ToLower(progress.Detail)
		switch {
		case strings.Contains(detail, "representative client"):
			return "Testing a client computer system"
		case strings.Contains(detail, "controller"):
			return "Testing the controller system"
		case strings.Contains(detail, "netboot"):
			return "Testing the network installer"
		case strings.Contains(detail, "pxe firmware"):
			return "Testing computer network boot"
		case strings.Contains(detail, "offline installer"):
			return "Testing the offline installer"
		default:
			return "Testing a required system before saving"
		}
	case domain.UpdatePlanPhaseReview:
		return "Preparing the update review"
	case domain.UpdatePlanPhaseVerify:
		return "Confirming that deployment files did not change"
	default:
		return "Starting the update safety checks"
	}
}

func updatePlanPhaseIndex(phase domain.UpdatePlanPhase) int {
	switch phase {
	case domain.UpdatePlanPhaseLock:
		return 1
	case domain.UpdatePlanPhaseEvaluate:
		return 2
	case domain.UpdatePlanPhaseBuild:
		return 3
	case domain.UpdatePlanPhaseReview:
		return 4
	case domain.UpdatePlanPhaseVerify:
		return 5
	default:
		return 0
	}
}

func displayRunningVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

func updateCheckFailureSummary(report domain.UpdateCheckReport) (string, string) {
	if len(report.Issues) > 0 {
		switch report.Issues[0].Field {
		case "input", "repository":
			return "Update setup needs attention", "Nixorium could not read a supported update source from this deployment."
		case "releases":
			return "No supported updates found", "The configured upstream responded but did not advertise a supported branch or release."
		}
	}
	return "Updates could not be fetched", "Nixorium could not obtain an update list from the configured upstream."
}

func updateReleaseAlreadyCurrent(report domain.UpdateCheckReport, release domain.UpdateRelease) bool {
	if release.Tag != report.CurrentRef {
		return false
	}
	if release.Channel != domain.UpdateChannelMoving {
		return true
	}
	return report.CurrentRev != "" && release.ObjectID != "" && report.CurrentRev == release.ObjectID
}

func updateReleaseStatus(report domain.UpdateCheckReport, release domain.UpdateRelease) string {
	if release.Tag != report.CurrentRef {
		return ""
	}
	if release.Channel != domain.UpdateChannelMoving {
		return "  Current"
	}
	if updateReleaseAlreadyCurrent(report, release) {
		return "  Current revision"
	}
	if report.CurrentRev != "" && release.ObjectID != "" {
		return "  New revision available"
	}
	return "  Configured target"
}

func (model dashboardModel) availableUpdateReleases() []domain.UpdateRelease {
	releases := append([]domain.UpdateRelease{}, model.updates.check.Development...)
	releases = append(releases, model.updates.check.Stable...)
	if model.updates.prerelease {
		releases = append(releases, model.updates.check.Prerelease...)
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
	model.updates.applying = true
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
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
	if model.updates.packageBase {
		height -= 4
	}
	if model.updateDetails {
		height -= len(model.updates.plan.Checks) + 2
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
	if model.controller.applying {
		elapsed := time.Since(model.controller.started).Truncate(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		lines = append(lines, "", fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		lines = append(lines, model.operationProgressView(model.controller.progress, "Current progress")...)
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Controller update is running", detail: "Wait for the verified result before closing Nixorium."})
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "l", label: "Progress details"}, {key: "F1", label: "Help"}}})
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.screen == dashboardControllerReview {
		body := strings.Join([]string{
			tuiTitle("Update this controller?", model.isDark),
			"",
			"Affects   " + model.controller.plan.Controller + " (this controller only)",
			"Revision  " + model.controller.plan.Revision,
			"",
			"Press Enter to build, activate, and verify this controller.",
		}, "\n")
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Services and networking may restart", detail: "This connection may be interrupted. Nixorium builds, activates and verifies the reviewed configuration; a reboot is not normally required."})
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: append(path, "Review"), body: body, notices: notices, actions: []tuiAction{{key: "Enter", label: "Update controller"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}})
	}
	if model.controller.result.Operation != "" {
		resultTitle := "Controller action needs attention"
		if !model.controller.result.HasErrors() && model.controller.result.Applied && model.controller.result.Verified {
			resultTitle = "Controller updated and verified"
		}
		lines = append(lines,
			tuiResult(resultTitle, !model.controller.result.HasErrors(), model.isDark),
			"",
			fmt.Sprintf("Last result: %s at phase %s", model.controller.result.State, model.controller.result.Phase),
			fmt.Sprintf("Applied: %t   Verified: %t", model.controller.result.Applied, model.controller.result.Verified),
		)
		if model.controller.details && model.controller.progress.Operation != "" {
			lines = append(lines, "")
			lines = append(lines, model.operationProgressView(model.controller.progress, "Last controller apply")...)
		}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		detailsLabel := "Show details"
		if model.controller.details {
			detailsLabel = "Hide details"
		}
		returnLabel := "Maintenance"
		if model.setupMode {
			returnLabel = "Setup"
		}
		actions := []tuiAction{{key: "Enter", label: returnLabel}, {key: "d", label: detailsLabel}, {key: "l", label: "Logs"}, {key: "r", label: "New review"}, {key: "F1", label: "Help"}}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: actions})
	}
	lines = append(lines,
		"",
		"Review the current committed controller configuration before rebuilding.",
	)
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "r", label: "Create review"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}})
}

func (model dashboardModel) servicesView() string {
	path := []string{"Maintenance", "Services"}
	lines := []string{
		tuiTitle("Controller services", model.isDark),
		tuiMuted("Normally no action is needed here. Use this view when Diagnostics reports a delivery or installation service problem.", model.isDark),
	}
	notices := []tuiNotice{}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.maintenance.serviceResult.Operation != "" {
		success := !model.maintenance.serviceResult.HasErrors() && model.maintenance.serviceResult.Verified
		title := "Service action needs attention"
		if success {
			title = "Binary cache restarted and verified"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Verified: %t   Retry safe: %t", model.maintenance.serviceResult.State, model.maintenance.serviceResult.Verified, model.maintenance.serviceResult.RetrySafe),
		)
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Enter", label: "Maintenance"}, {key: "l", label: "Logs"}, {key: "r", label: "Restart again"}, {key: "F1", label: "Help"}}})
	}
	if model.screen == dashboardServicesRestartReview {
		body := strings.Join([]string{
			tuiTitle("Restart the software cache?", model.isDark),
			"",
			"Affects  Controller cache; active installations may be affected",
		}, "\n")
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "The signed cache will be briefly unavailable", detail: "Active installers may retry downloads. PXE networking and listeners are unchanged; the cache is verified afterward."})
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{
			path:      append(path, "Review"),
			body:      body,
			fixedBody: tuiSection("Type RESTART to continue:", model.isDark) + "\n> " + model.confirmation + "_",
			notices:   notices,
			actions:   []tuiAction{{key: "Enter", label: "Restart cache"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}},
		})
	}
	for _, service := range model.maintenance.services.Services {
		kind := tuiStatusAttention
		if service.State == "healthy" || service.State == "active" || service.State == "standby" {
			kind = tuiStatusSuccess
		} else if service.State == "failed" || service.State == "degraded" {
			kind = tuiStatusFailure
		}
		lines = append(lines,
			fmt.Sprintf("%s — %s", tuiSection(service.Name, model.isDark), tuiStatus(service.State, kind, model.isDark)),
		)
		if service.ID == "cache" {
			lines = append(lines, "  Supplies already-built software to clients and network installers.")
		} else if service.ID == "pxe" {
			lines = append(lines, "  Provides network boot while PXE mode is active.")
		}
		lines = append(lines, tuiMuted("  "+service.Detail, model.isDark))
		if service.ID == "pxe" {
			lines = append(lines, "  Start, stop or recover it from Installation → PXE mode and network recovery.")
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
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) serviceRestartAvailable() bool {
	return len(model.maintenance.services.Services) > 0 && model.maintenance.services.Services[0].ID == "cache" && len(model.maintenance.services.Services[0].Units) > 0 && model.maintenance.services.Services[0].Units[0].Loaded
}

func (model dashboardModel) logsView() string {
	path := []string{"Maintenance", "History"}
	lines := []string{tuiTitle("Operation history", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	lines = append(lines, tuiSection("Recent actions", model.isDark))
	recordLimit := len(model.maintenance.logs.Records)
	if recordLimit > 3 {
		recordLimit = 3
	}
	if recordLimit == 0 {
		lines = append(lines, "  No recorded operation outcomes.")
	}
	for _, record := range model.maintenance.logs.Records[:recordLimit] {
		lines = append(lines, fmt.Sprintf("  %s  %-18s %-10s %s", record.RecordedAt.UTC().Format("2006-01-02 15:04Z"), record.Operation, record.State, record.Subject))
	}
	lines = append(lines, "", tuiSection("Deployment logs", model.isDark))
	if len(model.maintenance.logs.Logs) == 0 {
		lines = append(lines, "No deployment operation logs are available.")
	}
	start, end := listWindow(len(model.maintenance.logs.Logs), model.maintenance.logCursor, max(1, (model.height-16)/2))
	for index := start; index < end; index++ {
		entry := model.maintenance.logs.Logs[index]
		row := fmt.Sprintf("%s  %-10s %-11s %d bytes", entry.StartedAt.UTC().Format("2006-01-02 15:04Z"), entry.Kind, entry.State, entry.SizeBytes)
		lines = append(lines, tuiSelection(row, index == model.maintenance.logCursor, model.isDark))
		lines = append(lines, "    "+entry.ID)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if len(model.maintenance.logs.Logs) > 0 {
		actions = append(actions, tuiAction{key: "↑/↓", label: "Select"}, tuiAction{key: "Enter", label: "View tail"})
	}
	back := "Maintenance"
	if model.setupMode {
		back = "Setup"
	}
	actions = append(actions, tuiAction{key: "f", label: "Refresh"}, tuiAction{key: "Esc", label: back}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) logDetailView() string {
	path := []string{"Maintenance", "History", "Log"}
	lines := []string{tuiTitle("Operation log detail", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.maintenance.logDetail.Log == nil {
		lines = append(lines, "The selected operation log could not be read safely.")
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Esc", label: "History"}, {key: "F1", label: "Help"}}})
	}
	entry := model.maintenance.logDetail.Log
	contentLines := operationLogContentLines(model.maintenance.logDetail.Content)
	end := model.maintenance.logScroll + model.logDetailHeight()
	if end > len(contentLines) {
		end = len(contentLines)
	}
	lines = append(lines,
		fmt.Sprintf("%s — %s", entry.StartedAt.UTC().Format("2006-01-02 15:04:05Z"), entry.State),
		entry.ID,
		fmt.Sprintf("Showing lines %d-%d of %d%s", displayedLineStart(model.maintenance.logScroll, len(contentLines)), end, len(contentLines), truncatedLogLabel(model.maintenance.logDetail.Truncated)),
		"",
	)
	lines = append(lines, contentLines[model.maintenance.logScroll:end]...)
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "↑/↓/Pg/Home/End", label: "Scroll"}, {key: "Esc", label: "History"}, {key: "F1", label: "Help"}}})
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
	if model.computers.restoreMode {
		path = []string{"Computers", "Restore", "Reapply"}
	}
	shell := tuiShell{path: path}
	if model.deployment.usbRecovery != nil {
		return model.deploymentUSBRecoveryView(shell)
	}
	if model.deployment.applying {
		elapsed := time.Since(model.deployment.started).Truncate(time.Second)
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
		return model.renderShell(shell)
	}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	if model.deployment.context != "" {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusNeutral, title: model.deployment.context})
	}
	if model.screen == dashboardDeployReview {
		lines := []string{
			tuiTitle("Distribute the system?", model.isDark),
			"",
			fmt.Sprintf("Affects  %s · %d computer(s)", model.deployment.plan.ColmenaSelector, len(model.deployment.plan.Targets)),
			"",
			tuiMuted("Reviewed revision  "+model.deployment.plan.Revision, model.isDark),
		}
		shell.body = strings.Join(lines, "\n")
		shell.fixedBody = tuiSection("Type DEPLOY to continue:", model.isDark) + "\n> " + model.deployment.confirmation + "_"
		shell.notices = append(shell.notices, tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Target services may restart; unreachable computers may remain unchanged",
			detail: "Every selected configuration is built first. A failed apply may leave mixed target state; a fresh full retry is safe.",
		})
		if model.message != "" {
			shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Deploy"}, {key: "Esc", label: "Selection"}, {key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}

	if model.deployment.result.Operation != "" {
		success := !model.deployment.result.HasErrors()
		resultTitle := "Deployment needs attention"
		if success {
			resultTitle = "Deployment completed and verified"
		}
		lines := []string{
			tuiResult(resultTitle, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Phase: %s", model.deployment.result.State, model.deployment.result.Phase),
			fmt.Sprintf("Build complete: %t   Apply complete: %t", model.deployment.result.BuildCompleted, model.deployment.result.ApplyCompleted),
		}
		if model.deployment.result.Verification.Attempted > 0 {
			lines = append(lines, fmt.Sprintf("Authenticated: %d/%d   Recorded: %d", model.deployment.result.Verification.Verified, model.deployment.result.Verification.Attempted, model.deployment.result.Verification.Recorded))
		}
		if model.deployment.result.LogPath != "" {
			lines = append(lines, "Detailed log: "+model.deployment.result.LogPath)
		}
		if model.message != "" {
			shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
		}
		shell.body = strings.Join(lines, "\n")
		shell.actions = []tuiAction{{key: "r", label: "New review"}, {key: "l", label: "Logs"}, {key: "Enter", label: "Computers"}, {key: "?", label: "Help"}}
		return model.renderShell(shell)
	}

	lines := []string{
		tuiTitle("Distribute the prepared system", model.isDark),
		tuiMuted("Select → Review → Deploy → Verify", model.isDark),
		"",
	}
	hosts := model.report.Meta.Clients.Hosts
	selected := 0
	for _, host := range hosts {
		if model.deployment.chosen[host.Name] {
			selected++
		}
	}
	lines = append(lines, "Choose where to apply the saved configuration.", "", fmt.Sprintf("%d of %d computers selected", selected, len(hosts)), "")
	start, end := listWindow(len(hosts), model.deployment.cursor, max(3, model.height-20))
	for index := start; index < end; index++ {
		host := hosts[index]
		checked := " "
		if model.deployment.chosen[host.Name] {
			checked = "x"
		}
		lines = append(lines, tuiSelection(fmt.Sprintf("[%s] %-10s %s", checked, host.Name, host.IP), index == model.deployment.cursor, model.isDark))
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
	return model.renderShell(shell)
}

func (model dashboardModel) deploymentProgressView() []string {
	progressState := model.deployment.progress
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
	if len(model.deployment.recent) > 0 {
		lines = append(lines, "  Recent activity:")
		for _, activity := range model.deployment.recent {
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
	if model.installation.flow {
		path = []string{"Installation", "Install computers"}
		title = "Install computers"
	} else if model.setupMode {
		path = []string{"Installation", "Install computers"}
		title = "Install computers"
	} else if model.computers.restoreMode {
		path = []string{"Computers", "Restore", "Reinstall"}
		title = "Reinstall computers"
	}
	lines := []string{
		tuiTitle(title, model.isDark),
		fmt.Sprintf("Installation mode:  %s", tuiStatus(model.report.PXE.Mode, pxeStatusKind(model.report.PXE.Mode), model.isDark)),
		fmt.Sprintf("Prepared artifacts: %s", preparation),
		fmt.Sprintf("Interface:          %s", model.report.Meta.Network.Interface),
		fmt.Sprintf("Service address:    %s", model.report.Meta.Controller.DHCPIP),
	}
	if model.installation.flow {
		steps := []string{"Laboratory settings", "Save configuration", "Controller keys", "Activate controller", "Prepare clients", "Start PXE"}
		lines = append(lines, "")
		lines = append(lines, model.computerInstallationSteps(steps)...)
		if model.installation.failed {
			return model.renderShell(tuiShell{
				path:    path,
				body:    strings.Join(lines, "\n"),
				notices: []tuiNotice{{kind: tuiStatusFailure, title: "Computer installation could not continue", detail: model.message}},
				actions: []tuiAction{{key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}},
			})
		}
	}
	if model.controller.applying {
		if model.controller.progress.State != "completed" {
			elapsed := time.Since(model.controller.started).Truncate(time.Second)
			if elapsed < 0 {
				elapsed = 0
			}
			lines = append(lines, "", fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		} else {
			lines = append(lines, "")
		}
		lines = append(lines, model.operationProgressView(model.controller.progress, "Controller progress")...)
		notice := tuiNotice{kind: tuiStatusAttention, title: "Controller activation is running", detail: "Services and networking may restart while the reviewed configuration is activated and verified."}
		if model.controller.progress.State == "completed" {
			notice = tuiNotice{kind: tuiStatusSuccess, title: "Controller activation and verification completed", detail: "Continuing with client-system preparation."}
		}
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			notices: []tuiNotice{notice},
			actions: model.pxeActions(),
		})
	}
	if model.installation.pxePreparing {
		if model.installation.pxeProgress.State != "completed" {
			elapsed := time.Since(model.installation.pxeStarted).Truncate(time.Second)
			if elapsed < 0 {
				elapsed = 0
			}
			lines = append(lines, "", fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed))
		} else {
			lines = append(lines, "")
		}
		lines = append(lines, model.operationProgressView(model.installation.pxeProgress, "Current progress")...)
		notice := tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Preparation continues if this view closes",
			detail: "The systemd-owned operation is recorded and can be inspected again later.",
		}
		if model.installation.pxeProgress.State == "completed" {
			notice = tuiNotice{kind: tuiStatusSuccess, title: "Client preparation completed", detail: "The client systems and network installation files are ready."}
		}
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			notices: []tuiNotice{notice},
			actions: model.pxeActions(),
		})
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			actions: model.pxeActions(),
		})
	}
	if model.screen == dashboardPXEStartReview {
		scope := model.installation.startPlan.Interface + " · controller network"
		body := strings.Join([]string{
			tuiTitle("Start network installation?", model.isDark),
			"",
			"Affects  " + scope,
		}, "\n")
		notices := []tuiNotice{{
			kind:   tuiStatusAttention,
			title:  "Temporarily remove " + model.installation.startPlan.StaticCIDR + "; remote connections may be interrupted",
			detail: "Serve ProxyDHCP, TFTP, HTTP and cache via " + model.installation.startPlan.DHCPAddress + ". Institutional DHCP remains authoritative; stop or reboot recovery restores normal addressing.",
		}}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{
			path:      path,
			body:      body,
			fixedBody: tuiSection("Type START to continue:", model.isDark) + "\n> " + model.confirmation + "_",
			notices:   notices,
			actions:   model.pxeActions(),
		})
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
		}
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{
			path:      path,
			body:      strings.Join(leaveLines, "\n"),
			fixedBody: tuiSection("Keep it active", model.isDark) + "\n  Type LEAVE to continue:\n> " + model.confirmation + "_",
			notices:   notices,
			actions:   model.pxeActions(),
		})
	}
	lines = append(lines, "")
	lines = append(lines, model.pxeNextStepView()...)
	if model.installation.pxeProgress.Operation != "" {
		lines = append(lines, "")
		lines = append(lines, model.operationProgressView(model.installation.pxeProgress, "Last preparation")...)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: model.pxeActions()})
}

func (model dashboardModel) computerInstallationSteps(labels []string) []string {
	lines := []string{""}
	for index, label := range labels {
		switch {
		case index < model.installation.stage:
			lines = append(lines, tuiStatus(label, tuiStatusSuccess, model.isDark))
		case index == model.installation.stage && model.screen == dashboardPXEStartReview:
			lines = append(lines, tuiMuted("○ "+label+" · Awaiting confirmation", model.isDark))
		case index == model.installation.stage:
			lines = append(lines, tuiTitle("● "+label+" · Running", model.isDark))
		default:
			lines = append(lines, tuiMuted("○ "+label+" · Waiting", model.isDark))
		}
	}
	return lines
}

func (model dashboardModel) pxeActions() []tuiAction {
	if model.screen == dashboardPXEStartReview {
		return []tuiAction{{key: "Enter", label: "Start PXE"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	if model.screen == dashboardPXELeaveReview {
		return []tuiAction{{key: "x", label: "Stop and exit"}, {key: "Enter", label: "Leave active"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	if model.installation.pxePreparing {
		return []tuiAction{{key: "l", label: "Progress details"}, {key: "q", label: "Close view"}, {key: "F1", label: "Help"}}
	}
	if model.controller.applying {
		return []tuiAction{{key: "l", label: "Progress details"}, {key: "F1", label: "Help"}}
	}
	if model.busy != "" {
		return []tuiAction{{key: "q", label: "Close view"}, {key: "F1", label: "Help"}}
	}
	if model.installation.flow && model.installation.failed {
		return []tuiAction{{key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
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
	if model.computers.restoreMode {
		backLabel = "Computers"
	}
	if recovery {
		actions = append(actions, tuiAction{key: "r", label: "Recover network"})
	}
	return append(actions,
		tuiAction{key: "Esc", label: backLabel},
		tuiAction{key: "q", label: "Quit"},
		tuiAction{key: "F1", label: "Help"},
	)
}

func (model dashboardModel) pxeNextStepView() []string {
	switch model.report.PXE.Mode {
	case "active":
		title := "Next: install computers"
		if model.computers.restoreMode {
			title = "Next: reinstall computers"
		}
		lines := []string{
			tuiResult(title, true, model.isDark),
			"  1. Boot one configured computer using UEFI network boot.",
			"  2. In the downloaded installer, run /installer/setup.sh.",
		}
		if model.computers.restoreMode {
			lines = append(lines, "  3. Choose its configured identity and confirm the target disk locally.")
		} else {
			lines = append(lines, "  3. Choose its configured identity and inspect the target disk.")
		}
		return append(lines,
			"  4. When installations are finished, press x here to stop PXE.",
			tuiStatus("Only the disk confirmed locally in the installer is erased.", tuiStatusAttention, model.isDark),
		)
	case "degraded", "recovery-required":
		return []string{
			tuiResult("Next: recover normal controller networking", false, model.isDark),
			"  A previous PXE transition was interrupted.",
			"  Press r to restore its recorded network state and stop managed PXE services.",
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
	if !model.progressDetails && !model.controller.details {
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
	selected := selectedDeploymentTargetNames(hosts, chosen)
	if len(selected) == len(hosts) && len(hosts) > 0 {
		return "@lab"
	}
	return strings.Join(selected, ",")
}

func selectedDeploymentTargetNames(hosts []domain.HostMeta, chosen map[string]bool) []string {
	selected := []string{}
	for _, host := range hosts {
		if chosen[host.Name] {
			selected = append(selected, host.Name)
		}
	}
	return selected
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
