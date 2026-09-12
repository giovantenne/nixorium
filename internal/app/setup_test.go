package app

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSetupSource struct {
	data  []byte
	dirty bool
}

func (f fakeSetupSource) ReadSettings(string) ([]byte, error) {
	return f.data, nil
}

func (fakeSetupSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return domain.LabMeta{}, nil
}

func (fakeSetupSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	return domain.DeploymentStatus{Ready: false, Issues: []string{"keys missing"}}, nil
}

func (f fakeSetupSource) GitState(context.Context, string) (domain.GitState, error) {
	return domain.GitState{Available: true, Dirty: f.dirty, Changes: 1}, nil
}

func (fakeSetupSource) ControllerApplied(context.Context, string) (bool, string) {
	return false, "reviewed controller configuration is not active"
}

func (fakeSetupSource) ArtifactState(_ string, name, path string) domain.ArtifactState {
	return domain.ArtifactState{Name: name, Path: path, Present: false}
}

func (fakeSetupSource) PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState {
	return domain.PXEPreparationState{}
}

func (fakeSetupSource) CommandAvailable(string) bool {
	return true
}

func (fakeSetupSource) KeyMaterial(context.Context, string) []domain.KeyMaterialState {
	return []domain.KeyMaterialState{
		{Name: "cache", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
		{Name: "ssh", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
		{Name: "veyon", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
	}
}

func (fakeSetupSource) ReconcileKeyMaterial(context.Context, string) error {
	return nil
}

func TestReconcileKeysReportsVerifiedState(t *testing.T) {
	report, err := NewSetupManager(fakeSetupSource{}).ReconcileKeys(context.Background(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if report.Operation != "setup-keys" || report.State != "ready" || len(report.Keys) != 3 {
		t.Fatalf("report = %+v", report)
	}
}

func TestVerifyKeysIsReadOnlyAndReportsVerifiedState(t *testing.T) {
	report := NewSetupManager(fakeSetupSource{}).VerifyKeys(context.Background(), "/repo")
	if report.Operation != "setup-keys-verify" || report.State != "ready" || len(report.Keys) != 3 {
		t.Fatalf("report = %+v", report)
	}
}

func TestSetupStatusSelectsNetworkForFreshTemplate(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	report := NewSetupManager(fakeSetupSource{data: data}).Status(context.Background(), "/repo")
	if report.CurrentStage != domain.SetupStageNetwork {
		t.Fatalf("current stage = %q, want %q: %+v", report.CurrentStage, domain.SetupStageNetwork, report)
	}
}

func TestSetupStatusAdvancesToReviewAfterConfiguredInputs(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte(domain.MasterDHCPPlaceholder), []byte("192.0.2.10"))
	data = bytes.ReplaceAll(data, []byte(domain.DefaultPasswordHash), []byte("$6$salt$changed"))
	report := NewSetupManager(fakeSetupSource{data: data}).Status(context.Background(), "/repo")
	if report.CurrentStage != domain.SetupStageApply {
		t.Fatalf("current stage = %q, want %q: %+v", report.CurrentStage, domain.SetupStageApply, report)
	}
}

func TestSetupStatusStopsAtGitReviewWhenWorktreeIsDirty(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte(domain.MasterDHCPPlaceholder), []byte("192.0.2.10"))
	data = bytes.ReplaceAll(data, []byte(domain.DefaultPasswordHash), []byte("$6$salt$changed"))
	report := NewSetupManager(fakeSetupSource{data: data, dirty: true}).Status(context.Background(), "/repo")
	if report.CurrentStage != domain.SetupStageReview {
		t.Fatalf("current stage = %q, want %q: %+v", report.CurrentStage, domain.SetupStageReview, report)
	}
}
