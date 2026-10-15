package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// ScheduleStore keeps the schedule record on the controller: reading, and
// changing it under a lock, so that the classroom service that sets the
// schedule and the timer that runs it never overwrite one another.
type ScheduleStore interface {
	ReadSchedule() domain.ScheduleRecord
	UpdateSchedule(func(domain.ScheduleRecord) (domain.ScheduleRecord, bool)) error
}

// ScheduleManager shows, sets and clears the schedule. Like blocking the
// Internet it is an operational aid, not laboratory configuration: it changes
// no system and needs no review token.
type ScheduleManager struct {
	store ScheduleStore
	now   func() time.Time
}

func NewScheduleManager(store ScheduleStore) *ScheduleManager {
	return &ScheduleManager{store: store, now: time.Now}
}

func (m *ScheduleManager) report(operation string) domain.ScheduleReport {
	record := m.store.ReadSchedule()
	schedule := record.Schedule
	if schedule.Days == nil {
		schedule.Days = []string{}
	}
	zone, _ := m.now().Zone()
	summary := schedule.Summary()
	return domain.ScheduleReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     operation,
		State:         "completed",
		Schedule:      schedule,
		Zone:          zone,
		Summary:       summary,
		Message:       summary,
		Issues:        []domain.ValidationIssue{},
	}
}

// Show returns the current schedule.
func (m *ScheduleManager) Show() domain.ScheduleReport { return m.report("schedule-show") }

// Set replaces the schedule. A schedule without times clears it.
func (m *ScheduleManager) Set(requested domain.Schedule) domain.ScheduleReport {
	refuse := func(field, message string) domain.ScheduleReport {
		report := m.report("schedule-set")
		report.State, report.Message = "blocked", message
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
		return report
	}
	normalized, err := requested.Normalize()
	if err != nil {
		return refuse("schedule", "The schedule was not saved: "+err.Error()+".")
	}
	err = m.store.UpdateSchedule(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
		// The last minute that ran stays, so saving again within that
		// minute does not run it twice.
		record.Schedule = normalized
		return record, true
	})
	if err != nil {
		return refuse("store", "The schedule could not be saved: "+err.Error()+".")
	}
	report := m.report("schedule-set")
	if normalized.Empty() {
		report.Message = "The schedule was cleared."
	} else {
		report.Message = "Schedule saved. " + report.Summary
	}
	return report
}

// ScheduleRunner is what the timer runs every minute. It starts the switch-on
// or the shut-down that is due in the current minute, for every client
// computer, exactly once per minute: the minute is claimed in the schedule
// record before anything is sent.
//
// Switching on is the ordinary Wake-on-LAN operation (same address record,
// same packet sender; computers without a known address are skipped).
// Shutting down is the ordinary reviewed shut-down with the skip-active
// session policy, so a computer someone is using, or whose session cannot be
// read, is left on; the review and the check just before each request apply
// that rule, and the operation lock keeps it from running beside another
// client operation.
type ScheduleRunner struct {
	schedule *ScheduleManager
	wake     *WakeManager
	power    *ShutdownManager
	records  OperationRecordSink
}

func NewScheduleRunner(store ScheduleStore, wake *WakeManager, power *ShutdownManager, records OperationRecordSink) *ScheduleRunner {
	return &ScheduleRunner{schedule: NewScheduleManager(store), wake: wake, power: power, records: records}
}

// Run does what is due now. Nothing due, no schedule, or a minute that
// already ran is an idle report without any effect.
func (r *ScheduleRunner) Run(ctx context.Context, repository string) domain.ScheduleRunReport {
	now := r.schedule.now()
	report := domain.ScheduleRunReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "schedule-run",
		State:         "idle",
		Minute:        domain.ScheduleMinute(now),
		Targets:       []domain.ScheduleOutcome{},
	}
	if record := r.schedule.store.ReadSchedule(); !scheduleDue(record, now) {
		return report
	}
	// Claim the minute under the lock before doing anything, so a second run
	// in the same minute, or a run that stops half way, never repeats it.
	claimed := false
	var due domain.Schedule
	err := r.schedule.store.UpdateSchedule(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
		if !scheduleDue(record, now) {
			return record, false
		}
		record.LastRun = report.Minute
		due, claimed = record.Schedule, true
		return record, true
	})
	if err != nil {
		report.State = "blocked"
		report.Message = "The scheduled start was not done because the schedule could not be claimed: " + err.Error() + "."
		return report
	}
	if !claimed {
		return report
	}
	on, _ := due.Due(now)
	if on {
		report.Action = domain.ScheduleActionOn
		r.switchOn(ctx, repository, &report)
	} else {
		report.Action = domain.ScheduleActionOff
		r.shutDown(ctx, repository, &report)
	}
	if r.records != nil {
		if err := RecordOperationOutcome(r.records, report); err != nil {
			report.Message += " Its history record could not be saved."
		}
	}
	return report
}

