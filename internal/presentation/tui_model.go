package presentation

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardScreen int

const (
	dashboardHome dashboardScreen = iota
	dashboardComputersArea
	dashboardInstallationArea
	dashboardSetup
	dashboardSetupKeys
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
	dashboardGitUndoReview
	dashboardUpdate
	dashboardUpdateReview
	dashboardSettings
	dashboardSettingsEdit
	dashboardSettingsPasswords
	dashboardSettingsReview
	dashboardSettingsAccounts
	dashboardPXE
	dashboardPXEStartReview
	dashboardPXELeaveReview
	dashboardUSBInstall
	dashboardAdministration
	dashboardDiagnostics
	dashboardSoftware
	dashboardInternet
	dashboardLock
	dashboardShare
	dashboardDeferredUpdates
	dashboardNetworkChange
	dashboardCleanup
	dashboardRecovery
	dashboardRecoveryReview
	dashboardBackup
	dashboardShutdown
	dashboardShutdownReview
	dashboardShutdownResult
	dashboardWorkspace
	dashboardHomeFiles
	dashboardSupport
	dashboardTelemetry
	dashboardTemplateReset
	dashboardHostTrust
	dashboardManagedJobs
	dashboardDisclaimer
	dashboardAssistant
	dashboardAssistantReview
)

// deploymentModel owns target selection and the lifecycle of one reviewed
// client deployment. The dashboard root only routes its messages and effects.
type deploymentModel struct {
	cursor  int
	chosen  map[string]bool
	plan    domain.DeploymentPlanReport
	result  domain.DeploymentExecutionReport
	context string
	// origin is the screen a saved change opened this selection from; Esc
	// returns there. Without one, Esc returns to Computers.
	origin        dashboardScreen
	applying      bool
	progress      domain.DeploymentProgress
	recent        []string
	started       time.Time
	events        <-chan tea.Msg
	confirmation  string
	usbRecovery   *deploymentUSBRecovery
	usbRequestID  uint64
	cancel        context.CancelFunc
	stopReview    bool
	stopRequested bool
	// queueOff: computers that are off get the update when they answer
	// again (default when some did not answer the availability check).
	queueOff bool
}

// deploymentOffComputers names the reviewed computers that did not answer.
func deploymentOffComputers(plan domain.DeploymentPlanReport) []string {
	off := []string{}
	for _, target := range plan.Targets {
		reachable := false
		for _, observed := range plan.Availability {
			if observed.Name == target.Name && observed.IP == target.IP && observed.Reachability == domain.ReachabilityReachable && observed.SSH == domain.SSHAvailable {
				reachable = true
			}
		}
		if !reachable {
			off = append(off, target.Name)
		}
	}
	return off
}

// controllerModel is the shared activation boundary used after settings,
// software and update saves. It owns job identity so stale progress messages
// cannot be mistaken for the current activation.
type controllerModel struct {
	fromSave            bool
	autoApply           bool
	saveOrigin          dashboardScreen
	plan                domain.ControllerRebuildPlanReport
	result              domain.ControllerRebuildExecutionReport
	applying            bool
	progress            domain.OperationProgress
	started             time.Time
	progressID          uint64
	details             bool
	progressUnavailable bool
	// progressFailures counts consecutive unreadable progress polls; a
	// single miss, common while systemd reloads, is not reported.
	progressFailures int
}

// settingsModel owns the settings editor and its reviewed local save. The
// controller activation that may follow remains a separate shared model.
type settingsModel struct {
	current          domain.LabSettingsFile
	candidate        domain.LabSettingsFile
	menu             routineSettingsMenu
	passwordMenu     routinePasswordMenu
	accountsMenu     routineAccountsMenu
	editor           settingsWizardModel
	plan             domain.ConfigPlanReport
	result           domain.ConfigurationSaveReport
	applying         bool
	returnScreen     dashboardScreen
	collectPasswords bool
	// fromAccounts marks a names form or password list opened from the
	// Accounts submenu, so Esc returns there instead of to Settings.
	fromAccounts bool
	// repair lists problems of saved settings that no longer validate.
	repair []domain.ValidationIssue
	// addresses are the controller's current addresses offered for its DHCP
	// address, read when the settings were loaded.
	addresses domain.ControllerAddresses
	// saveAnyway acknowledges the computers whose named network device is
	// missing in the reviewed network check.
	saveAnyway bool
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
	// gitDiscard switches path selection and review to a reviewed discard.
	gitDiscard      bool
	gitCommitPlan   domain.GitCommitPlanReport
	gitCommitResult domain.GitCommitReport
	// gitUndoPlan and gitUndoResult belong to the reviewed undo of the last
	// saved change, opened with u on the Git review.
	gitUndoPlan      domain.GitUndoPlanReport
	gitUndoResult    domain.GitUndoReport
	gitFromWorkspace bool
	gitFromHomeFiles bool
}

