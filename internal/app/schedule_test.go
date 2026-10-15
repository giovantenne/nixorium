package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeScheduleStore struct {
	record  domain.ScheduleRecord
	failure error
	updates int
}

func (store *fakeScheduleStore) ReadSchedule() domain.ScheduleRecord {
	if store.record.Days == nil {
		store.record.Days = []string{}
	}
	return store.record
}

func (store *fakeScheduleStore) UpdateSchedule(change func(domain.ScheduleRecord) (domain.ScheduleRecord, bool)) error {
	if store.failure != nil {
		return store.failure
	}
	store.updates++
	next, save := change(store.ReadSchedule())
	if save {
		if next.Empty() {
			next = domain.ScheduleRecord{}
		}
		store.record = next
	}
	return nil
}

type scheduleRecords struct {
	records []domain.OperationRecord
	failure error
}

func (sink *scheduleRecords) RecordOperation(record domain.OperationRecord) error {
	if sink.failure != nil {
		return sink.failure
	}
	sink.records = append(sink.records, record)
	return nil
}

var (
	cest            = time.FixedZone("CEST", 2*3600)
	mondayMorning   = time.Date(2026, 10, 12, 7, 50, 0, 0, cest)
	mondayEvening   = time.Date(2026, 10, 12, 18, 0, 0, 0, cest)
	schoolDays      = []string{"mon", "tue", "wed", "thu", "fri"}
	schoolSchedule  = domain.Schedule{On: "07:50", Off: "18:00", Days: schoolDays}
	scheduleContext = context.Background()
)

func TestScheduleManagerShowsSetsAndClears(t *testing.T) {
	store := &fakeScheduleStore{}
	manager := NewScheduleManager(store)
	manager.now = func() time.Time { return mondayMorning }
	if shown := manager.Show(); shown.HasErrors() || shown.Message != "No schedule." || !shown.Schedule.Empty() || shown.Zone != "CEST" || shown.Schedule.Days == nil {
		t.Fatalf("empty show = %+v", shown)
	}
	set := manager.Set(domain.Schedule{On: "7:50", Off: "18:00", Days: []string{"fri", "mon", "tue", "wed", "thu"}})
	if set.HasErrors() || set.Message != "Schedule saved. Switch on at 07:50 and shut down at 18:00, Monday to Friday." || store.record.On != "07:50" || store.record.Days[0] != "mon" {
		t.Fatalf("set = %+v, record %+v", set, store.record)
	}
	if shown := manager.Show(); shown.Summary != set.Summary || shown.Schedule.Off != "18:00" {
		t.Fatalf("show = %+v", shown)
	}
	// The minute that ran last survives a new schedule.
	store.record.LastRun = "2026-10-12T07:50"
	manager.Set(domain.Schedule{On: "08:00", Days: []string{"mon"}})
	if store.record.LastRun != "2026-10-12T07:50" || store.record.On != "08:00" || store.record.Off != "" {
		t.Fatalf("record = %+v", store.record)
	}
	cleared := manager.Set(domain.Schedule{Days: []string{}})
	if cleared.HasErrors() || cleared.Message != "The schedule was cleared." || !store.record.Empty() || store.record.LastRun != "" {
		t.Fatalf("cleared = %+v, record %+v", cleared, store.record)
	}
}

func TestScheduleManagerRefusesAnInvalidSchedule(t *testing.T) {
	store := &fakeScheduleStore{record: domain.ScheduleRecord{SchemaVersion: 1, Schedule: domain.Schedule{On: "06:00", Days: []string{"mon"}}}}
	manager := NewScheduleManager(store)
	for name, schedule := range map[string]domain.Schedule{
		"no days":     {On: "07:50"},
		"bad time":    {Off: "6pm", Days: []string{"mon"}},
		"same time":   {On: "07:50", Off: "07:50", Days: []string{"mon"}},
		"bad weekday": {On: "07:50", Days: []string{"funday"}},
	} {
		report := manager.Set(schedule)
		if !report.HasErrors() || !strings.HasPrefix(report.Message, "The schedule was not saved: ") || len(report.Issues) != 1 {
			t.Fatalf("%s: %+v", name, report)
		}
		// What was there stays, and the report shows it.
		if store.record.On != "06:00" || report.Schedule.On != "06:00" {
			t.Fatalf("%s: record %+v, report %+v", name, store.record, report)
		}
	}
	store.failure = errors.New("the classroom state directory is not private to this account")
	if report := manager.Set(schoolSchedule); !report.HasErrors() || !strings.Contains(report.Message, "could not be saved") {
		t.Fatalf("store failure = %+v", report)
	}
}

