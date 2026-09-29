package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type supportFake struct {
	*fakeSource
	metaErr       error
	recordErr     error
	recordLimit   int
	writes        int
	saved         domain.SupportSnapshot
	revisionCalls int
	moveRevision  bool
	strictCalls   int
	cancelOnHosts context.CancelFunc
}

func (s *supportFake) LabMeta(ctx context.Context, root string) (domain.LabMeta, error) {
	if s.metaErr != nil {
		return domain.LabMeta{}, s.metaErr
	}
	return s.fakeSource.LabMeta(ctx, root)
}
func (s *supportFake) GitRevision(context.Context, string) (string, error) {
	s.revisionCalls++
	if s.moveRevision && s.revisionCalls > 1 {
		return strings.Repeat("b", 40), nil
	}
	return strings.Repeat("a", 40), nil
}
func (s *supportFake) OperationRecords(limit int) ([]domain.OperationRecord, error) {
	s.recordLimit = limit
	return []domain.OperationRecord{{Operation: "deploy-apply", State: "failed", Summary: "SECRET"}}, s.recordErr
}
func (s *supportFake) SupportCurrentSystems(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.HostSystemProbe {
	s.strictCalls++
	if s.cancelOnHosts != nil {
		s.cancelOnHosts()
	}
	return s.fakeSource.CurrentSystems(ctx, hosts, timeout)
}
func (s *supportFake) WriteSupport(_ context.Context, value domain.SupportSnapshot) domain.SupportExportResult {
	s.writes++
	s.saved = value
	return domain.SupportExportResult{State: "saved"}
}

func TestSupportPreviewReusesInspectorWithoutBuildOrWrite(t *testing.T) {
	source := &supportFake{fakeSource: readyFake()}
	manager := NewSupportManager(source)
	manager.now = func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	snapshot, err := manager.Preview(context.Background(), "/private/SECRET", "2.0.0")
	if err != nil || !snapshot.Valid() {
		t.Fatalf("preview: %v", err)
	}
	if source.built || source.writes != 0 || source.recordLimit != domain.SupportMaxOperations || source.strictCalls != 1 {
		t.Fatal("preview expanded its authority")
	}
	if strings.Contains(snapshot.JSON(), "SECRET") || strings.Contains(snapshot.JSON(), "CONTROLLER-BUILD") {
		t.Fatal("private or unrequested data included")
	}
	result := manager.Export(context.Background(), snapshot)
	if result.State != "saved" || source.writes != 1 || source.saved.JSON() != snapshot.JSON() {
		t.Fatal("export did not preserve reviewed bytes")
	}
	if source.revisionCalls != 3 {
		t.Fatal("export unexpectedly recollected observations")
	}
}

func TestSupportFaultFixturesRemainUsefulWithoutPrivateEvidence(t *testing.T) {
	for _, fixture := range []struct {
		name, id string
		level    domain.Level
		fault    func(*fakeSource)
	}{
		{"cache-unreachable", "CACHE-HEALTH", domain.LevelWarning, func(s *fakeSource) { s.cacheErr = errors.New("SECRET-SUPPORT-FIXTURE token in cache response") }},
		{"key-permissions", "CACHE-SIGNING-KEY", domain.LevelError, func(s *fakeSource) { s.key.PrivateMode = 0644 }},
		{"interface-absent", "NETWORK-INTERFACE", domain.LevelError, func(s *fakeSource) { s.addresses = nil }},
		{"disk-low", "DISK-FREE", domain.LevelWarning, func(s *fakeSource) { s.free = 1 }},
		{"git-dirty", "GIT-WORKTREE", domain.LevelWarning, func(s *fakeSource) { s.git.Dirty = true; s.git.Paths = []string{"SECRET-SUPPORT-FIXTURE"} }},
		{"client-offline", "CLIENT-SSH", domain.LevelWarning, func(s *fakeSource) { s.ssh = nil }},
		{"pxe-stale", "PXE-PREPARATION", domain.LevelWarning, func(s *fakeSource) {
			s.preparation = domain.PXEPreparationState{Present: true, Detail: "SECRET-SUPPORT-FIXTURE"}
		}},
		{"deployment-blocked", "DEPLOYMENT-READY", domain.LevelError, func(s *fakeSource) {
			s.deployment = domain.DeploymentStatus{Issues: []string{"SECRET-SUPPORT-FIXTURE password hash"}}
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			source := &supportFake{fakeSource: readyFake()}
			fixture.fault(source.fakeSource)
			snapshot, err := NewSupportManager(source).Preview(context.Background(), "/private/SECRET-SUPPORT-FIXTURE", "2.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if source.built || source.writes != 0 || strings.Contains(snapshot.JSON(), "SECRET-SUPPORT-FIXTURE") {
				t.Fatal("diagnostics built, wrote, or leaked")
			}
			var decoded struct {
				Doctor struct {
					Findings []struct {
						ID    string
						Level domain.Level
						Guide string
					}
				}
			}
			if err := json.Unmarshal([]byte(snapshot.JSON()), &decoded); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, finding := range decoded.Doctor.Findings {
				if finding.ID == fixture.id {
					found = true
					if finding.Level != fixture.level || finding.Guide != domain.SupportFindingGuide(fixture.id) {
						t.Fatalf("incorrect qualified fault: %+v", finding)
					}
				}
			}
			if !found {
				t.Fatalf("fault %s not retained", fixture.id)
			}
		})
	}
}

func TestSupportInterruptedCollectionDoesNotClaimRevisionConsistency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &supportFake{fakeSource: readyFake(), cancelOnHosts: cancel}
	snapshot, err := NewSupportManager(source).Preview(ctx, "/private", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot.JSON(), "deploymentRevision") || !strings.Contains(snapshot.JSON(), `"hosts": null`) {
		t.Fatal("unchecked revision consistency was retained")
	}
}

func TestSupportCollectionFailuresDoNotLeakErrors(t *testing.T) {
	source := &supportFake{fakeSource: readyFake(), metaErr: errors.New("SECRET password in evaluation error"), recordErr: errors.New("SECRET key in read error")}
	snapshot, err := NewSupportManager(source).Preview(context.Background(), "/SECRET", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot.JSON(), "SECRET") || !strings.Contains(snapshot.JSON(), `"doctor": null`) || !strings.Contains(snapshot.JSON(), `"operations": null`) {
		t.Fatal("unsafe failure payload")
	}
}

func TestSupportRejectsEmptyCancelledAndMovingCheckout(t *testing.T) {
	source := &supportFake{fakeSource: readyFake(), moveRevision: true}
	manager := NewSupportManager(source)
	if manager.Export(context.Background(), domain.SupportSnapshot{}).State != "blocked" || source.writes != 0 {
		t.Fatal("exported zero snapshot")
	}
	snapshot, err := manager.Preview(context.Background(), "/private", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot.JSON(), "deploymentRevision") || !strings.Contains(snapshot.JSON(), `"hosts": null`) {
		t.Fatal("moving checkout claimed one revision")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if manager.Export(ctx, snapshot).State != "cancelled" || source.writes != 0 {
		t.Fatal("cancelled export wrote")
	}
	if _, err := manager.Preview(ctx, "/private", "2.0.0"); err == nil {
		t.Fatal("cancelled preview started")
	}
}
