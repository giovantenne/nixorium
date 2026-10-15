package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestScheduleArguments(t *testing.T) {
	for _, test := range []struct {
		args       []string
		subcommand string
		schedule   scheduleArguments
	}{
		{[]string{"schedule", "show"}, "show", scheduleArguments{}},
		{[]string{"schedule", "show", "--json"}, "show", scheduleArguments{}},
		{[]string{"schedule", "set", "--on", "07:50", "--off", "18:00", "--days", "mon-fri"}, "set", scheduleArguments{on: "07:50", off: "18:00", days: "mon-fri"}},
		{[]string{"schedule", "set", "--days", "all", "--off", "20:00"}, "set", scheduleArguments{off: "20:00", days: "all"}},
		{[]string{"schedule", "set", "--on", "8:00", "--days", "mon,wed,fri", "--json"}, "set", scheduleArguments{on: "8:00", days: "mon,wed,fri"}},
		{[]string{"schedule", "clear"}, "clear", scheduleArguments{}},
		{[]string{"schedule", "run"}, "run", scheduleArguments{}},
		{[]string{"schedule", "run", "--repo", "/home/admin/nixorium-deployment"}, "run", scheduleArguments{}},
	} {
		options, err := parseArguments(test.args)
		if err != nil || options.command != "schedule" || options.subcommand != test.subcommand || options.schedule != test.schedule {
			t.Fatal(test.args, options, err)
		}
	}
	if options, err := parseArguments([]string{"schedule", "run", "--repo", "/srv/lab", "--json"}); err != nil || options.repository != "/srv/lab" || !options.json {
		t.Fatal(options, err)
	}
	if options, err := parseArguments([]string{"schedule", "--help"}); err != nil || !options.help {
		t.Fatal(options, err)
	}
	for _, args := range [][]string{
		{"schedule"},
		{"schedule", "plan"},
		{"schedule", "show", "clear"},
		{"schedule", "set"},
		{"schedule", "set", "--on", "07:50"},
		{"schedule", "set", "--days", "mon-fri"},
		{"schedule", "set", "--on", "7pm", "--days", "mon-fri"},
		{"schedule", "set", "--on", "25:00", "--days", "mon-fri"},
		{"schedule", "set", "--on", "07:50", "--days", "someday"},
		{"schedule", "set", "--on", "07:50", "--off", "07:50", "--days", "mon"},
		{"schedule", "set", "--on", "--days", "mon"},
		{"schedule", "set", "--on", "07:50", "--days", "mon", "--expect", "sha256:test"},
		{"schedule", "show", "--on", "07:50"},
		{"schedule", "clear", "--days", "mon"},
		{"schedule", "run", "--off", "18:00"},
		{"schedule", "show", "--repo", "/srv/lab"},
		{"schedule", "set", "--on", "07:50", "--days", "mon", "--yes"},
		{"schedule", "run", "--at", "07:50"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
	// Other commands do not take the schedule's options.
	for _, args := range [][]string{{"wake", "--on", "pc01", "--off", "18:00"}, {"shutdown", "plan", "--days", "mon"}} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("schedule option accepted by another command", args)
		}
	}
}

func scheduleReport(state, message string, schedule domain.Schedule) domain.ScheduleReport {
	if schedule.Days == nil {
		schedule.Days = []string{}
	}
	return domain.ScheduleReport{State: state, Schedule: schedule, Zone: "CEST", Summary: schedule.Summary(), Message: message}
}