type scheduleLab struct {
	runner  *ScheduleRunner
	store   *fakeScheduleStore
	wake    *fakeWakeSource
	power   *fakeShutdownSource
	records *scheduleRecords
	now     time.Time
}

// scheduleFixture is a laboratory of four computers for both halves. For
// switching on: pc01 is on, pc02 is off with a known address, pc03 and pc04
// are off and never seen. For shutting down: pc01 idle, pc02 in use, pc03 off, pc04
// logged in without use.
func scheduleFixture(schedule domain.Schedule) *scheduleLab {
	lab := &scheduleLab{store: &fakeScheduleStore{}, wake: wakeLab(), records: &scheduleRecords{}, now: mondayMorning}
	lab.store.record = domain.ScheduleRecord{SchemaVersion: 1, Schedule: schedule}
	lab.wake.meta.Clients.Hosts = append(lab.wake.meta.Clients.Hosts, domain.HostMeta{Name: "pc04", IP: "10.42.0.14"})
	power, powerManager := shutdownFixture()
	power.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}, {Name: "pc03", IP: "10.0.0.3"}, {Name: "pc04", IP: "10.0.0.4"}}
	reachable := func(session domain.ShutdownSessionState) domain.ShutdownObservation {
		return domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: session}
	}
	power.observations = map[string]domain.ShutdownObservation{
		"pc01": reachable(domain.ShutdownSessionIdle),
		"pc02": reachable(domain.ShutdownSessionActive),
		"pc03": {Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Session: domain.ShutdownSessionUnknown},
		"pc04": reachable(domain.ShutdownSessionUnused),
	}
	power.dispatch = map[string]domain.ShutdownDispatchResult{"pc01": {Accepted: true}, "pc02": {Accepted: true}, "pc04": {Accepted: true}}
	lab.power = power
	lab.runner = NewScheduleRunner(lab.store, NewWakeManager(lab.wake), powerManager, lab.records)
	lab.runner.schedule.now = func() time.Time { return lab.now }
	return lab
}

func outcomeStates(report domain.ScheduleRunReport) map[string]string {
	states := map[string]string{}
	for _, target := range report.Targets {
		states[target.Name] = target.State
	}
	return states
}

func TestScheduleRunDoesNothingWhenNothingIsDue(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	for name, now := range map[string]time.Time{
		"minute before": mondayMorning.Add(-time.Minute),
		"minute after":  mondayMorning.Add(time.Minute),
		"saturday":      time.Date(2026, 10, 17, 7, 50, 0, 0, cest),
		"utc clock":     mondayMorning.UTC(),
	} {
		lab.now = now
		if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "idle" || report.Action != "" || len(report.Targets) != 0 || report.HasErrors() {
			t.Fatalf("%s: %+v", name, report)
		}
	}
	empty := scheduleFixture(domain.Schedule{Days: []string{}})
	if report := empty.runner.Run(scheduleContext, "/lab"); report.State != "idle" {
		t.Fatalf("no schedule: %+v", report)
	}
	if lab.store.updates != 0 || len(lab.wake.sent) != 0 || len(lab.power.dispatchHosts) != 0 || len(lab.records.records) != 0 {
		t.Fatalf("idle runs changed something: %d updates, %v sent, %v asked, %v records", lab.store.updates, lab.wake.sent, lab.power.dispatchHosts, lab.records.records)
	}
}

