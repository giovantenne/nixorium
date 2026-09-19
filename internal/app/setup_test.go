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
	preparation domain.PXEPreparationState
	keyCalls    *int
	metaCalls   *int
}

func (f fakeSetupSource) ReadSettings(string) ([]byte, error) {
	return f.data, nil
}

func (f fakeSetupSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	if f.metaCalls != nil {
		*f.metaCalls++
	}
	return domain.LabMeta{}, nil
}

func (f fakeSetupSource) GitState(context.Context, string) (domain.GitState, error) {
	state := domain.GitState{Available: true, Dirty: f.dirty, Changes: 1}
	if f.dirty {
		state.Paths = []string{"lab-settings.json"}
	}
	return state, nil
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

func (f fakeSetupSource) KeyMaterial(context.Context, string) []domain.KeyMaterialState {
	if f.keyCalls != nil {
		*f.keyCalls++
	}
	return []domain.KeyMaterialState{
		{Name: "cache", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
		{Name: "ssh", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
		{Name: "veyon", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
	}
}

func (fakeSetupSource) ReconcileKeyMaterial(context.Context, string) error {
	return nil
}

func (fakeSetupSource) ImportKeyMaterial(_ context.Context, _, name, sourcePath string) (domain.KeyImportEvidence, error) {
	return domain.KeyImportEvidence{Name: name, Source: sourcePath, Fingerprint: "SHA256:test"}, nil
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

func TestImportKeyReportsVerifiedEvidenceWithoutExposingMaterial(t *testing.T) {
	report, err := NewSetupManager(fakeSetupSource{}).ImportKey(context.Background(), "/repo", "ssh", "/secure/admin-key")
	if err != nil || report.State != "imported" || report.Key != "ssh" || report.Fingerprint != "SHA256:test" || !strings.Contains(report.Message, "source file was left unchanged") {
		t.Fatalf("report = %+v, error = %v", report, err)
	}
}

func TestSetupStatusSelectsNetworkForFreshTemplate(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	keyCalls, metaCalls := 0, 0
	report := NewSetupManager(fakeSetupSource{data: data, keyCalls: &keyCalls, metaCalls: &metaCalls}).Status(context.Background(), "/repo")
	if report.CurrentStage != domain.SetupStageNetwork {
		t.Fatalf("current stage = %q, want %q: %+v", report.CurrentStage, domain.SetupStageNetwork, report)
	}
	if keyCalls != 0 || metaCalls != 0 {
		t.Fatalf("fresh setup ran deferred checks: keys=%d meta=%d", keyCalls, metaCalls)
	}
}

func TestControllerBootstrapDefersClientNetworkAndOpensAtControllerReview(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"masterDhcpIp": "MASTER_DHCP_IP",`), []byte(`"deploymentMode": "controller", "masterDhcpIp": "MASTER_DHCP_IP",`), 1)
	data = bytes.Replace(data, []byte(`"pcCount": 20`), []byte(`"pcCount": 0`), 1)
	data = bytes.ReplaceAll(data, []byte(domain.DefaultPasswordHash), []byte("$6$salt$changed"))
	report := NewSetupManager(fakeSetupSource{data: data}).Status(context.Background(), "/repo")
	if report.CurrentStage != domain.SetupStageApply {
		t.Fatalf("current stage = %q, want %q: %+v", report.CurrentStage, domain.SetupStageApply, report)
	}
	if len(report.Stages) < 2 || report.Stages[1].State != domain.SetupStageComplete || !strings.Contains(report.Stages[1].Detail, "deferred") {
		t.Fatalf("network stage = %+v", report.Stages)
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

func TestSetupStatusStopsAtLocalSaveWhenManagedConfigurationIsDirty(t *testing.T) {
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

func TestSetupStatusIgnoresUnrelatedHistoryChanges(t *testing.T) {
	if pending := pendingManagedSetupPaths([]string{"notes.txt", "modules/private.nix"}); len(pending) != 0 {
		t.Fatalf("unrelated paths became setup work: %v", pending)
	}
	if pending := pendingManagedSetupPaths([]string{"notes.txt", "keys/admin-ssh.pub"}); len(pending) != 1 || pending[0] != "keys/admin-ssh.pub" {
		t.Fatalf("managed paths were not isolated: %v", pending)
	}
}

func TestSetupStatusCompletesWhenInstallationArtifactsArePrepared(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte(domain.MasterDHCPPlaceholder), []byte("192.0.2.10"))
	data = bytes.ReplaceAll(data, []byte(domain.DefaultPasswordHash), []byte("$6$salt$changed"))
	report := NewSetupManager(fakeSetupSource{
		data:        data,
		applied:     true,
		preparation: domain.PXEPreparationState{Present: true, Ready: true},
	}).Status(context.Background(), "/repo")
	if report.State != "ready" || report.CurrentStage != "" {
		t.Fatalf("report = %+v", report)
	}
	artifacts := report.Stages[len(report.Stages)-1]
	if artifacts.ID != domain.SetupStageArtifacts || artifacts.State != domain.SetupStageComplete {
		t.Fatalf("artifact stage = %+v", artifacts)
	}
}
