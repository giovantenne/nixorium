package app

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type networkChangeRecoverySource struct {
	fakeRecoverySource
	lab       domain.LabSettings
	changing  bool
	addresses []string
}

func (s networkChangeRecoverySource) SavedNetworkChange(string) (domain.LabSettings, bool) {
	return s.lab, s.changing
}

func (s networkChangeRecoverySource) InterfaceAddresses(string) ([]string, error) {
	return s.addresses, nil
}

func TestRecoveryShowsANetworkChangeInProgress(t *testing.T) {
	lab := domain.LabSettings{NetworkBase: "10.1.0.0", NetworkPrefix: 24, MasterHostNumber: 99, PCCount: 2, InterfaceName: "eth1",
		NetworkTransition: &domain.NetworkTransition{PreviousBase: "10.0.0.0", PreviousPrefixLength: 24, StartedAt: "2026-10-15T08:00:00Z"}}
	source := networkChangeRecoverySource{lab: lab, changing: true, addresses: []string{"10.0.0.99"}}
	report := NewRecoveryInspector(source).Observe(context.Background(), "/srv/lab")
	if len(report.Conditions) != 1 {
		t.Fatalf("conditions = %+v", report.Conditions)
	}
	condition := report.Conditions[0]
	if condition.Kind != domain.RecoveryNetworkChanging || condition.Next.Code != "NETWORK-CHANGING" || condition.Blocks != "" ||
		!strings.Contains(condition.Detail, "Next: Apply to controller") {
		t.Fatalf("condition = %+v", condition)
	}
	source.addresses = []string{"10.1.0.99", "10.0.0.99"}
	if detail := NewRecoveryInspector(source).Observe(context.Background(), "/srv/lab").Conditions[0].Detail; !strings.Contains(detail, "update the computers") {
		t.Fatalf("detail after controller apply = %q", detail)
	}
	source.changing = false
	if report := NewRecoveryInspector(source).Observe(context.Background(), "/srv/lab"); len(report.Conditions) != 0 {
		t.Fatalf("conditions without a change = %+v", report.Conditions)
	}
}

// transitionDeploymentSource is a laboratory moving from 10.0.0.0/24 to
// 10.1.0.0/24 whose pc01 still answers only on its previous address.
func transitionDeploymentSource() *fakeDeploymentSource {
	source := readyDeploymentSource()
	source.revision = strings.Repeat("e", 40)
	for name, probe := range source.current {
		probe.Revision = source.revision
		source.current[name] = probe
	}
	for index := range source.meta.Clients.Hosts {
		host := &source.meta.Clients.Hosts[index]
		host.PreviousIP = host.IP
		host.IP = strings.Replace(host.IP, "10.0.0.", "10.1.0.", 1)
	}
	source.meta.Controller.StaticIP = "10.1.0.99"
	source.addresses = []string{"10.1.0.99", "10.0.0.99"}
	source.ssh = map[string]domain.SSHProbe{
		"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Network: domain.HostNetworkPrevious},
		"pc02": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Network: domain.HostNetworkCurrent},
		"pc03": {Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Network: domain.HostNetworkUnknown},
	}
	return source
}

func TestDeploymentReachesComputersOnEitherNetwork(t *testing.T) {
	source := transitionDeploymentSource()
	manager := NewDeploymentManager(source)
	plan := manager.Plan(context.Background(), "/deployment", "@lab")
	if plan.HasErrors() || plan.ReachableRequested != "pc01,pc02" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Targets[0] != (domain.DeploymentTarget{Name: "pc01", IP: "10.1.0.1", PreviousIP: "10.0.0.1"}) || plan.Availability[0].Network != domain.HostNetworkPrevious {
		t.Fatalf("targets = %+v, availability = %+v", plan.Targets, plan.Availability)
	}
	if len(source.probeHosts) != 3 || source.probeHosts[0].PreviousIP != "10.0.0.1" {
		t.Fatalf("probe hosts = %+v", source.probeHosts)
	}
	// Queued computers keep both addresses, so the queue finds them on
	// whichever network they come back.
	store := &memoryDeferredStore{queue: domain.DeferredUpdateQueue{SchemaVersion: domain.DeferredUpdateSchemaVersion}}
	queue := NewDeferredUpdateManager(store, source, nil)
	if err := queue.Queue(plan, []string{"pc03"}); err != nil {
		t.Fatal(err)
	}
	if entry := store.queue.Updates[0]; entry.IP != "10.1.0.3" || entry.PreviousIP != "10.0.0.3" {
		t.Fatalf("queued = %+v", entry)
	}
	source.current["pc01"] = domain.HostSystemProbe{SystemPath: source.current["pc01"].SystemPath, Revision: source.revision, Network: domain.HostNetworkCurrent}
	report := manager.Execute(context.Background(), "/deployment", "pc01,pc02", source.revision, "", io.Discard)
	if report.State != "completed" || report.Verification.Targets[0].Network != domain.HostNetworkCurrent {
		t.Fatalf("report = %+v", report)
	}
}
