package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
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
	internetAction       domain.InternetAction
	lockAction           domain.LockAction
	command              string
	subcommand           string
	repository           string
	file                 string
	homeFilesSource      string
	homeFilesDestination string
	homeFilesReplace     bool
	expect               string
	on                   string
	service              string
	logID                string
	host                 string
	operationID          string
	paths                string
	target               string
	softwarePackage      string
	softwarePreset       string
	softwareExclude      string
	softwareQuery        string
	extension            string
	softwareScope        string
	json                 bool
	full                 bool
	help                 bool
	guided               bool
	verifyOnly           bool
	yes                  bool
	allowPrerelease      bool
	allowDowngrade       bool
	allowUnverified      bool
	acknowledgeUnknown   bool
	// acknowledgeUnreachable confirms that computers that could not be
	// checked during a recovery review were inspected at their console.
	acknowledgeUnreachable bool
	queueUnreachable       bool
	// network is the new laboratory network of network migrate plan/start.
	network        string
	backupTarget   string
	backupRemote   string
	backupBranch   string
	backupFile     string
	passphraseFile string
	remove         bool
	// collectInto is the folder collect plan/apply saves desktops into
	// instead of ~/Nixorium/Collected.
	collectInto string
	// schedule holds the times and weekdays of the schedule command.
	schedule scheduleArguments
}