func TestScheduleRunSwitchesOnComputersWithAKnownAddress(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "completed" || report.Action != domain.ScheduleActionOn || report.Minute != "2026-10-12T07:50" || report.HasErrors() {
		t.Fatalf("report = %+v", report)
	}
	// The ordinary switch-on: one packet for the computer that is off and
	// has an address; the computer already on and the one never seen get none.
	if len(lab.wake.sent) != 1 || lab.wake.sent[0] != "52:54:00:00:00:02" {
		t.Fatalf("packets = %v", lab.wake.sent)
	}
	states := outcomeStates(report)
	if states["pc01"] != domain.ScheduleOutcomeAlreadyOn || states["pc02"] != domain.ScheduleOutcomeSent || states["pc03"] != domain.ScheduleOutcomeUnknownAddress {
		t.Fatalf("states = %v", states)
	}
	if !strings.HasPrefix(report.Message, "Scheduled switch-on at 07:50: Wake-on-LAN packets sent to 1 computer; 1 already on; 2 with no known address yet.") {
		t.Fatalf("message = %q", report.Message)
	}
	if len(lab.power.dispatchHosts) != 0 {
		t.Fatal("a switch-on shut a computer down")
	}
	if lab.store.record.LastRun != "2026-10-12T07:50" || lab.store.record.On != "07:50" {
		t.Fatalf("record = %+v", lab.store.record)
	}
	if len(lab.records.records) != 1 {
		t.Fatalf("history = %+v", lab.records.records)
	}
	record := lab.records.records[0]
	if record.Operation != "schedule-run" || record.State != "completed" || record.Subject != "scheduled switch-on at 07:50" ||
		record.Summary != "Wake-on-LAN packets: sent=1; already-on=1; unknown-address=2; not-sent=0" {
		t.Fatalf("record = %+v", record)
	}
}

func TestScheduleRunNeverRunsAMinuteTwice(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "completed" {
		t.Fatalf("first run = %+v", report)
	}
	for range 3 {
		lab.now = lab.now.Add(10 * time.Second)
		if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "idle" || len(report.Targets) != 0 {
			t.Fatalf("second run in the same minute = %+v", report)
		}
	}
	if len(lab.wake.sent) != 1 || len(lab.records.records) != 1 {
		t.Fatalf("%d packets, %d records", len(lab.wake.sent), len(lab.records.records))
	}
	// The same minute on the next Monday runs again.
	lab.now = mondayMorning.AddDate(0, 0, 7)
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "completed" || len(lab.wake.sent) != 2 {
		t.Fatalf("next week = %+v, %d packets", report, len(lab.wake.sent))
	}
	// A run that races another one for the minute loses the claim.
	lab.now = mondayMorning.AddDate(0, 0, 14)
	racing := &racingScheduleStore{fakeScheduleStore: lab.store, claim: domain.ScheduleMinute(lab.now)}
	lab.runner.schedule.store = racing
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "idle" || len(lab.wake.sent) != 2 {
		t.Fatalf("lost claim = %+v, %d packets", report, len(lab.wake.sent))
	}
}

// racingScheduleStore lets another run claim the minute between the read
// that finds the schedule due and the claim.
type racingScheduleStore struct {
	*fakeScheduleStore
	claim string
}

func (store *racingScheduleStore) UpdateSchedule(change func(domain.ScheduleRecord) (domain.ScheduleRecord, bool)) error {
	store.record.LastRun = store.claim
	return store.fakeScheduleStore.UpdateSchedule(change)
}

func TestScheduleRunShutsDownOnlyComputersNobodyIsUsing(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.now = mondayEvening
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "completed" || report.Action != domain.ScheduleActionOff || report.HasErrors() {
		t.Fatalf("report = %+v", report)
	}
	states := outcomeStates(report)
	if states["pc01"] != domain.ScheduleOutcomeAccepted || states["pc04"] != domain.ScheduleOutcomeAccepted ||
		states["pc02"] != domain.ScheduleOutcomeInUse || states["pc03"] != domain.ScheduleOutcomeNotReachable {
		t.Fatalf("states = %v", states)
	}
	// Only the computers nobody uses are asked, and to power off.
	if lab.power.action != domain.ClientPowerOff || len(lab.power.dispatchHosts) != 2 || lab.power.dispatchHosts[0].Name != "pc01" || lab.power.dispatchHosts[1].Name != "pc04" {
		t.Fatalf("asked %v for %s", lab.power.dispatchHosts, lab.power.action)
	}
	if report.Message != "Scheduled shut-down at 18:00: 2 computers asked to shut down; 1 left on because someone is using them; 1 off or not reachable." {
		t.Fatalf("message = %q", report.Message)
	}
	if len(lab.wake.sent) != 0 {
		t.Fatal("a shut-down sent Wake-on-LAN packets")
	}
	if len(lab.records.records) != 1 || lab.records.records[0].Subject != "scheduled shut-down at 18:00" ||
		lab.records.records[0].Summary != "shutdown requests: accepted=2; in-use=1; not-reachable=1; session-unknown=0; not-sent=0; unconfirmed=0" {
		t.Fatalf("history = %+v", lab.records.records)
	}
	// Not twice.
	if again := lab.runner.Run(scheduleContext, "/lab"); again.State != "idle" || len(lab.power.dispatchHosts) != 2 {
		t.Fatalf("second run = %+v", again)
	}
}

