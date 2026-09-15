package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeInstallationSessionSource struct {
	meta       domain.LabMeta
	revision   string
	record     domain.InstallationSessionRecord
	present    bool
	readErr    error
	writeErr   error
	writeCalls int
}

func (f *fakeInstallationSessionSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}

func (f *fakeInstallationSessionSource) GitRevision(context.Context, string) (string, error) {
	return f.revision, nil
}

func (f *fakeInstallationSessionSource) InstallationSession(string) (domain.InstallationSessionRecord, bool, error) {
	return f.record, f.present, f.readErr
}

func (f *fakeInstallationSessionSource) WriteInstallationSession(_ string, record domain.InstallationSessionRecord) error {
	f.writeCalls++
	if f.writeErr != nil {
		return f.writeErr
	}
	f.record = record
	f.present = true
	return nil
}

type fakeInstallationObserver struct {
	report domain.HostsReport
	err    error
	name   string
}

func (f *fakeInstallationObserver) Host(_ context.Context, _ string, name string) (domain.HostsReport, error) {
	f.name = name
	return f.report, f.err
}

func installationManagerFixture() (*fakeInstallationSessionSource, *fakeInstallationObserver, InstallationSessionManager, time.Time) {
	now := time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC)
	source := &fakeInstallationSessionSource{revision: strings.Repeat("a", 40)}
	source.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}}
	observer := &fakeInstallationObserver{}
	manager := NewInstallationSessionManager(source, observer)
	manager.now = func() time.Time { return now }
	return source, observer, manager, now
}

func TestInstallationSessionSelectPersistsEvaluatedTargetAndResumes(t *testing.T) {
	source, _, manager, now := installationManagerFixture()
	report := manager.Select(context.Background(), "/deployment", "pc02")
	if report.HasErrors() || report.Selected != "pc02" || source.writeCalls != 1 {
		t.Fatalf("selection report=%+v writes=%d", report, source.writeCalls)
	}
	if source.record.StartedAt != now || source.record.Revision != source.revision {
		t.Fatalf("stored session = %+v", source.record)
	}
	resumed := manager.Status(context.Background(), "/deployment")
	if resumed.State != domain.InstallationSessionActive || resumed.Selected != "pc02" || !strings.Contains(resumed.Message, "restored") {
		t.Fatalf("resumed session = %+v", resumed)
	}
}

func TestInstallationSessionSelectionRejectsUnknownTargetBeforeWrite(t *testing.T) {
	source, _, manager, _ := installationManagerFixture()
	report := manager.Select(context.Background(), "/deployment", "pc99")
	if !report.HasErrors() || source.writeCalls != 0 || !strings.Contains(report.Message, "evaluated inventory") {
		t.Fatalf("unknown selection report=%+v writes=%d", report, source.writeCalls)
	}
}

func TestInstallationSessionNewRevisionDoesNotReuseEvidence(t *testing.T) {
	source, _, manager, now := installationManagerFixture()
	oldPractical := now.Add(-time.Hour)
	source.present = true
	source.record = domain.InstallationSessionRecord{
		SchemaVersion: domain.InstallationSessionSchemaVersion,
		Repository:    "/deployment",
		State:         domain.InstallationSessionActive,
		Revision:      strings.Repeat("b", 40),
		Selected:      "pc01",
		StartedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     oldPractical,
		Evidence: []domain.InstallationEvidence{{
			Name: "pc01", Revision: strings.Repeat("b", 40), SystemPath: "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-pc01", TechnicalVerifiedAt: now.Add(-90 * time.Minute), PracticalConfirmedAt: &oldPractical,
		}},
	}
	report := manager.Select(context.Background(), "/deployment", "pc02")
	if report.HasErrors() || len(report.Evidence) != 0 || len(source.record.Evidence) != 0 || !strings.Contains(report.Message, "not reused") {
		t.Fatalf("new revision reused evidence: %+v", report)
	}
}