// scheduleDue tells whether the schedule starts something in the minute of
// now that did not run yet.
func scheduleDue(record domain.ScheduleRecord, now time.Time) bool {
	if record.Empty() || record.LastRun == domain.ScheduleMinute(now) {
		return false
	}
	on, off := record.Due(now)
	return on || off
}

func scheduleClock(report domain.ScheduleRunReport) string {
	if _, clock, found := strings.Cut(report.Minute, "T"); found {
		return clock
	}
	return report.Minute
}

func (r *ScheduleRunner) switchOn(ctx context.Context, repository string, report *domain.ScheduleRunReport) {
	label := "Scheduled switch-on at " + scheduleClock(*report)
	plan := r.wake.Plan(ctx, repository, "@lab")
	if plan.HasErrors() {
		report.State = "blocked"
		report.Message = label + " was not done: " + plan.Message
		return
	}
	applied := r.wake.Apply(ctx, plan)
	if applied.State == "blocked" {
		report.State = "blocked"
		report.Message = label + " was not done: " + applied.Message
		return
	}
	for _, target := range applied.Targets {
		outcome := domain.ScheduleOutcome{Name: target.Name, Detail: target.Detail}
		switch target.State {
		case domain.WakeOutcomeSent:
			outcome.State = domain.ScheduleOutcomeSent
		case domain.WakeOutcomeOn:
			outcome.State = domain.ScheduleOutcomeAlreadyOn
		case domain.WakeOutcomeUnknown:
			outcome.State = domain.ScheduleOutcomeUnknownAddress
		default:
			outcome.State = domain.ScheduleOutcomeNotSent
		}
		report.Targets = append(report.Targets, outcome)
	}
	report.State = "completed"
	if applied.Failed > 0 {
		report.State = "partial"
	}
	parts := []string{fmt.Sprintf("Wake-on-LAN packets sent to %s", wakeComputers(applied.Sent))}
	parts = appendCount(parts, applied.AlreadyOn, "already on")
	parts = appendCount(parts, applied.Unknown, "with no known address yet")
	parts = appendCount(parts, applied.Failed, "not sent")
	report.Message = label + ": " + strings.Join(parts, "; ") + "."
}

func (r *ScheduleRunner) shutDown(ctx context.Context, repository string, report *domain.ScheduleRunReport) {
	label := "Scheduled shut-down at " + scheduleClock(*report)
	plan := r.power.PlanAction(ctx, repository, "@lab", domain.ShutdownSkipActive, domain.ClientPowerOff)
	outcomes := map[string]domain.ScheduleOutcome{}
	for _, target := range plan.Targets {
		outcomes[target.Name] = scheduleShutdownSkip(target)
	}
	finish := func() {
		for _, target := range plan.Targets {
			outcome := outcomes[target.Name]
			if outcome.State == "" {
				outcome.State, outcome.Detail = domain.ScheduleOutcomeNotSent, "No request result came back; it was left on."
			}
			report.Targets = append(report.Targets, outcome)
		}
	}
	if plan.State != "ready" {
		if !scheduleNothingEligible(plan) {
			report.State = "blocked"
			report.Message = label + " was not done: " + plan.Message
			return
		}
		// Every computer is off or in use: nothing to ask, nothing wrong.
		report.State = "completed"
		finish()
		report.Message = label + ": " + scheduleShutdownMessage(report.Targets)
		return
	}
	applied := r.power.ApplyPlan(ctx, plan, plan.ReviewToken)
	if len(applied.Issues) > 0 {
		report.State = "blocked"
		report.Message = label + " was not done: " + applied.Message
		return
	}
	for _, target := range applied.Targets {
		planned := outcomes[target.Name]
		switch target.State {
		case "accepted":
			outcomes[target.Name] = domain.ScheduleOutcome{Name: target.Name, State: domain.ScheduleOutcomeAccepted, Detail: "The request was accepted."}
		case "unconfirmed":
			outcomes[target.Name] = domain.ScheduleOutcome{Name: target.Name, State: domain.ScheduleOutcomeUnconfirmed, Detail: "The request could not be confirmed; look at the computer."}
		default:
			// Not eligible in the review, or no longer eligible just
			// before the request: someone sat down, or it went off.
			if planned.State == "" {
				planned = domain.ScheduleOutcome{Name: target.Name, State: domain.ScheduleOutcomeNotSent, Detail: "It was left on: " + target.Detail + "."}
			}
			outcomes[target.Name] = planned
		}
	}
	finish()
	report.State = "completed"
	for _, outcome := range report.Targets {
		if outcome.State == domain.ScheduleOutcomeUnconfirmed {
			report.State = "partial"
		}
	}
	report.Message = label + ": " + scheduleShutdownMessage(report.Targets)
}

