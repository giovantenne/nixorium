package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

var nixoriumVersion = "development"

type options struct {
	internetAction     domain.InternetAction
	lockAction         domain.LockAction
	command            string
	subcommand         string
	repository         string
	file               string
	expect             string
	on                 string
	service            string
	logID              string
	host               string
	operationID        string
	paths              string
	target             string
	softwarePackage    string
	softwarePreset     string
	softwareExclude    string
	softwareQuery      string
	extension          string
	softwareScope      string
	json               bool
	full               bool
	help               bool
	guided             bool
	verifyOnly         bool
	yes                bool
	allowPrerelease    bool
	allowDowngrade     bool
	allowUnverified    bool
	acknowledgeUnknown bool
	// acknowledgeUnreachable confirms that computers that could not be
	// checked during a recovery review were inspected at their console.
	acknowledgeUnreachable bool
	queueUnreachable       bool
	backupTarget           string
	backupFile             string
	passphraseFile         string
	remove                 bool
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == classroomBrowserCommand {
		os.Exit(runClassroomBrowser(os.Args[2]))
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run adds the next step after a failed command whose output named a known
// blocker. JSON output carries the same steps in its issues instead.
func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	recorder := &nextStepRecorder{}
	commandStdout := recordWriter(stdout, recorder)
	for _, argument := range arguments {
		if argument == "--json" {
			commandStdout = stdout
		}
	}
	code := runCommand(ctx, arguments, commandStdout, recordWriter(stderr, recorder))
	if code != 0 {
		recorder.report(stderr)
	}
	return code
}

func runCommand(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	options, err := parseArguments(arguments)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		usage(stderr)
		return 2
	}
	if options.help {
		usage(stdout)
		return 0
	}
	if options.command == "telemetry" {
		return runTelemetryCommand(ctx, options, stdout, stderr)
	}
	// The classroom view needs only the classroom service: the teacher
	// cannot read the deployment repository.
	if options.command == "classroom-view" {
		message, err := openClassroomView(ctx)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		fmt.Fprintln(stdout, message)
		return 0
	}
	repository := options.repository
	resolvedRepository, resolveErr := resolveRepository(options.repository)
	if resolveErr == nil {
		repository = resolvedRepository
	} else {
		if options.command == "" && options.repository == "" {
			if handled, code := tryRunClassroomDashboard(ctx, stderr); handled {
				return code
			}
			if message, teacher := classroomUnavailable(); teacher {
				fmt.Fprintln(stderr, message)
				return 1
			}
		}
		if commandRequiresRepository(options) {
			fmt.Fprintln(stderr, "Error:", resolveErr)
			return 1
		}
	}

	local := adapters.Local{}
	inspector := app.NewInspector(local)
	switch options.command {
	case "":
		if options.json {
			report, inspectErr := inspector.Status(ctx, repository)
			if inspectErr != nil {
				fmt.Fprintln(stderr, "Error:", inspectErr)
				return 1
			}
			if jsonErr := presentation.JSON(stdout, report); jsonErr != nil {
				fmt.Fprintln(stderr, "Error:", jsonErr)
				return 1
			}
			return 0
		}
		return runDashboardProgram(ctx, repository, false, stderr)
	case "recovery":
		report := app.NewRecoveryInspector(local).Observe(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.RecoveryText(stdout, report)
		}
	case "status":
		report, inspectErr := inspector.Status(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.StatusText(stdout, report)
		}
	case "hosts":
		report, inspectErr := inspector.Hosts(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.HostsText(stdout, report)
		}
	case "host-key":
		return runHostTrustCommand(ctx, repository, options, stdout, stderr)
	case "deploy":
		if strings.HasPrefix(options.subcommand, "recover") {
			return runDeploymentRecoveryCommand(ctx, repository, options, stdout, stderr)
		}
		return runDeploymentCommand(ctx, repository, options, stdout, stderr)
	case "template-reset":
		return runTemplateResetRecoveryCommand(ctx, repository, options, stdout, stderr)
	case "backup":
		return runBackupCommand(ctx, repository, options, stdout, stderr)
	case "controller":
		return runControllerCommand(ctx, repository, options, stdout, stderr)
	case "services":
		return runServicesCommand(ctx, repository, options, stdout, stderr)
	case "logs":
		return runLogsCommand(options, stdout, stderr)
	case "support":
		return runSupportCommand(ctx, repository, options, stdout, stderr)
	case "git":
		return runGitCommand(ctx, repository, options, stdout, stderr)
	case "package-base":
		return runPackageBaseCommand(ctx, repository, options, stdout, stderr)
	case "update":
		return runUpdateCommand(ctx, repository, options, stdout, stderr)
	case "software":
		return runSoftwareCommand(ctx, repository, options, stdout, stderr)
	case "workspace":
		return runWorkspaceCommand(ctx, repository, options, stdout, stderr)
	case "internet":
		return runInternetCommand(ctx, repository, options, stdout, stderr)
	case "lock":
		return runLockCommand(ctx, repository, options, stdout, stderr)
	case "send":
		return runSendCommand(ctx, options, stdout, stderr)
	case "cleanup":
		return runCleanupCommand(ctx, repository, options, stdout, stderr)
	case "shutdown", "restart":
		return runShutdownCommand(ctx, repository, options, stdout, stderr)
	case "install":
		return runInstallCommand(ctx, repository, options, stdout, stderr)
	case "doctor":
		report, inspectErr := inspector.Doctor(ctx, repository, app.DoctorOptions{Full: options.full})
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.DoctorText(stdout, report)
		}
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		if report.HasErrors() {
			return 1
		}
	case "config":
		return runConfigCommand(ctx, repository, options, stdout, stderr)
	case "setup":
		return runSetupCommand(ctx, repository, options, stdout, stderr)
	case "bootstrap":
		return runBootstrapConfigure(ctx, repository, stdout, stderr)
	case "pxe":
		return runPXECommand(ctx, repository, options, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Error: unknown command %q\n", options.command)
		usage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func runDashboardProgram(ctx context.Context, repository string, setupMode bool, stderr io.Writer) int {
	local := adapters.Local{}
	inspector := app.NewInspector(local)
	setupManager := app.NewSetupManager(local)
	lifecycle := app.NewPXELifecycle(local)
	deploymentManager := app.NewDeploymentManager(local)
	controllerManager := app.NewControllerManager(local)
	configurationStateManager := app.NewConfigurationStateManager(inspector, controllerManager)
	serviceManager := app.NewServiceManager(local)
	operationLogManager := app.NewOperationLogManager(local)
	supportManager := app.NewSupportManager(local)
	gitReviewManager := app.NewGitReviewManager(local)
	gitCommitManager := app.NewGitCommitManager(local)
	updateManager := app.NewUpdateManager(local)
	updateNotifications := app.NewUpdateNotificationManager(local)
	baseSource := adapters.PackageBase{}
	baseManager := app.NewPackageBaseManager(baseSource)
	settingsManager := app.NewSettingsManager(local)
	settingsSaveManager := app.NewSettingsSaveManager(settingsManager, gitReviewManager, gitCommitManager)
	configurationSaveManager := app.NewManagedConfigurationSaveManager(gitReviewManager, gitCommitManager)
	updateSaveManager := app.NewUpdateSaveManager(updateManager, local, gitReviewManager, configurationSaveManager)
	baseSaveManager := app.NewUpdateSaveManager(baseManager, baseSource, gitReviewManager, configurationSaveManager)
	softwareManager := app.NewSoftwareManager(local)
	workspaceManager := app.NewWorkspaceManager(local)
	workspaceMarketplace := app.NewWorkspaceMarketplace(local)
	templateResetManager := app.NewTemplateResetManager(adapters.TemplateReset{})
	softwareSaveManager := app.NewSoftwareSaveManager(softwareManager, gitReviewManager, configurationSaveManager)
	softwarePresetSaveManager := app.NewSoftwarePresetSaveManager(softwareManager, gitReviewManager, configurationSaveManager)
	shutdownManager := app.NewShutdownManager(local)
	cleanupManager := app.NewCleanupManager(local)
	internetManager := app.NewInternetManager(local)
	lockManager := app.NewLockManager(lockSource{Local: local})
	progressManager := app.NewOperationProgressManager(local)
	actions := presentation.DashboardActions{
		LoadClientSetup: func(context.Context) (bool, error) {
			settings, _, err := settingsManager.CurrentForEditing(repository)
			return settings.InstallationSettingsComplete(), err
		},
		LoadDisclaimer:   func(context.Context) (bool, error) { return local.DisclaimerAccepted(repository) },
		AcceptDisclaimer: func() error { return local.AcceptDisclaimer(repository) },
		CheckUpdateNotification: func(ctx context.Context) (domain.UpdateNotification, error) {
			return updateNotifications.Check(ctx, repository)
		},
		DismissUpdateNotification: func(notice domain.UpdateNotification) error { return updateNotifications.Dismiss(repository, notice) },
		LoadTemplateReset: func(requestContext context.Context) domain.TemplateResetCatalog {
			return templateResetManager.Catalog(requestContext, repository)
		},
		PlanTemplateReset: func(requestContext context.Context, preset string, progress func(string)) domain.TemplateResetPlan {
			return templateResetManager.Plan(requestContext, repository, preset, progress)
		},
		ApplyTemplateReset: func(plan domain.TemplateResetPlan) domain.TemplateResetResult {
			return templateResetManager.Apply(ctx, plan)
		},
		LoadWorkspace: func(requestContext context.Context) domain.WorkspacePlanReport {
			return workspaceManager.Load(requestContext, repository)
		},
		PlanWorkspace: func(requestContext context.Context, candidate domain.WorkspaceProfile) domain.WorkspacePlanReport {
			data, _ := domain.MarshalWorkspaceProfile(candidate)
			return workspaceManager.Plan(requestContext, repository, data)
		},
		ResolveMarketplace: func(requestContext context.Context, id string) domain.WorkspaceMarketplaceReport {
			return workspaceMarketplace.Resolve(requestContext, repository, id)
		},
		SaveWorkspace: func(plan domain.WorkspacePlanReport) domain.WorkspaceApplyReport {
			return app.NewWorkspaceSaveManager(local, gitReviewManager).Save(ctx, plan)
		},
		RunningVersion:  nixoriumVersion,
		LoadPackageBase: func(ctx context.Context) domain.PackageBaseStatus { return baseManager.PackageBaseStatus(repository) },
		PlanPackageBase: func(ctx context.Context, target string, allowUnverified bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
			return baseManager.PlanWithProgress(ctx, repository, target, allowUnverified, false, progress)
		},
		SavePackageBase: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			report := baseSaveManager.Save(ctx, plan)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadInitial: func(ctx context.Context) (domain.StatusReport, domain.SetupReport, error) {
			if setupMode {
				return domain.StatusReport{}, setupManager.Status(ctx, repository), nil
			}
			setup := setupManager.InitialStatus(ctx, repository)
			if setupStartsBeforeDashboardInspection(setup) {
				return domain.StatusReport{}, setup, nil
			}
			report, err := inspector.Startup(ctx, repository)
			if err != nil {
				return domain.StatusReport{}, domain.SetupReport{}, err
			}
			return report, setup, nil
		},
		LoadInventory: func(requestContext context.Context) (domain.StatusReport, error) {
			return inspector.Inventory(requestContext, repository)
		},
		LoadDoctor: func(ctx context.Context) (domain.DoctorReport, error) {
			return inspector.Doctor(ctx, repository, app.DoctorOptions{})
		},
		Telemetry: app.NewTelemetryManager(adapters.DefaultTelemetry()).Run,
		PreviewSupport: func(requestContext context.Context) (domain.SupportSnapshot, error) {
			return supportManager.Preview(requestContext, repository, nixoriumVersion)
		},
		ExportSupport: func(snapshot domain.SupportSnapshot) domain.SupportExportResult {
			return supportManager.Export(ctx, snapshot)
		},
		Refresh: func(ctx context.Context) (domain.StatusReport, error) {
			return inspector.Status(ctx, repository)
		},
		LoadSetup: func(ctx context.Context) domain.SetupReport {
			return setupManager.Status(ctx, repository)
		},
		LoadSetupKeys: func(ctx context.Context) domain.KeyReconcileReport {
			return setupManager.VerifyKeys(ctx, repository)
		},
		ReconcileSetupKeys: func() (domain.KeyReconcileReport, error) {
			return setupManager.ReconcileKeys(ctx, repository)
		},
		ImportSetupKey: func(name, sourcePath string) (domain.KeyImportReport, error) {
			return setupManager.ImportKey(ctx, repository, name, sourcePath)
		},
		SaveSetupConfiguration: func() domain.ConfigurationSaveReport {
			return configurationSaveManager.SaveChanged(ctx, repository, []string{
				"lab-settings.json",
				"keys/cache-public-key",
				"keys/admin-ssh.pub",
			})
		},
		InstallSetupSecrets: func() domain.ActionReport {
			report := app.NewSystemActions(local).InstallSecrets(ctx)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadHosts: func(ctx context.Context) (domain.HostsReport, error) {
			return inspector.Hosts(ctx, repository)
		},
		PlanHostTrust: func(readContext context.Context, name string) domain.HostTrustPlan {
			return app.NewHostTrustManager(local).Plan(readContext, repository, name)
		},
		ApplyHostTrust: func(plan domain.HostTrustPlan) domain.HostTrustResult {
			report := app.NewHostTrustManager(local).Apply(ctx, plan, plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadConfigurationState: func(ctx context.Context) domain.ConfigurationStateReport {
			return configurationStateManager.Load(ctx, repository)
		},
		LoadSoftware: func(ctx context.Context) domain.SoftwareCatalogReport {
			return softwareManager.Catalog(ctx, repository)
		},
		SearchSoftware: func(searchContext context.Context, query string) domain.SoftwareSearchReport {
			return softwareManager.Search(searchContext, repository, query)
		},
		PlanSoftware: func(ctx context.Context, request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
			return softwareManager.Plan(ctx, repository, request)
		},
		SaveSoftware: func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
			return softwareSaveManager.Save(ctx, plan)
		},
		LoadSoftwarePresets: func(ctx context.Context) domain.SoftwarePresetCatalogReport {
			return softwareManager.Presets(ctx, repository)
		},
		PlanSoftwarePreset: func(ctx context.Context, request domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport {
			return softwareManager.PlanPreset(ctx, repository, request)
		},
		SaveSoftwarePreset: func(plan domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
			return softwarePresetSaveManager.Save(ctx, plan)
		},
		PlanInternet: func(ctx context.Context, requested string, action domain.InternetAction) domain.InternetPlan {
			return internetManager.Plan(ctx, repository, requested, action)
		},
		ApplyInternet: func(plan domain.InternetPlan) domain.InternetReport {
			report := internetManager.Apply(ctx, plan, plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		PlanCleanup: func(ctx context.Context, requested string) domain.CleanupPlanReport {
			return cleanupManager.Plan(ctx, repository, requested)
		},
		PlanLock: func(ctx context.Context, requested string, action domain.LockAction) domain.LockPlan {
			return lockManager.Plan(ctx, repository, requested, action)
		},
		ApplyLock: func(plan domain.LockPlan) domain.LockReport {
			return lockManager.Apply(ctx, plan, plan.ReviewToken)
		},
		// The administrator's files go through the classroom service too.
		PlanShare: planShare,
		ApplyShare: func(plan domain.SharePlan) domain.ShareReport {
			return applyShare(ctx, plan)
		},
		ApplyCleanup: func(plan domain.CleanupPlanReport) domain.CleanupApplyReport {
			report := cleanupManager.Apply(ctx, plan, plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		PlanShutdown: func(ctx context.Context, requested string, policy domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
			return shutdownManager.Plan(ctx, repository, requested, policy)
		},
		PlanPower: func(ctx context.Context, requested string, policy domain.ShutdownSessionPolicy, action domain.ClientPowerAction) domain.ShutdownPlanReport {
			return shutdownManager.PlanAction(ctx, repository, requested, policy, action)
		},
		ApplyShutdown: func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
			report := shutdownManager.ApplyPlan(ctx, plan, plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		PlanDeployment: func(ctx context.Context, requested string) domain.DeploymentPlanReport {
			return deploymentManager.Plan(ctx, repository, requested)
		},
		PlanReachableDeployment: func(ctx context.Context, reviewed domain.DeploymentPlanReport) domain.DeploymentPlanReport {
			return deploymentManager.PlanReachable(ctx, repository, reviewed)
		},
		ApplyDeployment: func(requestContext context.Context, plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			return executeDeploymentOperationWithProgress(requestContext, deploymentManager, repository, plan.Requested, plan.Revision, io.Discard, observe)
		},
		ApplyDeploymentQueued: func(requestContext context.Context, plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			return applyDeploymentQueued(requestContext, deploymentManager, repository, plan, observe)
		},
		PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
			return controllerManager.Plan(ctx, repository)
		},
		ApplyController: func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			report := controllerManager.Apply(ctx, repository, plan.Revision)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadControllerProgress: func() (domain.OperationProgress, error) {
			return progressManager.Current("controller-apply")
		},
		LoadManagedJobs: local.ObserveManagedJobs,
		LoadServices: func(ctx context.Context) domain.ServicesReport {
			return serviceManager.Status(ctx, repository)
		},
		RestartService: func(service string) domain.ServiceActionReport {
			report := serviceManager.Restart(ctx, repository, service)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadLogs: func(ctx context.Context) domain.OperationLogsReport {
			return operationLogManager.List()
		},
		LoadLog: func(ctx context.Context, id string) domain.OperationLogReport {
			return operationLogManager.Show(id)
		},
		LoadGitReview: func(ctx context.Context) domain.GitReviewReport {
			return gitReviewManager.Review(ctx, repository)
		},
		PlanGitDiscard: func(ctx context.Context, paths string) domain.GitCommitPlanReport {
			return app.NewGitDiscardManager(local).Plan(ctx, repository, paths)
		},
		ApplyGitDiscard: func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
			report := app.NewGitDiscardManager(local).Apply(ctx, repository, strings.Join(plan.Paths, ","), plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		PlanGitCommit: func(ctx context.Context, paths string) domain.GitCommitPlanReport {
			return gitCommitManager.Plan(ctx, repository, paths)
		},
		ApplyGitCommit: func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
			report := gitCommitManager.Apply(ctx, repository, strings.Join(plan.Paths, ","), plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		CheckUpdate: func(ctx context.Context) domain.UpdateCheckReport {
			return updateManager.Check(ctx, repository)
		},
		PlanUpdate: func(ctx context.Context, target string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			return updateManager.Plan(ctx, repository, target, allowPrerelease, allowDowngrade)
		},
		PlanUpdateWithProgress: func(ctx context.Context, target string, allowPrerelease, allowDowngrade bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
			return updateManager.PlanWithProgress(ctx, repository, target, allowPrerelease, allowDowngrade, progress)
		},
		SaveUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			report := updateSaveManager.Save(ctx, plan)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		SettingsRepairIssues: func() []domain.ValidationIssue {
			_, issues, _ := settingsManager.CurrentForEditing(repository)
			return issues
		},
		LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) {
			settings, _, err := settingsManager.CurrentForEditing(repository)
			if err != nil {
				return domain.LabSettingsFile{}, err
			}
			if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder {
				staticAddress, _ := domain.ControllerStaticAddress(settings.Lab)
				settings = applyDetectedNetworkDefaults(settings, local.DetectNetworkDefaults(staticAddress))
			}
			return settings, nil
		},
		PlanSettings: func(ctx context.Context, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
			return settingsManager.PlanSettings(ctx, repository, candidate)
		},
		SaveSettings: func(candidate domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			return settingsSaveManager.Save(ctx, repository, candidate, plan)
		},
		ChangePassword: func(account string, settings domain.LabSettingsFile, input *os.File, output io.Writer) (domain.LabSettingsFile, error) {
			if account == "all" {
				reader := presentation.TerminalSecretReader{Input: input, Output: output}
				if err := collectSetupCredentials(ctx, reader, local, output, &settings); err != nil {
					return settings, err
				}
				return settings, nil
			}
			return collectSettingsPassword(ctx, local, input, output, account, settings)
		},
		PreparePXE: func() domain.ActionReport {
			report := app.NewSystemActions(local).PreparePXE(ctx)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadPXEProgress: func() (domain.OperationProgress, error) {
			return progressManager.Current("pxe-prepare")
		},
		PlanPXEStart: func(ctx context.Context) domain.PXELifecycleReport {
			return lifecycle.PlanStart(ctx, repository)
		},
		StartPXE: func() domain.PXELifecycleReport {
			report := lifecycle.Start(ctx, repository)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		StopPXE: func() domain.PXELifecycleReport {
			report := lifecycle.Stop(ctx, repository)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		RecoverPXE: func() domain.PXELifecycleReport {
			report := lifecycle.Recover(ctx, repository)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		PrepareRemoteInstall: func(host string) (domain.RemoteInstallResponse, error) {
			return dashboardRemoteInstallRequest(ctx, repository, domain.RemoteInstallRequest{
				Operation: domain.RemoteInstallPrepareOperation, Host: host,
			}, nil)
		},
		ObserveRemoteInstall: func(ctx context.Context, address string) (string, error) {
			return adapters.NewLiveBootstrap().ObserveHostFingerprint(ctx, address)
		},
		BootstrapRemoteInstall: func(host, address, fingerprint string, password []byte) (domain.RemoteInstallResponse, error) {
			return dashboardRemoteInstallRequest(ctx, repository, domain.RemoteInstallRequest{
				Operation: domain.RemoteInstallBootstrapOperation, Host: host, Address: address, Fingerprint: fingerprint,
			}, password)
		},
		RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			return dashboardRemoteInstallRequest(ctx, repository, request, nil)
		},
		RemoteReservationPresent: local.RemoteReservationPresent,
		LoadRecovery: func(ctx context.Context) domain.RecoveryReport {
			return app.NewRecoveryInspector(local).Observe(ctx, repository)
		},
		LoadDeferredUpdates: func(ctx context.Context) domain.DeferredUpdateStatus {
			return newDeferredUpdateManager().Status(ctx, repository)
		},
		CancelDeferredUpdates: func(hosts []string) error {
			_, err := newDeferredUpdateManager().Cancel(hosts)
			return err
		},
		PlanDeploymentRecovery: func(ctx context.Context, acknowledge bool) domain.DeploymentRecoveryPlan {
			return app.NewDeploymentRecoveryManager(local).Plan(ctx, repository, acknowledge)
		},
		ApplyDeploymentRecovery: func(plan domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult {
			result := app.NewDeploymentRecoveryManager(local).Apply(ctx, plan, plan.ReviewToken)
			result.Message = operationRecordMessage(result.Message, result)
			return result
		},
		PlanResetRecovery: func(ctx context.Context) domain.TemplateResetRecoveryPlan {
			return adapters.TemplateReset{}.PlanTemplateResetRecovery(ctx, repository)
		},
		CreateBackup: func(destination string, passphrase []byte) domain.BackupReport {
			return local.CreateBackup(ctx, repository, destination, passphrase, nixoriumVersion)
		},
		BackupDestination: defaultBackupDestination,
		OpenClassroomView: classroomViewAction(),
		ApplyResetRecovery: func(plan domain.TemplateResetRecoveryPlan) domain.TemplateResetRecoveryResult {
			result := adapters.TemplateReset{}.ApplyTemplateResetRecovery(ctx, plan)
			result.Message = operationRecordMessage(result.Message, result)
			return result
		},
		LoadRemoteInstall: func(ctx context.Context) (domain.RemoteInstallResponse, error) {
			return dashboardRemoteInstallRequest(ctx, repository, domain.RemoteInstallRequest{Operation: domain.RemoteInstallWorkerProbeOperation}, nil)
		},
	}
	// Locking screens needs the classroom agents on the clients.
	if !classroomViewSetting(local, repository) {
		actions.PlanLock, actions.ApplyLock = nil, nil
		actions.PlanShare, actions.ApplyShare = nil, nil
	}
	tuiErr := presentation.RunLoadingDashboard(actions, setupMode)
	if tuiErr != nil {
		fmt.Fprintln(stderr, "Error:", tuiErr)
		return 1
	}
	return 0
}

func writePXEPreparationActivity(writer io.Writer) {
	fmt.Fprintln(writer, "Preparing PXE artifacts and client closures; verbose output: journalctl -fu nixorium-prepare-pxe.service")
}

func writeControllerApplyActivity(writer io.Writer, unit string) {
	fmt.Fprintf(writer, "Building and activating the reviewed controller; verbose output: journalctl -fu %s\n", unit)
}

func runWithManagedProgress[T any](action func() T, load func() (domain.OperationProgress, error), writer io.Writer) T {
	started := time.Now().UTC()
	results := make(chan T, 1)
	go func() {
		results <- action()
	}()
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	var lastUpdate time.Time
	render := func() {
		progress, err := load()
		if err != nil || progress.StartedAt.Before(started) || !progress.UpdatedAt.After(lastUpdate) {
			return
		}
		lastUpdate = progress.UpdatedAt
		activity := "Waiting for the next managed activity"
		if len(progress.Recent) > 0 {
			activity = progress.Recent[len(progress.Recent)-1]
		}
		counter := ""
		if progress.Total > 0 {
			counter = fmt.Sprintf(" (%d/%d)", progress.Current, progress.Total)
		}
		fmt.Fprintf(writer, "Progress [%s]%s: %s\n", progress.Phase, counter, activity)
	}
	for {
		select {
		case result := <-results:
			render()
			return result
		case <-ticker.C:
			render()
		}
	}
}

func commandRequiresRepository(options options) bool {
	return options.command != "logs" && (options.command != "backup" || options.subcommand == "create") && (options.command != "pxe" || (options.subcommand != "stop" && options.subcommand != "recover"))
}

func parseArguments(arguments []string) (options, error) {
	if len(arguments) > 0 && arguments[0] == "telemetry" {
		return parseTelemetryArguments(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "host-key" {
		return parseHostTrustArguments(arguments[1:])
	}
	result := options{}
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--to", "--passphrase-file":
			flag := arguments[index]
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, fmt.Errorf("%s requires a path", flag)
			}
			if flag == "--to" {
				result.backupTarget = arguments[index]
			} else {
				result.passphraseFile = arguments[index]
			}
		case "--repo":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--repo requires a path")
			}
			result.repository = arguments[index]
		case "--file":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--file requires a path")
			}
			result.file = arguments[index]
		case "--expect":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--expect requires a review token")
			}
			result.expect = arguments[index]
		case "--action":
			index++
			if index >= len(arguments) {
				return options{}, errors.New("--action requires block/unblock or lock/unlock")
			}
			result.internetAction = domain.InternetAction(arguments[index])
		case "--on":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--on requires a client or @lab")
			}
			result.on = arguments[index]
		case "--host":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--host requires an inventory client name")
			}
			result.host = arguments[index]
		case "--id":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--id requires a remote installation operation ID")
			}
			result.operationID = arguments[index]
		case "--paths":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--paths requires comma-separated repository paths")
			}
			result.paths = arguments[index]
		case "--target":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--target requires a release tag")
			}
			result.target = arguments[index]
		case "--package":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--package requires a catalog identifier")
			}
			result.softwarePackage = arguments[index]
		case "--preset":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--preset requires a software profile id")
			}
			result.softwarePreset = arguments[index]
		case "--exclude":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--exclude requires comma-separated package ids")
			}
			result.softwareExclude = arguments[index]
		case "--query":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--query requires a package-name fragment")
			}
			result.softwareQuery = arguments[index]
		case "--extension":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--extension requires a Marketplace identifier such as publisher.name")
			}
			result.extension = arguments[index]
		case "--scope":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--scope requires shared, controller, all-clients, group:NAME, or clients:pcNN,...")
			}
			result.softwareScope = arguments[index]
		case "--remove":
			result.remove = true
		case "--allow-prerelease":
			result.allowPrerelease = true
		case "--allow-downgrade":
			result.allowDowngrade = true
		case "--allow-unverified":
			result.allowUnverified = true
		case "--acknowledge-unknown-sessions":
			result.acknowledgeUnknown = true
		case "--acknowledge-unreachable":
			result.acknowledgeUnreachable = true
		case "--queue-unreachable":
			result.queueUnreachable = true
		case "queue", "run":
			// deploy queue status|cancel|run: client updates that wait for
			// their computers.
			if result.command != "deploy" {
				return options{}, fmt.Errorf("%s must follow deploy", arguments[index])
			}
			if arguments[index] == "queue" && result.subcommand == "" {
				result.subcommand = "queue"
				continue
			}
			if arguments[index] != "queue" && result.subcommand == "queue" {
				result.subcommand = "queue-" + arguments[index]
				continue
			}
			return options{}, errors.New("use deploy queue status, deploy queue cancel or deploy queue run")
		case "--json":
			result.json = true
		case "--full":
			result.full = true
		case "--verify-only":
			result.verifyOnly = true
		case "--yes":
			result.yes = true
		case "-h", "--help", "help":
			result.help = true
		case "status":
			if result.command == "deploy" && result.subcommand == "queue" {
				result.subcommand = "queue-status"
				continue
			}
			if result.command == "install" && result.subcommand == "usb" {
				result.subcommand = "usb-status"
				continue
			}
			if (result.command == "setup" || result.command == "package-base" || result.command == "recovery") && result.subcommand == "" {
				result.subcommand = "status"
				continue
			}
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "doctor", "hosts", "deploy", "controller", "services", "logs", "git", "config", "setup", "bootstrap", "pxe", "update", "package-base", "software", "workspace", "shutdown", "internet", "lock", "send", "classroom-view", "install", "support", "cleanup", "recovery", "template-reset", "backup":
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "preview", "export":
			if result.command != "support" || result.subcommand != "" {
				return options{}, errors.New("preview/export must follow support")
			}
			result.subcommand = arguments[index]
		case "usb":
			if result.command != "install" || result.subcommand != "" {
				return options{}, errors.New("usb must follow install")
			}
			result.subcommand = "usb"
		case "validate":
			if result.command != "config" || result.subcommand != "" {
				return options{}, errors.New("validate must follow config")
			}
			result.subcommand = "validate"
		case "check":
			if result.command != "update" || result.subcommand != "" {
				return options{}, errors.New("check must follow update")
			}
			result.subcommand = "check"
		case "catalog":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("catalog must follow software")
			}
			result.subcommand = "catalog"
		case "presets":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("presets must follow software")
			}
			result.subcommand = "presets"
		case "preset":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("preset must follow software")
			}
			result.subcommand = "preset"
		case "marketplace":
			if result.command != "workspace" || result.subcommand != "" {
				return options{}, errors.New("marketplace must follow workspace")
			}
			result.subcommand = "marketplace"
		case "search":
			if result.command != "software" || result.subcommand != "" {
				return options{}, errors.New("search must follow software")
			}
			result.subcommand = "search"
		case "plan":
			if result.command == "git" && result.subcommand == "discard" {
				result.subcommand = "discard-plan"
				continue
			}
			if result.command == "git" && result.subcommand == "commit" {
				result.subcommand = "commit-plan"
				continue
			}
			if result.command == "software" && result.subcommand == "preset" {
				result.subcommand = "preset-plan"
				continue
			}
			if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "recover" {
				result.subcommand = "recover-plan"
				continue
			}
			if (result.command != "config" && result.command != "deploy" && result.command != "controller" && result.command != "update" && result.command != "package-base" && result.command != "software" && result.command != "workspace" && result.command != "shutdown" && result.command != "restart" && result.command != "internet" && result.command != "lock" && result.command != "send" && result.command != "cleanup") || result.subcommand != "" {
				return options{}, errors.New("plan must follow config, deploy, controller, update, software, workspace, shutdown, restart, Internet, or git commit")
			}
			result.subcommand = "plan"
		case "keys":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("keys must follow setup")
			}
			result.subcommand = "keys"
		case "configure":
			if (result.command != "setup" && result.command != "bootstrap") || result.subcommand != "" {
				return options{}, errors.New("configure must follow setup or bootstrap")
			}
			result.subcommand = "configure"
		case "install-secrets":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("install-secrets must follow setup")
			}
			result.subcommand = "install-secrets"
		case "apply":
			if result.command == "git" && result.subcommand == "discard" {
				result.subcommand = "discard-apply"
				continue
			}
			if result.command == "git" && result.subcommand == "commit" {
				result.subcommand = "commit-apply"
				continue
			}
			if result.command == "software" && result.subcommand == "preset" {
				result.subcommand = "preset-apply"
				continue
			}
			if (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "recover" {
				result.subcommand = "recover-apply"
				continue
			}
			if (result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "" {
				result.subcommand = "apply"
				continue
			}
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("apply must follow config, deploy, controller, update, software, workspace, shutdown, restart, Internet, or setup")
			}
			result.subcommand = "apply"
		case "restart":
			if result.command == "" {
				result.command = "restart"
				continue
			}
			if result.command != "services" || result.subcommand != "" {
				return options{}, errors.New("restart must follow services")
			}
			result.subcommand = "restart"
		case "show":
			if result.command != "logs" || result.subcommand != "" {
				return options{}, errors.New("show must follow logs")
			}
			result.subcommand = "show"
		case "review":
			if result.command != "git" || result.subcommand != "" {
				return options{}, errors.New("review must follow git")
			}
			result.subcommand = "review"
		case "commit", "discard":
			if result.command != "git" || result.subcommand != "" {
				return options{}, fmt.Errorf("%s must follow git", arguments[index])
			}
			result.subcommand = arguments[index]
		case "cache":
			if result.command != "services" || result.subcommand != "restart" || result.service != "" {
				return options{}, fmt.Errorf("unexpected argument %q", arguments[index])
			}
			result.service = "cache"
		case "prepare":
			if result.command == "install" && result.subcommand == "usb" {
				result.subcommand = "usb-prepare"
				continue
			}
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, errors.New("prepare must follow pxe")
			}
			result.subcommand = "prepare"
		case "start":
			if result.command == "install" && result.subcommand == "usb" {
				result.subcommand = "usb-start"
				continue
			}
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, errors.New("start must follow pxe or install usb")
			}
			result.subcommand = "start"
		case "stop", "recover":
			if arguments[index] == "recover" && (result.command == "deploy" || result.command == "template-reset") && result.subcommand == "" {
				result.subcommand = "recover"
				continue
			}
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, fmt.Errorf("%s must follow pxe", arguments[index])
			}
			result.subcommand = arguments[index]
		case "create", "verify", "restore":
			if result.command == "backup" && result.subcommand == "" {
				result.subcommand = arguments[index]
				continue
			}
			if arguments[index] != "verify" {
				return options{}, fmt.Errorf("%s must follow backup", arguments[index])
			}
			if result.command != "install" || result.subcommand != "usb" {
				return options{}, errors.New("verify must follow install usb or backup")
			}
			result.subcommand = "usb-verify"
		case "reconcile", "reboot", "cancel", "close":
			if arguments[index] == "cancel" && result.command == "deploy" && result.subcommand == "queue" {
				result.subcommand = "queue-cancel"
				continue
			}
			if result.command != "install" || result.subcommand != "usb" {
				return options{}, fmt.Errorf("%s must follow install usb", arguments[index])
			}
			result.subcommand = "usb-" + arguments[index]
		default:
			if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
				result.logID = arguments[index]
				continue
			}
			if result.command == "backup" && (result.subcommand == "verify" || result.subcommand == "restore") && result.backupFile == "" {
				result.backupFile = arguments[index]
				continue
			}
			return options{}, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}
	if result.full && result.command != "doctor" {
		return options{}, errors.New("--full is only valid with doctor")
	}
	if result.command == "support" {
		if result.subcommand != "preview" && result.subcommand != "export" {
			return options{}, errors.New("support requires preview or export")
		}
		if result.subcommand == "export" && result.json {
			return options{}, errors.New("support export requires interactive review; use support preview --json for machine-readable output")
		}
	}
	if result.verifyOnly && (result.command != "setup" || result.subcommand != "keys") {
		return options{}, errors.New("--verify-only is only valid with setup keys")
	}
	if result.yes && !((result.command == "setup" && result.subcommand == "apply") || (result.command == "pxe" && result.subcommand == "start") || ((result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "apply") || (result.command == "software" && result.subcommand == "preset-apply") || (result.command == "services" && result.subcommand == "restart") || (result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply")) || result.subcommand == "recover-apply") {
		return options{}, errors.New("--yes is only valid with setup apply, pxe start, deploy apply, controller apply, update apply, software apply, workspace apply, shutdown/restart apply, cleanup apply, services restart, or git commit apply")
	}
	if result.command == "config" && result.subcommand != "validate" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("config requires the validate, plan, or apply subcommand")
	}
	if result.command == "bootstrap" && result.subcommand != "configure" {
		return options{}, errors.New("bootstrap requires the configure subcommand")
	}
	if result.file != "" && ((result.command != "config" && result.command != "workspace" && result.command != "send") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--file is only valid with config, workspace or send plan/apply")
	}
	if result.command == "config" && (result.subcommand == "plan" || result.subcommand == "apply") && result.file == "" {
		return options{}, fmt.Errorf("config %s requires --file", result.subcommand)
	}
	if result.expect != "" && !(((result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "package-base" || result.command == "software" || result.command == "workspace" || result.command == "shutdown" || result.command == "restart" || result.command == "internet" || result.command == "lock" || result.command == "send" || result.command == "cleanup") && result.subcommand == "apply") || (result.command == "software" && result.subcommand == "preset-apply") || (result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply")) || result.subcommand == "recover-apply") {
		return options{}, errors.New("--expect is only valid with config apply, deploy apply, controller apply, update apply, software apply, workspace apply, shutdown/restart apply, cleanup apply, or git commit apply")
	}
	if result.extension != "" && (result.command != "workspace" || result.subcommand != "marketplace") {
		return options{}, errors.New("--extension is only valid with workspace marketplace")
	}
	if result.command == "workspace" && result.subcommand == "marketplace" && result.extension == "" {
		return options{}, errors.New("workspace marketplace requires --extension")
	}
	if result.command == "workspace" && result.subcommand != "marketplace" {
		if result.subcommand != "plan" && result.subcommand != "apply" {
			return options{}, errors.New("workspace requires plan, apply or marketplace")
		}
		if result.file == "" {
			return options{}, errors.New("workspace plan/apply requires --file")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("workspace apply requires --expect from workspace plan")
		}
	}
	if result.on != "" && ((result.command != "deploy" && result.command != "shutdown" && result.command != "restart" && result.command != "internet" && result.command != "lock" && result.command != "send" && result.command != "cleanup") || (result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "queue-cancel")) {
		return options{}, errors.New("--on is only valid with deploy, shutdown, restart, Internet or cleanup plan/apply, and deploy queue cancel")
	}
	if result.command == "cleanup" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" {
			return options{}, errors.New("cleanup requires plan or apply and --on <controller,pcNN,...|@lab>")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("cleanup apply requires --expect from cleanup plan")
		}
	}
	if (result.command == "deploy" || result.command == "template-reset") && strings.HasPrefix(result.subcommand, "recover") {
		if result.subcommand != "recover-plan" && result.subcommand != "recover-apply" {
			return options{}, fmt.Errorf("%s recover requires plan or apply", result.command)
		}
		if result.on != "" {
			return options{}, errors.New("--on is not used by recovery; the interrupted record names its computers")
		}
		if result.subcommand == "recover-apply" && result.expect == "" {
			return options{}, fmt.Errorf("%s recover apply requires --expect from %s recover plan", result.command, result.command)
		}
		if result.acknowledgeUnreachable && result.command != "deploy" {
			return options{}, errors.New("--acknowledge-unreachable is only valid with deploy recover")
		}
	} else if result.command == "template-reset" {
		return options{}, errors.New("template-reset accepts only recover plan or recover apply")
	} else if result.acknowledgeUnreachable {
		return options{}, errors.New("--acknowledge-unreachable is only valid with deploy recover")
	}
	recovering := strings.HasPrefix(result.subcommand, "recover-")
	queueing := strings.HasPrefix(result.subcommand, "queue")
	if result.command == "deploy" && queueing {
		switch result.subcommand {
		case "queue-status", "queue-run":
			if result.on != "" {
				return options{}, fmt.Errorf("deploy %s does not take --on", strings.ReplaceAll(result.subcommand, "-", " "))
			}
		case "queue-cancel":
			if result.on == "" {
				return options{}, errors.New("deploy queue cancel requires --on <pcNN,...|@all>")
			}
		default:
			return options{}, errors.New("use deploy queue status, deploy queue cancel or deploy queue run")
		}
	}
	if result.queueUnreachable && (result.command != "deploy" || result.subcommand != "apply") {
		return options{}, errors.New("--queue-unreachable is only valid with deploy apply")
	}
	if result.command == "deploy" && !recovering && !queueing && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("deploy requires the plan or apply subcommand")
	}
	if result.command == "controller" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("controller requires the plan or apply subcommand")
	}
	if result.command == "update" && result.subcommand != "check" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("update requires the check, plan, or apply subcommand")
	}
	if result.command == "package-base" && result.subcommand != "status" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("package-base requires status, plan or apply")
	}
	if result.allowUnverified && (result.command != "package-base" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--allow-unverified is only valid with package-base plan/apply")
	}
	if result.command == "package-base" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("package-base apply requires --expect from a reviewed plan")
	}
	if result.target != "" && ((result.command != "update" && result.command != "package-base") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--target is only valid with update plan or update apply")
	}
	if (result.allowPrerelease || result.allowDowngrade) && (result.command != "update" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("update policy flags are only valid with update plan or update apply")
	}
	if result.command == "update" && (result.subcommand == "plan" || result.subcommand == "apply") && result.target == "" {
		return options{}, fmt.Errorf("update %s requires --target", result.subcommand)
	}
	if result.command == "software" && result.subcommand != "catalog" && result.subcommand != "search" && result.subcommand != "presets" && result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "preset-plan" && result.subcommand != "preset-apply" {
		return options{}, errors.New("software requires catalog, search, presets, plan, apply, preset plan, or preset apply")
	}
	if result.command == "software" && result.subcommand == "search" && result.softwareQuery == "" {
		return options{}, errors.New("software search requires --query")
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") && (result.softwarePackage == "" || result.softwareScope == "") {
		return options{}, fmt.Errorf("software %s requires --package and --scope", result.subcommand)
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return options{}, err
		}
	}
	if result.command == "software" && (result.subcommand == "preset-plan" || result.subcommand == "preset-apply") && (result.softwarePreset == "" || result.softwareScope == "") {
		return options{}, fmt.Errorf("software %s requires --preset and --scope", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	if result.command == "software" && (result.subcommand == "preset-plan" || result.subcommand == "preset-apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return options{}, err
		}
	}
	if (result.softwarePackage != "" || result.remove) && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--package and --remove are only valid with software plan or apply")
	}
	if result.softwareScope != "" && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "preset-plan" && result.subcommand != "preset-apply")) {
		return options{}, errors.New("--scope is only valid with software plan/apply or software preset plan/apply")
	}
	if (result.softwarePreset != "" || result.softwareExclude != "") && (result.command != "software" || (result.subcommand != "preset-plan" && result.subcommand != "preset-apply")) {
		return options{}, errors.New("--preset and --exclude are only valid with software preset plan or apply")
	}
	if result.softwareQuery != "" && (result.command != "software" || result.subcommand != "search") {
		return options{}, errors.New("--query is only valid with software search")
	}
	if result.command == "software" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("software apply requires --expect from software plan")
	}
	if result.command == "software" && result.subcommand == "preset-apply" && result.expect == "" {
		return options{}, errors.New("software preset apply requires --expect from software preset plan")
	}
	if (result.command == "shutdown" || result.command == "restart") && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, fmt.Errorf("%s requires the plan or apply subcommand", result.command)
	}
	if (result.command == "shutdown" || result.command == "restart") && result.on == "" {
		return options{}, fmt.Errorf("%s %s requires --on", result.command, result.subcommand)
	}
	if (result.command == "shutdown" || result.command == "restart") && result.subcommand == "apply" && result.expect == "" {
		return options{}, fmt.Errorf("%s apply requires --expect from %s plan", result.command, result.command)
	}
	if result.command == "lock" {
		// --action is parsed once; lock reads it as lock or unlock.
		result.lockAction, result.internetAction = domain.LockAction(result.internetAction), ""
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || !result.lockAction.Valid() {
			return options{}, errors.New("lock requires plan/apply, --on and --action lock/unlock")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("lock apply requires --expect from lock plan")
		}
	}
	if result.command == "send" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || result.file == "" {
			return options{}, errors.New("send requires plan/apply, --file and --on")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("send apply requires --expect from send plan")
		}
	}
	if result.internetAction != "" && result.command != "internet" {
		return options{}, errors.New("--action is only valid with internet or lock")
	}
	if result.command == "internet" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || !result.internetAction.Valid() {
			return options{}, errors.New("internet requires plan/apply, --on and --action block/unblock")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return options{}, errors.New("internet apply requires --expect from internet plan")
		}
	}
	if result.acknowledgeUnknown && result.command != "shutdown" && result.command != "restart" {
		return options{}, errors.New("--acknowledge-unknown-sessions is only valid with shutdown or restart")
	}
	if result.command == "services" && result.subcommand != "" && result.subcommand != "restart" {
		return options{}, errors.New("services accepts only the restart subcommand")
	}
	if result.command == "services" && result.subcommand == "restart" && result.service == "" {
		return options{}, errors.New("services restart requires cache")
	}
	if result.command == "backup" {
		switch {
		case result.subcommand != "create" && result.subcommand != "verify" && result.subcommand != "restore":
			return options{}, errors.New("backup requires create, verify or restore")
		case result.subcommand == "create" && result.backupTarget == "":
			return options{}, errors.New("backup create requires --to <directory>")
		case result.subcommand == "create" && result.backupFile != "":
			return options{}, errors.New("backup create does not take a file")
		case result.subcommand != "create" && result.backupFile == "":
			return options{}, fmt.Errorf("backup %s requires the backup file", result.subcommand)
		case result.subcommand == "verify" && result.backupTarget != "":
			return options{}, errors.New("--to is only valid with backup create or restore")
		case result.subcommand == "restore" && result.backupTarget == "":
			return options{}, errors.New("backup restore requires --to <empty directory>")
		}
	} else if result.backupTarget != "" || result.passphraseFile != "" {
		return options{}, errors.New("--to and --passphrase-file are only valid with backup")
	}
	if result.command == "recovery" && result.subcommand != "status" {
		return options{}, errors.New("recovery accepts only the status subcommand")
	}
	if result.command == "logs" && result.subcommand != "" && result.subcommand != "show" {
		return options{}, errors.New("logs accepts only the show subcommand")
	}
	if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
		return options{}, errors.New("logs show requires an operation log ID")
	}
	gitPaths := result.subcommand == "commit-plan" || result.subcommand == "commit-apply" || result.subcommand == "discard-plan" || result.subcommand == "discard-apply"
	if result.paths != "" && (result.command != "git" || !gitPaths) {
		return options{}, errors.New("--paths is only valid with git commit or git discard plan/apply")
	}
	if result.command == "git" && result.subcommand != "review" && !gitPaths {
		return options{}, errors.New("git requires review, commit plan/apply, or discard plan/apply")
	}
	if result.command == "git" && gitPaths && result.paths == "" {
		return options{}, fmt.Errorf("git %s requires --paths", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	if result.command == "deploy" && !recovering && !queueing && result.on == "" {
		return options{}, fmt.Errorf("deploy %s requires --on", result.subcommand)
	}
	if result.command == "config" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("config apply requires --expect from config plan")
	}
	if result.command == "deploy" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("deploy apply requires --expect from deploy plan")
	}
	if result.command == "controller" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("controller apply requires --expect from controller plan")
	}
	if result.command == "git" && (result.subcommand == "commit-apply" || result.subcommand == "discard-apply") && result.expect == "" {
		return options{}, errors.New("git commit apply requires --expect from git commit plan")
	}
	if result.command == "update" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("update apply requires --expect from update plan")
	}
	if result.command == "setup" && result.subcommand == "" {
		result.subcommand = "configure"
		result.guided = true
	}
	if result.command == "setup" && result.subcommand != "status" && result.subcommand != "keys" && result.subcommand != "configure" && result.subcommand != "install-secrets" && result.subcommand != "apply" {
		return options{}, errors.New("setup requires configure, status, keys, install-secrets, or apply")
	}
	if result.command == "pxe" && result.subcommand != "prepare" && result.subcommand != "start" && result.subcommand != "stop" && result.subcommand != "recover" {
		return options{}, errors.New("pxe requires prepare, start, stop, or recover")
	}
	if result.command == "setup" && result.subcommand == "configure" && result.json {
		return options{}, errors.New("--json is not valid with interactive setup configure")
	}
	if result.command == "install" {
		valid := map[string]bool{
			"usb-prepare": true, "usb-start": true, "usb-status": true, "usb-reconcile": true,
			"usb-reboot": true, "usb-verify": true, "usb-cancel": true, "usb-close": true,
		}
		if !valid[result.subcommand] {
			return options{}, errors.New("install requires usb prepare, start, status, reconcile, reboot, verify, cancel, or close")
		}
		if (result.subcommand == "usb-prepare" || result.subcommand == "usb-start") != (result.host != "") {
			return options{}, errors.New("install usb prepare/start require --host; other USB operations do not accept it")
		}
		if result.host != "" && !domain.ValidRemoteHostName(result.host) {
			return options{}, errors.New("--host must be a canonical pcNN inventory name")
		}
		needsID := result.subcommand != "usb-prepare" && result.subcommand != "usb-start"
		// status without --id shows the unfinished operation, if any.
		if needsID != (result.operationID != "") && !(result.subcommand == "usb-status" && result.operationID == "") {
			return options{}, errors.New("install usb reconcile/reboot/verify/cancel/close require --id; prepare/start do not accept it")
		}
		if result.operationID != "" && !remoteInstallOperationIDPattern.MatchString(result.operationID) {
			return options{}, errors.New("--id must contain exactly 32 lowercase hexadecimal characters")
		}
		if result.json && (result.subcommand == "usb-start" || result.subcommand == "usb-reboot" || result.subcommand == "usb-close") {
			return options{}, errors.New("--json is not valid with interactive install usb start, reboot, or close")
		}
	}
	if result.host != "" && result.command != "install" {
		return options{}, errors.New("--host is only valid with install usb prepare or start")
	}
	if result.operationID != "" && result.command != "install" {
		return options{}, errors.New("--id is only valid with install usb status, reconcile, reboot, verify, cancel, or close")
	}
	return result, nil
}