func main() {
	sanitizeRuntimeDir(os.Getenv, os.Setenv, os.Unsetenv, pathOwner, os.Getuid())
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
	// Works on a directory, never on the configured deployment (ADR 0034).
	if len(arguments) >= 2 && arguments[0] == "deployment" && arguments[1] == "migrate" {
		return runDeploymentMigrate(arguments[2:], stdout, stderr)
	}
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
	// Switching on goes through the classroom service unless the
	// deployment is readable (the administrator).
	if options.command == "wake" {
		return runWakeCommand(ctx, options, stdout, stderr)
	}
	// The schedule is read and set through the classroom service; the
	// controller's timer runs it as the service user.
	if options.command == "schedule" {
		return runScheduleCommand(ctx, options, stdout, stderr)
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
	// Sending and collecting files need only the classroom service, like
	// the view: the teacher cannot read the deployment repository.
	if options.command == "send" {
		return runSendCommand(ctx, options, stdout, stderr)
	}
	if options.command == "collect" {
		return runCollectCommand(ctx, options, stdout, stderr)
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
	case "network":
		return runNetworkCommand(ctx, repository, options, stdout, stderr)
	case "deploy":
		if strings.HasPrefix(options.subcommand, "recover") {
			return runDeploymentRecoveryCommand(ctx, repository, options, stdout, stderr)
		}
		return runDeploymentCommand(ctx, repository, options, stdout, stderr)
	case "template-reset":
		return runTemplateResetRecoveryCommand(ctx, repository, options, stdout, stderr)
	case "backup":
		if options.subcommand == "plan" || options.subcommand == "publish" || options.subcommand == "clone" {
			return runGitBackupCommand(ctx, repository, options, stdout, stderr)
		}
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
	case "home-files":
		return runHomeFilesCommand(ctx, repository, options, stdout, stderr)
	case "internet":
		return runInternetCommand(ctx, repository, options, stdout, stderr)
	case "lock":
		return runLockCommand(ctx, repository, options, stdout, stderr)
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
		if options.subcommand == "source" {
			return runEvaluationSource(ctx, repository, options, stdout, stderr)
		}
		return runConfigCommand(ctx, repository, options, stdout, stderr)
	case "setup":
		return runSetupCommand(ctx, repository, options, stdout, stderr)
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
	updateManager.RunningVersion = nixoriumVersion
	updateNotifications := app.NewUpdateNotificationManager(local)
	baseSource := adapters.PackageBase{}
	baseManager := app.NewPackageBaseManager(baseSource)
	settingsManager := app.NewSettingsManager(local).WithNetworkObserver(local)
	settingsSaveManager := app.NewSettingsSaveManager(settingsManager, gitReviewManager, gitCommitManager)
	configurationSaveManager := app.NewManagedConfigurationSaveManager(gitReviewManager, gitCommitManager)
	updateSaveManager := app.NewUpdateSaveManager(updateManager, local, gitReviewManager, configurationSaveManager)
	baseSaveManager := app.NewUpdateSaveManager(baseManager, baseSource, gitReviewManager, configurationSaveManager)
	softwareManager := app.NewSoftwareManager(local)
	workspaceManager := app.NewWorkspaceManager(local)
	homeFilesManager := app.NewHomeFilesManager(local)
	workspaceMarketplace := app.NewWorkspaceMarketplace(local)
	templateResetManager := app.NewTemplateResetManager(adapters.TemplateReset{})
	softwareSaveManager := app.NewSoftwareSaveManager(softwareManager, gitReviewManager, configurationSaveManager)
	softwarePresetSaveManager := app.NewSoftwarePresetSaveManager(softwareManager, gitReviewManager, configurationSaveManager)
	shutdownManager := app.NewShutdownManager(local)
	wakeManager := app.NewWakeManager(local)
	cleanupManager := app.NewCleanupManager(local)
	internetManager := app.NewInternetManager(local)
	lockManager := app.NewLockManager(lockSource{Local: local})
	progressManager := app.NewOperationProgressManager(local)
	assistantConfig, _ := adapters.LoadAssistantConfig("/etc/nixorium/assistant.json")
	assistantClient := adapters.AssistantClient{Endpoint: assistantConfig.Endpoint, Version: nixoriumVersion}
	assistantStore := adapters.AssistantStateStore{Directory: "/var/lib/nixorium/assistant"}
	assistantWorkspace := adapters.AssistantWorkspaceStore{Directory: assistantStore.Directory, Version: nixoriumVersion}
	assistantReporter := app.AssistantConversationReporter{Client: assistantClient, Store: assistantWorkspace, Version: nixoriumVersion}
	assistantLabStatus := app.AssistantLabStatus{Inspector: inspector, DeferredUpdates: newDeferredUpdateManager()}
	assistantDiagnostics := app.AssistantDiagnostics{Source: local}
	assistantSystemFiles := app.AssistantSystemFiles{Source: local, Errors: map[error]string{
		adapters.ErrSystemFileRefused: "refused_path", adapters.ErrSystemFileMissing: "not_found",
		adapters.ErrSystemFileUnreadable: "not_readable", adapters.ErrSystemFileOldComputer: "computer_needs_update",
	}}
	assistantRunner := app.AssistantTurnRunner{Client: assistantClient, Tools: adapters.AssistantWorkspaceTools{Store: assistantWorkspace, LabStatus: assistantLabStatus.Execute, Diagnostics: assistantDiagnostics.Execute, SystemFiles: assistantSystemFiles.Execute}, Store: assistantWorkspace, Version: nixoriumVersion, Sharing: &assistantReporter}
	actions := presentation.DashboardActions{
		OpenURL:   desktopOpener(),
		Assistant: assistantConfig,
		AssistantService: &presentation.AssistantActions{
			ReadToken: assistantStore.ReadToken, WriteToken: assistantStore.WriteToken,
			DeleteToken: assistantStore.DeleteToken, ReadConsent: assistantStore.ReadConsent,
			WriteConsent: assistantStore.WriteConsent, WriteConsentSharing: assistantStore.WriteConsentSharing,
			ReadSharing: assistantStore.ReadSharing, WriteSharing: assistantStore.WriteSharing, Report: assistantReporter.Report, Activate: assistantClient.Activate,
			Account:       assistantClient.Account,
			SetLabName:    assistantClient.SetLabName,
			BillingPortal: assistantClient.BillingPortal,
			Offer:         assistantClient.Offer,
			Cleanup:       func(current string) error { return assistantWorkspace.Cleanup(current) },
			ListConversations: func(ctx context.Context, current string) ([]domain.AssistantConversationHistory, error) {
				return assistantWorkspace.ListConversations(ctx, repository, current)
			},
			ConversationHistory: func(ctx context.Context, id string) (domain.AssistantConversationHistory, error) {
				return assistantWorkspace.ConversationHistory(ctx, repository, id)
			},
			PrepareConversation: func(id string) (domain.AssistantConversation, error) {
				return assistantWorkspace.PrepareConversation(repository, id)
			},
			CarryOverProposal: assistantWorkspace.CarryOverProposal,
			Run:               assistantRunner.Run,
			ChangedFiles:      assistantWorkspace.ChangedFiles,
			PlanSave: func(ctx context.Context, id string) domain.AssistantSavePlan {
				return adapters.PlanAssistantSave(ctx, assistantWorkspace, id)
			},
			Save: func(ctx context.Context, plan domain.AssistantSavePlan) domain.TemplateResetResult {
				return adapters.SaveAssistant(ctx, assistantWorkspace, plan)
			},
		},
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
		LoadHomeFiles: func(requestContext context.Context) domain.HomeFilesPlanReport {
			return homeFilesManager.Load(requestContext, repository)
		},
		CaptureHomeFiles: func(requestContext context.Context, source, destination string) domain.HomeFilesCaptureReport {
			return homeFilesManager.Capture(requestContext, repository, source, destination)
		},
		DiscardHomeFilesCapture: func(requestContext context.Context, id string) error {
			return homeFilesManager.DiscardCapture(requestContext, repository, id)
		},
		CompleteHomeFilesPath: local.CompleteHomeFilesPath,
		PlanHomeFiles: func(requestContext context.Context, draft domain.HomeFilesDraft) domain.HomeFilesPlanReport {
			return homeFilesManager.Plan(requestContext, repository, draft)
		},
		SaveHomeFiles: func(plan domain.HomeFilesPlanReport) domain.HomeFilesApplyReport {
			return app.NewHomeFilesSaveManager(local, gitReviewManager).Save(ctx, repository, *plan.Candidate, plan.ReviewToken)
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
		PlanWake: func(ctx context.Context, requested string) domain.WakePlan {
			return wakeManager.Plan(ctx, repository, requested)
		},
		ApplyWake: func(plan domain.WakePlan) domain.WakeReport {
			return wakeManager.Apply(ctx, plan)
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
		PlanGitUndo: func(ctx context.Context) domain.GitUndoPlanReport {
			return app.NewGitUndoManager(local).Plan(ctx, repository)
		},
		ApplyGitUndo: func(plan domain.GitUndoPlanReport) domain.GitUndoReport {
			report := app.NewGitUndoManager(local).Apply(ctx, repository, plan.ReviewToken)
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
		LoadControllerAddresses: func(ctx context.Context, settings domain.LabSettingsFile) domain.ControllerAddresses {
			return settingsManager.ControllerAddresses(ctx, settings)
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
		LoadNetworkMigration: func(ctx context.Context) domain.NetworkMigrationStatus {
			return newNetworkMigrationManager().Status(ctx, repository)
		},
		PlanNetworkMigration: func(ctx context.Context, action, network string, acknowledgeOff bool) domain.NetworkMigrationPlan {
			request := app.NetworkMigrationRequest{Action: action, AcknowledgeOff: acknowledgeOff}
			if action == domain.NetworkMigrationStart {
				request.Base, request.Prefix, _ = domain.ParseLabNetwork(network)
			}
			return newNetworkMigrationManager().Plan(ctx, repository, request)
		},
		ApplyNetworkMigration: func(plan domain.NetworkMigrationPlan) domain.NetworkMigrationResult {
			return newNetworkMigrationManager().Apply(ctx, plan, plan.ReviewToken)
		},
		CancelDeferredUpdates: func(hosts []string) error {
			_, err := newDeferredUpdateManager().Cancel(hosts)
			return err
		},
		PlanDeploymentRecovery: func(ctx context.Context, acknowledge bool) domain.DeploymentRecoveryPlan {
			return app.NewDeploymentRecoveryManager(local).ForProcess(os.Getpid()).Plan(ctx, repository, acknowledge)
		},
		ApplyDeploymentRecovery: func(plan domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult {
			result := app.NewDeploymentRecoveryManager(local).ForProcess(os.Getpid()).Apply(ctx, plan, plan.ReviewToken)
			result.Message = operationRecordMessage(result.Message, result)
			return result
		},
		PlanResetRecovery: func(ctx context.Context) domain.TemplateResetRecoveryPlan {
			return adapters.TemplateReset{}.PlanTemplateResetRecovery(ctx, repository)
		},
		CreateBackup: func(destination string, passphrase []byte) domain.BackupReport {
			return local.CreateBackup(ctx, repository, destination, passphrase, nixoriumVersion)
		},
		PlanGitBackup: func(readCtx context.Context, remote, branch string) (domain.GitBackupPlan, error) {
			return (adapters.GitBackup{}).Plan(readCtx, repository, remote, branch)
		},
		PublishGitBackup: func(plan domain.GitBackupPlan, passphrase []byte) domain.BackupReport {
			return (adapters.GitBackup{}).Publish(ctx, plan, passphrase)
		},
		RestoreLab: func(remote, branch, target string, passphrase []byte) domain.BackupReport {
			return (adapters.GitBackup{}).Restore(ctx, remote, branch, target, passphrase)
		},
		GitBackupDestination: func() (string, string) { return (adapters.GitBackup{}).Destination(repository) },
		BackupFileFolder: func() string {
			// A connected USB drive keeps the backup away from this controller.
			if current, err := user.Current(); err == nil {
				if drives := adapters.MountedDrives(current.Username); len(drives) > 0 {
					return drives[0]
				}
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			return home
		},
		SyncGitBackup: func(syncCtx context.Context, skip string) domain.BackupReport {
			return (adapters.GitBackup{}).Sync(syncCtx, repository, skip)
		},
		RestoreLabArchive: func(source, target string, passphrase []byte) domain.BackupReport {
			return local.RestoreLab(ctx, source, passphrase, target)
		},
		RestoreFolder: func() string { return adapters.RestoreFolder(repository) },
		FindBackupFiles: func() []string {
			home, _ := os.UserHomeDir()
			name := ""
			if current, err := user.Current(); err == nil {
				name = current.Username
			}
			return adapters.FindBackupFiles(home, name)
		},
		UseRestoredLab: func(restored domain.BackupReport, backupFile string) domain.BackupReport {
			// The controller installs the keys of its configured deployment;
			// using a restored lab elsewhere would install other keys.
			if fixed, err := os.ReadFile(remoteInstallDeploymentPathFile); err == nil && strings.TrimSpace(string(fixed)) != repository {
				message := "Nixorium is open on " + repository + ", but this controller manages " + strings.TrimSpace(string(fixed)) + ". Open Nixorium without --repo, then use the restored lab."
				return domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-use-restored", State: "blocked", Message: message, PrivateKeys: []string{}, Issues: []domain.ValidationIssue{{Field: "repository", Message: message}}}
			}
			adminKey := ""
			if home, err := os.UserHomeDir(); err == nil {
				adminKey = filepath.Join(home, ".ssh", "id_ed25519")
			}
			return app.RestoredLab{
				Source:     local,
				VerifyKeys: setupManager.VerifyKeys,
				InstallSecrets: func(installCtx context.Context) domain.ActionReport {
					return app.NewSystemActions(local).InstallSecrets(installCtx)
				},
			}.Use(ctx, repository, adminKey, backupFile, restored)
		},
		LabComputerCount: func() (int, bool) {
			raw, err := local.ReadSettings(repository)
			if err != nil {
				return 0, false
			}
			settings, issues := domain.DecodeLabSettings(raw)
			if len(issues) > 0 {
				return 0, false
			}
			return settings.Lab.PCCount, true
		},
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
	actions.InstalledInterface = func() (presentation.InterfaceReload, bool) {
		return detectInterfaceReload(os.Executable, installedInterfacePath)
	}
	notice := os.Getenv(presentation.ReopenNoticeEnvironment)
	_ = os.Unsetenv(presentation.ReopenNoticeEnvironment)
	reload, outcome, tuiErr := presentation.RunLoadingDashboardReload(actions, setupMode, notice)
	if tuiErr != nil {
		fmt.Fprintln(stderr, "Error:", tuiErr)
		return 1
	}
	if reload {
		// The terminal is restored: bubbletea released the alternate screen
		// and raw mode when Run returned.
		environment := append(os.Environ(), presentation.ReopenNoticeEnvironment+"="+outcome)
		return reopenInterface(installedInterfacePath, os.Args[1:], environment, stderr)
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
	return options.command != "logs" && (options.command != "backup" || (options.subcommand == "create" || options.subcommand == "plan" || options.subcommand == "publish")) && (options.command != "pxe" || (options.subcommand != "stop" && options.subcommand != "recover"))
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
	fmt.Fprintln(writer, "Usage: nixorium [status|hosts|doctor|install usb prepare|install usb start|install usb status|install usb reconcile|install usb reboot|install usb verify|install usb cancel|install usb close|software catalog|software search|software presets|software plan|software apply|software preset plan|software preset apply|shutdown plan|shutdown apply|restart plan|restart apply|internet plan|internet apply|lock plan|lock apply|send plan|send apply|wake|schedule show|schedule set|schedule clear|schedule run|classroom-view|cleanup plan|cleanup apply|recovery status|backup create|backup verify|backup restore|deploy recover plan|deploy recover apply|template-reset recover plan|template-reset recover apply|deployment migrate|deploy plan|deploy apply|controller plan|controller apply|services|services restart cache|logs|logs show|git review|git commit plan|git commit apply|git discard plan|git discard apply|git undo plan|git undo apply|update check|update plan|update apply|package-base status|package-base plan|package-base apply|workspace plan|workspace apply|workspace marketplace|home-files import plan|home-files plan|home-files apply|host-key plan|host-key apply|network migrate plan|network migrate start|network migrate status|network migrate finish|network migrate undo|support preview|support export|config validate|config plan|config apply|setup|setup configure|setup status|setup keys|setup install-secrets|setup apply|pxe prepare|pxe start|pxe stop|pxe recover] [options]")
	fmt.Fprintln(writer, "       workspace plan --file <candidate.json> previews student preferences without saving")
	fmt.Fprintln(writer, "       home-files import plan --source <local-file-or-folder> --destination <relative-home-path> [--replace] [--json] captures materials for review")
	fmt.Fprintln(writer, "       home-files plan --file <draft-or-profile.json> [--json] reviews shared student initial files")
	fmt.Fprintln(writer, "       home-files apply --file <draft-or-profile.json> --expect <review-token> [--yes] saves only the manifest and captured copies")
	fmt.Fprintln(writer, "       workspace marketplace --extension <publisher.name> [--json] downloads one Marketplace version and prints its pin")
	fmt.Fprintln(writer, "       host-key plan --host <pcNN> [--json] reviews changed SSH trust after reinstall")
	fmt.Fprintln(writer, "       host-key apply --host <pcNN> --expect <review-token> [--yes] rotates only the reviewed key")
	fmt.Fprintln(writer, "       network migrate plan --to <network/prefix> [--acknowledge-unreachable] [--json] reviews a change of laboratory network")
	fmt.Fprintln(writer, "       network migrate start --to <network/prefix> --expect <review-token> [--acknowledge-unreachable] [--yes] saves it; then apply to the controller and update computers")
	fmt.Fprintln(writer, "       network migrate status [--json] shows which computers moved and the next step")
	fmt.Fprintln(writer, "       network migrate finish|undo [--expect <review-token> [--yes]] [--json] reviews, then saves the end or the undo of the change")
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
	fmt.Fprintln(writer, "       collect plan --on <clients|@lab> [--into <folder>] reviews copying students' desktops into ~/Nixorium/Collected or the folder (classroom view)")
	fmt.Fprintln(writer, "       collect apply --on <clients|@lab> [--into <folder>] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       classroom-view opens the classroom view page (teacher and administrator)")
	fmt.Fprintln(writer, "       wake --on <clients|@lab> [--json] sends Wake-on-LAN packets to the selected computers that are off (teacher and administrator)")
	fmt.Fprintln(writer, "       schedule show|clear [--json] shows or clears the daily switch-on and shut-down of the computers (teacher and administrator)")
	fmt.Fprintln(writer, "       schedule set [--on <HH:MM>] [--off <HH:MM>] [--days <mon-fri|all|mon,wed,fri>] [--json] sets it, replacing the old one; --days is mon-fri when left out")
	fmt.Fprintln(writer, "       schedule run [--repo <deployment>] [--json] does what is due this minute (started by nixorium-classroom-schedule.timer)")
	fmt.Fprintln(writer, "       recovery status lists what blocks operations and the next step for each")
	fmt.Fprintln(writer, "       backup plan --remote <ssh-url> [--branch <branch>] [--repo <deployment>]")
	fmt.Fprintln(writer, "       backup publish --remote <ssh-url> [--branch <branch>] --expect <token> --yes [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup clone [--remote <ssh-url> [--branch <branch>] --to <new-directory> --yes] [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup create --to <directory> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup verify <file> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       backup restore <file> --to <empty-directory> [--passphrase-file <file>]")
	fmt.Fprintln(writer, "       deploy recover plan [--acknowledge-unreachable]  reviews an interrupted client update; the flag continues without the computers that do not answer")
	fmt.Fprintln(writer, "       deploy recover apply --expect <review-token> [--acknowledge-unreachable] [--yes]  marks it as reviewed after a yes/no question")
	fmt.Fprintln(writer, "       deployment migrate --root <deployment> [--contract N] [--json]  applies this release's changes to deployment files in that directory; review them with git diff")
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
	fmt.Fprintln(writer, "       git undo plan [--json] reviews undoing the last saved change")
	fmt.Fprintln(writer, "       git undo apply --expect <review-token> [--yes] [--json] saves the undo; then apply to the controller and update computers")
	fmt.Fprintln(writer, "       update check explicitly queries the configured public upstream")
	fmt.Fprintln(writer, "       update plan --target <vMAJOR.MINOR.PATCH[-PRERELEASE]>")
	fmt.Fprintln(writer, "       update apply --target <release> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       package-base status shows the effective deployment-owned system/package pin")
	fmt.Fprintln(writer, "       package-base plan [--target <nixos-YY.MM>] [--allow-unverified]")
	fmt.Fprintln(writer, "       package-base apply [--target <nixos-YY.MM>] [--allow-unverified] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       channel changes require --allow-unverified; apply saves files only, never activates machines")
	fmt.Fprintln(writer, "       config source [--revision <git-revision>] prepares local Nix input with account hashes")
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

var remoteInstallOperationIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

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
		if *credential.value == "" || *credential.value == domain.DefaultPasswordHash {
			pending++
		}
	}
	if pending == 0 {
		return nil
	}
	fmt.Fprintln(output, "Set account passwords. Each password must contain at least 8 bytes and must not use the public default.")
	fmt.Fprintln(output, "A short or mismatched password can be retried without restarting configuration.")
	candidate.Lab.CredentialsVersion++
	completed := 0
	for _, credential := range credentials {
		if *credential.value != "" && *credential.value != domain.DefaultPasswordHash {
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
			settings.Lab.CredentialsVersion++
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
