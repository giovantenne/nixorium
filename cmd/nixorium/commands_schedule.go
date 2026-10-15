package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// Scheduled switch-on and shut-down of the student computers. show, set and
// clear go through the classroom service, which keeps the schedule on the
// controller, so the teacher and the administrator use them alike; run is
// what the controller's timer starts every minute, as the service user.

// scheduleArguments are the options of the schedule command.
type scheduleArguments struct {
	on, off, days string
}

func parseScheduleArguments(arguments []string) (options, error) {
	o := options{command: "schedule"}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "show", "set", "clear", "run":
			if o.subcommand != "" {
				return o, errors.New("only one schedule action may be selected")
			}
			o.subcommand = argument
		case "--on", "--off", "--days", "--repo":
			index++
			if index >= len(arguments) || arguments[index] == "" {
				return o, fmt.Errorf("%s requires a value", argument)
			}
			switch argument {
			case "--on":
				o.schedule.on = arguments[index]
			case "--off":
				o.schedule.off = arguments[index]
			case "--days":
				o.schedule.days = arguments[index]
			default:
				o.repository = arguments[index]
			}
		case "--json":
			o.json = true
		case "-h", "--help", "help":
			o.help = true
		default:
			return o, fmt.Errorf("unknown schedule argument %q", argument)
		}
	}
	if o.help {
		return o, nil
	}
	switch o.subcommand {
	case "set":
		if o.schedule.on == "" && o.schedule.off == "" {
			return o, errors.New("schedule set requires --on HH:MM, --off HH:MM or both")
		}
		if _, err := scheduleFromArguments(o.schedule); err != nil {
			return o, err
		}
	case "show", "clear", "run":
		if o.schedule != (scheduleArguments{}) {
			return o, fmt.Errorf("schedule %s takes no --on, --off or --days", o.subcommand)
		}
	default:
		return o, errors.New("schedule requires show, set, clear or run")
	}
	if o.repository != "" && o.subcommand != "run" {
		return o, errors.New("--repo is only valid with schedule run")
	}
	return o, nil
}

// defaultScheduleDays are the weekdays of a schedule set without --days, the
// ones the classroom view's dialog starts with.
const defaultScheduleDays = "mon-fri"

// scheduleFromArguments reads and checks the times and the weekdays.
func scheduleFromArguments(arguments scheduleArguments) (domain.Schedule, error) {
	if arguments.days == "" {
		arguments.days = defaultScheduleDays
	}
	days, err := domain.ParseScheduleDays(arguments.days)
	if err != nil {
		return domain.Schedule{}, err
	}
	schedule := domain.Schedule{On: arguments.on, Off: arguments.off, Days: days}
	return schedule.Normalize()
}

func runScheduleCommand(ctx context.Context, options options, stdout, stderr io.Writer) int {
	if options.subcommand == "run" {
		repository, err := resolveRepository(options.repository)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
		local := adapters.Local{}
		runner := app.NewScheduleRunner(local, app.NewWakeManager(local), app.NewShutdownManager(local), local)
		return scheduleRunCommand(ctx, options, func(ctx context.Context) domain.ScheduleRunReport {
			return runner.Run(ctx, repository)
		}, stdout)
	}
	return scheduleCommand(ctx, options, classroomSchedule, stdout, stderr)
}

// scheduleCommand shows, sets or clears the schedule.
func scheduleCommand(ctx context.Context, options options, request func(context.Context, domain.ClassroomOperation, *domain.Schedule) domain.ScheduleReport, stdout, stderr io.Writer) int {
	operation, schedule := domain.ClassroomScheduleGetOperation, (*domain.Schedule)(nil)
	switch options.subcommand {
	case "set":
		requested, err := scheduleFromArguments(options.schedule)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 2
		}
		operation, schedule = domain.ClassroomScheduleSetOperation, &requested
	case "clear":
		operation, schedule = domain.ClassroomScheduleSetOperation, &domain.Schedule{Days: []string{}}
	}
	report := request(ctx, operation, schedule)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else if report.HasErrors() {
		fmt.Fprintln(stderr, report.Message)
	} else {
		fmt.Fprintln(stdout, report.Message)
		if !report.Schedule.Empty() && report.Zone != "" {
			fmt.Fprintf(stdout, "Times are in the controller's time zone (%s).\n", report.Zone)
		}
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func classroomSchedule(ctx context.Context, operation domain.ClassroomOperation, schedule *domain.Schedule) domain.ScheduleReport {
	response, err := classroomRequest(ctx, operation, func(request *domain.ClassroomRequest) {
		request.Schedule = schedule
	})
	if err != nil || response.Schedule == nil {
		message := classroomMessage(response, err, "The schedule is unavailable.")
		return domain.ScheduleReport{SchemaVersion: domain.SchemaVersion, Operation: "schedule-show", State: "blocked", Schedule: domain.Schedule{Days: []string{}}, Message: message, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
	}
	return *response.Schedule
}

// scheduleRunCommand does what is due now and prints what it did, one line
// for each computer and then a summary. When nothing is due it prints
// nothing, so the timer's every-minute run leaves the journal quiet.
func scheduleRunCommand(ctx context.Context, options options, run func(context.Context) domain.ScheduleRunReport, stdout io.Writer) int {
	report := run(ctx)
	switch {
	case options.json:
		_ = presentation.JSON(stdout, report)
	case report.State != "idle":
		for _, target := range report.Targets {
			line := target.Name + "  " + scheduleOutcomeWords(target.State)
			fmt.Fprintln(stdout, line)
		}
		fmt.Fprintln(stdout, report.Message)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func scheduleOutcomeWords(state string) string {
	switch state {
	case domain.ScheduleOutcomeSent:
		return "packet sent"
	case domain.ScheduleOutcomeAlreadyOn:
		return "already on"
	case domain.ScheduleOutcomeUnknownAddress:
		return "address not known yet"
	case domain.ScheduleOutcomeNotSent:
		return "not sent"
	case domain.ScheduleOutcomeAccepted:
		return "asked to shut down"
	case domain.ScheduleOutcomeInUse:
		return "in use, left on"
	case domain.ScheduleOutcomeNotReachable:
		return "off or not reachable"
	case domain.ScheduleOutcomeSessionUnknown:
		return "session unknown, left on"
	case domain.ScheduleOutcomeUnconfirmed:
		return "not confirmed"
	}
	return state
}