func TestInstallationSessionChangedInventoryDoesNotReuseEvidence(t *testing.T) {
	source, _, manager, now := installationManagerFixture()
	practical := now.Add(-time.Hour)
	source.present = true
	source.record = domain.InstallationSessionRecord{
		SchemaVersion: domain.InstallationSessionSchemaVersion,
		Repository:    "/deployment",
		State:         domain.InstallationSessionActive,
		Revision:      source.revision,
		Selected:      "pc02",
		StartedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     practical,
		Evidence: []domain.InstallationEvidence{{
			Name: "pc02", Revision: source.revision,
			SystemPath:          "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc02-system",
			TechnicalVerifiedAt: now.Add(-90 * time.Minute), PracticalConfirmedAt: &practical,
		}},
	}
	source.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	report := manager.Select(context.Background(), "/deployment", "pc01")
	if report.HasErrors() || report.Selected != "pc01" || len(report.Evidence) != 0 || !strings.Contains(report.Message, "not reused") {
		t.Fatalf("changed inventory reused stale evidence: %+v", report)
	}
}

func TestInstallationSessionVerifyRecordsOnlyAuthenticatedCurrentSystem(t *testing.T) {
	source, observer, manager, now := installationManagerFixture()
	selected := manager.Select(context.Background(), "/deployment", "pc01")
	if selected.HasErrors() {
		t.Fatal(selected.Message)
	}
	observer.report = domain.HostsReport{Hosts: []domain.HostStatus{{
		Name: "pc01", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable,
		Deployment: domain.DeploymentCurrent, CurrentRevision: source.revision,
		CurrentSystem: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system",
	}}}
	manager.now = func() time.Time { return now.Add(time.Minute) }
	report := manager.Verify(context.Background(), "/deployment", "pc01")
	if report.HasErrors() || observer.name != "pc01" || len(report.Evidence) != 1 || report.Evidence[0].TechnicalVerifiedAt != now.Add(time.Minute) {
		t.Fatalf("verification report=%+v observed=%q", report, observer.name)
	}

	observer.report.Hosts[0].Deployment = domain.DeploymentUnknown
	before := source.writeCalls
	report = manager.Verify(context.Background(), "/deployment", "pc01")
	if report.State != "attention" || source.writeCalls != before || !strings.Contains(report.Message, "Revision not verified") {
		t.Fatalf("weak observation changed evidence: report=%+v writes=%d", report, source.writeCalls)
	}
}

func TestInstallationSessionPracticalCheckRequiresStoredTechnicalEvidence(t *testing.T) {
	source, observer, manager, now := installationManagerFixture()
	manager.Select(context.Background(), "/deployment", "pc01")
	report := manager.ConfirmPractical(context.Background(), "/deployment", "pc01")
	if !report.HasErrors() || !strings.Contains(report.Message, "technical verification") {
		t.Fatalf("practical check without evidence = %+v", report)
	}
	observer.report = domain.HostsReport{Hosts: []domain.HostStatus{{
		Name: "pc01", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable,
		Deployment: domain.DeploymentCurrent, CurrentRevision: source.revision,
		CurrentSystem: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system",
	}}}
	manager.now = func() time.Time { return now.Add(time.Minute) }
	if verified := manager.Verify(context.Background(), "/deployment", "pc01"); verified.HasErrors() {
		t.Fatal(verified.Message)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	report = manager.ConfirmPractical(context.Background(), "/deployment", "pc01")
	if report.HasErrors() || report.Evidence[0].PracticalConfirmedAt == nil || !report.Evidence[0].PracticalConfirmedAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("practical evidence was not recorded: %+v", report)
	}
}

func TestInstallationSessionSurfacesPrivateStateWriteFailure(t *testing.T) {
	source, _, manager, _ := installationManagerFixture()
	source.writeErr = errors.New("read-only state")
	report := manager.Select(context.Background(), "/deployment", "pc01")
	if !report.HasErrors() || !strings.Contains(report.Message, "read-only state") {
		t.Fatalf("write failure = %+v", report)
	}
}

func TestInstallationSessionDoesNotCorruptEvidenceWhenClockMovesBack(t *testing.T) {
	source, observer, manager, now := installationManagerFixture()
	if selected := manager.Select(context.Background(), "/deployment", "pc01"); selected.HasErrors() {
		t.Fatal(selected.Message)
	}
	observer.report = domain.HostsReport{Hosts: []domain.HostStatus{{
		Name: "pc01", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable,
		Deployment: domain.DeploymentCurrent, CurrentRevision: source.revision,
		CurrentSystem: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system",
	}}}
	manager.now = func() time.Time { return now.Add(-time.Minute) }
	report := manager.Verify(context.Background(), "/deployment", "pc01")
	if report.HasErrors() || !report.UpdatedAt.Equal(now) || !report.Evidence[0].TechnicalVerifiedAt.Equal(now) {
		t.Fatalf("clock rollback produced invalid evidence: %+v", report)
	}
}