func resolveRepository(explicit string) (string, error) {
	if explicit != "" {
		return requireDeploymentRoot(explicit)
	}
	if configured := os.Getenv("NIXORIUM_REPO"); configured != "" {
		return requireDeploymentRoot(configured)
	}
	if current, err := os.Getwd(); err == nil && isDeploymentRoot(current) {
		return filepath.Abs(current)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "nixorium-deployment")
		if isDeploymentRoot(candidate) {
			return filepath.Abs(candidate)
		}
	}
	return "", errors.New("deployment repository not found; run from its root or pass --repo <path>")
}

func requireDeploymentRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve deployment repository: %w", err)
	}
	if !isDeploymentRoot(absolute) {
		return "", fmt.Errorf("%s is not a deployment repository (missing flake.nix)", absolute)
	}
	return absolute, nil
}

func isDeploymentRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, "flake.nix"))
	return err == nil && info.Mode().IsRegular()
}

func readCandidateSettings(path string) ([]byte, error) {
	const maximumBytes = int64(1024 * 1024)
	fileDescriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileDescriptor), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("candidate must be a regular file and not a symlink")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximumBytes {
		return nil, errors.New("candidate settings file is unexpectedly large")
	}
	return data, nil
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage: nixorium [status|hosts|doctor|install usb prepare|install usb start|install usb status|install usb reconcile|install usb reboot|install usb verify|install usb cancel|install usb close|software catalog|software search|software presets|software plan|software apply|software preset plan|software preset apply|shutdown plan|shutdown apply|restart plan|restart apply|internet plan|internet apply|lock plan|lock apply|send plan|send apply|classroom-view|cleanup plan|cleanup apply|recovery status|backup create|backup verify|backup restore|deploy recover plan|deploy recover apply|template-reset recover plan|template-reset recover apply|deploy plan|deploy apply|controller plan|controller apply|services|services restart cache|logs|logs show|git review|git commit plan|git commit apply|git discard plan|git discard apply|update check|update plan|update apply|package-base status|package-base plan|package-base apply|workspace plan|workspace apply|workspace marketplace|host-key plan|host-key apply|support preview|support export|config validate|config plan|config apply|bootstrap configure|setup|setup configure|setup status|setup keys|setup install-secrets|setup apply|pxe prepare|pxe start|pxe stop|pxe recover] [options]")
	fmt.Fprintln(writer, "       workspace plan --file <candidate.json> previews student preferences without saving")
	fmt.Fprintln(writer, "       workspace marketplace --extension <publisher.name> [--json] downloads one Marketplace version and prints its pin")
	fmt.Fprintln(writer, "       host-key plan --host <pcNN> [--json] reviews changed SSH trust after reinstall")
	fmt.Fprintln(writer, "       host-key apply --host <pcNN> --expect <review-token> [--yes] rotates only the reviewed key")
	fmt.Fprintln(writer, "       telemetry status|preview [--json], telemetry enable|disable: optional controller adoption statistics")
	fmt.Fprintln(writer, "       support preview [--json] shows the filtered diagnostic payload without saving")
	fmt.Fprintln(writer, "       support export reviews and confirms a private local file; no upload or build")
	fmt.Fprintln(writer, "       workspace apply --file <candidate.json> --expect <review-token> [--yes] saves only the profile JSON")
	fmt.Fprintln(writer, "       install usb prepare --host <pcNN> builds only pinned target-independent artifacts")
	fmt.Fprintln(writer, "       install usb start --host <pcNN> interactively verifies the live ISO, disk, and destructive review")
	fmt.Fprintln(writer, "       install usb status [--id <operation-id>] shows the given or the unfinished operation")
	fmt.Fprintln(writer, "       install usb {reconcile|reboot|verify|cancel|close} --id <operation-id>")
	fmt.Fprintln(writer, "       software catalog")
	fmt.Fprintln(writer, "       software search --query <package-name>")
	fmt.Fprintln(writer, "       software presets")
	fmt.Fprintln(writer, "       software preset plan --preset <id> --scope <scope> [--exclude <id[,id...]>]")
	fmt.Fprintln(writer, "       software preset apply --preset <id> --scope <scope> [--exclude <ids>] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       software plan --package <id> --scope <shared|controller|all-clients|group:NAME|clients:pcNN,...> [--remove]")
	fmt.Fprintln(writer, "       software apply --package <id> --scope <scope> [--remove] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       internet plan --on <clients|@lab> --action <block|unblock>")
	fmt.Fprintln(writer, "       internet apply --on <clients|@lab> --action <block|unblock> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       lock plan --on <clients|@lab> --action <lock|unlock> reviews locking students' screens (classroom view)")
	fmt.Fprintln(writer, "       lock apply --on <clients|@lab> --action <lock|unlock> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       deploy apply --on <clients|@lab> --expect <revision> --queue-unreachable  also queues computers that are off")
	fmt.Fprintln(writer, "       deploy queue status|run  shows or applies queued client updates; deploy queue cancel --on <clients|@all>")
	fmt.Fprintln(writer, "       send plan --file <file-or-folder> --on <clients|@lab> reviews copying it to students' desktops (classroom view)")
	fmt.Fprintln(writer, "       send apply --file <file-or-folder> --on <clients|@lab> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       classroom-view opens the classroom view page (teacher and administrator)")
	fmt.Fprintln(writer, "       recovery status lists what blocks operations and the next step for each")
	fmt.Fprintln(writer, "       backup create --to <directory> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup verify <file> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup restore <file> --to <empty-directory> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       deploy recover plan [--acknowledge-unreachable]")
	fmt.Fprintln(writer, "       deploy recover apply --expect <review-token> [--acknowledge-unreachable] [--yes]")
	fmt.Fprintln(writer, "       template-reset recover plan")
	fmt.Fprintln(writer, "       template-reset recover apply --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       cleanup plan --on <controller,pcNN,...|@lab>")
	fmt.Fprintln(writer, "       cleanup apply --on <targets> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       shutdown plan --on <pcNN[,pcNN...]|@lab> [--acknowledge-unknown-sessions]")
	fmt.Fprintln(writer, "       shutdown apply --on <targets> [--acknowledge-unknown-sessions] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       restart plan --on <pcNN[,pcNN...]|@lab> [--acknowledge-unknown-sessions]")
	fmt.Fprintln(writer, "       restart apply --on <targets> [--acknowledge-unknown-sessions] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       deploy plan --on <pcNN[,pcNN...]|@lab>")
	fmt.Fprintln(writer, "       deploy apply --on <targets> --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       controller plan")
	fmt.Fprintln(writer, "       controller apply --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       services restart cache [--yes]")
	fmt.Fprintln(writer, "       logs show <operation-log-id>")
	fmt.Fprintln(writer, "       git review shows bounded staged and unstaged changes without mutating Git")
	fmt.Fprintln(writer, "       git commit plan --paths <path[,path...]> creates an isolated proposal")
	fmt.Fprintln(writer, "       git commit apply --paths <paths> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       git discard plan --paths <paths> restores files to the last commit after review")
	fmt.Fprintln(writer, "       git discard apply --paths <paths> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       update check explicitly queries the configured public upstream")
	fmt.Fprintln(writer, "       update plan --target <vMAJOR.MINOR.PATCH[-PRERELEASE]>")
	fmt.Fprintln(writer, "       update apply --target <release> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       package-base status shows the effective deployment-owned system/package pin")
	fmt.Fprintln(writer, "       package-base plan [--target <nixos-YY.MM>] [--allow-unverified]")
	fmt.Fprintln(writer, "       package-base apply [--target <nixos-YY.MM>] [--allow-unverified] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       channel changes require --allow-unverified; apply saves files only, never activates machines")
	fmt.Fprintln(writer, "       config plan --file <candidate.json>")
	fmt.Fprintln(writer, "       config apply --file <candidate.json> --expect <sha256:fingerprint>")
	fmt.Fprintln(writer, "       setup keys --verify-only performs read-only correspondence checks")
	fmt.Fprintln(writer, "       nixorium opens the read-only management dashboard")
	fmt.Fprintln(writer, "       doctor --full also builds the controller configuration")
}

