package app

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDeploymentPlanAvailabilityAndExplicitSubset(t *testing.T) {
	source := readyDeploymentSource()
	source.ssh = map[string]domain.SSHProbe{
		"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
		"pc02": {Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown},
		"pc03": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHUnavailable},
		"pc99": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
	}
	manager := NewDeploymentManager(source)
	plan := manager.Plan(context.Background(), "/deployment", "pc02,pc01,pc03")
	if plan.HasErrors() || len(plan.Availability) != 3 || plan.ReachableRequested != "pc01" || plan.ColmenaSelector != "pc02,pc01,pc03" {
		t.Fatalf("plan: %+v", plan)
	}
	if len(source.probeHosts) != 3 || source.probeHosts[0].Name != "pc02" {
		t.Fatalf("unexpected probe targets: %+v", source.probeHosts)
	}
	old := plan.Revision
	source.revision = "a-new-reviewed-revision"
	source.meta.Clients.Hosts = append(source.meta.Clients.Hosts, domain.HostMeta{Name: "pc04", IP: "10.0.0.4"})
	subset := manager.PlanReachable(context.Background(), "/deployment", plan)
	if subset.HasErrors() || subset.Requested != "pc01" || subset.ColmenaSelector != "pc01" || len(subset.Targets) != 1 || subset.Revision == old || subset.Revision != source.revision {
		t.Fatalf("subset: %+v", subset)
	}
	if plan.Revision != old || len(plan.Targets) != 3 || len(source.runPhases) != 0 {
		t.Fatal("subset mutated the old review or dispatched an operation")
	}
	if len(source.probeHosts) != 1 || source.probeHosts[0].Name != "pc01" {
		t.Fatal("subset widened after inventory changed")
	}
	source.ssh["pc01"] = domain.SSHProbe{Reachability: domain.ReachabilityUnreachable}
	again := manager.PlanReachable(context.Background(), "/deployment", plan)
	if again.HasErrors() || again.Availability[0].Reachability != domain.ReachabilityUnreachable || again.ReachableRequested != "" {
		t.Fatalf("stale reachability reused: %+v", again)
	}
}

func TestDeploymentSubsetRefusesEmptyUnknownOrMismatchedObservations(t *testing.T) {
	source := readyDeploymentSource()
	manager := NewDeploymentManager(source)
	for _, probes := range []map[string]domain.SSHProbe{nil, {}, {"pc01": {Reachability: domain.ReachabilityUnreachable}}, {"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHUnavailable}}} {
		source.ssh = probes
		plan := manager.Plan(context.Background(), "/deployment", "@lab")
		if plan.HasErrors() {
			t.Fatalf("availability must not silently prohibit the explicit plan: %+v", plan)
		}
		if plan.ReachableRequested != "" || !manager.PlanReachable(context.Background(), "/deployment", plan).HasErrors() {
			t.Fatal("offered an empty or unchanged subset")
		}
	}
	plan := manager.Plan(context.Background(), "/deployment", "pc01,pc02")
	plan.Availability = []domain.DeploymentTargetAvailability{{Name: "pc01", IP: "wrong-address", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}, {Name: "pc99", IP: "10.0.0.99", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}}
	plan.ReachableRequested = "pc99"
	if !manager.PlanReachable(context.Background(), "/deployment", plan).HasErrors() {
		t.Fatal("trusted an injected selector or mismatched identity")
	}
}

func TestDeploymentProbeCancellationAndRepositoryDriftInvalidatePlan(t *testing.T) {
	for _, mode := range []string{"cancel", "dirty", "revision"} {
		t.Run(mode, func(t *testing.T) {
			source := readyDeploymentSource()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source.afterProbe = func() {
				switch mode {
				case "cancel":
					cancel()
				case "dirty":
					source.git.Dirty = true
				case "revision":
					source.revision = "changed"
				}
			}
			plan := NewDeploymentManager(source).Plan(ctx, "/deployment", "pc01")
			if !plan.HasErrors() || plan.State != "blocked" || plan.ReachableRequested != "" || len(source.runPhases) != 0 {
				t.Fatalf("unsafe plan: %+v", plan)
			}
		})
	}
}

func TestDeploymentResultClassificationPreservesEvidence(t *testing.T) {
	for _, test := range []struct {
		name                                          string
		applyErr                                      error
		offline, oldRevision, changedKey, recordError bool
		headline, outcome                             string
		recovery                                      bool
	}{
		{name: "updated", headline: "Deployment completed and verified", outcome: "Updated"},
		{name: "not reached", offline: true, headline: "Some computers were not reached", outcome: "Not reached"},
		{name: "failed update", applyErr: errors.New("confirmed batch failure"), oldRevision: true, headline: "An update failed", outcome: "Update failed"},
		{name: "unknown authentication", changedKey: true, headline: "Deployment needs attention", outcome: "Not verified"},
		{name: "recording failed", recordError: true, headline: "Deployment needs attention", outcome: "Updated"},
		{name: "uncertain activation", applyErr: &domain.DeploymentUnconfirmedError{Err: errors.New("disconnected")}, headline: "Deployment requires recovery", outcome: "Not verified", recovery: true},
		{name: "uncertain and offline", applyErr: &domain.DeploymentUnconfirmedError{Err: errors.New("disconnected")}, offline: true, headline: "Deployment requires recovery — some computers were not reached", outcome: "Not reached", recovery: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := readyDeploymentSource()
			source.runErrors = map[domain.DeploymentPhase]error{domain.DeploymentPhaseApply: test.applyErr}
			if test.offline {
				delete(source.current, "pc02")
				source.ssh = map[string]domain.SSHProbe{"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}, "pc02": {Reachability: domain.ReachabilityUnreachable}}
			}
			if test.oldRevision {
				source.current["pc02"] = domain.HostSystemProbe{Revision: "old-revision", SystemPath: "/nix/store/old"}
			}
			if test.changedKey {
				source.current["pc02"] = domain.HostSystemProbe{HostKeyCondition: domain.HostKeyChanged}
			}
			if test.recordError {
				source.recordErr = errors.New("history unavailable")
			}
			report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "pc01,pc02", source.revision, "", io.Discard)
			before := report
			summary := report.ResultSummary()
			if summary.Headline != test.headline || len(summary.Computers) != 2 || summary.Computers[1].Outcome != test.outcome {
				t.Fatalf("summary: %+v; report: %+v", summary, report)
			}
			if !reflect.DeepEqual(before, report) || report.RecoveryRequired != test.recovery {
				t.Fatal("classification changed execution evidence")
			}
			if test.recovery && (report.RetrySafe || report.Verification.Recorded != 0 || !strings.Contains(summary.Computers[0].Guidance, "reviewed recovery")) {
				t.Fatal("recovery requirement lost")
			}
			if test.changedKey && !strings.Contains(summary.Computers[1].Guidance, "physical identity") {
				t.Fatal("host trust failure mistaken for offline computer")
			}
		})
	}
}
