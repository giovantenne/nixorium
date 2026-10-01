package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeCleanupSource struct {
	meta         domain.LabMeta
	pxeActive    bool
	busy         bool
	controller   []domain.CleanupObservation
	clients      map[string]domain.CleanupObservation
	results      map[string]domain.CleanupDispatchResult
	controllerOK error
	cleanedWith  []string
	sentExpect   map[string]string
	leaseHeld    bool
	unitWithLock bool
}

type fakeLease struct{ source *fakeCleanupSource }

func (lease fakeLease) Close() error { lease.source.leaseHeld = false; return nil }

func (source *fakeCleanupSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return source.meta, nil
}

func (source *fakeCleanupSource) ServiceState(_ context.Context, unit string) domain.ServiceState {
	return domain.ServiceState{Name: unit, Loaded: true, Active: source.pxeActive}
}

func (source *fakeCleanupSource) ClientOperationActive() (bool, error) { return source.busy, nil }

func (source *fakeCleanupSource) AcquireClientOperation() (io.Closer, error) {
	if source.busy {
		return nil, errors.New("busy")
	}
	source.leaseHeld = true
	return fakeLease{source}, nil
}

func (source *fakeCleanupSource) ObserveControllerGenerations(context.Context) domain.CleanupObservation {
	observation := source.controller[0]
	if len(source.controller) > 1 {
		source.controller = source.controller[1:]
	}
	return observation
}

func (source *fakeCleanupSource) ObserveClientGenerations(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.CleanupObservation {
	result := map[string]domain.CleanupObservation{}
	for _, host := range hosts {
		result[host.Name] = source.clients[host.Name]
	}
	return result
}

func (source *fakeCleanupSource) CleanControllerGenerations(_ context.Context, expect string) error {
	source.cleanedWith = append(source.cleanedWith, expect)
	source.unitWithLock = source.leaseHeld
	return source.controllerOK
}

func (source *fakeCleanupSource) CleanClientGenerations(_ context.Context, expect map[string]string, hosts []domain.HostMeta, _ time.Duration) map[string]domain.CleanupDispatchResult {
	source.sentExpect = expect
	return source.results
}

func cleanupMeta() domain.LabMeta {
	meta := domain.LabMeta{}
	meta.Controller.Name = "pc99"
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}, {Name: "pc03", IP: "10.0.0.3"}}
	return meta
}

func observed(expect string, remove ...int) domain.CleanupObservation {
	return domain.CleanupObservation{
		Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Valid: true,
		Keep: []domain.CleanupGeneration{{Number: 20, Reason: "newest"}}, Remove: remove, FreeBytes: 1 << 30, Expect: expect,
	}
}

func TestCleanupPlanReviewsOnlyComputersWithSomethingToRemove(t *testing.T) {
	source := &fakeCleanupSource{
		meta:       cleanupMeta(),
		controller: []domain.CleanupObservation{observed("aaaaaaaaaaaaaaaa", 3, 4)},
		clients: map[string]domain.CleanupObservation{
			"pc01": observed("bbbbbbbbbbbbbbbb", 1),
			"pc02": observed("cccccccccccccccc"),
			"pc03": {Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnavailable},
		},
	}
	manager := NewCleanupManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "controller,@lab")
	if plan.State != "ready" || plan.Eligible != 2 || plan.Confirmation != "CLEAN" || plan.KeepGenerations != 10 || plan.Requested != "controller,@lab" {
		t.Fatalf("plan = %+v", plan)
	}
	if !plan.Targets[0].Controller || plan.Targets[2].Eligible || plan.Targets[3].Eligible || plan.Targets[3].Detail == "" {
		t.Fatalf("targets = %+v", plan.Targets)
	}
	source.controller = []domain.CleanupObservation{observed("aaaaaaaaaaaaaaaa", 3, 4), observed("dddddddddddddddd")}
	source.results = map[string]domain.CleanupDispatchResult{"pc01": {State: "cleaned", Removed: []int{1}, FreeBefore: 1, FreeAfter: 2}}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if report.State != "partial" || report.Cleaned != 2 || report.NotSent != 2 || report.Unconfirmed != 0 {
		t.Fatalf("report = %+v", report)
	}
	if len(source.cleanedWith) != 1 || source.cleanedWith[0] != "aaaaaaaaaaaaaaaa" || source.unitWithLock {
		t.Fatalf("controller unit = %v, ran with lock %v", source.cleanedWith, source.unitWithLock)
	}
	if len(source.sentExpect) != 1 || source.sentExpect["pc01"] != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("clients sent = %v", source.sentExpect)
	}
}

func TestCleanupApplyRefusesStaleOrChangedReviews(t *testing.T) {
	source := &fakeCleanupSource{meta: cleanupMeta(), controller: []domain.CleanupObservation{observed("aaaaaaaaaaaaaaaa", 3)}}
	manager := NewCleanupManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "controller")
	if report := manager.Apply(context.Background(), plan, "sha256:other"); report.State != "blocked" || len(report.Issues) == 0 {
		t.Fatalf("wrong token accepted: %+v", report)
	}
	manager.now = func() time.Time { return time.Now().Add(time.Hour) }
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" || source.cleanedWith != nil {
		t.Fatalf("expired review accepted: %+v", report)
	}
	manager.now = time.Now
	source.controller = []domain.CleanupObservation{observed("eeeeeeeeeeeeeeee", 3, 5)}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if report.State != "blocked" || report.NotSent != 1 || source.cleanedWith != nil {
		t.Fatalf("changed controller cleaned: %+v", report)
	}
	source.pxeActive = true
	if plan := manager.Plan(context.Background(), "/srv/lab", "controller"); plan.State != "blocked" {
		t.Fatalf("cleanup planned during network installation: %+v", plan)
	}
}

func TestCleanupSelectionAndResults(t *testing.T) {
	for requested, wantIssue := range map[string]bool{"pc99": false, "pc01,pc01": true, "pc42": true, "": true, "controller,pc99": true} {
		_, _, _, issues := selectCleanupTargets(cleanupMeta(), requested)
		if (len(issues) > 0) != wantIssue {
			t.Fatalf("%q issues = %v", requested, issues)
		}
	}
	if outcome := cleanupOutcome("pc01", domain.CleanupDispatchResult{State: "changed"}); outcome.State != "not-sent" {
		t.Fatalf("changed computer = %+v", outcome)
	}
	if outcome := cleanupOutcome("pc01", domain.CleanupDispatchResult{State: "failed", Detail: "timeout"}); outcome.State != "unconfirmed" || outcome.TechnicalDetail != "timeout" {
		t.Fatalf("failed computer = %+v", outcome)
	}
	if outcome := cleanupOutcome("pc01", domain.CleanupDispatchResult{State: "cleaned", BootMenuFailed: true}); outcome.State != "cleaned" || outcome.Detail == "old versions removed" {
		t.Fatalf("boot menu failure hidden: %+v", outcome)
	}
}
