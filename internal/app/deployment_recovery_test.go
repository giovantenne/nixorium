package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeDeploymentRecovery struct {
	pending  domain.PendingDeployment
	present  bool
	running  bool
	activity map[string]domain.DeploymentActivity
	archived int
	hosts    []domain.HostMeta
	starts   []string
}

func (f *fakeDeploymentRecovery) PendingDeployment() (domain.PendingDeployment, bool, error) {
	return f.pending, f.present, nil
}
func (f *fakeDeploymentRecovery) ProcessRunning(_ int, start string) bool {
	f.starts = append(f.starts, start)
	return f.running
}
func (f *fakeDeploymentRecovery) ObserveDeploymentActivity(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.DeploymentActivity {
	result := map[string]domain.DeploymentActivity{}
	f.hosts = append(f.hosts, hosts...)
	for _, host := range hosts {
		result[host.Name] = f.activity[host.Name]
	}
	return result
}
func (f *fakeDeploymentRecovery) ArchivePendingDeployment(domain.PendingDeployment) (string, error) {
	f.archived++
	f.present = false
	return "/var/lib/nixorium/coordination/recovered/deployment-x.json", nil
}

func TestDeploymentRecoveryRequiresFinishedOrAcknowledgedComputers(t *testing.T) {
	revision := strings.Repeat("a", 40)
	source := &fakeDeploymentRecovery{
		present: true,
		pending: domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: revision, StartedAt: time.Now().UTC(), ControllerPID: 7,
			Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}, {Name: "pc03", IP: "10.0.0.3"}}},
		activity: map[string]domain.DeploymentActivity{
			"pc01": {Reachable: true, Revision: revision},
			"pc02": {Reachable: true, Activating: true},
		},
	}
	manager := NewDeploymentRecoveryManager(source)
	if plan := manager.Plan(context.Background(), "/srv/lab", true); plan.State != "blocked" || !strings.Contains(plan.Message, "still applying") {
		t.Fatalf("activating computer allowed recovery: %+v", plan)
	}
	source.activity["pc02"] = domain.DeploymentActivity{Reachable: true, Revision: strings.Repeat("b", 40)}
	plan := manager.Plan(context.Background(), "/srv/lab", false)
	if plan.State != "blocked" || plan.Unreachable != 1 || !strings.Contains(plan.Message, "acknowledge") {
		t.Fatalf("unreachable computer not acknowledged: %+v", plan)
	}
	plan = manager.Plan(context.Background(), "/srv/lab", true)
	if plan.State != "ready" || plan.Confirmation != "RECOVERED" || plan.Targets[0].State != domain.RecoveryTargetReviewed || plan.Targets[1].State != domain.RecoveryTargetOther || plan.Targets[2].State != domain.RecoveryTargetUnreachable {
		t.Fatalf("plan = %+v", plan)
	}
	if result := manager.Apply(context.Background(), plan, "sha256:wrong"); result.State != "blocked" || source.archived != 0 {
		t.Fatalf("wrong token accepted: %+v", result)
	}
	source.activity["pc01"] = domain.DeploymentActivity{Reachable: true, Activating: true}
	if result := manager.Apply(context.Background(), plan, plan.ReviewToken); result.State != "blocked" || source.archived != 0 {
		t.Fatalf("renewed activity ignored: %+v", result)
	}
	source.activity["pc01"] = domain.DeploymentActivity{Reachable: true, Revision: revision}
	result := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if result.State != "completed" || source.archived != 1 || len(result.Targets) != 3 {
		t.Fatalf("result = %+v", result)
	}
	if plan := manager.Plan(context.Background(), "/srv/lab", false); plan.State != "unchanged" {
		t.Fatalf("after recovery = %+v", plan)
	}
	source.present, source.running = true, true
	if plan := manager.Plan(context.Background(), "/srv/lab", true); plan.State != "blocked" || !strings.Contains(plan.Message, "still running") {
		t.Fatalf("running process ignored: %+v", plan)
	}
}

type ownApplyRecovery struct {
	*fakeDeploymentRecovery
	applying bool
}

func (f ownApplyRecovery) OwnDeploymentApplyRunning() bool { return f.applying }

