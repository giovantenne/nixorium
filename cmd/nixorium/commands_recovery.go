package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func runDeploymentRecoveryCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewDeploymentRecoveryManager(adapters.Local{})
	plan := manager.Plan(ctx, repository, options.acknowledgeUnreachable)
	if options.subcommand == "recover-plan" || plan.State != "ready" {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.DeploymentRecoveryPlanText(stdout, plan)
		}
		if plan.HasErrors() || (options.subcommand == "recover-apply" && plan.State != "ready") {
			return 1
		}
		return 0
	}
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "The computers or the interrupted update record changed after review; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "deploy recover apply requires an interactive terminal or --yes.")
			return 2
		}
		presentation.DeploymentRecoveryPlanText(stderr, plan)
		approved, err := presentation.ConfirmWord(os.Stdin, stderr, plan.Confirmation)
		if err != nil || !approved {
			fmt.Fprintln(stderr, "Confirmation did not match; nothing was changed.")
			return 1
		}
	}
	result := manager.Apply(ctx, plan, options.expect)
	result.Message = operationRecordMessage(result.Message, result)
	if options.json {
		_ = presentation.JSON(stdout, result)
	} else {
		presentation.DeploymentRecoveryResultText(stdout, result)
	}
	if result.HasErrors() {
		return 1
	}
	return 0
}

func runTemplateResetRecoveryCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	reset := adapters.TemplateReset{}
	plan := reset.PlanTemplateResetRecovery(ctx, repository)
	if options.subcommand == "recover-plan" || plan.State != "ready" {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.TemplateResetRecoveryPlanText(stdout, plan)
		}
		if plan.HasErrors() || (options.subcommand == "recover-apply" && plan.State != "ready") {
			return 1
		}
		return 0
	}
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "The repository changed after review; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "template-reset recover apply requires an interactive terminal or --yes.")
			return 2
		}
		presentation.TemplateResetRecoveryPlanText(stderr, plan)
		approved, err := presentation.ConfirmWord(os.Stdin, stderr, plan.Confirmation)
		if err != nil || !approved {
			fmt.Fprintln(stderr, "Confirmation did not match; nothing was changed.")
			return 1
		}
	}
	result := reset.ApplyTemplateResetRecovery(ctx, plan)
	result.Message = operationRecordMessage(result.Message, result)
	if options.json {
		_ = presentation.JSON(stdout, result)
	} else {
		presentation.TemplateResetRecoveryResultText(stdout, result)
	}
	if result.HasErrors() {
		return 1
	}
	return 0
}

func runGitDiscardApply(ctx context.Context, manager *app.GitDiscardManager, repository string, options options, stdout, stderr io.Writer) int {
	plan := manager.Plan(ctx, repository, options.paths)
	if plan.HasErrors() {
		presentation.GitCommitPlanText(stderr, plan)
		return 1
	}
	if plan.ReviewToken != options.expect {
		fmt.Fprintln(stderr, "The selected files changed after review; create a fresh discard plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "git discard apply requires an interactive terminal or --yes.")
			return 2
		}
		presentation.GitCommitPlanText(stderr, plan)
		approved, err := presentation.ConfirmWord(os.Stdin, stderr, plan.Confirmation)
		if err != nil || !approved {
			fmt.Fprintln(stderr, "Confirmation did not match; nothing was discarded.")
			return 1
		}
	}
	report := manager.Apply(ctx, repository, options.paths, options.expect)
	report.Message = operationRecordMessage(report.Message, report)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.GitCommitText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
