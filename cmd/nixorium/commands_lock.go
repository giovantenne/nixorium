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

// classroomViewSetting reads only the classroomView switch; a missing or
// unreadable settings file means it is off.
func classroomViewSetting(reader interface{ ReadSettings(string) ([]byte, error) }, repository string) bool {
	data, err := reader.ReadSettings(repository)
	if err != nil {
		return false
	}
	settings, _ := domain.DecodeLabSettings(data)
	return settings.Lab.ClassroomViewOn()
}

func runLockCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
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
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "The computers changed since the review; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Lock apply requires an interactive terminal or --yes.")
			return 2
		}
		approved, err := presentation.ConfirmLock(os.Stdin, stderr, plan)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !approved {
			return 0
		}
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
