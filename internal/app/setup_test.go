package app

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSetupSource struct {
	data []byte
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

func (fakeSetupSource) ArtifactState(_ string, name, path string) domain.ArtifactState {
	return domain.ArtifactState{Name: name, Path: path, Present: false}
}

func (fakeSetupSource) CommandAvailable(string) bool {
	return true
}

func (fakeSetupSource) KeyMaterial(string) []domain.KeyMaterialState {
	return []domain.KeyMaterialState{
		{Name: "cache", PrivatePresent: true, PublicPresent: true, Safe: true},
		{Name: "ssh", PrivatePresent: true, PublicPresent: true, Safe: true},
		{Name: "veyon", PrivatePresent: true, PublicPresent: true, Safe: true},
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
	if report.CurrentStage != domain.SetupStageReview {
		t.Fatalf("current stage = %q, want %q: %+v", report.CurrentStage, domain.SetupStageReview, report)
	}
}
