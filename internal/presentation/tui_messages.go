package presentation

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardStatusMsg struct {
	report domain.StatusReport
	err    error
}

type dashboardInitialMsg struct {
	usbReserved bool
	recovery    domain.RecoveryReport
	jobs        []domain.ManagedJob
	jobsErr     error
	jobsID      uint64
	report      domain.StatusReport
	setup       domain.SetupReport
	err         error
}

type dashboardBeginInitialMsg struct{}

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
	recovery  *domain.RecoveryReport
	// refreshLater means the status arrives separately, as a
	// controllerStatusRefreshMsg, after the result is shown.
	refreshLater bool
}

type controllerStatusRefreshMsg struct {
	status domain.StatusReport
	err    error
	// id is the controller apply it follows; a later operation makes it
	// stale.
	id uint64
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

type dashboardGitUndoPlanMsg struct {
	report domain.GitUndoPlanReport
}

type dashboardGitUndoResultMsg struct {
	report domain.GitUndoReport
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
	// recovery is observed after the activation, so the result names the
	// records that still hold the student computers; nil when unavailable.
	recovery *domain.RecoveryReport
}

type dashboardSettingsMsg struct {
	settings  domain.LabSettingsFile
	repair    []domain.ValidationIssue
	addresses domain.ControllerAddresses
	err       error
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
