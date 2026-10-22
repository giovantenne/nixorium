package presentation

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Loads, plans, discovery, previews and fingerprint observations are cancellable
// reads. They may evaluate/build isolated candidates, but never activate or save
// deployment state. SearchSoftware has its own debounced, bounded context.
// The two progress loaders only read bounded local records of independent jobs.
// All other callbacks are protected mutations, including the mixed-operation
// RemoteInstallRequest. ApplyDeployment's context is for its separately reviewed
// stop action, not the read-only Esc path. PreparePXE is systemd-owned;
// ApplyController waits for a managed job while the foreground UI stays protected.
type DashboardActions struct {
	// InstalledInterface reports whether the installed Nixorium differs from
	// the running one. It is nil outside the administrator dashboard.
	InstalledInterface func() (InterfaceReload, bool)
	// OpenURL starts the desktop browser without waiting for it. It is nil
	// when no graphical session is available (for example over SSH).
	OpenURL                   func(string) error
	LoadClientSetup           func(context.Context) (bool, error)
	LoadDisclaimer            func(context.Context) (bool, error)
	AcceptDisclaimer          func() error
	CheckUpdateNotification   func(context.Context) (domain.UpdateNotification, error)
	DismissUpdateNotification func(domain.UpdateNotification) error
	LoadManagedJobs           func(context.Context) ([]domain.ManagedJob, error)
	PlanHostTrust             func(context.Context, string) domain.HostTrustPlan
	ApplyHostTrust            func(domain.HostTrustPlan) domain.HostTrustResult
	LoadTemplateReset         func(context.Context) domain.TemplateResetCatalog
	PlanTemplateReset         func(context.Context, string, func(string)) domain.TemplateResetPlan
	ApplyTemplateReset        func(domain.TemplateResetPlan) domain.TemplateResetResult
	ClassroomMode             bool
	Assistant                 domain.AssistantConfig
	AssistantService          *AssistantActions
	// OpenClassroomView opens the classroom view page and
	// returns a sentence for the operator.
	OpenClassroomView func(context.Context) (string, error)
	PlanInternet      func(context.Context, string, domain.InternetAction) domain.InternetPlan
	ApplyInternet     func(domain.InternetPlan) domain.InternetReport
	// PlanLock and ApplyLock lock or unlock students' screens; nil when the
	// laboratory has no classroom view.
	PlanLock  func(context.Context, string, domain.LockAction) domain.LockPlan
	ApplyLock func(domain.LockPlan) domain.LockReport
	// PlanShare prepares a file or folder (its path is the last argument)
	// and reviews sending it to the selected students' desktops; nil
	// without the classroom view.
	PlanShare               func(context.Context, string, string) domain.SharePlan
	ApplyShare              func(domain.SharePlan) domain.ShareReport
	PlanCleanup             func(context.Context, string) domain.CleanupPlanReport
	ApplyCleanup            func(domain.CleanupPlanReport) domain.CleanupApplyReport
	RunningVersion          string
	LoadInventory           func(context.Context) (domain.StatusReport, error)
	LoadInitial             func(context.Context) (domain.StatusReport, domain.SetupReport, error)
	LoadDoctor              func(context.Context) (domain.DoctorReport, error)
	Telemetry               func(context.Context, string) (domain.TelemetryReport, error)
	PreviewSupport          func(context.Context) (domain.SupportSnapshot, error)
	ExportSupport           func(domain.SupportSnapshot) domain.SupportExportResult
	Refresh                 func(context.Context) (domain.StatusReport, error)
	LoadSetup               func(context.Context) domain.SetupReport
	LoadSetupKeys           func(context.Context) domain.KeyReconcileReport
	ReconcileSetupKeys      func() (domain.KeyReconcileReport, error)
	ImportSetupKey          func(string, string) (domain.KeyImportReport, error)
	SaveSetupConfiguration  func() domain.ConfigurationSaveReport
	InstallSetupSecrets     func() domain.ActionReport
	LoadHosts               func(context.Context) (domain.HostsReport, error)
	LoadConfigurationState  func(context.Context) domain.ConfigurationStateReport
	LoadSoftware            func(context.Context) domain.SoftwareCatalogReport
	SearchSoftware          func(context.Context, string) domain.SoftwareSearchReport
	PlanSoftware            func(context.Context, domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport
	SaveSoftware            func(domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport
	LoadSoftwarePresets     func(context.Context) domain.SoftwarePresetCatalogReport
	PlanSoftwarePreset      func(context.Context, domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport
	SaveSoftwarePreset      func(domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport
	PlanShutdown            func(context.Context, string, domain.ShutdownSessionPolicy) domain.ShutdownPlanReport
	PlanPower               func(context.Context, string, domain.ShutdownSessionPolicy, domain.ClientPowerAction) domain.ShutdownPlanReport
	ApplyShutdown           func(domain.ShutdownPlanReport) domain.ShutdownApplyReport
	PlanWake                func(context.Context, string) domain.WakePlan
	ApplyWake               func(domain.WakePlan) domain.WakeReport
	PlanDeployment          func(context.Context, string) domain.DeploymentPlanReport
	PlanReachableDeployment func(context.Context, domain.DeploymentPlanReport) domain.DeploymentPlanReport
	ApplyDeployment         func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport
	// ApplyDeploymentQueued updates the reachable computers of the reviewed
	// plan now and queues the others for when they answer; nil hides the
	// choice.
	ApplyDeploymentQueued func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport
	// LoadDeferredUpdates and CancelDeferredUpdates show and edit the queue.
	LoadDeferredUpdates   func(context.Context) domain.DeferredUpdateStatus
	CancelDeferredUpdates func([]string) error
	// LoadNetworkMigration, PlanNetworkMigration and ApplyNetworkMigration
	// drive Maintenance → Change network addressing. Plan takes the step
	// (start, finish or undo), the new network for start, and whether the
	// computers that are off are acknowledged.
	LoadNetworkMigration    func(context.Context) domain.NetworkMigrationStatus
	PlanNetworkMigration    func(context.Context, string, string, bool) domain.NetworkMigrationPlan
	ApplyNetworkMigration   func(domain.NetworkMigrationPlan) domain.NetworkMigrationResult
	PlanController          func(context.Context) domain.ControllerRebuildPlanReport
	ApplyController         func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport
	LoadControllerProgress  func() (domain.OperationProgress, error)
	LoadServices            func(context.Context) domain.ServicesReport
	RestartService          func(string) domain.ServiceActionReport
	LoadLogs                func(context.Context) domain.OperationLogsReport
	LoadLog                 func(context.Context, string) domain.OperationLogReport
	LoadGitReview           func(context.Context) domain.GitReviewReport
	PlanGitCommit           func(context.Context, string) domain.GitCommitPlanReport
	PlanGitDiscard          func(context.Context, string) domain.GitCommitPlanReport
	ApplyGitDiscard         func(domain.GitCommitPlanReport) domain.GitCommitReport
	ApplyGitCommit          func(domain.GitCommitPlanReport) domain.GitCommitReport
	PlanGitUndo             func(context.Context) domain.GitUndoPlanReport
	ApplyGitUndo            func(domain.GitUndoPlanReport) domain.GitUndoReport
	CheckUpdate             func(context.Context) domain.UpdateCheckReport
	PlanUpdate              func(context.Context, string, bool, bool) domain.UpdatePlanReport
	PlanUpdateWithProgress  func(context.Context, string, bool, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport
	SaveUpdate              func(domain.UpdatePlanReport) domain.UpdateApplyReport
	LoadPackageBase         func(context.Context) domain.PackageBaseStatus
	PlanPackageBase         func(context.Context, string, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport
	SavePackageBase         func(domain.UpdatePlanReport) domain.UpdateApplyReport
	SettingsRepairIssues    func() []domain.ValidationIssue
	LoadSettings            func(context.Context) (domain.LabSettingsFile, error)
	LoadWorkspace           func(context.Context) domain.WorkspacePlanReport
	PlanWorkspace           func(context.Context, domain.WorkspaceProfile) domain.WorkspacePlanReport
	SaveWorkspace           func(domain.WorkspacePlanReport) domain.WorkspaceApplyReport
	LoadHomeFiles           func(context.Context) domain.HomeFilesPlanReport
	CaptureHomeFiles        func(context.Context, string, string) domain.HomeFilesCaptureReport
	DiscardHomeFilesCapture func(context.Context, string) error
	CompleteHomeFilesPath   func(context.Context, string) ([]string, error)
	PlanHomeFiles           func(context.Context, domain.HomeFilesDraft) domain.HomeFilesPlanReport
	SaveHomeFiles           func(domain.HomeFilesPlanReport) domain.HomeFilesApplyReport
	PlanSettings            func(context.Context, domain.LabSettingsFile) domain.ConfigPlanReport
	// LoadControllerAddresses lists the controller's current addresses that
	// can be its DHCP address; it is read-only.
	LoadControllerAddresses func(context.Context, domain.LabSettingsFile) domain.ControllerAddresses
	SaveSettings            func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport
	ChangePassword          SettingsPasswordAction
	PreparePXE              func() domain.ActionReport
	LoadPXEProgress         func() (domain.OperationProgress, error)
	PlanPXEStart            func(context.Context) domain.PXELifecycleReport
	StartPXE                func() domain.PXELifecycleReport
	StopPXE                 func() domain.PXELifecycleReport
	RecoverPXE              func() domain.PXELifecycleReport
	PrepareRemoteInstall    func(string) (domain.RemoteInstallResponse, error)
	ObserveRemoteInstall    func(context.Context, string) (string, error)
	BootstrapRemoteInstall  func(string, string, string, []byte) (domain.RemoteInstallResponse, error)
	RemoteInstallRequest    func(domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error)
	LoadRemoteInstall       func(context.Context) (domain.RemoteInstallResponse, error)
	// RemoteReservationPresent reports, without starting the worker, whether
	// a USB installation still holds the controller reservation.
	RemoteReservationPresent func() bool
	// LoadRecovery observes persistent blockers without Nix evaluation.
	LoadRecovery func(context.Context) domain.RecoveryReport
	// Reviewed recoveries of interrupted operations.
	PlanDeploymentRecovery  func(context.Context, bool) domain.DeploymentRecoveryPlan
	ApplyDeploymentRecovery func(domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult
	PlanResetRecovery       func(context.Context) domain.TemplateResetRecoveryPlan
	CreateBackup            func(string, []byte) domain.BackupReport
	PlanGitBackup           func(context.Context, string, string) (domain.GitBackupPlan, error)
	PublishGitBackup        func(domain.GitBackupPlan, []byte) domain.BackupReport
	RestoreLab              func(string, string, string, []byte) domain.BackupReport
	GitBackupDestination    func() (string, string)
	SyncGitBackup           func(context.Context, string) domain.BackupReport
	BackupFileFolder        func() string
	RestoreLabArchive       func(string, string, []byte) domain.BackupReport
	// RestoreFolder proposes where Restore lab puts the restored lab;
	// FindBackupFiles lists backup files on connected drives, newest first.
	RestoreFolder   func() string
	FindBackupFiles func() []string
	// UseRestoredLab makes a restored lab the one this controller manages;
	// the second argument is the backup file it came from, if any.
	UseRestoredLab func(domain.BackupReport, string) domain.BackupReport
	// LabComputerCount reads the number of client computers in the saved
	// settings without evaluating the configuration.
	LabComputerCount   func() (int, bool)
	ApplyResetRecovery func(domain.TemplateResetRecoveryPlan) domain.TemplateResetRecoveryResult
}