func runSetupConfigure(ctx context.Context, repository string, stdout, stderr io.Writer, reconcileKeys bool) int {
	local := adapters.Local{}
	data, err := local.ReadSettings(repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	settings, issues := domain.DecodeLabSettings(data)
	if len(issues) > 0 {
		fmt.Fprintf(stderr, "Error: current settings are invalid: %s: %s\n", issues[0].Field, issues[0].Message)
		return 1
	}
	if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder {
		staticAddress, _ := domain.ControllerStaticAddress(settings.Lab)
		settings = applyDetectedNetworkDefaults(settings, local.DetectNetworkDefaults(staticAddress))
	}

	candidate, accepted, err := presentation.RunSettingsWizard(settings)
	if err != nil {
		fmt.Fprintln(stderr, "Error: configuration wizard:", err)
		return 1
	}
	if !accepted {
		fmt.Fprintln(stdout, "Configuration cancelled; no files changed.")
		return 0
	}

	secretReader := presentation.TerminalSecretReader{Input: os.Stdin, Output: stdout}
	if err := collectSetupCredentials(ctx, secretReader, local, stdout, &candidate); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		fmt.Fprintln(stderr, "Error: prepare candidate:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Validating the complete candidate through Nix...")
	settingsManager := app.NewSettingsManager(local)
	plan := settingsManager.Plan(ctx, repository, candidateData)
	if plan.HasErrors() {
		presentation.ConfigPlanText(stderr, plan)
		return 1
	}
	gitState, err := local.GitState(ctx, repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error: inspect Git worktree:", err)
		return 1
	}
	approved, err := presentation.RunConfigReview(plan, gitState)
	if err != nil {
		fmt.Fprintln(stderr, "Error: configuration review:", err)
		return 1
	}
	if !approved {
		fmt.Fprintln(stdout, "Configuration cancelled; no files changed.")
		return 0
	}
	report := settingsManager.Apply(ctx, repository, candidateData, plan.BaseFingerprint)
	recordOperationWarning(stderr, report)
	presentation.ConfigApplyText(stdout, report)
	if report.HasErrors() {
		return 1
	}
	if reconcileKeys {
		keyReport, keyErr := app.NewSetupManager(local).ReconcileKeys(ctx, repository)
		recordOperationWarning(stderr, keyReport)
		presentation.KeyReconcileText(stdout, keyReport)
		if keyErr != nil {
			fmt.Fprintln(stderr, "Error:", keyErr)
			return 1
		}
		actionReport := app.NewSystemActions(local).InstallSecrets(ctx)
		actionReport.Message = operationRecordMessage(actionReport.Message, actionReport)
		presentation.ActionText(stdout, actionReport)
		if actionReport.HasErrors() {
			fmt.Fprintln(stderr, "The settings and repository keys are intact; retry with `nixorium setup install-secrets` on the controller.")
			return 1
		}
		fmt.Fprintln(stdout, "Configuration and keys are ready. Opening the resumable first-run guide…")
	}
	return 0
}

func applyDetectedNetworkDefaults(settings domain.LabSettingsFile, detected domain.NetworkDefaults) domain.LabSettingsFile {
	if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder && detected.DHCPAddress != "" {
		settings.Lab.MasterDHCPIP = detected.DHCPAddress
	}
	if detected.InterfaceName != "" {
		settings.Lab.ControllerInterfaceName = detected.InterfaceName
	}
	return settings
}

var bootstrapUserNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,30}$`)
var remoteInstallOperationIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func runBootstrapConfigure(ctx context.Context, repository string, stdout, stderr io.Writer) int {
	local := adapters.Local{}
	settingsManager := app.NewSettingsManager(local)
	candidate, err := settingsManager.Current(repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error: load controller settings:", err)
		return 1
	}
	lineReader := bufio.NewReader(os.Stdin)
	secretReader := presentation.TerminalSecretReader{Input: os.Stdin, Output: stdout}
	if err := collectBootstrapConfiguration(ctx, lineReader, secretReader, local, local, stdout, &candidate); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "Controller installation review")
	fmt.Fprintln(stdout, "  Administrator: admin")
	fmt.Fprintf(stdout, "  Teacher:       %s\n", candidate.Lab.TeacherUser)
	fmt.Fprintf(stdout, "  Student:       %s\n", candidate.Lab.StudentUser)
	fmt.Fprintf(stdout, "  Time zone:     %s\n", candidate.Lab.TimeZone)
	fmt.Fprintf(stdout, "  Keyboard:      %s\n", candidate.Lab.KeyboardLayout)
	fmt.Fprintln(stdout, "  Passwords:     set locally and hidden")
	fmt.Fprintln(stdout, "  Client setup:  available later from Nixorium")
	confirmed, err := promptBootstrapConfirmation(lineReader, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "Error: read controller installation confirmation:", err)
		return 1
	}
	if !confirmed {
		fmt.Fprintln(stderr, "Controller installation cancelled; no settings were changed.")
		return 1
	}

	fmt.Fprintln(stdout, "Validating controller settings...")
	plan := settingsManager.PlanSettings(ctx, repository, candidate)
	if plan.HasErrors() {
		presentation.ConfigPlanText(stderr, plan)
		return 1
	}
	reviewManager := app.NewGitReviewManager(local)
	commitManager := app.NewGitCommitManager(local)
	saveManager := app.NewSettingsSaveManager(settingsManager, reviewManager, commitManager)
	report := saveManager.Save(ctx, repository, candidate, plan)
	if report.HasErrors() || (report.State != "saved" && report.State != "unchanged") {
		fmt.Fprintln(stderr, "Error:", report.Message)
		for _, issue := range report.Issues {
			fmt.Fprintf(stderr, "  %s: %s\n", issue.Field, issue.Message)
		}
		return 1
	}
	fmt.Fprintln(stdout, "Controller settings saved. Installation can now start.")
	return 0
}

type bootstrapKeyboardActivator interface {
	ActivateBootstrapKeyboard(context.Context, string) error
}

func collectBootstrapConfiguration(ctx context.Context, reader *bufio.Reader, secrets app.SecretReader, hasher app.PasswordHasher, keyboardActivator bootstrapKeyboardActivator, output io.Writer, candidate *domain.LabSettingsFile) error {
	fmt.Fprintln(output, "Nixorium controller setup")
	fmt.Fprintln(output, "Recommended environment: official NixOS Minimal ISO in UEFI mode.")
	fmt.Fprintln(output, "Choose the keyboard first, then the accounts and regional settings used after the first reboot.")
	fmt.Fprintln(output, "The administrator account name is fixed as 'admin'.")

	keyboard, consoleKeyMap, err := promptBootstrapKeyboard(reader, output, candidate.Lab.KeyboardLayout)
	if err != nil {
		return err
	}
	if err := keyboardActivator.ActivateBootstrapKeyboard(ctx, consoleKeyMap); err != nil {
		return fmt.Errorf("activate the selected keyboard before continuing: %w", err)
	}
	fmt.Fprintf(output, "Active console keyboard: %s (%s).\n", keyboard, consoleKeyMap)
	fmt.Fprintln(output, "All remaining input, including account passwords, uses this layout.")

	teacher, err := promptBootstrapUser(reader, output, "Teacher username", candidate.Lab.TeacherUser, "")
	if err != nil {
		return err
	}
	student, err := promptBootstrapUser(reader, output, "Student username", candidate.Lab.StudentUser, teacher)
	if err != nil {
		return err
	}
	timeZone, err := promptBootstrapValue(reader, output, "Time zone", candidate.Lab.TimeZone)
	if err != nil {
		return err
	}
	candidate.Lab.DeploymentMode = "controller"
	candidate.Lab.PCCount = 0
	candidate.Lab.MasterDHCPIP = domain.MasterDHCPPlaceholder
	candidate.Lab.TeacherUser = teacher
	candidate.Lab.StudentUser = student
	candidate.Lab.StudentGitName = student
	candidate.Lab.TimeZone = timeZone
	candidate.Lab.KeyboardLayout = keyboard
	candidate.Lab.ConsoleKeyMap = consoleKeyMap
	candidate.Lab.DefaultLocale = "en_US.UTF-8"
	candidate.Lab.ExtraLocale = "en_US.UTF-8"
	if err := collectSetupCredentials(ctx, secrets, hasher, output, candidate); err != nil {
		return err
	}
	if issues := candidate.Validate(); len(issues) > 0 {
		return fmt.Errorf("%s: %s", issues[0].Field, issues[0].Message)
	}
	return nil
}

func promptBootstrapUser(reader *bufio.Reader, output io.Writer, label, current, differentFrom string) (string, error) {
	for {
		value, err := promptBootstrapValue(reader, output, label, current)
		if err != nil {
			return "", err
		}
		if !bootstrapUserNamePattern.MatchString(value) {
			fmt.Fprintln(output, "Use a lowercase Unix username (letters, numbers, '_' or '-').")
			continue
		}
		if value == "root" || value == "admin" {
			fmt.Fprintln(output, "That username is reserved by the controller.")
			continue
		}
		if value == differentFrom {
			fmt.Fprintln(output, "Teacher and student usernames must be different.")
			continue
		}
		return value, nil
	}
}

func promptBootstrapValue(reader *bufio.Reader, output io.Writer, label, current string) (string, error) {
	fmt.Fprintf(output, "%s [%s]: ", label, current)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = current
	}
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", strings.ToLower(label))
	}
	return value, nil
}

func promptBootstrapKeyboard(reader *bufio.Reader, output io.Writer, current string) (string, string, error) {
	keyMaps := map[string]string{"us": "us", "it": "it2", "gb": "uk", "fr": "fr", "de": "de", "es": "es"}
	for {
		fmt.Fprintln(output, "Keyboard choices: us, it, gb, fr, de, es")
		value, err := promptBootstrapValue(reader, output, "Keyboard layout", current)
		if err != nil {
			return "", "", err
		}
		if keyMap, ok := keyMaps[value]; ok {
			return value, keyMap, nil
		}
		fmt.Fprintln(output, "Choose one of the listed keyboard layouts.")
	}
}

func promptBootstrapConfirmation(reader *bufio.Reader, output io.Writer) (bool, error) {
	for {
		fmt.Fprint(output, "Continue with these settings? [Y/n]: ")
		value, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(output, "Enter y or n.")
		}
	}
}

func setupStartsBeforeDashboardInspection(report domain.SetupReport) bool {
	switch report.CurrentStage {
	case domain.SetupStageInspectEnvironment, domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials:
		return true
	default:
		return false
	}
}

func collectSetupCredentials(ctx context.Context, reader app.SecretReader, hasher app.PasswordHasher, output io.Writer, candidate *domain.LabSettingsFile) error {
	credentials := []struct {
		label string
		value *string
	}{
		{label: "Administrator password", value: &candidate.Lab.AdminPassword},
		{label: "Teacher password", value: &candidate.Lab.TeacherPassword},
		{label: "Student password", value: &candidate.Lab.StudentPassword},
	}
	pending := 0
	for _, credential := range credentials {
		if *credential.value == domain.DefaultPasswordHash {
			pending++
		}
	}
	if pending == 0 {
		return nil
	}
	fmt.Fprintln(output, "Set account passwords. Each password must contain at least 8 bytes and must not use the public default.")
	fmt.Fprintln(output, "A short or mismatched password can be retried without restarting configuration.")
	completed := 0
	for _, credential := range credentials {
		if *credential.value != domain.DefaultPasswordHash {
			continue
		}
		for {
			hash, err := app.CollectNamedPasswordHash(ctx, reader, hasher, credential.label)
			if err == nil {
				*credential.value = hash
				break
			}
			if !app.IsPasswordInputError(err) {
				return fmt.Errorf("%s: %w", credential.label, err)
			}
			fmt.Fprintf(output, "Invalid %s: %v. Try again.\n", strings.ToLower(credential.label), err)
		}
		completed++
		fmt.Fprintf(output, "%s accepted (%d/%d).\n", credential.label, completed, pending)
	}
	return nil
}

func collectSettingsPassword(ctx context.Context, hasher app.PasswordHasher, input *os.File, output io.Writer, account string, settings domain.LabSettingsFile) (domain.LabSettingsFile, error) {
	reader := presentation.TerminalSecretReader{Input: input, Output: output}
	return collectSettingsPasswordWithReader(ctx, reader, hasher, output, account, settings)
}

func collectSettingsPasswordWithReader(ctx context.Context, reader app.SecretReader, hasher app.PasswordHasher, output io.Writer, account string, settings domain.LabSettingsFile) (domain.LabSettingsFile, error) {
	label := ""
	target := (*string)(nil)
	switch account {
	case "admin":
		label = "Administrator password"
		target = &settings.Lab.AdminPassword
	case "teacher":
		label = "Teacher password"
		target = &settings.Lab.TeacherPassword
	case "student":
		label = "Student password"
		target = &settings.Lab.StudentPassword
	default:
		return settings, fmt.Errorf("unsupported password account %q", account)
	}

	fmt.Fprintln(output, "Set one account password. It must contain at least 8 bytes and must not use the public default.")
	fmt.Fprintln(output, "A short or mismatched password can be retried without leaving this password step.")
	for {
		hash, err := app.CollectNamedPasswordHash(ctx, reader, hasher, label)
		if err == nil {
			*target = hash
			fmt.Fprintln(output, label+" accepted. Returning to settings review…")
			return settings, nil
		}
		if !app.IsPasswordInputError(err) {
			return settings, fmt.Errorf("%s: %w", label, err)
		}
		fmt.Fprintf(output, "Invalid %s: %v. Try again.\n", strings.ToLower(label), err)
	}
}

func operationRecordMessage(message string, outcome any) string {
	if err := app.RecordOperationOutcome(adapters.Local{}, outcome); err != nil {
		warning := "operation outcome was not recorded: " + err.Error()
		if message == "" {
			return warning
		}
		return message + "; " + warning
	}
	return message
}

func recordOperationWarning(writer io.Writer, outcome any) {
	if err := app.RecordOperationOutcome(adapters.Local{}, outcome); err != nil {
		fmt.Fprintln(writer, "Warning: operation outcome was not recorded:", err)
	}
}