// A computer whose session cannot be read is left on, like one in use.
func TestScheduleRunLeavesComputersWithAnUnknownSession(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.now = mondayEvening
	lab.power.observations["pc01"] = domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnknown, Detail: "session helper unavailable"}
	report := lab.runner.Run(scheduleContext, "/lab")
	if outcomeStates(report)["pc01"] != domain.ScheduleOutcomeSessionUnknown || len(lab.power.dispatchHosts) != 1 || lab.power.dispatchHosts[0].Name != "pc04" {
		t.Fatalf("report = %+v, asked %v", report, lab.power.dispatchHosts)
	}
}

// Someone sits down between the review and the request: the check just
// before the request leaves that computer on.
type seatedScheduleSource struct {
	*fakeShutdownSource
	checks int
}

func (source *seatedScheduleSource) ShutdownObservations(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.ShutdownObservation {
	source.checks++
	observations := source.fakeShutdownSource.ShutdownObservations(ctx, hosts, timeout)
	if source.checks > 1 {
		observations["pc01"] = domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive}
	}
	return observations
}

func TestScheduleRunLeavesAComputerSomeoneSatDownAt(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.now = mondayEvening
	seated := &seatedScheduleSource{fakeShutdownSource: lab.power}
	manager := NewShutdownManager(seated)
	manager.now = func() time.Time { return time.Unix(1000, 0).UTC() }
	lab.runner.power = manager
	report := lab.runner.Run(scheduleContext, "/lab")
	states := outcomeStates(report)
	if states["pc01"] != domain.ScheduleOutcomeNotSent || states["pc04"] != domain.ScheduleOutcomeAccepted || len(lab.power.dispatchHosts) != 1 || lab.power.dispatchHosts[0].Name != "pc04" {
		t.Fatalf("report = %+v, asked %v", report, lab.power.dispatchHosts)
	}
	if report.State != "completed" || !strings.Contains(report.Message, "1 left on at the last check") {
		t.Fatalf("message = %q (%s)", report.Message, report.State)
	}
}

func TestScheduleRunHasNothingToShutDownWhenEveryComputerIsOffOrInUse(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.now = mondayEvening
	lab.power.observations["pc01"] = domain.ShutdownObservation{Reachability: domain.ReachabilityUnreachable, Session: domain.ShutdownSessionUnknown}
	lab.power.observations["pc04"] = domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive}
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "completed" || report.HasErrors() || len(report.Targets) != 4 || len(lab.power.dispatchHosts) != 0 {
		t.Fatalf("report = %+v", report)
	}
	if report.Message != "Scheduled shut-down at 18:00: 2 left on because someone is using them; 2 off or not reachable." {
		t.Fatalf("message = %q", report.Message)
	}
}

func TestScheduleRunReportsUnconfirmedRequests(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.now = mondayEvening
	lab.power.dispatch = map[string]domain.ShutdownDispatchResult{"pc01": {Accepted: true}}
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "partial" || !report.HasErrors() || outcomeStates(report)["pc04"] != domain.ScheduleOutcomeUnconfirmed || !strings.Contains(report.Message, "1 not confirmed") {
		t.Fatalf("report = %+v", report)
	}
	// Requests were sent, so the minute is spent: no try again.
	if lab.store.record.Retry != "" {
		t.Fatalf("record = %+v", lab.store.record)
	}
	lab.now = lab.now.Add(time.Minute)
	if again := lab.runner.Run(scheduleContext, "/lab"); again.State != "idle" || len(lab.power.dispatchHosts) != 2 {
		t.Fatalf("after a partial run = %+v, asked %v", again, lab.power.dispatchHosts)
	}
}