func TestDeploymentRecoveryAcceptsOwnReturnedProcessOnly(t *testing.T) {
	revision := strings.Repeat("a", 40)
	fake := &fakeDeploymentRecovery{
		present: true, running: true,
		pending: domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: revision, StartedAt: time.Now().UTC(), ControllerPID: 4242,
			Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}}},
		activity: map[string]domain.DeploymentActivity{"pc01": {Reachable: true, Revision: revision}},
	}
	source := ownApplyRecovery{fakeDeploymentRecovery: fake}
	// This process wrote the record and its apply returned: not refused.
	plan := NewDeploymentRecoveryManager(source).ForProcess(4242).Plan(context.Background(), "/srv/lab", false)
	if plan.State != "ready" {
		t.Fatalf("own returned process refused: %+v", plan)
	}
	if result := NewDeploymentRecoveryManager(source).ForProcess(4242).Apply(context.Background(), plan, plan.ReviewToken); result.State != "completed" {
		t.Fatalf("own returned process refused at apply: %+v", result)
	}
	fake.present = true
	// Another live Nixorium process is still refused.
	if plan := NewDeploymentRecoveryManager(source).ForProcess(1).Plan(context.Background(), "/srv/lab", false); plan.State != "blocked" || plan.Issues[0].Field != "process" {
		t.Fatalf("foreign process accepted: %+v", plan)
	}
	// So is this process while it is still applying.
	source.applying = true
	if plan := NewDeploymentRecoveryManager(source).ForProcess(4242).Plan(context.Background(), "/srv/lab", false); plan.State != "blocked" || plan.Issues[0].Field != "process" {
		t.Fatalf("own running apply accepted: %+v", plan)
	}
	// A source that cannot tell fails closed.
	if plan := NewDeploymentRecoveryManager(fake).ForProcess(4242).Plan(context.Background(), "/srv/lab", false); plan.State != "blocked" || plan.Issues[0].Field != "process" {
		t.Fatalf("unknown own state accepted: %+v", plan)
	}
}

type pxeOpenRecovery struct {
	*fakeDeploymentRecovery
	network bool
}

func (f pxeOpenRecovery) ServiceState(_ context.Context, unit string) domain.ServiceState {
	return domain.ServiceState{Name: unit, Loaded: true, Active: f.network && unit == PXENetworkUnit}
}

func TestDeploymentRecoveryReportsAnOpenNetworkInstallation(t *testing.T) {
	revision := strings.Repeat("a", 40)
	fake := &fakeDeploymentRecovery{
		present: true,
		pending: domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: revision, StartedAt: time.Now().UTC(),
			Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}}},
		activity: map[string]domain.DeploymentActivity{},
	}
	plan := NewDeploymentRecoveryManager(pxeOpenRecovery{fakeDeploymentRecovery: fake, network: true}).Plan(context.Background(), "/srv/lab", false)
	if plan.PXEMode != "recovery-required" || plan.Unreachable != 1 {
		t.Fatalf("open network installation not reported: %+v", plan)
	}
	token := plan.ReviewToken
	acknowledged := NewDeploymentRecoveryManager(pxeOpenRecovery{fakeDeploymentRecovery: fake, network: true}).Plan(context.Background(), "/srv/lab", true)
	if acknowledged.State != "ready" || token != "" {
		t.Fatalf("PXE observation changed the review rules: %+v", acknowledged)
	}
	if plan := NewDeploymentRecoveryManager(pxeOpenRecovery{fakeDeploymentRecovery: fake}).Plan(context.Background(), "/srv/lab", false); plan.PXEMode != "" {
		t.Fatalf("stopped network installation reported: %+v", plan)
	}
}

func TestDeploymentRecoveryChecksThePreviousAddressAndRecorderStart(t *testing.T) {
	revision := strings.Repeat("a", 40)
	fake := &fakeDeploymentRecovery{
		present: true,
		pending: domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: revision, StartedAt: time.Now().UTC(), ControllerPID: 7, ControllerPIDStart: "12345",
			Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.1.1", PreviousIP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.1.2"}}},
		activity: map[string]domain.DeploymentActivity{"pc01": {Reachable: true, Revision: revision}, "pc02": {Reachable: true, Revision: revision}},
	}
	plan := NewDeploymentRecoveryManager(fake).Plan(context.Background(), "/srv/lab", false)
	if plan.State != "ready" {
		t.Fatalf("plan = %+v", plan)
	}
	want := []domain.HostMeta{{Name: "pc01", IP: "10.0.1.1", PreviousIP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.1.2"}}
	if len(fake.hosts) != 2 || fake.hosts[0] != want[0] || fake.hosts[1] != want[1] {
		t.Fatalf("plan observed %+v, want %+v", fake.hosts, want)
	}
	fake.hosts = nil
	if result := NewDeploymentRecoveryManager(fake).Apply(context.Background(), plan, plan.ReviewToken); result.State != "completed" {
		t.Fatalf("apply = %+v", result)
	}
	if len(fake.hosts) != 2 || fake.hosts[0] != want[0] || fake.hosts[1] != want[1] {
		t.Fatalf("apply observed %+v, want %+v", fake.hosts, want)
	}
	for _, start := range fake.starts {
		if start != "12345" {
			t.Fatalf("recorder start time %q was not passed to the source", start)
		}
	}
}
