package app

import (
	"errors"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeOperationLogSource struct {
	records   []domain.OperationRecord
	logs      []domain.OperationLogEntry
	listLimit int
	detail    domain.OperationLogReport
	showID    string
	showLimit int64
	err       error
}

func (f *fakeOperationLogSource) OperationRecords(limit int) ([]domain.OperationRecord, error) {
	f.listLimit = limit
	return f.records, f.err
}

func (f *fakeOperationLogSource) OperationLogs(limit int) ([]domain.OperationLogEntry, error) {
	f.listLimit = limit
	return f.logs, f.err
}

func (f *fakeOperationLogSource) OperationLog(id string, limit int64) (domain.OperationLogReport, error) {
	f.showID = id
	f.showLimit = limit
	return f.detail, f.err
}

func TestOperationLogsAreBoundedAndPreserveUnavailableEntries(t *testing.T) {
	source := &fakeOperationLogSource{records: []domain.OperationRecord{{Operation: "pxe-start", State: "completed"}}, logs: []domain.OperationLogEntry{
		{ID: "new.log", StartedAt: time.Now(), State: "completed", Available: true},
		{ID: "unsafe.log", State: "unavailable", Detail: "mode is 0644, want 0600"},
	}}
	report := NewOperationLogManager(source).List()
	if source.listLimit != 50 || report.State != "partial" || len(report.Records) != 1 || len(report.Logs) != 2 || len(report.Issues) != 1 {
		t.Fatalf("report = %+v, limit = %d", report, source.listLimit)
	}
}

type fakeOperationRecordSink struct {
	record domain.OperationRecord
	err    error
}

func (f *fakeOperationRecordSink) RecordOperation(record domain.OperationRecord) error {
	f.record = record
	return f.err
}

func TestRecordOperationOutcomeUsesOnlyTypedSafeSummary(t *testing.T) {
	sink := &fakeOperationRecordSink{}
	report := domain.DeploymentExecutionReport{
		Operation: "deploy-apply", State: "partial", ColmenaSelector: "pc01,pc02",
		Phase: domain.DeploymentPhaseVerify, Verification: domain.DeploymentVerificationSummary{Attempted: 2, Verified: 1},
		Message: "raw adapter failure that must not be copied",
	}
	if err := RecordOperationOutcome(sink, report); err != nil {
		t.Fatal(err)
	}
	if sink.record.Operation != "deploy-apply" || sink.record.State != "partial" || sink.record.Subject != "pc01,pc02" || sink.record.Summary != "phase verify; verified 1/2 target(s)" {
		t.Fatalf("record = %+v", sink.record)
	}
	if sink.record.Summary == report.Message {
		t.Fatal("raw report message entered the operation record")
	}
}

func TestRecordOperationOutcomeRejectsUnsupportedValues(t *testing.T) {
	if err := RecordOperationOutcome(&fakeOperationRecordSink{}, domain.StatusReport{}); err == nil {
		t.Fatal("unsupported read-only report was recorded")
	}
}

func TestOperationLogsReportDirectoryFailure(t *testing.T) {
	source := &fakeOperationLogSource{err: errors.New("unsafe operation directory")}
	report := NewOperationLogManager(source).List()
	if !report.HasErrors() || report.State != "blocked" || len(report.Logs) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestOperationLogShowUsesFixedReadLimit(t *testing.T) {
	source := &fakeOperationLogSource{detail: domain.OperationLogReport{Operation: "logs-show", State: "available", Content: "result"}}
	report := NewOperationLogManager(source).Show("deploy-id.log")
	if report.HasErrors() || source.showID != "deploy-id.log" || source.showLimit != 64*1024 {
		t.Fatalf("report = %+v, source = %+v", report, source)
	}
}
