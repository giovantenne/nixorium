package app

import (
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type recordedOperations []domain.OperationRecord

func (r recordedOperations) OperationRecords(int) ([]domain.OperationRecord, error) { return r, nil }

func TestDiagnosticsReportTheLastFailedControllerApply(t *testing.T) {
	at := time.Date(2026, 10, 20, 11, 58, 0, 0, time.UTC)
	failed := recordedOperations{
		{RecordedAt: at, Operation: "pxe-stop", State: "completed"},
		{RecordedAt: at, Operation: "controller-apply", State: "failed", Summary: "the controller cannot reach the Internet"},
	}
	finding, found := lastControllerApplyFailure(failed)
	if !found || finding.ID != "LAST-CONTROLLER-APPLY" || !strings.Contains(finding.Evidence, "cannot reach the Internet") || !strings.Contains(finding.Remediation, "Apply to controller") {
		t.Fatalf("finding = %+v, %v", finding, found)
	}
	// A later success clears it; no record at all reports nothing.
	if _, found := lastControllerApplyFailure(recordedOperations{{Operation: "controller-apply", State: "completed"}, failed[1]}); found {
		t.Fatal("a later successful apply still reported the failure")
	}
	if _, found := lastControllerApplyFailure(recordedOperations{}); found {
		t.Fatal("no record reported a failure")
	}
	if _, found := lastControllerApplyFailure(struct{}{}); found {
		t.Fatal("a source without records reported a failure")
	}
}
