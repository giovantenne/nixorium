package domain

import "time"

type OperationLogEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	StartedAt time.Time `json:"startedAt"`
	SizeBytes int64     `json:"sizeBytes"`
	State     string    `json:"state"`
	Available bool      `json:"available"`
	Detail    string    `json:"detail,omitempty"`
}

type OperationLogsReport struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Operation     string              `json:"operation"`
	State         string              `json:"state"`
	Limit         int                 `json:"limit"`
	Logs          []OperationLogEntry `json:"logs"`
	Issues        []ValidationIssue   `json:"issues"`
}

func (r OperationLogsReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "partial" || len(r.Issues) > 0
}

type OperationLogReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Operation     string             `json:"operation"`
	State         string             `json:"state"`
	Log           *OperationLogEntry `json:"log,omitempty"`
	Content       string             `json:"content,omitempty"`
	Truncated     bool               `json:"truncated"`
	Issues        []ValidationIssue  `json:"issues"`
}

func (r OperationLogReport) HasErrors() bool {
	return r.State == "blocked" || len(r.Issues) > 0
}
