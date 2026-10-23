package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func runInternetCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewInternetManager(adapters.Local{})
	plan := manager.Plan(ctx, repository, options.on, options.internetAction)
	if options.subcommand == "apply" {
		plan = manager.Replan(ctx, repository, options.on, options.internetAction, options.expect)
	}
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
	if code, proceed := confirmReviewedApply(options.yes, stderr, stderr, applyConfirmation{noTerminal: "Internet apply requires an interactive terminal or --yes."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmInternet(os.Stdin, output, plan)
	}); !proceed {
		return code
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

// parseInternetLockFlags reads --action, shared by internet and lock.
func parseInternetLockFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--action":
		index++
		if index >= len(arguments) {
			return index, true, errors.New("--action requires block/unblock or lock/unlock")
		}
		result.internetAction = domain.InternetAction(arguments[index])
	default:
		return index, false, nil
	}
	return index, true, nil
}

// validateInternet rejects option combinations that do not belong to the command.
func validateInternet(result *options) error {
	if result.internetAction != "" && result.command != "internet" {
		return errors.New("--action is only valid with internet or lock")
	}
	if result.command == "internet" {
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || !result.internetAction.Valid() {
			return errors.New("internet requires plan/apply, --on and --action block/unblock")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return errors.New("internet apply requires --expect from internet plan")
		}
	}
	return nil
}
