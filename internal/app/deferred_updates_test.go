package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type memoryDeferredStore struct {
	queue domain.DeferredUpdateQueue
	busy  bool
}

func (s *memoryDeferredStore) Read() (domain.DeferredUpdateQueue, error) {
	queue := s.queue
	queue.Updates = append([]domain.DeferredUpdate{}, s.queue.Updates...)
	return queue, nil
}

func (s *memoryDeferredStore) Mutate(change func(*domain.DeferredUpdateQueue) error) error {
	if s.busy {
		return errors.New("another operation is running")
	}
	queue, _ := s.Read()
	if err := change(&queue); err != nil {
		return err
	}
	if err := queue.Validate(); err != nil {
		return err
	}
	s.queue = queue
	return nil
}

func deferredManagerFixture(t *testing.T) (*DeferredUpdateManager, *memoryDeferredStore, *fakeDeploymentSource, *[]string) {
	t.Helper()
	source := readyDeploymentSource()
	source.revision = strings.Repeat("a", 40)
	store := &memoryDeferredStore{queue: domain.DeferredUpdateQueue{SchemaVersion: domain.DeferredUpdateSchemaVersion}}
	deployed := []string{}
	manager := NewDeferredUpdateManager(store, source, func(_ context.Context, _ string, hosts []string, revision string) (domain.DeploymentExecutionReport, bool) {
		deployed = append(deployed, strings.Join(hosts, "+")+"@"+revision[:4])
		return verifiedDeferredReport(hosts), false
	})
	manager.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	return manager, store, source, &deployed
}

// verifiedDeferredReport is the result of an update that verified every
// named computer.
func verifiedDeferredReport(hosts []string) domain.DeploymentExecutionReport {
	report := domain.DeploymentExecutionReport{State: "completed", ApplyCompleted: true}
	for _, host := range hosts {
		report.Verification.Targets = append(report.Verification.Targets, domain.DeploymentTargetVerification{Name: host, State: "verified"})
	}
	return report
}

func TestDeferredUpdatesQueueOnlyReviewedComputers(t *testing.T) {
	manager, store, source, _ := deferredManagerFixture(t)
	plan := domain.DeploymentPlanReport{Revision: source.revision, Targets: []domain.DeploymentTarget{{Name: "pc02", IP: "10.0.0.2"}}}
	if err := manager.Queue(plan, []string{"pc03"}); err == nil {
		t.Fatal("computer outside the review was queued")
	}
	if err := manager.Queue(domain.DeploymentPlanReport{Revision: source.revision, Issues: []domain.ValidationIssue{{Message: "dirty"}}}, nil); err == nil {
		t.Fatal("blocked review queued computers")
	}
	if err := manager.Queue(plan, []string{"pc02"}); err != nil || len(store.queue.Updates) != 1 || store.queue.Updates[0].IP != "10.0.0.2" {
		t.Fatalf("queue = %+v, %v", store.queue.Updates, err)
	}
}

func TestDeferredUpdatesRunAppliesReachableCurrentEntriesOnly(t *testing.T) {
	manager, store, source, deployed := deferredManagerFixture(t)
	queuedAt := time.Unix(1_799_999_000, 0).UTC()
	stale := domain.DeferredUpdate{Host: "pc01", IP: "10.0.0.1", Revision: strings.Repeat("b", 40), QueuedAt: queuedAt}
	off := domain.DeferredUpdate{Host: "pc02", IP: "10.0.0.2", Revision: source.revision, QueuedAt: queuedAt}
	on := domain.DeferredUpdate{Host: "pc03", IP: "10.0.0.3", Revision: source.revision, QueuedAt: queuedAt}
	for _, update := range []domain.DeferredUpdate{stale, off, on} {
		store.queue.Put(update)
	}
	source.ssh = map[string]domain.SSHProbe{"pc03": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}}
	report := manager.Run(context.Background(), "/deployment")
	if strings.Join(*deployed, ",") != "pc03@aaaa" {
		t.Fatalf("deployed %v", *deployed)
	}
	states := map[string]string{}
	for _, result := range report.Results {
		states[result.Host] = result.State
	}
	if states["pc01"] != "stale" || states["pc02"] != "waiting" || states["pc03"] != "updated" {
		t.Fatalf("results = %+v", report.Results)
	}
	if _, found := store.queue.Find("pc03"); found || len(store.queue.Updates) != 2 {
		t.Fatalf("queue after run = %+v", store.queue.Updates)
	}
	status := manager.Status(context.Background(), "/deployment")
	if status.Waiting() != 1 || !strings.Contains(status.Message, "need a new review") {
		t.Fatalf("status = %+v", status)
	}
}

