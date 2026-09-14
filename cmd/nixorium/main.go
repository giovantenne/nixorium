package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

type options struct {
	command    string
	subcommand string
	repository string
	file       string
	expect     string
	on         string
	json       bool
	full       bool
	help       bool
	guided     bool
	verifyOnly bool
	yes        bool
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
		lifecycle := app.NewPXELifecycle(local)
		deploymentManager := app.NewDeploymentManager(local)
		actions := presentation.DashboardActions{
			Refresh: func() (domain.StatusReport, error) {
				return inspector.Status(ctx, repository)
			},
			LoadHosts: func() (domain.HostsReport, error) {
				return inspector.Hosts(ctx, repository)
			},
			PlanDeployment: func(requested string) domain.DeploymentPlanReport {
				return deploymentManager.Plan(ctx, repository, requested)
			},
			ApplyDeployment: func(plan domain.DeploymentPlanReport) domain.DeploymentExecutionReport {
				return executeDeploymentOperation(ctx, deploymentManager, repository, plan.Requested, plan.Revision, io.Discard)
			},
			PreparePXE: func() domain.ActionReport {
				return app.NewSystemActions(local).PreparePXE(ctx)
			},
			PlanPXEStart: func() domain.PXELifecycleReport {
				return lifecycle.PlanStart(ctx, repository)
			},
			StartPXE: func() domain.PXELifecycleReport {
				return lifecycle.Start(ctx, repository)
			},
			StopPXE: func() domain.PXELifecycleReport {
				return lifecycle.Stop(ctx, repository)
			},
			RecoverPXE: func() domain.PXELifecycleReport {
				return lifecycle.Recover(ctx, repository)
			},
		}
		if tuiErr := presentation.RunDashboard(report, actions); tuiErr != nil {
			fmt.Fprintln(stderr, "Error:", tuiErr)
			return 1
		}
		return 0
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
			return runSetupConfigure(ctx, repository, stdout, stderr, options.guided)
		} else if options.subcommand == "apply" {
			return runSetupApply(ctx, repository, stdout, stderr, options.yes, options.json)
		} else if options.subcommand == "install-secrets" {
			report := app.NewSystemActions(adapters.Local{}).InstallSecrets(ctx)
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
			report := app.NewSystemActions(adapters.Local{}).PreparePXE(ctx)
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

func commandRequiresRepository(options options) bool {
	return options.command != "pxe" || (options.subcommand != "stop" && options.subcommand != "recover")
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
		case "doctor", "hosts", "deploy", "config", "setup", "pxe":
			if result.command != "" {
				return options{}, errors.New("only one command may be selected")
			}
			result.command = arguments[index]
		case "validate":
			if result.command != "config" || result.subcommand != "" {
				return options{}, errors.New("validate must follow config")
			}
			result.subcommand = "validate"
		case "plan":
			if (result.command != "config" && result.command != "deploy") || result.subcommand != "" {
				return options{}, errors.New("plan must follow config or deploy")
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
			if (result.command == "config" || result.command == "deploy") && result.subcommand == "" {
				result.subcommand = "apply"
				continue
			}
			if result.command != "setup" || result.subcommand != "" {
				return options{}, errors.New("apply must follow config or setup")
			}
			result.subcommand = "apply"
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
			return options{}, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}
	if result.full && result.command != "doctor" {
		return options{}, errors.New("--full is only valid with doctor")
	}
	if result.verifyOnly && (result.command != "setup" || result.subcommand != "keys") {
		return options{}, errors.New("--verify-only is only valid with setup keys")
	}
	if result.yes && !((result.command == "setup" && result.subcommand == "apply") || (result.command == "pxe" && result.subcommand == "start") || (result.command == "deploy" && result.subcommand == "apply")) {
		return options{}, errors.New("--yes is only valid with setup apply, pxe start, or deploy apply")
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
	if result.expect != "" && !((result.command == "config" || result.command == "deploy") && result.subcommand == "apply") {
		return options{}, errors.New("--expect is only valid with config apply or deploy apply")
	}
	if result.on != "" && (result.command != "deploy" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return options{}, errors.New("--on is only valid with deploy plan or deploy apply")
	}
	if result.command == "deploy" && result.subcommand != "plan" && result.subcommand != "apply" {
		return options{}, errors.New("deploy requires the plan or apply subcommand")
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
	fmt.Fprintln(writer, "Usage: nixorium [status|hosts|doctor|deploy plan|deploy apply|config validate|config plan|config apply|setup|setup configure|setup status|setup keys|setup install-secrets|setup apply|pxe prepare|pxe start|pxe stop|pxe recover] [options]")
	fmt.Fprintln(writer, "       deploy plan --on <pcNN[,pcNN...]|@lab>")
	fmt.Fprintln(writer, "       deploy apply --on <targets> --expect <git-revision> [--yes]")
	fmt.Fprintln(writer, "       config plan --file <candidate.json>")
	fmt.Fprintln(writer, "       config apply --file <candidate.json> --expect <sha256:fingerprint>")
	fmt.Fprintln(writer, "       setup keys --verify-only performs read-only correspondence checks")
	fmt.Fprintln(writer, "       nixorium opens the read-only management dashboard")
	fmt.Fprintln(writer, "       doctor --full also builds the controller configuration")
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
	report := manager.Execute(ctx, repository, requested, expectedRevision, operation.Path, progress)
	fmt.Fprintf(operation.Writer(), "\nResult: %s\nPhase: %s\nBuild completed: %t\nApply completed: %t\nVerified targets: %d/%d\nRecorded targets: %d\nDetail: %s\n", report.State, report.Phase, report.BuildCompleted, report.ApplyCompleted, report.Verification.Verified, report.Verification.Attempted, report.Verification.Recorded, report.Message)
	if closeErr := operation.Close(); closeErr != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("%s; finalize durable log: %v", report.Message, closeErr)
	}
	return report
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
		approved, confirmErr := presentation.ConfirmControllerApply(os.Stdin, stdout, meta.Controller.Name)
		if confirmErr != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", confirmErr)
			return 1
		}
		if !approved {
			fmt.Fprintln(stdout, "Controller apply cancelled; no action started.")
			return 0
		}
	}
	report := app.NewSystemActions(local).ApplyController(ctx)
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
		detected := local.DetectNetworkDefaults()
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
	credentials := []struct {
		label string
		value *string
	}{
		{label: "Administrator password", value: &candidate.Lab.AdminPassword},
		{label: "Teacher password", value: &candidate.Lab.TeacherPassword},
		{label: "Student password", value: &candidate.Lab.StudentPassword},
	}
	for _, credential := range credentials {
		if *credential.value != domain.DefaultPasswordHash {
			continue
		}
		hash, hashErr := app.CollectNamedPasswordHash(ctx, secretReader, local, credential.label)
		if hashErr != nil {
			fmt.Fprintf(stderr, "Error: %s: %v\n", credential.label, hashErr)
			return 1
		}
		*credential.value = hash
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
	presentation.ConfigApplyText(stdout, report)
	if report.HasErrors() {
		return 1
	}
	if reconcileKeys {
		keyReport, keyErr := app.NewSetupManager(local).ReconcileKeys(ctx, repository)
		presentation.KeyReconcileText(stdout, keyReport)
		if keyErr != nil {
			fmt.Fprintln(stderr, "Error:", keyErr)
			return 1
		}
		actionReport := app.NewSystemActions(local).InstallSecrets(ctx)
		presentation.ActionText(stdout, actionReport)
		if actionReport.HasErrors() {
			fmt.Fprintln(stderr, "The settings and repository keys are intact; retry with `nixorium setup install-secrets` on the controller.")
			return 1
		}
		fmt.Fprintln(stdout, "Review and commit lab-settings.json and the public files under keys/ before applying the controller.")
	}
	return 0
}
