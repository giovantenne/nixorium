package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type shutdownLease struct{}

func (shutdownLease) Close() error { return nil }

type fakeShutdownSource struct {
	meta          domain.LabMeta
	listener      domain.ServiceState
	network       domain.ServiceState
	active        bool
	activeErr     error
	acquireErr    error
	observations  map[string]domain.ShutdownObservation
	dispatch      map[string]domain.ShutdownDispatchResult
	dispatchHosts []domain.HostMeta
}

func (f *fakeShutdownSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}
func (f *fakeShutdownSource) ServiceState(_ context.Context, name string) domain.ServiceState {
	if name == PXEListenerUnit {
		return f.listener
	}
	return f.network
}
func (f *fakeShutdownSource) ClientOperationActive() (bool, error) { return f.active, f.activeErr }
func (f *fakeShutdownSource) AcquireClientOperation() (io.Closer, error) {
	return shutdownLease{}, f.acquireErr
}
func (f *fakeShutdownSource) ShutdownObservations(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.ShutdownObservation {
	result := map[string]domain.ShutdownObservation{}
	for _, host := range hosts {
		if observation, found := f.observations[host.Name]; found {
			result[host.Name] = observation
		}
	}
	return result
}
func (f *fakeShutdownSource) DispatchShutdowns(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.ShutdownDispatchResult {
	f.dispatchHosts = append([]domain.HostMeta(nil), hosts...)
	return f.dispatch
}

func shutdownFixture() (*fakeShutdownSource, *ShutdownManager) {
	source := &fakeShutdownSource{
		listener: domain.ServiceState{Loaded: true, State: "inactive"},
		network:  domain.ServiceState{Loaded: true, State: "inactive"},
		observations: map[string]domain.ShutdownObservation{
			"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionIdle},
			"pc02": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnknown, Detail: "session helper unavailable"},
			"pc03": {Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Session: domain.ShutdownSessionUnknown},
		},
		dispatch: map[string]domain.ShutdownDispatchResult{
			"pc01": {Accepted: true},
			"pc02": {Accepted: true},
		},
	}
	source.meta.Controller.Name = "pc99"
	source.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}, {Name: "pc03", IP: "10.0.0.3"}}
	manager := NewShutdownManager(source)
	manager.now = func() time.Time { return time.Unix(1000, 0).UTC() }
	return source, manager
}

func TestShutdownPlanIncludesActiveSessionsAndProtectsUnknownSessions(t *testing.T) {
	source, manager := shutdownFixture()
	source.observations["pc01"] = domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive}
	plan := manager.Plan(context.Background(), "/deployment", "pc01,pc02,pc03", domain.ShutdownProtectUnknown)
	if plan.HasErrors() || plan.State != "ready" || plan.Eligible != 1 || len(plan.Targets) != 3 || plan.Confirmation != "SHUTDOWN" {
		t.Fatalf("plan = %+v", plan)
	}
	if !plan.Targets[0].Eligible || plan.Targets[0].Session != domain.ShutdownSessionActive || !strings.Contains(plan.Targets[0].Detail, "unsaved work") || plan.Targets[1].Eligible || !strings.Contains(plan.Targets[1].Detail, "session helper") || plan.Targets[2].Eligible {
		t.Fatalf("target eligibility = %+v", plan.Targets)
	}
	acknowledged := manager.Plan(context.Background(), "/deployment", "pc01,pc02,pc03", domain.ShutdownAcknowledgeUnknown)
	if acknowledged.HasErrors() || acknowledged.Eligible != 2 || !acknowledged.Targets[1].Eligible || acknowledged.Targets[2].Eligible {
		t.Fatalf("acknowledged plan = %+v", acknowledged)
	}
}

func TestShutdownPlanRejectsControllerUnknownDuplicateAndConflicts(t *testing.T) {
	for _, requested := range []string{"pc99", "pc04", "pc01,pc01", ""} {
		_, manager := shutdownFixture()
		if report := manager.Plan(context.Background(), "/deployment", requested, domain.ShutdownProtectUnknown); !report.HasErrors() {
			t.Fatalf("unsafe targets %q accepted: %+v", requested, report)
		}
	}
	source, manager := shutdownFixture()
	source.active = true
	if report := manager.Plan(context.Background(), "/deployment", "pc01", domain.ShutdownProtectUnknown); !report.HasErrors() || !strings.Contains(report.Message, "already running") {
		t.Fatalf("active operation accepted: %+v", report)
	}
	source, manager = shutdownFixture()
	source.listener.Active = true
	if report := manager.Plan(context.Background(), "/deployment", "pc01", domain.ShutdownProtectUnknown); !report.HasErrors() || !strings.Contains(report.Message, "installation") {
		t.Fatalf("active installation accepted: %+v", report)
	}
}

