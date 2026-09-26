package app

import (
	"fmt"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	operationLogListLimit = 50
	operationLogReadLimit = int64(64 * 1024)
)

type OperationLogSource interface {
	OperationRecords(int) ([]domain.OperationRecord, error)
	OperationLogs(int) ([]domain.OperationLogEntry, error)
	OperationLog(string, int64) (domain.OperationLogReport, error)
}

type OperationRecordSink interface {
	RecordOperation(domain.OperationRecord) error
}

type OperationLogManager struct {
	source OperationLogSource
}

func NewOperationLogManager(source OperationLogSource) *OperationLogManager {
	return &OperationLogManager{source: source}
}

func (m *OperationLogManager) List() domain.OperationLogsReport {
	report := domain.OperationLogsReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "logs-list",
		State:         "available",
		Limit:         operationLogListLimit,
		Records:       []domain.OperationRecord{},
		Logs:          []domain.OperationLogEntry{},
		Issues:        []domain.ValidationIssue{},
	}
	records, recordErr := m.source.OperationRecords(operationLogListLimit)
	if recordErr != nil {
		report.State = "partial"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "records", Message: recordErr.Error()})
	} else {
		report.Records = records
	}
	logs, err := m.source.OperationLogs(operationLogListLimit)
	if err != nil {
		if recordErr != nil {
			report.State = "blocked"
		} else {
			report.State = "partial"
		}
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "logs", Message: err.Error()})
		return report
	}
	report.Logs = logs
	for _, entry := range logs {
		if !entry.Available {
			report.State = "partial"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: entry.ID, Message: entry.Detail})
		}
	}
	return report
}

func RecordOperationOutcome(sink OperationRecordSink, outcome any) error {
	record, ok := operationRecordFor(outcome)
	if !ok {
		return fmt.Errorf("unsupported operation outcome %T", outcome)
	}
	return sink.RecordOperation(record)
}

func operationRecordFor(outcome any) (domain.OperationRecord, bool) {
	record := domain.OperationRecord{}
	switch report := outcome.(type) {
	case domain.ConfigApplyReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = fmt.Sprintf("%d setting change(s)", len(report.Changes))
		record.Summary = "managed laboratory settings apply finished"
	case domain.KeyReconcileReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = fmt.Sprintf("%d key pair(s)", len(report.Keys))
		record.Summary = "key reconciliation finished"
	case domain.ActionReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = report.Unit
		record.Summary = "fixed controller action finished"
	case domain.PXELifecycleReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = report.Mode
		record.Summary = "PXE lifecycle transition finished"
	case domain.DeploymentExecutionReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = report.ColmenaSelector
		record.Summary = fmt.Sprintf("phase %s; verified %d/%d target(s)", report.Phase, report.Verification.Verified, report.Verification.Attempted)
	case domain.ControllerRebuildExecutionReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = report.Controller
		record.Summary = fmt.Sprintf("phase %s; applied=%t; verified=%t", report.Phase, report.Applied, report.Verified)
	case domain.ServiceActionReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = report.Service
		record.Summary = fmt.Sprintf("%s finished; verified=%t", report.Action, report.Verified)
	case domain.GitCommitReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = fmt.Sprintf("%d path(s)", len(report.Paths))
		record.Summary = fmt.Sprintf("reviewed local commit finished; committed=%t", report.Committed)
	case domain.UpdateApplyReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = report.Target
		record.Summary = fmt.Sprintf("upstream release update finished; files updated=%t", report.Updated)
	case domain.ShutdownApplyReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = fmt.Sprintf("%d selected client(s)", len(report.Targets))
		record.Summary = fmt.Sprintf("shutdown requests: accepted=%d; not-sent=%d; unconfirmed=%d", report.Accepted, report.NotSent, report.Unconfirmed)
	case domain.InternetReport:
		record.Operation = report.Operation
		record.State = report.State
		record.Subject = fmt.Sprintf("%d selected client(s)", len(report.Targets))
		verified := 0
		for _, target := range report.Targets {
			if target.State == "verified" {
				verified++
			}
		}
		record.Summary = fmt.Sprintf("Internet %s: verified=%d; reboot restores Internet", report.Action, verified)
	default:
		return domain.OperationRecord{}, false
	}
	return record, record.Operation != "" && record.State != ""
}

func (m *OperationLogManager) Show(id string) domain.OperationLogReport {
	report, err := m.source.OperationLog(id, operationLogReadLimit)
	if err == nil {
		return report
	}
	return domain.OperationLogReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "logs-show",
		State:         "blocked",
		Issues:        []domain.ValidationIssue{{Field: "log", Message: err.Error()}},
	}
}
