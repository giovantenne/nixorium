package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

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