func TestDeferredUpdatesRecordFailuresAndWaitWhenBusy(t *testing.T) {
	manager, store, source, _ := deferredManagerFixture(t)
	store.queue.Put(domain.DeferredUpdate{Host: "pc01", IP: "10.0.0.1", Revision: source.revision, QueuedAt: time.Unix(1_799_999_000, 0).UTC()})
	manager.deploy = func(context.Context, string, []string, string) (domain.DeploymentExecutionReport, bool) {
		return domain.DeploymentExecutionReport{}, true
	}
	if report := manager.Run(context.Background(), "/deployment"); report.Results[0].State != "busy" || store.queue.Updates[0].Attempts != 0 {
		t.Fatalf("busy pass = %+v / %+v", report.Results, store.queue.Updates)
	}
	manager.deploy = func(context.Context, string, []string, string) (domain.DeploymentExecutionReport, bool) {
		return domain.DeploymentExecutionReport{State: "failed", Message: "build failed"}, false
	}
	manager.Run(context.Background(), "/deployment")
	if update := store.queue.Updates[0]; update.Attempts != 1 || update.LastError != "build failed" {
		t.Fatalf("failure not recorded: %+v", update)
	}
	// Backoff: the next pass a few seconds later does not try again.
	calls := 0
	manager.deploy = func(_ context.Context, _ string, hosts []string, _ string) (domain.DeploymentExecutionReport, bool) {
		calls++
		return verifiedDeferredReport(hosts), false
	}
	if manager.Run(context.Background(), "/deployment"); calls != 0 {
		t.Fatal("failed entry retried before its backoff")
	}
	if removed, err := manager.Cancel([]string{"@all"}); err != nil || removed != 1 || len(store.queue.Updates) != 0 {
		t.Fatalf("cancel = %d, %v", removed, err)
	}
}

func TestDeferredUpdatesRunUpdatesAnsweringComputersTogether(t *testing.T) {
	manager, store, source, deployed := deferredManagerFixture(t)
	queuedAt := time.Unix(1_799_999_000, 0).UTC()
	for _, host := range []string{"pc01", "pc02", "pc03", "pc04"} {
		store.queue.Put(domain.DeferredUpdate{Host: host, IP: "10.0.0." + host[3:], Revision: source.revision, QueuedAt: queuedAt})
	}
	answer := domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
	source.ssh = map[string]domain.SSHProbe{"pc01": answer, "pc02": answer, "pc03": answer, "pc04": answer}
	// pc04 already runs the queued revision: it leaves the queue unchanged.
	source.current = map[string]domain.HostSystemProbe{"pc04": {Revision: source.revision, SystemPath: "/nix/store/current"}}
	manager.deploy = func(_ context.Context, _ string, hosts []string, revision string) (domain.DeploymentExecutionReport, bool) {
		*deployed = append(*deployed, strings.Join(hosts, "+")+"@"+revision[:4])
		report := verifiedDeferredReport([]string{"pc01"})
		report.State = "partial"
		report.Skipped = []string{"pc02"}
		report.Verification.Targets = append(report.Verification.Targets, domain.DeploymentTargetVerification{Name: "pc03", State: "unverified", Detail: "authenticated host revision differs"})
		return report, false
	}
	report := manager.Run(context.Background(), "/deployment")
	if strings.Join(*deployed, ",") != "pc01+pc02+pc03@aaaa" {
		t.Fatalf("one update for the answering computers, got %v", *deployed)
	}
	states := map[string]string{}
	for _, result := range report.Results {
		states[result.Host] = result.State
	}
	if states["pc01"] != "updated" || states["pc02"] != "waiting" || states["pc03"] != "failed" || states["pc04"] != "updated" {
		t.Fatalf("results = %+v", report.Results)
	}
	if _, found := store.queue.Find("pc01"); found {
		t.Fatal("verified computer kept in the queue")
	}
	if _, found := store.queue.Find("pc04"); found {
		t.Fatal("computer already current kept in the queue")
	}
	if entry, _ := store.queue.Find("pc02"); entry.Attempts != 0 {
		t.Fatalf("computer that stopped answering counted as a failure: %+v", entry)
	}
	if entry, _ := store.queue.Find("pc03"); entry.Attempts != 1 || !strings.Contains(entry.LastError, "differs") {
		t.Fatalf("failure not recorded: %+v", entry)
	}
}

type busyDeferredSource struct {
	*fakeDeploymentSource
	active bool
	err    error
}

func (s busyDeferredSource) ClientOperationActive() (bool, error) { return s.active, s.err }

func TestDeferredUpdatesRunStopsWhenTheControllerIsBusy(t *testing.T) {
	for _, test := range []struct {
		name   string
		source func(*fakeDeploymentSource) busyDeferredSource
		detail string
	}{
		{name: "busy", source: func(f *fakeDeploymentSource) busyDeferredSource { return busyDeferredSource{f, true, nil} }, detail: "another operation"},
		{name: "pending update", source: func(f *fakeDeploymentSource) busyDeferredSource {
			return busyDeferredSource{f, true, errors.New("an interrupted client update blocks new operations")}
		}, detail: "interrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, store, source, _ := deferredManagerFixture(t)
			manager.source = test.source(source)
			store.queue.Put(domain.DeferredUpdate{Host: "pc01", IP: "10.0.0.1", Revision: source.revision, QueuedAt: time.Unix(1_799_999_000, 0).UTC()})
			source.ssh = map[string]domain.SSHProbe{"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}}
			calls := 0
			manager.deploy = func(context.Context, string, []string, string) (domain.DeploymentExecutionReport, bool) {
				calls++
				return domain.DeploymentExecutionReport{}, false
			}
			report := manager.Run(context.Background(), "/deployment")
			if calls != 0 || report.Results[0].State != "busy" || !strings.Contains(report.Results[0].Detail, test.detail) {
				t.Fatalf("calls %d, results %+v", calls, report.Results)
			}
			if store.queue.Updates[0].Attempts != 0 {
				t.Fatal("a busy controller was recorded as a failure")
			}
		})
	}
}