// installationModel owns guided installation and PXE lifecycle state,
// including progress identity for delayed service messages.
type installationModel struct {
	savedSummary     bool
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
	stateError       bool
	remote           remoteInstallationModel
	// pxeResult is the last stop or recovery of network installation.
	pxeResult domain.PXELifecycleReport
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
	remoteInstallInterfaceMismatch
	remoteInstallRotateHostKey
	remoteInstallReview
	remoteInstallApplying
	remoteInstallResult
	remoteInstallConfirmReboot
	remoteInstallConfirmClose
	remoteInstallConfirmCancel
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
	// reopened: an unfinished record was opened again (Overview, a refused
	// operation or the deployment check), so the screen is its review.
	reopened       bool
	bootstrapError string
	// liveInterface is the network card that carries the live address when
	// it differs from the one configured for the chosen computer.
	liveInterface string
	// passwordAgain asks for a new passwd after a restarted preparation:
	// the previous connection locked the temporary password.
	passwordAgain bool
	response      domain.RemoteInstallResponse
	plan          domain.RemoteInstallPlanReport
	// details shows the raw result fields; watch paces automatic refresh.
	details      bool
	watch        uint64
	watchStarted time.Time
}

// computersModel owns inventory, filtering, detail and restore navigation.
// Deployment and shutdown remain separate because they have independent jobs.
type computersModel struct {
	areaCursor         int
	hostCursor         int
	hostQuery          string
	hostSearching      bool
	hostDetail         bool
	hostTechnical      bool
	configurationState domain.ConfigurationStateReport
	hosts              domain.HostsReport
	// refreshReturn is the selection list that asked for a computer check.
	refreshReturn dashboardScreen
}

// setupKeysModel is the key reconciliation screen of the setup flow.
type setupKeysModel struct {
	report    domain.KeyReconcileReport
	back      dashboardScreen
	cursor    int
	importing bool
	path      string
}

// diagnosticsModel is the diagnostics screen.
type diagnosticsModel struct {
	report  domain.DoctorReport
	cursor  int
	details bool
	back    dashboardScreen
}

// recoveryModel is the persistent-blockers recovery state.
type recoveryModel struct {
	// report is the last observation of persistent blockers.
	report domain.RecoveryReport
	cursor int
	review recoveryReview
}

// navigationModel holds cursors, scrolling and paging shared by screens.
type navigationModel struct {
	menuRows               int // set while fitting the task menu to the terminal; 0 means not yet fitted
	installationAreaCursor int
	areaReturn             dashboardScreen
	adminCursor            int
	helpOpen               bool
	pageScroll             int
}

type dashboardModel struct {
	keys                  setupKeysModel
	diagnostics           diagnosticsModel
	recovery              recoveryModel
	navigation            navigationModel
	assistantNoColors     bool
	backupSync            backupSyncState
	controllerObservation controllerOverviewObservation
	jobs                  managedJobsModel
	read                  readActivity
	busyStarted           time.Time
	hostTrust             hostTrustModel
	updateDetails         bool
	returnAdmin           bool
	disclaimerChecking    bool
	computerSetup         string
	computers             computersModel
	setupDetails          bool
	progressDetails       bool
	report                domain.StatusReport
	setup                 domain.SetupReport
	setupMode             bool
	homeMenu              dashboardTaskMenu
	actions               DashboardActions
	assistant             assistantModel
	screen                dashboardScreen
	busy                  string
	message               string
	confirmation          string
	installation          installationModel
	deployment            deploymentModel
	deferred              deferredModel
	networkChange         networkChangeModel
	controller            controllerModel
	reload                interfaceReloadModel
	// overviewReads numbers Overview reads; a background result older than
	// the last read is dropped.
	overviewReads uint64
	// reopenNotice is the outcome of the update that reopened this
	// interface, shown on the first screen.
	reopenNotice       string
	maintenance        maintenanceModel
	updates            updateModel
	settings           settingsModel
	workspace          workspaceModel
	homeFiles          homeFilesModel
	updateNotification domain.UpdateNotification
	telemetry          telemetryModel
	support            supportModel
	templateReset      templateResetModel
	software           softwareModel
	internet           internetModel
	lock               lockModel
	share              shareModel
	cleanup            cleanupModel
	shutdown           shutdownModel
	width              int
	height             int
	isDark             bool
	activitySpinner    spinner.Model

	inventory    inventoryLoad
	initializing bool
	initialError bool
	// usbReserved is the last local observation of an unfinished USB
	// installation; the worker is consulted only when it is opened.
	usbReserved bool
	// recovery.report is the last observation of persistent blockers.
	// noticeDetails shows the technical text of a situation notice on a
	// result screen (d); it resets when the screen or its message changes.
	noticeDetails bool
	backup        backupModel
}
