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
func (overviewGuard) ArtifactState(string, string, string) domain.ArtifactState {
	panic("overview must not inspect artifact paths")
}
func TestOverviewKeepsEvaluatedInventoryAndActivePXEWithoutFleetProbes(t *testing.T) {
	source := overviewGuard{readyFake()}
	source.services = map[string]domain.ServiceState{
		PXEListenerUnit: {Loaded: true, Active: true, State: "active"}, PXENetworkUnit: {Loaded: true, Active: true, State: "active"},
	}
	report, err := NewInspector(source).Overview(context.Background(), ".")
	if err != nil || report.PXE.Mode != "active" || len(report.Meta.Clients.Hosts) != 2 || len(report.Artifacts) != 0 || source.sshCalls != 0 || source.currentCalls != 0 {
		t.Fatalf("overview: %+v error=%v", report, err)
	}
}
