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

// lockSource reads the laboratory identities and reaches the classroom agents.
type lockSource struct {
	adapters.Local
	adapters.ClassroomAgentConnector
}

func runLockCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	if !adapters.ClassroomViewApplied() {
		if options.json {
			_ = presentation.JSON(stdout, domain.LockPlan{SchemaVersion: domain.SchemaVersion, Operation: "lock-plan", State: "blocked", Action: options.lockAction, Message: domain.ClassroomViewOffMessage, Issues: []domain.ValidationIssue{{Field: "classroom", Message: domain.ClassroomViewOffMessage}}})
		} else {
			fmt.Fprintln(stderr, domain.ClassroomViewOffMessage)
		}
		return 1
	}
	manager := app.NewLockManager(lockSource{})
	plan := manager.Plan(ctx, repository, options.on, options.lockAction)
	if options.subcommand == "plan" || plan.HasErrors() {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.LockPlanText(stdout, plan)
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	// The plan above is fresh, so compare what it covers with the review.
	plan, match := manager.Reviewed(plan, options.expect)
	if match != app.ReviewMatched {
		reviewMismatch(stderr, match, "The computers changed since the review; create a fresh plan.")
		return 1
	}
	if code, proceed := confirmReviewedApply(options.yes, stderr, stderr, applyConfirmation{noTerminal: "Lock apply requires an interactive terminal or --yes."}, func(output io.Writer) (bool, error) {
		return presentation.ConfirmLock(os.Stdin, output, plan)
	}); !proceed {
		return code
	}
	report := manager.Apply(ctx, plan, options.expect)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.LockReportText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

// validateLock rejects option combinations that do not belong to the command.
func validateLock(result *options) error {
	if result.command == "lock" {
		// --action is parsed once; lock reads it as lock or unlock.
		result.lockAction, result.internetAction = domain.LockAction(result.internetAction), ""
		if (result.subcommand != "plan" && result.subcommand != "apply") || result.on == "" || !result.lockAction.Valid() {
			return errors.New("lock requires plan/apply, --on and --action lock/unlock")
		}
		if result.subcommand == "apply" && result.expect == "" {
			return errors.New("lock apply requires --expect from lock plan")
		}
	}
	return nil
}
