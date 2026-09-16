package domain

// ConfigurationSaveReport describes the application-level result of saving a
// managed configuration. Repository mechanics deliberately remain outside the
// ordinary presentation contract.
type ConfigurationSaveReport struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	Paths            []string          `json:"paths"`
	Changes          []SettingChange   `json:"changes"`
	Revision         string            `json:"revision,omitempty"`
	RecoveryRequired bool              `json:"recoveryRequired"`
	Message          string            `json:"message,omitempty"`
	Issues           []ValidationIssue `json:"issues"`
}

func (r ConfigurationSaveReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "partial" || r.State == "conflict" || r.State == "invalid" || len(r.Issues) > 0
}
