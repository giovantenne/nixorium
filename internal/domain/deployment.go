package domain

type DeploymentPhase string

const (
	DeploymentPhasePreflight DeploymentPhase = "preflight"
	DeploymentPhaseBuild     DeploymentPhase = "build"
	DeploymentPhaseApply     DeploymentPhase = "apply"
	DeploymentPhaseComplete  DeploymentPhase = "complete"
)

type DeploymentTarget struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type DeploymentPlanReport struct {
	SchemaVersion   int                `json:"schemaVersion"`
	Operation       string             `json:"operation"`
	State           string             `json:"state"`
	Repository      string             `json:"repository"`
	Requested       string             `json:"requested"`
	Revision        string             `json:"revision,omitempty"`
	ColmenaSelector string             `json:"colmenaSelector,omitempty"`
	Targets         []DeploymentTarget `json:"targets"`
	BuildFirst      bool               `json:"buildFirst"`
	Issues          []ValidationIssue  `json:"issues"`
}

func (r DeploymentPlanReport) HasErrors() bool {
	return len(r.Issues) > 0
}

type DeploymentExecutionReport struct {
	SchemaVersion   int                `json:"schemaVersion"`
	Operation       string             `json:"operation"`
	State           string             `json:"state"`
	Repository      string             `json:"repository"`
	Requested       string             `json:"requested"`
	Revision        string             `json:"revision,omitempty"`
	ColmenaSelector string             `json:"colmenaSelector,omitempty"`
	Targets         []DeploymentTarget `json:"targets"`
	Phase           DeploymentPhase    `json:"phase"`
	BuildCompleted  bool               `json:"buildCompleted"`
	ApplyCompleted  bool               `json:"applyCompleted"`
	RetrySafe       bool               `json:"retrySafe"`
	LogPath         string             `json:"logPath,omitempty"`
	Message         string             `json:"message,omitempty"`
	Issues          []ValidationIssue  `json:"issues"`
}

func (r DeploymentExecutionReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}
