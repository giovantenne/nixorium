package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

type options struct {
	command            string
	subcommand         string
	repository         string
	file               string
	expect             string
	on                 string
	service            string
	logID              string
	paths              string
	target             string
	softwarePackage    string
	softwareScope      string
	json               bool
	full               bool
	help               bool
	guided             bool
	verifyOnly         bool
	yes                bool
	allowPrerelease    bool
	allowDowngrade     bool
	acknowledgeUnknown bool
	remove             bool
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
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
	repository := options.repository
	resolvedRepository, resolveErr := resolveRepository(options.repository)
	if resolveErr == nil {
		repository = resolvedRepository
	} else if commandRequiresRepository(options) {
		fmt.Fprintln(stderr, "Error:", resolveErr)
		return 1
	}

	local := adapters.Local{}
	inspector := app.NewInspector(local)
	switch options.command {
	case "":
		report, inspectErr := inspector.Status(ctx, repository)
		if inspectErr != nil {
			fmt.Fprintln(stderr, "Error:", inspectErr)
			return 1
		}
		if options.json {
			if jsonErr := presentation.JSON(stdout, report); jsonErr != nil {
				fmt.Fprintln(stderr, "Error:", jsonErr)
				return 1
			}
			return 0
		}
		return runDashboardProgram(ctx, repository, report, false, stderr)
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
	case "deploy":
		if options.subcommand == "apply" {
			return runDeploymentApply(ctx, repository, stdout, stderr, options.on, options.expect, options.yes, options.json)
		} else {
			report := app.NewDeploymentManager(local).Plan(ctx, repository, options.on)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.DeploymentPlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		}
	case "controller":
		manager := app.NewControllerManager(local)
		if options.subcommand == "apply" {
			return runControllerApply(ctx, manager, repository, stdout, stderr, options.expect, options.yes, options.json)
		}
		report := manager.Plan(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.ControllerRebuildPlanText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "services":
		manager := app.NewServiceManager(local)
		if options.subcommand == "restart" {
			return runServiceRestart(ctx, manager, repository, stdout, stderr, options.service, options.yes, options.json)
		}
		report := manager.Status(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.ServicesText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "logs":
		manager := app.NewOperationLogManager(local)
		if options.subcommand == "show" {
			report := manager.Show(options.logID)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.OperationLogText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else {
			report := manager.List()
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.OperationLogsText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		}
	case "git":
		if options.subcommand == "review" {
			report := app.NewGitReviewManager(local).Review(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.GitReviewText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else if options.subcommand == "commit-plan" {
			report := app.NewGitCommitManager(local).Plan(ctx, repository, options.paths)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.GitCommitPlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else {
			return runGitCommitApply(ctx, app.NewGitCommitManager(local), repository, stdout, stderr, options.paths, options.expect, options.yes, options.json)
		}
	case "update":
		manager := app.NewUpdateManager(local)
		if options.subcommand == "check" {
			report := manager.Check(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.UpdateCheckText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else if options.subcommand == "plan" {
			report := manager.Plan(ctx, repository, options.target, options.allowPrerelease, options.allowDowngrade)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.UpdatePlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else {
			return runUpdateApply(ctx, manager, repository, stdout, stderr, options.target, options.expect, options.allowPrerelease, options.allowDowngrade, options.yes, options.json)
		}
	case "software":
		manager := app.NewSoftwareManager(local)
		if options.subcommand == "catalog" {
			report := manager.Catalog(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.SoftwareCatalogText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else {
			scope, scopeErr := parseSoftwareScope(options.softwareScope)
			if scopeErr != nil {
				fmt.Fprintln(stderr, "Error:", scopeErr)
				return 2
			}
			request := domain.SoftwareChangeRequest{Package: options.softwarePackage, Present: !options.remove, Scope: scope}
			if options.subcommand == "plan" {
				report := manager.Plan(ctx, repository, request)
				if options.json {
					err = presentation.JSON(stdout, report)
				} else {
					presentation.SoftwareChangePlanText(stdout, report)
				}
				if report.HasErrors() {
					return 1
				}
			} else {
				plan := manager.Plan(ctx, repository, request)
				if plan.HasErrors() {
					if options.json {
						err = presentation.JSON(stdout, plan)
					} else {
						presentation.SoftwareChangePlanText(stderr, plan)
					}
					return 1
				}
				if !options.yes {
					if !presentation.IsInteractive(os.Stdin) {
						fmt.Fprintln(stderr, "Error: software apply requires an interactive terminal or explicit --yes")
						return 2
					}
					confirmationOutput := stdout
					if options.json {
						confirmationOutput = stderr
					}
					approved, confirmErr := presentation.ConfirmSoftwareChange(os.Stdin, confirmationOutput, plan)
					if confirmErr != nil {
						fmt.Fprintln(stderr, "Error: read confirmation:", confirmErr)
						return 1
					}
					if !approved {
						fmt.Fprintln(confirmationOutput, "Software change cancelled; lab-software.json was not changed.")
						return 0
					}
				}
				report := manager.ApplyPlan(ctx, plan, options.expect)
				if options.json {
					err = presentation.JSON(stdout, report)
				} else {
					presentation.SoftwareChangeApplyText(stdout, report)
				}
				if report.HasErrors() {
					return 1
				}
			}
		}
	case "shutdown":
		manager := app.NewShutdownManager(local)
		policy := domain.ShutdownRequireIdle
		if options.acknowledgeUnknown {
			policy = domain.ShutdownAcknowledgeUnknown
		}
		if options.subcommand == "plan" {
			report := manager.Plan(ctx, repository, options.on, policy)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.ShutdownPlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else {
			return runShutdownApply(ctx, manager, repository, stdout, stderr, options.on, policy, options.expect, options.yes, options.json)
		}
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
		manager := app.NewSettingsManager(adapters.Local{})
		switch options.subcommand {
		case "validate":
			report := manager.Validate(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.ConfigValidationText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		case "plan", "apply":
			candidate, readErr := readCandidateSettings(options.file)
			if readErr != nil {
				fmt.Fprintln(stderr, "Error: read candidate settings:", readErr)
				return 1
			}
			if options.subcommand == "plan" {
				report := manager.Plan(ctx, repository, candidate)
				if options.json {
					err = presentation.JSON(stdout, report)
				} else {
					presentation.ConfigPlanText(stdout, report)
				}
				if report.HasErrors() {
					return 1
				}
			} else {
				report := manager.Apply(ctx, repository, candidate, options.expect)
				recordOperationWarning(stderr, report)
				if options.json {
					err = presentation.JSON(stdout, report)
				} else {
					presentation.ConfigApplyText(stdout, report)
				}
				if report.HasErrors() {
					return 1
				}
			}
		}
	case "setup":
		manager := app.NewSetupManager(adapters.Local{})
		if options.subcommand == "configure" {
			if !options.guided {
				return runSetupConfigure(ctx, repository, stdout, stderr, false)
			}
			setup := manager.Status(ctx, repository)
			if setupNeedsConfiguration(setup) {
				if code := runSetupConfigure(ctx, repository, stdout, stderr, true); code != 0 {
					return code
				}
				setup = manager.Status(ctx, repository)
				if setupNeedsConfiguration(setup) {
					presentation.SetupText(stdout, setup)
					fmt.Fprintln(stdout, "Setup paused before configuration was complete. Run `nixorium setup` to resume.")
					return 0
				}
			}
			report, inspectErr := inspector.Status(ctx, repository)
			if inspectErr != nil {
				fmt.Fprintln(stderr, "Error:", inspectErr)
				return 1
			}
			return runDashboardProgram(ctx, repository, report, true, stderr)
		} else if options.subcommand == "apply" {
			return runSetupApply(ctx, repository, stdout, stderr, options.yes, options.json)
		} else if options.subcommand == "install-secrets" {
			report := app.NewSystemActions(adapters.Local{}).InstallSecrets(ctx)
			report.Message = operationRecordMessage(report.Message, report)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.ActionText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		} else if options.subcommand == "keys" {
			report := domain.KeyReconcileReport{}
			var reconcileErr error
			if options.verifyOnly {
				report = manager.VerifyKeys(ctx, repository)
			} else {
				report, reconcileErr = manager.ReconcileKeys(ctx, repository)
				recordOperationWarning(stderr, report)
			}
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.KeyReconcileText(stdout, report)
			}
			if err == nil && reconcileErr != nil {
				err = reconcileErr
			}
			if report.State != "ready" && err == nil {
				return 1
			}
		} else {
			report := manager.Status(ctx, repository)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.SetupText(stdout, report)
			}
		}
	case "pxe":
		switch options.subcommand {
		case "prepare":
			writePXEPreparationActivity(stderr)
			local := adapters.Local{}
			report := runWithManagedProgress(
				func() domain.ActionReport { return app.NewSystemActions(local).PreparePXE(ctx) },
				func() (domain.OperationProgress, error) {
					return app.NewOperationProgressManager(local).Current("pxe-prepare")
				},
				stderr,
			)
			report.Message = operationRecordMessage(report.Message, report)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.ActionText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		case "start":
			return runPXEStart(ctx, repository, stdout, stderr, options.yes, options.json)
		case "stop", "recover":
			manager := app.NewPXELifecycle(adapters.Local{})
			report := domain.PXELifecycleReport{}
			if options.subcommand == "stop" {
				report = manager.Stop(ctx, repository)
			} else {
				report = manager.Recover(ctx, repository)
			}
			report.Message = operationRecordMessage(report.Message, report)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.PXELifecycleText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
		}
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

func setupNeedsConfiguration(report domain.SetupReport) bool {
	switch report.CurrentStage {
	case "", domain.SetupStageReview, domain.SetupStageApply, domain.SetupStageArtifacts, domain.SetupStageReadiness, domain.SetupStageInstall:
		return false
	default:
		return true
	}
}

func runDashboardProgram(ctx context.Context, repository string, report domain.StatusReport, setupMode bool, stderr io.Writer) int {
	local := adapters.Local{}
	inspector := app.NewInspector(local)
	installationManager := app.NewInstallationSessionManager(local, inspector)
	setupManager := app.NewSetupManager(local)
	lifecycle := app.NewPXELifecycle(local)
	deploymentManager := app.NewDeploymentManager(local)
	controllerManager := app.NewControllerManager(local)
	serviceManager := app.NewServiceManager(local)
	operationLogManager := app.NewOperationLogManager(local)
	gitReviewManager := app.NewGitReviewManager(local)
	gitCommitManager := app.NewGitCommitManager(local)
	updateManager := app.NewUpdateManager(local)
	settingsManager := app.NewSettingsManager(local)
	softwareManager := app.NewSoftwareManager(local)
	shutdownManager := app.NewShutdownManager(local)
	progressManager := app.NewOperationProgressManager(local)
	setup := setupManager.Status(ctx, repository)
	actions := presentation.DashboardActions{
		LoadDoctor: func() (domain.DoctorReport, error) {
			return inspector.Doctor(ctx, repository, app.DoctorOptions{})
		},
		Refresh: func() (domain.StatusReport, error) {
			return inspector.Status(ctx, repository)
		},
		LoadSetup: func() domain.SetupReport {
			return setupManager.Status(ctx, repository)
		},
		LoadHosts: func() (domain.HostsReport, error) {
			return inspector.Hosts(ctx, repository)
		},
		LoadInstallationSession: func() domain.InstallationSessionReport {
			return installationManager.Status(ctx, repository)
		},
		SelectInstallationTarget: func(name string) domain.InstallationSessionReport {
			return installationManager.Select(ctx, repository, name)
		},
		VerifyInstallationTarget: func(name string) domain.InstallationSessionReport {
			return installationManager.Verify(ctx, repository, name)
		},
		ConfirmInstallationTarget: func(name string) domain.InstallationSessionReport {
			return installationManager.ConfirmPractical(ctx, repository, name)
		},
		LoadSoftware: func() domain.SoftwareCatalogReport { return softwareManager.Catalog(ctx, repository) },
		PlanSoftware: func(request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
			return softwareManager.Plan(ctx, repository, request)
		},
		ApplySoftware: func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
			return softwareManager.ApplyPlan(ctx, plan, plan.ReviewToken)
		},
		PlanShutdown: func(requested string, policy domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
			return shutdownManager.Plan(ctx, repository, requested, policy)
		},
		ApplyShutdown: func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
			report := shutdownManager.ApplyPlan(ctx, plan, plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		PlanDeployment: func(requested string) domain.DeploymentPlanReport {
			return deploymentManager.Plan(ctx, repository, requested)
		},
		ApplyDeployment: func(plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			return executeDeploymentOperationWithProgress(ctx, deploymentManager, repository, plan.Requested, plan.Revision, io.Discard, observe)
		},
		PlanController: func() domain.ControllerRebuildPlanReport {
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
		LoadServices: func() domain.ServicesReport {
			return serviceManager.Status(ctx, repository)
		},
		RestartService: func(service string) domain.ServiceActionReport {
			report := serviceManager.Restart(ctx, repository, service)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadLogs: func() domain.OperationLogsReport {
			return operationLogManager.List()
		},
		LoadLog: func(id string) domain.OperationLogReport {
			return operationLogManager.Show(id)
		},
		LoadGitReview: func() domain.GitReviewReport {
			return gitReviewManager.Review(ctx, repository)
		},
		PlanGitCommit: func(paths string) domain.GitCommitPlanReport {
			return gitCommitManager.Plan(ctx, repository, paths)
		},
		ApplyGitCommit: func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
			report := gitCommitManager.Apply(ctx, repository, strings.Join(plan.Paths, ","), plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		CheckUpdate: func() domain.UpdateCheckReport {
			return updateManager.Check(ctx, repository)
		},
		PlanUpdate: func(target string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			return updateManager.Plan(ctx, repository, target, allowPrerelease, allowDowngrade)
		},
		ApplyUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			report := updateManager.ApplyPlan(ctx, plan, plan.ReviewToken)
			report.Message = operationRecordMessage(report.Message, report)
			return report
		},
		LoadSettings: func() (domain.LabSettingsFile, error) {
			return settingsManager.Current(repository)
		},
		PlanSettings: func(candidate domain.LabSettingsFile) domain.ConfigPlanReport {
			return settingsManager.PlanSettings(ctx, repository, candidate)
		},
		ApplySettings: func(candidate domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigApplyReport {
			return settingsManager.ApplySettings(ctx, repository, candidate, plan.BaseFingerprint)
		},
		ChangePassword: func(account string, settings domain.LabSettingsFile, input *os.File, output io.Writer) (domain.LabSettingsFile, error) {
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
		PlanPXEStart: func() domain.PXELifecycleReport {
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
	}
	var tuiErr error
	if setupMode {
		tuiErr = presentation.RunSetupDashboard(report, setup, actions)
	} else {
		tuiErr = presentation.RunDashboard(report, setup, actions)
	}
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
	return options.command != "logs" && (options.command != "pxe" || (options.subcommand != "stop" && options.subcommand != "recover"))
}

func parseArguments(arguments []string) (options, error) {
	result := options{}
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
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
		case "--on":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--on requires a client or @lab")
			}
			result.on = arguments[index]
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
		case "--scope":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return options{}, errors.New("--scope requires all-clients, group:NAME, or clients:pcNN,...")
			}
			result.softwareScope = arguments[index]
		case "--remove":
			result.remove = true
		case "--allow-prerelease":
			result.allowPrerelease = true
		case "--allow-downgrade":
			result.allowDowngrade = true
		case "--acknowledge-unknown-sessions":
			result.acknowledgeUnknown = true
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
			if result.command == "setup" && result.subcommand == "" {
				result.subcommand = "status"
				continue
			}
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "doctor", "hosts", "deploy", "controller", "services", "logs", "git", "config", "setup", "pxe", "update", "software", "shutdown":
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
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
		case "plan":
			if result.command == "git" && result.subcommand == "commit" {
				result.subcommand = "commit-plan"
				continue
			}
			if (result.command != "config" && result.command != "deploy" && result.command != "controller" && result.command != "update" && result.command != "software" && result.command != "shutdown") || result.subcommand != "" {
				return options{}, errors.New("plan must follow config, deploy, controller, update, software, shutdown, or git commit")
			}
			result.subcommand = "plan"
		case "keys":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("keys must follow setup")
			}
			result.subcommand = "keys"
		case "configure":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("configure must follow setup")
			}
			result.subcommand = "configure"
		case "install-secrets":
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("install-secrets must follow setup")
			}
			result.subcommand = "install-secrets"
		case "apply":
			if result.command == "git" && result.subcommand == "commit" {
				result.subcommand = "commit-apply"
				continue
			}
			if (result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "software" || result.command == "shutdown") && result.subcommand == "" {
				result.subcommand = "apply"
				continue
			}
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("apply must follow config, deploy, controller, update, software, shutdown, or setup")
			}
			result.subcommand = "apply"
		case "restart":
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
		case "commit":
			if result.command != "git" || result.subcommand != "" {
				return options{}, errors.New("commit must follow git")
			}
			result.subcommand = "commit"
		case "cache":
			if result.command != "services" || result.subcommand != "restart" || result.service != "" {
				return options{}, fmt.Errorf("unexpected argument %q", arguments[index])
			}
			result.service = "cache"
		case "prepare":
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, errors.New("prepare must follow pxe")
			}
			result.subcommand = "prepare"
		case "start", "stop", "recover":
			if result.command != "pxe" || result.subcommand != "" {
				return options{}, fmt.Errorf("%s must follow pxe", arguments[index])
			}
			result.subcommand = arguments[index]
		default:
			if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
				result.logID = arguments[index]
				continue
			}
			return options{}, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}
	if result.full && result.command != "doctor" {
		return options{}, errors.New("--full is only valid with doctor")
	}
	if result.verifyOnly && (result.command != "setup" || result.subcommand != "keys") {
		return options{}, errors.New("--verify-only is only valid with setup keys")
	}
	if result.yes && !((result.command == "setup" && result.subcommand == "apply") || (result.command == "pxe" && result.subcommand == "start") || ((result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "software" || result.command == "shutdown") && result.subcommand == "apply") || (result.command == "services" && result.subcommand == "restart") || (result.command == "git" && result.subcommand == "commit-apply")) {
		return options{}, errors.New("--yes is only valid with setup apply, pxe start, deploy apply, controller apply, update apply, software apply, shutdown apply, services restart, or git commit apply")
	}
	if result.command == "config" && result.subcommand != "validate" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("config requires the validate, plan, or apply subcommand")
	}
	if result.file != "" && (result.command != "config" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--file is only valid with config plan or config apply")
	}
	if result.command == "config" && (result.subcommand == "plan" || result.subcommand == "apply") && result.file == "" {
		return options{}, fmt.Errorf("config %s requires --file", result.subcommand)
	}
	if result.expect != "" && !(((result.command == "config" || result.command == "deploy" || result.command == "controller" || result.command == "update" || result.command == "software" || result.command == "shutdown") && result.subcommand == "apply") || (result.command == "git" && result.subcommand == "commit-apply")) {
		return options{}, errors.New("--expect is only valid with config apply, deploy apply, controller apply, update apply, software apply, shutdown apply, or git commit apply")
	}
	if result.on != "" && ((result.command != "deploy" && result.command != "shutdown") || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--on is only valid with deploy or shutdown plan/apply")
	}
	if result.command == "deploy" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("deploy requires the plan or apply subcommand")
	}
	if result.command == "controller" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("controller requires the plan or apply subcommand")
	}
	if result.command == "update" && result.subcommand != "check" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("update requires the check, plan, or apply subcommand")
	}
	if result.target != "" && (result.command != "update" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--target is only valid with update plan or update apply")
	}
	if (result.allowPrerelease || result.allowDowngrade) && (result.command != "update" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("update policy flags are only valid with update plan or update apply")
	}
	if result.command == "update" && (result.subcommand == "plan" || result.subcommand == "apply") && result.target == "" {
		return options{}, fmt.Errorf("update %s requires --target", result.subcommand)
	}
	if result.command == "software" && result.subcommand != "catalog" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("software requires catalog, plan, or apply")
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") && (result.softwarePackage == "" || result.softwareScope == "") {
		return options{}, fmt.Errorf("software %s requires --package and --scope", result.subcommand)
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return options{}, err
		}
	}
	if (result.softwarePackage != "" || result.softwareScope != "" || result.remove) && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("software change flags are only valid with software plan or apply")
	}
	if result.command == "software" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("software apply requires --expect from software plan")
	}
	if result.command == "shutdown" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("shutdown requires the plan or apply subcommand")
	}
	if result.command == "shutdown" && result.on == "" {
		return options{}, fmt.Errorf("shutdown %s requires --on", result.subcommand)
	}
	if result.command == "shutdown" && result.subcommand == "apply" && result.expect == "" {
		return options{}, errors.New("shutdown apply requires --expect from shutdown plan")
	}
	if result.acknowledgeUnknown && result.command != "shutdown" {
		return options{}, errors.New("--acknowledge-unknown-sessions is only valid with shutdown")
	}
	if result.command == "services" && result.subcommand != "" && result.subcommand != "restart" {
		return options{}, errors.New("services accepts only the restart subcommand")
	}
	if result.command == "services" && result.subcommand == "restart" && result.service == "" {
		return options{}, errors.New("services restart requires cache")
	}
	if result.command == "logs" && result.subcommand != "" && result.subcommand != "show" {
		return options{}, errors.New("logs accepts only the show subcommand")
	}
	if result.command == "logs" && result.subcommand == "show" && result.logID == "" {
		return options{}, errors.New("logs show requires an operation log ID")
	}
	if result.paths != "" && (result.command != "git" || (result.subcommand != "commit-plan" && result.subcommand != "commit-apply")) {
		return options{}, errors.New("--paths is only valid with git commit plan or git commit apply")
	}
	if result.command == "git" && result.subcommand != "review" && result.subcommand != "commit-plan" && result.subcommand != "commit-apply" {
		return options{}, errors.New("git requires review, commit plan, or commit apply")
	}
	if result.command == "git" && (result.subcommand == "commit-plan" || result.subcommand == "commit-apply") && result.paths == "" {
		return options{}, fmt.Errorf("git %s requires --paths", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	if result.command == "deploy" && result.on == "" {
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
	if result.command == "git" && result.subcommand == "commit-apply" && result.expect == "" {
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
	fmt.Fprintln(writer, "Usage: nixorium [status|hosts|doctor|software catalog|software plan|software apply|shutdown plan|shutdown apply|deploy plan|deploy apply|controller plan|controller apply|services|services restart cache|logs|logs show|git review|git commit plan|git commit apply|update check|update plan|update apply|config validate|config plan|config apply|setup|setup configure|setup status|setup keys|setup install-secrets|setup apply|pxe prepare|pxe start|pxe stop|pxe recover] [options]")
	fmt.Fprintln(writer, "       software catalog")
	fmt.Fprintln(writer, "       software plan --package <id> --scope <all-clients|group:NAME|clients:pcNN,...> [--remove]")
	fmt.Fprintln(writer, "       software apply --package <id> --scope <scope> [--remove] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       shutdown plan --on <pcNN[,pcNN...]|@lab> [--acknowledge-unknown-sessions]")
	fmt.Fprintln(writer, "       shutdown apply --on <targets> [--acknowledge-unknown-sessions] --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       deploy plan --on <pcNN[,pcNN...]|@lab>")
	fmt.Fprintln(writer, "       deploy apply --on <targets> --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       controller plan")
	fmt.Fprintln(writer, "       controller apply --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       services restart cache [--yes]")
	fmt.Fprintln(writer, "       logs show <operation-log-id>")
	fmt.Fprintln(writer, "       git review shows bounded staged and unstaged changes without mutating Git")
	fmt.Fprintln(writer, "       git commit plan --paths <path[,path...]> creates an isolated proposal")
	fmt.Fprintln(writer, "       git commit apply --paths <paths> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       update check explicitly queries the configured public upstream")
	fmt.Fprintln(writer, "       update plan --target <vMAJOR.MINOR.PATCH[-PRERELEASE]>")
	fmt.Fprintln(writer, "       update apply --target <release> --expect <review-token> [--yes]")
	fmt.Fprintln(writer, "       config plan --file <candidate.json>")
	fmt.Fprintln(writer, "       config apply --file <candidate.json> --expect <sha256:fingerprint>")
	fmt.Fprintln(writer, "       setup keys --verify-only performs read-only correspondence checks")
	fmt.Fprintln(writer, "       nixorium opens the read-only management dashboard")
	fmt.Fprintln(writer, "       doctor --full also builds the controller configuration")
}

func runShutdownApply(ctx context.Context, manager *app.ShutdownManager, repository string, stdout, stderr io.Writer, requested string, policy domain.ShutdownSessionPolicy, expectedToken string, assumeYes, jsonOutput bool) int {
	plan := manager.Plan(ctx, repository, requested, policy)
	if plan.HasErrors() {
		if jsonOutput {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.ShutdownPlanText(stderr, plan)
		}
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: shutdown apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmShutdown(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Shutdown cancelled; no request was sent.")
			return 0
		}
	}
	report := manager.ApplyPlan(ctx, plan, expectedToken)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.ShutdownApplyText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func parseSoftwareScope(value string) (domain.SoftwareScope, error) {
	var scope domain.SoftwareScope
	if value == domain.SoftwareScopeAllClients {
		scope = domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}
	} else if group, found := strings.CutPrefix(value, "group:"); found && group != "" {
		scope = domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: group}
	} else if clients, found := strings.CutPrefix(value, "clients:"); found && clients != "" {
		scope = domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: strings.Split(clients, ",")}
	} else {
		return domain.SoftwareScope{}, errors.New("software scope must be all-clients, group:NAME, or clients:pcNN,...")
	}
	if err := domain.ValidateSoftwareScope(scope); err != nil {
		return domain.SoftwareScope{}, fmt.Errorf("invalid software scope: %w", err)
	}
	return scope, nil
}

func runDeploymentApply(ctx context.Context, repository string, stdout, stderr io.Writer, requested, expectedRevision string, assumeYes, jsonOutput bool) int {
	local := adapters.Local{}
	manager := app.NewDeploymentManager(local)
	plan := manager.Plan(ctx, repository, requested)
	if plan.HasErrors() {
		if jsonOutput {
			if err := presentation.JSON(stdout, plan); err != nil {
				fmt.Fprintln(stderr, "Error:", err)
			}
		} else {
			presentation.DeploymentPlanText(stderr, plan)
		}
		return 1
	}
	if plan.Revision != expectedRevision {
		report := manager.Execute(ctx, repository, requested, expectedRevision, "", io.Discard)
		if jsonOutput {
			if err := presentation.JSON(stdout, report); err != nil {
				fmt.Fprintln(stderr, "Error:", err)
			}
		} else {
			presentation.DeploymentExecutionText(stderr, report)
		}
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: deploy apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmDeploymentApply(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Deployment cancelled; no build or apply was started.")
			return 0
		}
	}

	stream := stdout
	if jsonOutput {
		stream = stderr
	}
	report := executeDeploymentOperation(ctx, manager, repository, requested, expectedRevision, stream)
	var renderErr error
	if jsonOutput {
		renderErr = presentation.JSON(stdout, report)
	} else {
		presentation.DeploymentExecutionText(stdout, report)
	}
	if renderErr != nil {
		fmt.Fprintln(stderr, "Error:", renderErr)
		return 1
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func executeDeploymentOperation(ctx context.Context, manager *app.DeploymentManager, repository, requested, expectedRevision string, stream io.Writer) domain.DeploymentExecutionReport {
	return executeDeploymentOperationWithProgress(ctx, manager, repository, requested, expectedRevision, stream, nil)
}

func executeDeploymentOperationWithProgress(ctx context.Context, manager *app.DeploymentManager, repository, requested, expectedRevision string, stream io.Writer, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
	operation, err := adapters.OpenDeploymentOperation()
	if err != nil {
		plan := manager.Plan(ctx, repository, requested)
		return domain.DeploymentExecutionReport{
			SchemaVersion:   domain.SchemaVersion,
			Operation:       "deploy-apply",
			State:           "failed",
			Repository:      plan.Repository,
			Requested:       plan.Requested,
			Revision:        plan.Revision,
			ColmenaSelector: plan.ColmenaSelector,
			Targets:         plan.Targets,
			Phase:           domain.DeploymentPhasePreflight,
			RetrySafe:       true,
			Message:         "prepare deployment operation: " + err.Error(),
			Issues:          plan.Issues,
		}
	}
	progress := io.MultiWriter(stream, operation.Writer())
	report := manager.ExecuteWithProgress(ctx, repository, requested, expectedRevision, operation.Path, progress, observe)
	fmt.Fprintf(operation.Writer(), "\nResult: %s\nPhase: %s\nBuild completed: %t\nApply completed: %t\nVerified targets: %d/%d\nRecorded targets: %d\nDetail: %s\n", report.State, report.Phase, report.BuildCompleted, report.ApplyCompleted, report.Verification.Verified, report.Verification.Attempted, report.Verification.Recorded, report.Message)
	if closeErr := operation.Close(); closeErr != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("%s; finalize durable log: %v", report.Message, closeErr)
	}
	report.Message = operationRecordMessage(report.Message, report)
	return report
}

func runGitCommitApply(ctx context.Context, manager *app.GitCommitManager, repository string, stdout, stderr io.Writer, paths, expectedToken string, assumeYes, jsonOutput bool) int {
	plan := manager.Plan(ctx, repository, paths)
	if plan.HasErrors() {
		if jsonOutput {
			if err := presentation.JSON(stdout, plan); err != nil {
				fmt.Fprintln(stderr, "Error:", err)
			}
		} else {
			presentation.GitCommitPlanText(stderr, plan)
		}
		return 1
	}
	if plan.ReviewToken != expectedToken {
		report := manager.Apply(ctx, repository, paths, expectedToken)
		if jsonOutput {
			_ = presentation.JSON(stdout, report)
		} else {
			presentation.GitCommitText(stderr, report)
		}
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: git commit apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmGitCommit(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Git commit cancelled; HEAD, index, and worktree were not changed.")
			return 0
		}
	}
	report := manager.Apply(ctx, repository, paths, expectedToken)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.GitCommitText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runUpdateApply(ctx context.Context, manager *app.UpdateManager, repository string, stdout, stderr io.Writer, target, expectedToken string, allowPrerelease, allowDowngrade, assumeYes, jsonOutput bool) int {
	plan := manager.Plan(ctx, repository, target, allowPrerelease, allowDowngrade)
	if plan.HasErrors() {
		if jsonOutput {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.UpdatePlanText(stderr, plan)
		}
		return 1
	}
	if plan.ReviewToken != expectedToken {
		report := manager.ApplyPlan(ctx, plan, expectedToken)
		if jsonOutput {
			_ = presentation.JSON(stdout, report)
		} else {
			presentation.UpdateApplyText(stderr, report)
		}
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: update apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmUpdate(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Update cancelled; flake.nix and flake.lock were not changed.")
			return 0
		}
	}
	report := manager.ApplyPlan(ctx, plan, expectedToken)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.UpdateApplyText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runPXEStart(ctx context.Context, repository string, stdout, stderr io.Writer, assumeYes, jsonOutput bool) int {
	manager := app.NewPXELifecycle(adapters.Local{})
	plan := manager.PlanStart(ctx, repository)
	if plan.HasErrors() {
		if jsonOutput {
			if err := presentation.JSON(stdout, plan); err != nil {
				fmt.Fprintln(stderr, "Error:", err)
			}
		} else {
			presentation.PXELifecycleText(stderr, plan)
		}
		return 1
	}
	if !assumeYes && plan.Mode != "active" {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: pxe start requires an interactive terminal or explicit --yes")
			return 2
		}
		approved, err := presentation.ConfirmPXEStart(os.Stdin, stdout, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(stdout, "PXE start cancelled; networking was not changed.")
			return 0
		}
	}
	report := manager.Start(ctx, repository)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.PXELifecycleText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runControllerApply(ctx context.Context, manager *app.ControllerManager, repository string, stdout, stderr io.Writer, expectedRevision string, assumeYes, jsonOutput bool) int {
	plan := manager.Plan(ctx, repository)
	if plan.HasErrors() || plan.Revision != expectedRevision {
		if plan.Revision != expectedRevision {
			plan.State = "blocked"
			plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: "review", Message: "deployment revision differs from the reviewed controller plan"})
		}
		if jsonOutput {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.ControllerRebuildPlanText(stderr, plan)
		}
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: controller apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmControllerRebuild(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Controller rebuild cancelled; no action started.")
			return 0
		}
	}
	writeControllerApplyActivity(stderr, app.ControllerApplyUnit(expectedRevision))
	report := runWithManagedProgress(
		func() domain.ControllerRebuildExecutionReport {
			return manager.Apply(ctx, repository, expectedRevision)
		},
		func() (domain.OperationProgress, error) {
			return app.NewOperationProgressManager(adapters.Local{}).Current("controller-apply")
		},
		stderr,
	)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.ControllerRebuildExecutionText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runServiceRestart(ctx context.Context, manager *app.ServiceManager, repository string, stdout, stderr io.Writer, service string, assumeYes, jsonOutput bool) int {
	status := manager.Status(ctx, repository)
	if len(status.Issues) > 0 || len(status.Services) == 0 || status.Services[0].ID != service || len(status.Services[0].Units) == 0 || !status.Services[0].Units[0].Loaded {
		if jsonOutput {
			_ = presentation.JSON(stdout, status)
		} else {
			presentation.ServicesText(stderr, status)
		}
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: services restart requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmServiceRestart(os.Stdin, confirmationOutput, status.Services[0])
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Service restart cancelled; no action started.")
			return 0
		}
	}
	report := manager.Restart(ctx, repository, service)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.ServiceActionText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runSetupApply(ctx context.Context, repository string, stdout, stderr io.Writer, assumeYes, jsonOutput bool) int {
	local := adapters.Local{}
	setupReport := app.NewSetupManager(local).Status(ctx, repository)
	for _, stage := range setupReport.Stages {
		if stage.ID == domain.SetupStageApply {
			break
		}
		if stage.State != domain.SetupStageComplete {
			presentation.SetupText(stderr, setupReport)
			fmt.Fprintln(stderr, "Error: complete and commit every setup stage before controller apply")
			return 1
		}
	}
	meta, err := local.LabMeta(ctx, repository)
	if err != nil {
		fmt.Fprintln(stderr, "Error: evaluate controller identity:", err)
		return 1
	}
	if !assumeYes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: setup apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if jsonOutput {
			confirmationOutput = stderr
		}
		approved, confirmErr := presentation.ConfirmControllerApply(os.Stdin, confirmationOutput, meta.Controller.Name)
		if confirmErr != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", confirmErr)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Controller apply cancelled; no action started.")
			return 0
		}
	}
	writeControllerApplyActivity(stderr, app.ApplyControllerUnit)
	report := runWithManagedProgress(
		func() domain.ActionReport { return app.NewSystemActions(local).ApplyController(ctx) },
		func() (domain.OperationProgress, error) {
			return app.NewOperationProgressManager(local).Current("controller-apply")
		},
		stderr,
	)
	report.Message = operationRecordMessage(report.Message, report)
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.ActionText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
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
		detected := local.DetectNetworkDefaults(staticAddress)
		if detected.DHCPAddress != "" {
			settings.Lab.MasterDHCPIP = detected.DHCPAddress
		}
		if detected.InterfaceName != "" {
			settings.Lab.InterfaceName = detected.InterfaceName
		}
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