func TestScheduleRunDoesNothingWhenTheMinuteCannotBeClaimed(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.store.failure = errors.New("the classroom state record is busy")
	// Reading still works; claiming does not.
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "blocked" || len(lab.wake.sent) != 0 || len(lab.records.records) != 0 {
		t.Fatalf("report = %+v, %d packets", report, len(lab.wake.sent))
	}
}

func TestScheduleRunKeepsGoingWhenTheHistoryCannotBeSaved(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.records.failure = errors.New("read-only")
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "completed" || len(lab.wake.sent) != 1 || !strings.HasSuffix(report.Message, "Its history record could not be saved.") {
		t.Fatalf("report = %+v", report)
	}
}

func TestScheduleRunRecordsAreAcceptedByTheHistory(t *testing.T) {
	// The history takes counts only, in tokens it can store.
	for _, report := range []domain.ScheduleRunReport{
		{Operation: "schedule-run", State: "completed", Minute: "2026-10-12T07:50", Action: domain.ScheduleActionOn, Targets: []domain.ScheduleOutcome{{Name: "pc01", State: domain.ScheduleOutcomeSent}}},
		{Operation: "schedule-run", State: "partial", Minute: "2026-10-12T18:00", Action: domain.ScheduleActionOff, Targets: []domain.ScheduleOutcome{{Name: "pc01", State: domain.ScheduleOutcomeUnconfirmed}}},
	} {
		sink := &scheduleRecords{}
		if err := RecordOperationOutcome(sink, report); err != nil || len(sink.records) != 1 || sink.records[0].Summary == "" || len(sink.records[0].Summary) > 256 {
			t.Fatalf("%+v: %v, %+v", report, err, sink.records)
		}
	}
}

// The skip-active session policy is the form of the review without a person
// to confirm it: a computer in use, or whose session is unknown, stays out.
func TestShutdownPlanSkipActiveLeavesComputersInUse(t *testing.T) {
	source, manager := shutdownFixture()
	source.meta.Clients.Hosts = append(source.meta.Clients.Hosts, domain.HostMeta{Name: "pc04", IP: "10.0.0.4"})
	reachable := func(session domain.ShutdownSessionState) domain.ShutdownObservation {
		return domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: session}
	}
	source.observations["pc01"] = reachable(domain.ShutdownSessionActive)
	source.observations["pc04"] = reachable(domain.ShutdownSessionUnused)
	plan := manager.Plan(scheduleContext, "/deployment", "@lab", domain.ShutdownSkipActive)
	if plan.HasErrors() || plan.Policy != domain.ShutdownSkipActive || plan.Eligible != 1 || plan.ReviewToken == "" {
		t.Fatalf("plan = %+v", plan)
	}
	eligible := map[string]bool{}
	for _, target := range plan.Targets {
		eligible[target.Name] = target.Eligible
	}
	// pc01 in use, pc02 unknown, pc03 off: out. pc04 logged in unused: in.
	if eligible["pc01"] || eligible["pc02"] || eligible["pc03"] || !eligible["pc04"] {
		t.Fatalf("eligible = %v", eligible)
	}
	// The policy is part of the review token, and the unknown policies stay refused.
	if acknowledged := manager.Plan(scheduleContext, "/deployment", "@lab", domain.ShutdownAcknowledgeUnknown); acknowledged.ReviewToken == plan.ReviewToken {
		t.Fatal("the review token does not bind the policy")
	}
	if invalid := manager.Plan(scheduleContext, "/deployment", "@lab", "ignore"); !invalid.HasErrors() || !strings.Contains(invalid.Message, "skip-active") {
		t.Fatalf("invalid policy = %+v", invalid)
	}
}

// Switching on has no lock to wait for, but a laboratory that cannot be
// read yet is waited for like one that is busy: nothing was sent.
type flakyWakeSource struct {
	*fakeWakeSource
	failing bool
}

