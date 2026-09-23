package main

import (
	"context"
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func commandOutputError(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func runDeploymentCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	if options.subcommand == "apply" {
		return runDeploymentApply(ctx, repository, stdout, stderr, options.on, options.expect, options.yes, options.json)
	}
	report := app.NewDeploymentManager(adapters.Local{}).Plan(ctx, repository, options.on)
	var err error
	if options.json {
		err = presentation.JSON(stdout, report)
	} else {
		presentation.DeploymentPlanText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return commandOutputError(err, stderr)
}

func runControllerCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewControllerManager(adapters.Local{})
	if options.subcommand == "apply" {
		return runControllerApply(ctx, manager, repository, stdout, stderr, options.expect, options.yes, options.json)
	}
	report := manager.Plan(ctx, repository)
	var err error
	if options.json {
		err = presentation.JSON(stdout, report)
	} else {
		presentation.ControllerRebuildPlanText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return commandOutputError(err, stderr)
}

func runServicesCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewServiceManager(adapters.Local{})
	if options.subcommand == "restart" {
		return runServiceRestart(ctx, manager, repository, stdout, stderr, options.service, options.yes, options.json)
	}
	report := manager.Status(ctx, repository)
	var err error
	if options.json {
		err = presentation.JSON(stdout, report)
	} else {
		presentation.ServicesText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return commandOutputError(err, stderr)
}

func runLogsCommand(options options, stdout, stderr io.Writer) int {
	manager := app.NewOperationLogManager(adapters.Local{})
	var err error
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
	return commandOutputError(err, stderr)
}

func runGitCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	local := adapters.Local{}
	var err error
	switch options.subcommand {
	case "review":
		report := app.NewGitReviewManager(local).Review(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.GitReviewText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "commit-plan":
		report := app.NewGitCommitManager(local).Plan(ctx, repository, options.paths)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.GitCommitPlanText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	default:
		return runGitCommitApply(ctx, app.NewGitCommitManager(local), repository, stdout, stderr, options.paths, options.expect, options.yes, options.json)
	}
	return commandOutputError(err, stderr)
}

func runPackageBaseCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewPackageBaseManager(adapters.PackageBase{})
	var err error
	switch options.subcommand {
	case "status":
		report := manager.PackageBaseStatus(repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.PackageBaseStatusText(stdout, report)
		}
		if len(report.Issues) > 0 {
			return 1
		}
	case "plan":
		report := manager.Plan(ctx, repository, options.target, options.allowUnverified, false)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.UpdatePlanText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "apply":
		return runUpdateApply(ctx, manager, repository, stdout, stderr, options.target, options.expect, options.allowUnverified, false, options.yes, options.json)
	}
	return commandOutputError(err, stderr)
}

func runUpdateCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewUpdateManager(adapters.Local{})
	var err error
	switch options.subcommand {
	case "check":
		report := manager.Check(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.UpdateCheckText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "plan":
		report := manager.Plan(ctx, repository, options.target, options.allowPrerelease, options.allowDowngrade)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.UpdatePlanText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	default:
		return runUpdateApply(ctx, manager, repository, stdout, stderr, options.target, options.expect, options.allowPrerelease, options.allowDowngrade, options.yes, options.json)
	}
	return commandOutputError(err, stderr)
}

func runShutdownCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewShutdownManager(adapters.Local{})
	policy := domain.ShutdownProtectUnknown
	if options.acknowledgeUnknown {
		policy = domain.ShutdownAcknowledgeUnknown
	}
	if options.subcommand != "plan" {
		return runShutdownApply(ctx, manager, repository, stdout, stderr, options.on, policy, options.expect, options.yes, options.json)
	}
	report := manager.Plan(ctx, repository, options.on, policy)
	var err error
	if options.json {
		err = presentation.JSON(stdout, report)
	} else {
		presentation.ShutdownPlanText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return commandOutputError(err, stderr)
}
