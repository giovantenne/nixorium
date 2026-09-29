package app

import (
	"context"
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
	if source.built || source.writes != 0 || source.recordLimit != domain.SupportMaxOperations {
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