func TestShutdownApplyRechecksAndReportsPerTargetOutcomes(t *testing.T) {
	source, manager := shutdownFixture()
	plan := manager.Plan(context.Background(), "/deployment", "@lab", domain.ShutdownAcknowledgeUnknown)
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if report.State != "partial" || report.Accepted != 2 || report.NotSent != 1 || report.Unconfirmed != 0 || len(source.dispatchHosts) != 2 || report.RetrySafe {
		t.Fatalf("apply = %+v dispatched=%+v", report, source.dispatchHosts)
	}
	if report.Targets[0].State != "accepted" && report.Targets[1].State != "accepted" {
		t.Fatalf("outcomes = %+v", report.Targets)
	}
}

func TestShutdownApplyBlocksStaleExpiredAndChangedPlansBeforeDispatch(t *testing.T) {
	for _, mutate := range []func(*fakeShutdownSource, *ShutdownManager, *domain.ShutdownPlanReport, *string){
		func(_ *fakeShutdownSource, _ *ShutdownManager, _ *domain.ShutdownPlanReport, token *string) {
			*token = "sha256:wrong"
		},
		func(_ *fakeShutdownSource, manager *ShutdownManager, plan *domain.ShutdownPlanReport, _ *string) {
			manager.now = func() time.Time { return plan.ExpiresAt }
		},
		func(source *fakeShutdownSource, _ *ShutdownManager, _ *domain.ShutdownPlanReport, _ *string) {
			source.meta.Clients.Hosts[0].IP = "10.0.0.9"
		},
		func(source *fakeShutdownSource, _ *ShutdownManager, _ *domain.ShutdownPlanReport, _ *string) {
			source.acquireErr = errors.New("operation locked")
		},
		func(source *fakeShutdownSource, _ *ShutdownManager, _ *domain.ShutdownPlanReport, _ *string) {
			source.observations["pc01"] = domain.ShutdownObservation{Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnavailable, Session: domain.ShutdownSessionUnknown}
		},
		func(source *fakeShutdownSource, _ *ShutdownManager, _ *domain.ShutdownPlanReport, _ *string) {
			source.listener.Active = true
		},
	} {
		source, manager := shutdownFixture()
		plan := manager.Plan(context.Background(), "/deployment", "pc01", domain.ShutdownProtectUnknown)
		token := plan.ReviewToken
		mutate(source, manager, &plan, &token)
		report := manager.ApplyPlan(context.Background(), plan, token)
		if !report.HasErrors() || len(source.dispatchHosts) != 0 || !report.RetrySafe {
			t.Fatalf("unsafe plan dispatched: %+v hosts=%+v", report, source.dispatchHosts)
		}
	}
}

func TestShutdownApplyDispatchesWhenSessionBecomesActive(t *testing.T) {
	source, manager := shutdownFixture()
	plan := manager.Plan(context.Background(), "/deployment", "pc01", domain.ShutdownProtectUnknown)
	source.observations["pc01"] = domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive}
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if report.HasErrors() || report.Accepted != 1 || len(source.dispatchHosts) != 1 || source.dispatchHosts[0].Name != "pc01" {
		t.Fatalf("active session was not dispatched: report=%+v hosts=%+v", report, source.dispatchHosts)
	}
}

func TestShutdownApplyDoesNotRetryBlindlyAfterUnconfirmedDispatch(t *testing.T) {
	source, manager := shutdownFixture()
	source.dispatch["pc01"] = domain.ShutdownDispatchResult{Detail: "SSH connection closed during request"}
	plan := manager.Plan(context.Background(), "/deployment", "pc01", domain.ShutdownProtectUnknown)
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if report.State != "partial" || report.Unconfirmed != 1 || report.RetrySafe || !strings.Contains(report.Message, "Do not retry blindly") || !strings.Contains(report.Targets[0].TechnicalDetail, "connection closed") || strings.Contains(report.Targets[0].Detail, "SSH") {
		t.Fatalf("unconfirmed result = %+v", report)
	}
}