// scheduleNothingEligible tells a review that found no computer to shut down
// from one that could not be made at all.
func scheduleNothingEligible(plan domain.ShutdownPlanReport) bool {
	if len(plan.Targets) == 0 || len(plan.Issues) == 0 {
		return false
	}
	for _, issue := range plan.Issues {
		if issue.Field != "targets" {
			return false
		}
	}
	return true
}

// scheduleShutdownSkip says why a computer of the review is left on. An
// eligible computer has no outcome yet.
func scheduleShutdownSkip(target domain.ShutdownTargetPlan) domain.ScheduleOutcome {
	outcome := domain.ScheduleOutcome{Name: target.Name}
	switch {
	case target.Eligible:
	case target.Reachability != domain.ReachabilityReachable || target.SSH != domain.SSHAvailable:
		outcome.State, outcome.Detail = domain.ScheduleOutcomeNotReachable, "Off or not reachable."
	case target.Session == domain.ShutdownSessionActive:
		outcome.State, outcome.Detail = domain.ScheduleOutcomeInUse, "Someone is using it; it was left on."
	default:
		outcome.State, outcome.Detail = domain.ScheduleOutcomeSessionUnknown, "Its session could not be checked; it was left on."
	}
	return outcome
}

func scheduleShutdownMessage(targets []domain.ScheduleOutcome) string {
	counts := map[string]int{}
	for _, target := range targets {
		counts[target.State]++
	}
	parts := []string{}
	if asked := counts[domain.ScheduleOutcomeAccepted]; asked > 0 {
		parts = append(parts, wakeComputers(asked)+" asked to shut down")
	}
	parts = appendCount(parts, counts[domain.ScheduleOutcomeInUse], "left on because someone is using them")
	parts = appendCount(parts, counts[domain.ScheduleOutcomeSessionUnknown], "left on because their session could not be checked")
	parts = appendCount(parts, counts[domain.ScheduleOutcomeNotSent], "left on at the last check")
	parts = appendCount(parts, counts[domain.ScheduleOutcomeNotReachable], "off or not reachable")
	parts = appendCount(parts, counts[domain.ScheduleOutcomeUnconfirmed], "not confirmed")
	if len(parts) == 0 {
		return "no computer to shut down."
	}
	return strings.Join(parts, "; ") + "."
}

func appendCount(parts []string, count int, words string) []string {
	if count == 0 {
		return parts
	}
	return append(parts, fmt.Sprintf("%d %s", count, words))
}

// scheduleRunRecord is the history record of a run: counts only, never a
// message.
func scheduleRunRecord(report domain.ScheduleRunReport) domain.OperationRecord {
	kind := "scheduled switch-on"
	if report.Action == domain.ScheduleActionOff {
		kind = "scheduled shut-down"
	}
	record := domain.OperationRecord{Operation: report.Operation, State: report.State, Subject: kind + " at " + scheduleClock(report)}
	counts := map[string]int{}
	for _, target := range report.Targets {
		counts[target.State]++
	}
	switch {
	case report.State == "blocked":
		record.Summary = "nothing was sent; see the journal of the schedule service"
	case report.Action == domain.ScheduleActionOff:
		record.Summary = fmt.Sprintf("shutdown requests: accepted=%d; in-use=%d; not-reachable=%d; session-unknown=%d; not-sent=%d; unconfirmed=%d",
			counts[domain.ScheduleOutcomeAccepted], counts[domain.ScheduleOutcomeInUse], counts[domain.ScheduleOutcomeNotReachable],
			counts[domain.ScheduleOutcomeSessionUnknown], counts[domain.ScheduleOutcomeNotSent], counts[domain.ScheduleOutcomeUnconfirmed])
	default:
		record.Summary = fmt.Sprintf("Wake-on-LAN packets: sent=%d; already-on=%d; unknown-address=%d; not-sent=%d",
			counts[domain.ScheduleOutcomeSent], counts[domain.ScheduleOutcomeAlreadyOn], counts[domain.ScheduleOutcomeUnknownAddress], counts[domain.ScheduleOutcomeNotSent])
	}
	return record
}
