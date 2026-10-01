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

func runCleanupCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewCleanupManager(adapters.Local{})
	plan := manager.Plan(ctx, repository, options.on)
	if options.subcommand == "plan" || plan.HasErrors() || plan.State != "ready" {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.CleanupPlanText(stdout, plan)
		}
		if plan.HasErrors() || (options.subcommand == "apply" && plan.State != "ready") {
			return 1
		}
		return 0
	}
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "The computers changed after review; create a fresh cleanup plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "cleanup apply requires an interactive terminal or --yes.")
			return 2
		}
		approved, err := presentation.ConfirmCleanup(os.Stdin, stderr, plan)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !approved {
			fmt.Fprintln(stderr, "Confirmation did not match; nothing was removed.")
			return 1
		}
	}
	report := manager.Apply(ctx, plan, options.expect)
	report.Message = operationRecordMessage(report.Message, report)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.CleanupReportText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