func (source *flakyWakeSource) LabMeta(ctx context.Context, repository string) (domain.LabMeta, error) {
	if source.failing {
		return domain.LabMeta{}, errors.New("nix evaluation failed")
	}
	return source.fakeWakeSource.LabMeta(ctx, repository)
}

func TestScheduleRunTriesTheSwitchOnAgainWhenTheInventoryCannotBeRead(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	flaky := &flakyWakeSource{fakeWakeSource: lab.wake, failing: true}
	lab.runner.wake = NewWakeManager(flaky)
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "waiting" || report.Scheduled != "2026-10-12T07:50" || len(lab.wake.sent) != 0 || !strings.HasPrefix(report.Message, "Scheduled switch-on at 07:50 is waiting: ") ||
		!strings.HasSuffix(report.Message, "It is tried again every minute until 08:00.") {
		t.Fatalf("report = %+v", report)
	}
	flaky.failing = false
	lab.now = mondayMorning.Add(2 * time.Minute)
	report = lab.runner.Run(scheduleContext, "/lab")
	if report.State != "completed" || report.Scheduled != "2026-10-12T07:50" || report.Minute != "2026-10-12T07:52" || len(lab.wake.sent) != 1 ||
		!strings.HasPrefix(report.Message, "Scheduled switch-on at 07:50: Wake-on-LAN packets sent to 1 computer") {
		t.Fatalf("second try = %+v, %d packets", report, len(lab.wake.sent))
	}
	if len(lab.records.records) != 1 || lab.records.records[0].Subject != "scheduled switch-on at 07:50" || lab.store.record.Retry != "" {
		t.Fatalf("history = %+v, record %+v", lab.records.records, lab.store.record)
	}
}

// A shut-down that cannot start has sent nothing, so it does not count as
// run: it waits, and the next minutes try again.
func TestScheduleRunTriesAgainEveryMinuteWhileAnotherClientOperationRuns(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.power.active = true
	for minute := 0; minute <= 4; minute++ {
		lab.now = mondayEvening.Add(time.Duration(minute) * time.Minute)
		report := lab.runner.Run(scheduleContext, "/lab")
		if report.State != "waiting" || report.HasErrors() || report.Action != domain.ScheduleActionOff || report.Scheduled != "2026-10-12T18:00" ||
			len(lab.power.dispatchHosts) != 0 || !strings.HasPrefix(report.Message, "Scheduled shut-down at 18:00 is waiting: another client") ||
			!strings.HasSuffix(report.Message, "It is tried again every minute until 18:10.") {
			t.Fatalf("minute %d: %+v", minute, report)
		}
	}
	// Waiting leaves no history line, and keeps the action for the next try.
	if len(lab.records.records) != 0 || lab.store.record.Retry != "2026-10-12T18:00" || lab.store.record.LastRun != "2026-10-12T18:00" {
		t.Fatalf("history = %+v, record %+v", lab.records.records, lab.store.record)
	}
	// The operation ends: the next minute shuts the computers down, once.
	lab.power.active = false
	lab.now = mondayEvening.Add(5 * time.Minute)
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "completed" || report.Scheduled != "2026-10-12T18:00" || report.Minute != "2026-10-12T18:05" || len(lab.power.dispatchHosts) != 2 ||
		!strings.HasPrefix(report.Message, "Scheduled shut-down at 18:00: 2 computers asked to shut down") {
		t.Fatalf("report = %+v, asked %v", report, lab.power.dispatchHosts)
	}
	if len(lab.records.records) != 1 || lab.records.records[0].State != "completed" || lab.records.records[0].Subject != "scheduled shut-down at 18:00" || lab.store.record.Retry != "" {
		t.Fatalf("history = %+v, record %+v", lab.records.records, lab.store.record)
	}
	for minute := 6; minute <= 12; minute++ {
		lab.now = mondayEvening.Add(time.Duration(minute) * time.Minute)
		if again := lab.runner.Run(scheduleContext, "/lab"); again.State != "idle" || len(lab.power.dispatchHosts) != 2 || len(lab.records.records) != 1 {
			t.Fatalf("minute %d after it ran: %+v", minute, again)
		}
	}
}

