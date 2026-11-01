package main

import (
	"context"
	"errors"
	"io"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// runWakeCommand switches on student computers with Wake-on-LAN. The
// administrator, who can read the deployment, uses the application
// directly; the teacher goes through the classroom service, which validates
// the same targets. A packet changes no system state, so there is no review
// token and no administrative operation lock; the output has one line for
// each computer.
func runWakeCommand(ctx context.Context, options options, stdout, stderr io.Writer) int {
	plan, apply := classroomWakePlan, classroomWakeApply
	if repository, err := resolveRepository(options.repository); err == nil {
		manager := app.NewWakeManager(adapters.Local{})
		plan = func(ctx context.Context, requested string) domain.WakePlan {
			return manager.Plan(ctx, repository, requested)
		}
		apply = manager.Apply
	}
	return wakeCommand(ctx, options, plan, apply, stdout)
}

func wakeCommand(ctx context.Context, options options, plan func(context.Context, string) domain.WakePlan, apply func(context.Context, domain.WakePlan) domain.WakeReport, stdout io.Writer) int {
	reviewed := plan(ctx, options.on)
	if reviewed.HasErrors() {
		if options.json {
			_ = presentation.JSON(stdout, reviewed)
		} else {
			presentation.WakePlanText(stdout, reviewed)
		}
		return 1
	}
	report := apply(ctx, reviewed)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.WakeReportText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func classroomWakePlan(ctx context.Context, requested string) domain.WakePlan {
	response, err := classroomRequest(ctx, domain.ClassroomWakePlanOperation, func(request *domain.ClassroomRequest) {
		request.Requested = requested
	})
	if err != nil || response.WakePlan == nil {
		message := classroomMessage(response, err, "Switching computers on is unavailable.")
		return domain.WakePlan{SchemaVersion: domain.SchemaVersion, Operation: "wake-plan", State: "blocked", Requested: requested, Targets: []domain.WakeTarget{}, Message: message, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
	}
	return *response.WakePlan
}

func classroomWakeApply(ctx context.Context, plan domain.WakePlan) domain.WakeReport {
	response, err := classroomRequest(ctx, domain.ClassroomWakeApplyOperation, func(request *domain.ClassroomRequest) {
		request.WakePlan = &plan
	})
	if err != nil || response.WakeReport == nil {
		return domain.WakeReport{SchemaVersion: domain.SchemaVersion, Operation: "wake-apply", State: "blocked", Targets: []domain.WakeOutcome{}, Message: classroomMessage(response, err, "The packets could not be sent.")}
	}
	return *response.WakeReport
}

// validateWake rejects option combinations that do not belong to the command.
func validateWake(result *options) error {
	if result.command == "wake" && (result.subcommand != "" || result.on == "") {
		return errors.New("wake requires --on <clients|@lab> and no subcommand")
	}
	return nil
}
