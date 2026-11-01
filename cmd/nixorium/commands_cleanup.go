package main

import (
	"context"
	"errors"
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
	plan, match := manager.Reviewed(plan, options.expect)
	if match != app.ReviewMatched {
		reviewMismatch(stderr, match, "The computers changed after review; create a fresh cleanup plan.")
		return 1
	}
	if code, proceed := confirmReviewedApply(options.yes, stderr, stderr, applyConfirmation{noTerminal: "cleanup apply requires an interactive terminal or --yes.", cancelled: "Confirmation did not match; nothing was removed.", cancelCode: 1}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmCleanup(os.Stdin, output, plan)
	}); !proceed {
		return code
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

// validateCleanup rejects option combinations that do not belong to the command.
func validateCleanup(result *options) error {
	if result.command == "cleanup" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" {
			return errors.New("cleanup requires plan or apply and --on <controller,pcNN,...|@lab>")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return errors.New("cleanup apply requires --expect from cleanup plan")
		}
	}
	return nil
}
