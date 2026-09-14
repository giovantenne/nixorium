package domain

type ControllerRebuildPhase string

const (
	ControllerRebuildPhasePreflight ControllerRebuildPhase = "preflight"
	ControllerRebuildPhaseApply     ControllerRebuildPhase = "apply"
	ControllerRebuildPhaseVerify    ControllerRebuildPhase = "verify"
	ControllerRebuildPhaseComplete  ControllerRebuildPhase = "complete"
)

type ControllerRebuildPlanReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Controller    string            `json:"controller,omitempty"`
	Revision      string            `json:"revision,omitempty"`
	Current       bool              `json:"current"`
	CurrentDetail string            `json:"currentDetail,omitempty"`
	Confirmation  string            `json:"confirmation,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r ControllerRebuildPlanReport) HasErrors() bool {
	return len(r.Issues) > 0
}

type ControllerRebuildExecutionReport struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Operation     string                 `json:"operation"`
	State         string                 `json:"state"`
	Repository    string                 `json:"repository"`
	Controller    string                 `json:"controller,omitempty"`
	Revision      string                 `json:"revision,omitempty"`
	Phase         ControllerRebuildPhase `json:"phase"`
	Unit          string                 `json:"unit,omitempty"`
	Applied       bool                   `json:"applied"`
	Verified      bool                   `json:"verified"`
	RetrySafe     bool                   `json:"retrySafe"`
	Message       string                 `json:"message,omitempty"`
	Issues        []ValidationIssue      `json:"issues"`
}

func (r ControllerRebuildExecutionReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}
