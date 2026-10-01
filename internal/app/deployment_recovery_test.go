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
}

func (f *fakeDeploymentRecovery) PendingDeployment() (domain.PendingDeployment, bool, error) {
	return f.pending, f.present, nil
}
func (f *fakeDeploymentRecovery) ProcessRunning(int) bool { return f.running }
func (f *fakeDeploymentRecovery) ObserveDeploymentActivity(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.DeploymentActivity {
	result := map[string]domain.DeploymentActivity{}
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
