package app

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type initialSetupGuard struct{ fakeSetupSource }

func (initialSetupGuard) ControllerApplied(context.Context, string) (bool, string) {
	panic("startup must not evaluate a system closure")
}
func (initialSetupGuard) PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState {
	panic("startup must not check installation artifacts")
}
func TestInitialSetupDefersExpensiveChecksWithoutClaimingReadiness(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	keys, meta := 0, 0
	source := initialSetupGuard{fakeSetupSource{data: data, keyCalls: &keys, metaCalls: &meta}}
	if report := NewSetupManager(source).InitialStatus(context.Background(), "/repo"); report.CurrentStage != domain.SetupStageNetwork {
		t.Fatal(report)
	}
	source.data = bytes.ReplaceAll(data, []byte(domain.MasterDHCPPlaceholder), []byte("192.0.2.10"))
	source.data = bytes.ReplaceAll(source.data, []byte(domain.DefaultPasswordHash), []byte("$6$salt$changed"))
	report := NewSetupManager(source).InitialStatus(context.Background(), "/repo")
	if report.State != "unchecked" || report.CurrentStage != "" || len(report.Stages) != 0 || keys != 0 || meta != 0 {
		t.Fatalf("unexpected startup checks or readiness: %+v keys=%d meta=%d", report, keys, meta)
	}
}

type overviewGuard struct{ *fakeSource }

func (overviewGuard) PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState {
	panic("overview must not evaluate artifact derivations")
}

type startupGuard struct{ overviewGuard }

func (startupGuard) LabMeta(context.Context, string) (domain.LabMeta, error) {
	panic("startup must not evaluate even metadata")
}
func (startupGuard) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	panic("startup must not evaluate deployment readiness")
}

type inventoryGuard struct{ overviewGuard }

func (inventoryGuard) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	panic("inventory selection must defer readiness to operation planning")
}

func TestInventoryLoadsIdentitiesWithoutClaimingReadiness(t *testing.T) {
	source := &fakeSource{}
	source.meta.Controller.Name = "pc99"
	source.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01"}}
	report, err := NewInspector(inventoryGuard{overviewGuard{source}}).Inventory(t.Context(), "/repo")
	if err != nil || report.State != "unchecked" || report.Deployment.Ready || report.Meta.Controller.Name != "pc99" || len(report.Meta.Clients.Hosts) != 1 {
		t.Fatalf("inventory=%+v err=%v", report, err)
	}
}

func TestStartupAvoidsAllNixEvaluationAndPreservesNetworkWarnings(t *testing.T) {
	source := startupGuard{overviewGuard{readyFake()}}
	source.services = map[string]domain.ServiceState{
		PXEListenerUnit: {Loaded: true, Active: true, State: "active"},
		PXENetworkUnit:  {Loaded: true, Active: true, State: "active"},
	}
	report, err := NewInspector(source).Startup(context.Background(), ".")
	if err != nil || report.State != "unchecked" || report.Deployment.Ready || report.Meta.Controller.Name != "" || report.PXE.Mode != "active" || !report.Git.Available {
		t.Fatalf("startup claimed readiness or lost local state: %+v err=%v", report, err)
	}
	source.services[PXEListenerUnit] = domain.ServiceState{Loaded: true, State: "inactive"}
	report, err = NewInspector(source).Startup(context.Background(), ".")
	if err != nil || report.PXE.Mode != "recovery-required" || len(report.Warnings) == 0 {
		t.Fatalf("startup lost interrupted PXE warning: %+v err=%v", report, err)
	}
}

func TestStartupObservesLowStoreSpaceWithoutEvaluation(t *testing.T) {
	source := startupGuard{overviewGuard{readyFake()}}
	source.free = 1 << 30
	report, err := NewInspector(source).Startup(t.Context(), ".")
	if err != nil || report.StoreFreeBytes == nil || *report.StoreFreeBytes != source.free || !report.StoreSpaceLow || source.sshCalls != 0 || source.currentCalls != 0 {
		t.Fatalf("local space observation: %+v %v", report, err)
	}
}