func TestScheduleRunWaitsWhileANetworkInstallationIsActive(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.now = mondayEvening
	lab.power.listener.Active = true
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "waiting" || !strings.Contains(report.Message, "installation") || len(lab.power.dispatchHosts) != 0 {
		t.Fatalf("report = %+v", report)
	}
	lab.power.listener.Active = false
	lab.now = mondayEvening.Add(time.Minute)
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "completed" || len(lab.power.dispatchHosts) != 2 {
		t.Fatalf("after the installation = %+v", report)
	}
}

// Ten minutes after the set time, the last try that cannot start is the end:
// one history line, no more tries that day, and the next day runs as usual.
func TestScheduleRunGivesUpTenMinutesAfterTheSetTime(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.power.active = true
	for minute := 0; minute < domain.ScheduleRetryMinutes; minute++ {
		lab.now = mondayEvening.Add(time.Duration(minute) * time.Minute)
		if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "waiting" {
			t.Fatalf("minute %d: %+v", minute, report)
		}
	}
	lab.now = mondayEvening.Add(10 * time.Minute)
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "gave-up" || !report.HasErrors() || report.Scheduled != "2026-10-12T18:00" ||
		!strings.HasPrefix(report.Message, "Scheduled shut-down at 18:00 was given up after 10 minutes: another client") {
		t.Fatalf("report = %+v", report)
	}
	if len(lab.records.records) != 1 || lab.records.records[0].State != "gave-up" || lab.records.records[0].Subject != "scheduled shut-down at 18:00" ||
		lab.records.records[0].Summary != "nothing was sent within 10 minutes; see the journal of the schedule service" || lab.store.record.Retry != "" {
		t.Fatalf("history = %+v, record %+v", lab.records.records, lab.store.record)
	}
	// Nothing more that day, even once the lock is free.
	lab.power.active = false
	for minute := 11; minute <= 30; minute++ {
		lab.now = mondayEvening.Add(time.Duration(minute) * time.Minute)
		if again := lab.runner.Run(scheduleContext, "/lab"); again.State != "idle" || len(lab.power.dispatchHosts) != 0 || len(lab.records.records) != 1 {
			t.Fatalf("minute %d after giving up: %+v", minute, again)
		}
	}
	// The next day shuts down as usual.
	lab.now = mondayEvening.AddDate(0, 0, 1)
	if next := lab.runner.Run(scheduleContext, "/lab"); next.State != "completed" || next.Scheduled != "2026-10-13T18:00" || len(lab.power.dispatchHosts) != 2 {
		t.Fatalf("next day = %+v", next)
	}
}

// When the controller's timer did not run during the last minutes of the
// window, the next run gives up without trying: it is too late.
func TestScheduleRunGivesUpWhenTheTriesRanOutBetweenRuns(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.power.active = true
	lab.now = mondayEvening
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "waiting" {
		t.Fatalf("first try = %+v", report)
	}
	lab.power.active = false
	lab.now = mondayEvening.Add(15 * time.Minute)
	report := lab.runner.Run(scheduleContext, "/lab")
	if report.State != "gave-up" || len(lab.power.dispatchHosts) != 0 || report.Message != "Scheduled shut-down at 18:00 was given up: it could not start within 10 minutes." {
		t.Fatalf("report = %+v", report)
	}
	if len(lab.records.records) != 1 || lab.records.records[0].State != "gave-up" {
		t.Fatalf("history = %+v", lab.records.records)
	}
	lab.now = lab.now.Add(time.Minute)
	if again := lab.runner.Run(scheduleContext, "/lab"); again.State != "idle" || len(lab.records.records) != 1 {
		t.Fatalf("after giving up = %+v", again)
	}
}

