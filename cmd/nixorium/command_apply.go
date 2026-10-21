package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func runShutdownApply(ctx context.Context, manager *app.ShutdownManager, repository string, stdout, stderr io.Writer, requested string, policy domain.ShutdownSessionPolicy, action domain.ClientPowerAction, expectedToken string, assumeYes, jsonOutput bool) int {
	plan := manager.PlanAction(ctx, repository, requested, policy, action)
	if plan.HasErrors() {
		if jsonOutput {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.ShutdownPlanText(stderr, plan)
		}
		return 1
	}
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: fmt.Sprintf("Error: %s apply requires an interactive terminal or explicit --yes", optionsPowerCommand(action)), readFailure: "Error: read confirmation:", cancelled: "Power action cancelled; no request was sent."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmShutdown(os.Stdin, output, plan)
	}); !proceed {
		return code
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

func optionsPowerCommand(action domain.ClientPowerAction) string {
	if action == domain.ClientRestart {
		return "restart"
	}
	return "shutdown"
}

func parseSoftwareScope(value string) (domain.SoftwareScope, error) {
	var scope domain.SoftwareScope
	if value == domain.SoftwareScopeAllClients || value == domain.SoftwareScopeShared || value == domain.SoftwareScopeController {
		scope = domain.SoftwareScope{Kind: value}
	} else if group, found := strings.CutPrefix(value, "group:"); found && group != "" {
		scope = domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: group}
	} else if clients, found := strings.CutPrefix(value, "clients:"); found && clients != "" {
		scope = domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: strings.Split(clients, ",")}
	} else {
		return domain.SoftwareScope{}, errors.New("software scope must be shared, controller, all-clients, group:NAME, or clients:pcNN,...")
	}
	if err := domain.ValidateSoftwareScope(scope); err != nil {
		return domain.SoftwareScope{}, fmt.Errorf("invalid software scope: %w", err)
	}
	return scope, nil
}

func runDeploymentApply(ctx context.Context, repository string, stdout, stderr io.Writer, requested, expectedRevision string, assumeYes, jsonOutput, queueUnreachable bool) int {
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
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: "Error: deploy apply requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "Deployment cancelled; no build or apply was started."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmDeploymentApply(os.Stdin, output, plan)
	}); !proceed {
		return code
	}

	stream := stdout
	if jsonOutput {
		stream = stderr
	}
	if queueUnreachable {
		// Computers that did not answer get this revision when they do.
		reachable, err := queueUnreachableComputers(plan, stream)
		if err != nil {
			fmt.Fprintln(stderr, "Error: queue the unreachable computers:", err)
			return 1
		}
		if reachable == "" {
			fmt.Fprintln(stream, "No selected computer is reachable now; all of them update when switched on.")
			return 0
		}
		requested = reachable
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
	report, _ := executeDeploymentOperationGated(ctx, manager, repository, requested, expectedRevision, stream, observe)
	return report
}

// executeDeploymentOperationGated also reports a busy operation gate, in
// which case nothing was attempted.
func executeDeploymentOperationGated(ctx context.Context, manager *app.DeploymentManager, repository, requested, expectedRevision string, stream io.Writer, observe func(domain.DeploymentProgress)) (domain.DeploymentExecutionReport, bool) {
	operation, err := adapters.OpenDeploymentOperation()
	if err != nil {
		// Nothing was attempted: report from the inputs, without evaluating
		// or probing the laboratory again (the queue runner asks every minute).
		var busy *adapters.OperationBusyError
		root, rootErr := filepath.Abs(repository)
		if rootErr != nil {
			root = repository
		}
		return domain.DeploymentExecutionReport{
			SchemaVersion: domain.SchemaVersion,
			Operation:     "deploy-apply",
			State:         "failed",
			Repository:    root,
			Requested:     requested,
			Revision:      expectedRevision,
			Targets:       []domain.DeploymentTarget{},
			Phase:         domain.DeploymentPhasePreflight,
			RetrySafe:     true,
			Message:       "prepare deployment operation: " + err.Error(),
			Issues:        []domain.ValidationIssue{{Field: "operation", Message: err.Error()}},
		}, errors.As(err, &busy)
	}
	progress := io.MultiWriter(stream, operation.Writer())
	report := manager.ExecuteWithProgress(ctx, repository, requested, expectedRevision, operation.Path, progress, observe)
	fmt.Fprintf(operation.Writer(), "\nResult: %s\nPhase: %s\nBuild completed: %t\nApply completed: %t\nVerified targets: %d/%d\nRecorded targets: %d\nDetail: %s\n", report.State, report.Phase, report.BuildCompleted, report.ApplyCompleted, report.Verification.Verified, report.Verification.Attempted, report.Verification.Recorded, report.Message)
	if closeErr := operation.Close(); closeErr != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("%s; finalize durable log: %v", report.Message, closeErr)
	}
	report.Message = operationRecordMessage(report.Message, report)
	forgetQueuedUpdates(report)
	return report, false
}

// forgetQueuedUpdates removes queue entries of computers this update just
// verified: they run the current revision, so an older queued update for
// them is obsolete. A refused queue change is harmless: the next pass finds
// them current and removes them without updating them again.
func forgetQueuedUpdates(report domain.DeploymentExecutionReport) {
	if report.RecoveryRequired || !report.ApplyCompleted {
		return
	}
	verified := []string{}
	for _, target := range report.Verification.Targets {
		if target.State == "verified" {
			verified = append(verified, target.Name)
		}
	}
	if len(verified) == 0 {
		return
	}
	status, err := adapters.ManagedDeferredUpdates().Read()
	if err != nil {
		return
	}
	queued := []string{}
	for _, name := range verified {
		if _, found := status.Find(name); found {
			queued = append(queued, name)
		}
	}
	if len(queued) > 0 {
		_, _ = newDeferredUpdateManager().Cancel(queued)
	}
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
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: "Error: git commit apply requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "Git commit cancelled; HEAD, index, and worktree were not changed."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmGitCommit(os.Stdin, output, plan)
	}); !proceed {
		return code
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
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: "Error: update apply requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "Update cancelled; flake.nix and flake.lock were not changed."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmUpdate(os.Stdin, output, plan)
	}); !proceed {
		return code
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
	if code, proceed := confirmReviewedApply(assumeYes || plan.Mode == "active", stderr, stdout, applyConfirmation{noTerminal: "Error: pxe start requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "PXE start cancelled; networking was not changed."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmPXEStart(os.Stdin, output, plan)
	}); !proceed {
		return code
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
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: "Error: controller apply requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "Apply to controller cancelled; nothing was started."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmControllerRebuild(os.Stdin, output, plan)
	}); !proceed {
		return code
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
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: "Error: services restart requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "Service restart cancelled; no action started."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmServiceRestart(os.Stdin, output, status.Services[0])
	}); !proceed {
		return code
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
	if code, proceed := confirmReviewedApply(assumeYes, stderr, confirmationStream(stdout, stderr, jsonOutput), applyConfirmation{noTerminal: "Error: setup apply requires an interactive terminal or explicit --yes", readFailure: "Error: read confirmation:", cancelled: "Controller apply cancelled; no action started."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmControllerApply(os.Stdin, output, meta.Controller.Name)
	}); !proceed {
		return code
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
