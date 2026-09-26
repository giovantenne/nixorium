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

func runInternetCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewInternetManager(adapters.Local{})
	plan := manager.Plan(ctx, repository, options.on, options.internetAction)
	if options.subcommand == "plan" || plan.HasErrors() {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.InternetPlanText(stdout, plan)
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "Internet state or review changed; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Internet apply requires an interactive terminal or --yes.")
			return 2
		}
		approved, err := presentation.ConfirmInternet(os.Stdin, stderr, plan)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !approved {
			return 0
		}
	}
	report := manager.Apply(ctx, plan, options.expect)
	report.Message = operationRecordMessage(report.Message, report)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.InternetReportText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