func TestScheduleCommandShowsSetsAndClears(t *testing.T) {
	var requested []domain.Schedule
	var operations []domain.ClassroomOperation
	request := func(_ context.Context, operation domain.ClassroomOperation, schedule *domain.Schedule) domain.ScheduleReport {
		operations = append(operations, operation)
		if schedule == nil {
			return scheduleReport("completed", "Switch on at 07:50 and shut down at 18:00, Monday to Friday.", domain.Schedule{On: "07:50", Off: "18:00", Days: []string{"mon", "tue", "wed", "thu", "fri"}})
		}
		requested = append(requested, *schedule)
		if schedule.Empty() {
			return scheduleReport("completed", "The schedule was cleared.", domain.Schedule{})
		}
		return scheduleReport("completed", "Schedule saved. "+schedule.Summary(), *schedule)
	}
	run := func(options options) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := scheduleCommand(context.Background(), options, request, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	if code, out, errOut := run(options{subcommand: "show"}); code != 0 || errOut != "" ||
		out != "Switch on at 07:50 and shut down at 18:00, Monday to Friday.\nTimes are in the controller's time zone (CEST).\n" {
		t.Fatalf("show: %d %q %q", code, out, errOut)
	}
	code, out, _ := run(options{subcommand: "set", schedule: scheduleArguments{on: "7:50", off: "18:00", days: "mon-fri"}})
	if code != 0 || !strings.HasPrefix(out, "Schedule saved. Switch on at 07:50 and shut down at 18:00, Monday to Friday.\n") {
		t.Fatalf("set: %d %q", code, out)
	}
	// What is sent is normalized: times as HH:MM and days listed.
	if len(requested) != 1 || requested[0].On != "07:50" || len(requested[0].Days) != 5 || operations[1] != domain.ClassroomScheduleSetOperation {
		t.Fatalf("requested = %+v, operations %v", requested, operations)
	}
	if code, out, _ := run(options{subcommand: "clear"}); code != 0 || out != "The schedule was cleared.\n" || !requested[1].Empty() || operations[2] != domain.ClassroomScheduleSetOperation {
		t.Fatalf("clear: %d %q %+v", code, out, requested)
	}
	if code, out, _ := run(options{subcommand: "show", json: true}); code != 0 || !strings.Contains(out, `"summary"`) || !strings.Contains(out, `"schedule"`) {
		t.Fatalf("json: %d %q", code, out)
	}
	// Arguments the parser would have refused do not reach the service.
	if code, _, errOut := run(options{subcommand: "set", schedule: scheduleArguments{on: "7pm", days: "mon"}}); code != 2 || errOut == "" || len(requested) != 2 {
		t.Fatalf("bad time: %d %q %+v", code, errOut, requested)
	}
}

func TestScheduleCommandReportsWhatTheServiceRefused(t *testing.T) {
	request := func(context.Context, domain.ClassroomOperation, *domain.Schedule) domain.ScheduleReport {
		return scheduleReport("blocked", "The schedule could not be saved: the classroom state directory is not private to this account.", domain.Schedule{})
	}
	var stdout, stderr bytes.Buffer
	options := options{subcommand: "set", schedule: scheduleArguments{on: "07:50", days: "mon"}}
	if code := scheduleCommand(context.Background(), options, request, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "could not be saved") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestScheduleRunPrintsWhatItDidAndNothingWhenIdle(t *testing.T) {
	run := func(options options, report domain.ScheduleRunReport) (int, string) {
		var output bytes.Buffer
		code := scheduleRunCommand(context.Background(), options, func(context.Context) domain.ScheduleRunReport { return report }, &output)
		return code, output.String()
	}
	if code, out := run(options{}, domain.ScheduleRunReport{State: "idle"}); code != 0 || out != "" {
		t.Fatalf("idle: %d %q", code, out)
	}
	if code, out := run(options{json: true}, domain.ScheduleRunReport{State: "idle", Minute: "2026-10-12T07:51", Targets: []domain.ScheduleOutcome{}}); code != 0 || !strings.Contains(out, `"state": "idle"`) {
		t.Fatalf("idle json: %d %q", code, out)
	}
	done := domain.ScheduleRunReport{State: "completed", Action: domain.ScheduleActionOn, Message: "Scheduled switch-on at 07:50: Wake-on-LAN packets sent to 1 computer; 1 already on; 1 with no known address yet.",
		Targets: []domain.ScheduleOutcome{{Name: "pc01", State: domain.ScheduleOutcomeAlreadyOn}, {Name: "pc02", State: domain.ScheduleOutcomeSent}, {Name: "pc03", State: domain.ScheduleOutcomeUnknownAddress}}}
	if code, out := run(options{}, done); code != 0 || out != "pc01  already on\npc02  packet sent\npc03  address not known yet\n"+done.Message+"\n" {
		t.Fatalf("switch-on: %d %q", code, out)
	}
	off := domain.ScheduleRunReport{State: "completed", Action: domain.ScheduleActionOff, Message: "Scheduled shut-down at 18:00: 1 computer asked to shut down; 1 left on because someone is using them.",
		Targets: []domain.ScheduleOutcome{{Name: "pc01", State: domain.ScheduleOutcomeAccepted}, {Name: "pc02", State: domain.ScheduleOutcomeInUse}}}
	if code, out := run(options{}, off); code != 0 || out != "pc01  asked to shut down\npc02  in use, left on\n"+off.Message+"\n" {
		t.Fatalf("shut-down: %d %q", code, out)
	}
	// A run that could not do its work fails, so the timer's unit shows it.
	blocked := domain.ScheduleRunReport{State: "blocked", Action: domain.ScheduleActionOff, Message: "Scheduled shut-down at 18:00 was not done: another client operation is running.", Targets: []domain.ScheduleOutcome{}}
	if code, out := run(options{}, blocked); code != 1 || out != blocked.Message+"\n" {
		t.Fatalf("blocked: %d %q", code, out)
	}
	if code, _ := run(options{}, domain.ScheduleRunReport{State: "partial", Message: "x"}); code != 1 {
		t.Fatalf("partial: %d", code)
	}
}