func TestAChangedScheduleDropsAnActionThatWasWaiting(t *testing.T) {
	lab := scheduleFixture(schoolSchedule)
	lab.power.active = true
	lab.now = mondayEvening
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "waiting" || lab.store.record.Retry == "" {
		t.Fatalf("first try = %+v", report)
	}
	lab.runner.schedule.now = func() time.Time { return lab.now }
	if saved := lab.runner.schedule.Set(domain.Schedule{On: "07:50", Off: "19:00", Days: schoolDays}); saved.HasErrors() || lab.store.record.Retry != "" {
		t.Fatalf("saved = %+v, record %+v", saved, lab.store.record)
	}
	lab.power.active = false
	lab.now = mondayEvening.Add(time.Minute)
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "idle" || len(lab.power.dispatchHosts) != 0 {
		t.Fatalf("after the change = %+v", report)
	}
	// A marker that names a minute the schedule no longer sets is ignored too.
	lab.store.record.Retry = "2026-10-12T18:00"
	lab.now = mondayEvening.Add(2 * time.Minute)
	if report := lab.runner.Run(scheduleContext, "/lab"); report.State != "idle" || len(lab.power.dispatchHosts) != 0 {
		t.Fatalf("stale marker = %+v", report)
	}
}

func TestScheduleNextFindsTheOccurrenceToTry(t *testing.T) {
	record := func(schedule domain.Schedule, retry string) domain.ScheduleRecord {
		return domain.ScheduleRecord{SchemaVersion: 1, Schedule: schedule, LastRun: retry, Retry: retry}
	}
	night := domain.Schedule{Off: "23:55", Days: []string{"mon", "fri"}}
	for name, test := range map[string]struct {
		record domain.ScheduleRecord
		now    time.Time
		want   scheduleAttempt
		found  bool
	}{
		"due now":     {domain.ScheduleRecord{Schedule: schoolSchedule}, mondayEvening, scheduleAttempt{scheduled: "2026-10-12T18:00", action: domain.ScheduleActionOff, until: "18:10"}, true},
		"already ran": {domain.ScheduleRecord{Schedule: schoolSchedule, LastRun: "2026-10-12T18:00"}, mondayEvening, scheduleAttempt{}, false},
		"not due":     {domain.ScheduleRecord{Schedule: schoolSchedule}, mondayEvening.Add(time.Minute), scheduleAttempt{}, false},
		"retry, a few minutes on": {record(schoolSchedule, "2026-10-12T18:00"), mondayEvening.Add(3 * time.Minute),
			scheduleAttempt{scheduled: "2026-10-12T18:00", action: domain.ScheduleActionOff, until: "18:10"}, true},
		"retry, last minute": {record(schoolSchedule, "2026-10-12T18:00"), mondayEvening.Add(10 * time.Minute),
			scheduleAttempt{scheduled: "2026-10-12T18:00", action: domain.ScheduleActionOff, until: "18:10", last: true}, true},
		"retry, too late": {record(schoolSchedule, "2026-10-12T18:00"), mondayEvening.Add(11 * time.Minute),
			scheduleAttempt{scheduled: "2026-10-12T18:00", action: domain.ScheduleActionOff, until: "18:10", last: true, expired: true}, true},
		// The window runs over midnight, and the weekday is the one of the set time.
		"retry over midnight": {record(night, "2026-10-12T23:55"), time.Date(2026, 10, 13, 0, 3, 0, 0, cest),
			scheduleAttempt{scheduled: "2026-10-12T23:55", action: domain.ScheduleActionOff, until: "00:05"}, true},
		"retry over midnight into the weekend": {record(night, "2026-10-16T23:55"), time.Date(2026, 10, 17, 0, 5, 0, 0, cest),
			scheduleAttempt{scheduled: "2026-10-16T23:55", action: domain.ScheduleActionOff, until: "00:05", last: true, expired: false}, true},
		"retry of a minute the schedule does not set": {record(schoolSchedule, "2026-10-12T17:00"), mondayEvening.Add(time.Minute), scheduleAttempt{}, false},
		"retry of a day the schedule does not list":   {record(domain.Schedule{Off: "18:00", Days: []string{"tue"}}, "2026-10-12T18:00"), mondayEvening.Add(time.Minute), scheduleAttempt{}, false},
		"no schedule": {domain.ScheduleRecord{Retry: "2026-10-12T18:00"}, mondayEvening, scheduleAttempt{}, false},
	} {
		got, found := scheduleNext(test.record, test.now)
		if found != test.found || got != test.want {
			t.Errorf("%s: got %+v (%v), want %+v (%v)", name, got, found, test.want, test.found)
		}
	}
}
