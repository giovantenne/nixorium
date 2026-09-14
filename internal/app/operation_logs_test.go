package app

import (
	"errors"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeOperationLogSource struct {
	logs      []domain.OperationLogEntry
	listLimit int
	detail    domain.OperationLogReport
	showID    string
	showLimit int64
	err       error
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
	source := &fakeOperationLogSource{logs: []domain.OperationLogEntry{
		{ID: "new.log", StartedAt: time.Now(), State: "completed", Available: true},
		{ID: "unsafe.log", State: "unavailable", Detail: "mode is 0644, want 0600"},
	}}
	report := NewOperationLogManager(source).List()
	if source.listLimit != 50 || report.State != "partial" || len(report.Logs) != 2 || len(report.Issues) != 1 {
		t.Fatalf("report = %+v, limit = %d", report, source.listLimit)
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
