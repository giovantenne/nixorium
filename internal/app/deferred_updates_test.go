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
	manager := NewDeferredUpdateManager(store, source, func(_ context.Context, _, host, revision string) (domain.DeploymentExecutionReport, bool) {
		deployed = append(deployed, host+"@"+revision[:4])
		return domain.DeploymentExecutionReport{State: "completed"}, false
	})
	manager.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	return manager, store, source, &deployed
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
	manager.deploy = func(context.Context, string, string, string) (domain.DeploymentExecutionReport, bool) {
		return domain.DeploymentExecutionReport{}, true
	}
	if report := manager.Run(context.Background(), "/deployment"); report.Results[0].State != "busy" || store.queue.Updates[0].Attempts != 0 {
		t.Fatalf("busy pass = %+v / %+v", report.Results, store.queue.Updates)
	}
	manager.deploy = func(context.Context, string, string, string) (domain.DeploymentExecutionReport, bool) {
		return domain.DeploymentExecutionReport{State: "failed", Message: "build failed"}, false
	}
	manager.Run(context.Background(), "/deployment")
	if update := store.queue.Updates[0]; update.Attempts != 1 || update.LastError != "build failed" {
		t.Fatalf("failure not recorded: %+v", update)
	}
	// Backoff: the next pass a few seconds later does not try again.
	calls := 0
	manager.deploy = func(context.Context, string, string, string) (domain.DeploymentExecutionReport, bool) {
		calls++
		return domain.DeploymentExecutionReport{State: "completed"}, false
	}
	if manager.Run(context.Background(), "/deployment"); calls != 0 {
		t.Fatal("failed entry retried before its backoff")
	}
	if removed, err := manager.Cancel([]string{"@all"}); err != nil || removed != 1 || len(store.queue.Updates) != 0 {
		t.Fatalf("cancel = %d, %v", removed, err)
	}
}
