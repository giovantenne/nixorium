package app

import "github.com/giovantenne/nixorium/internal/domain"

const (
	operationLogListLimit = 50
	operationLogReadLimit = int64(64 * 1024)
)

type OperationLogSource interface {
	OperationLogs(int) ([]domain.OperationLogEntry, error)
	OperationLog(string, int64) (domain.OperationLogReport, error)
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
		Logs:          []domain.OperationLogEntry{},
		Issues:        []domain.ValidationIssue{},
	}
	logs, err := m.source.OperationLogs(operationLogListLimit)
	if err != nil {
		report.State = "blocked"
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
