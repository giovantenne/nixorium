package app

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSetupSource struct {
	data        []byte
	dirty       bool
	applied     bool
	ready       bool
	preparation domain.PXEPreparationState
}

func (f fakeSetupSource) ReadSettings(string) ([]byte, error) {
	return f.data, nil
}

func (fakeSetupSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return domain.LabMeta{}, nil
}

func (f fakeSetupSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	if f.ready {
		return domain.DeploymentStatus{Ready: true}, nil
	}
	return domain.DeploymentStatus{Ready: false, Issues: []string{"keys missing"}}, nil
}

func (f fakeSetupSource) GitState(context.Context, string) (domain.GitState, error) {
	return domain.GitState{Available: true, Dirty: f.dirty, Changes: 1}, nil
}

func (f fakeSetupSource) ControllerApplied(context.Context, string) (bool, string) {
	if f.applied {
		return true, "reviewed controller configuration is active"
	}
	return false, "reviewed controller configuration is not active"
}

func (fakeSetupSource) ArtifactState(_ string, name, path string) domain.ArtifactState {
	return domain.ArtifactState{Name: name, Path: path, Present: false}
}

func (f fakeSetupSource) PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState {
	return f.preparation
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

func TestFreshTemplateUsesUSRegionalDefaults(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	settings, issues := domain.DecodeLabSettings(data)
	if len(issues) > 0 {
		t.Fatalf("fresh template is invalid: %+v", issues)
	}
	if settings.Lab.TimeZone != "America/New_York" ||
		settings.Lab.DefaultLocale != "en_US.UTF-8" ||
		settings.Lab.ExtraLocale != "en_US.UTF-8" ||
		settings.Lab.KeyboardLayout != "us" ||
		settings.Lab.ConsoleKeyMap != "us" {
		t.Fatalf("fresh template regional defaults are not US defaults: %+v", settings.Lab)
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

func TestSetupStatusCompletesWhenFirstInstallWorkflowIsAvailable(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte(domain.MasterDHCPPlaceholder), []byte("192.0.2.10"))
	data = bytes.ReplaceAll(data, []byte(domain.DefaultPasswordHash), []byte("$6$salt$changed"))
	report := NewSetupManager(fakeSetupSource{
		data:        data,
		applied:     true,
		ready:       true,
		preparation: domain.PXEPreparationState{Present: true, Ready: true},
	}).Status(context.Background(), "/repo")
	if report.State != "ready" || report.CurrentStage != "" {
		t.Fatalf("report = %+v", report)
	}
	install := report.Stages[len(report.Stages)-1]
	if install.ID != domain.SetupStageInstall || install.State != domain.SetupStageComplete || !strings.Contains(install.Detail, "Install computers over network") {
		t.Fatalf("install stage = %+v", install)
	}
}
