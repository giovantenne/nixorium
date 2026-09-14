package domain

import "time"

type DeploymentPhase string

const (
	DeploymentPhasePreflight DeploymentPhase = "preflight"
	DeploymentPhaseBuild     DeploymentPhase = "build"
	DeploymentPhaseApply     DeploymentPhase = "apply"
	DeploymentPhaseVerify    DeploymentPhase = "verify"
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
	SchemaVersion   int                           `json:"schemaVersion"`
	Operation       string                        `json:"operation"`
	State           string                        `json:"state"`
	Repository      string                        `json:"repository"`
	Requested       string                        `json:"requested"`
	Revision        string                        `json:"revision,omitempty"`
	ColmenaSelector string                        `json:"colmenaSelector,omitempty"`
	Targets         []DeploymentTarget            `json:"targets"`
	Phase           DeploymentPhase               `json:"phase"`
	BuildCompleted  bool                          `json:"buildCompleted"`
	ApplyCompleted  bool                          `json:"applyCompleted"`
	Verification    DeploymentVerificationSummary `json:"verification"`
	RetrySafe       bool                          `json:"retrySafe"`
	LogPath         string                        `json:"logPath,omitempty"`
	Message         string                        `json:"message,omitempty"`
	Issues          []ValidationIssue             `json:"issues"`
}

type LastSuccessfulDeployment struct {
	Revision   string    `json:"revision"`
	SystemPath string    `json:"systemPath"`
	VerifiedAt time.Time `json:"verifiedAt"`
}

type DeploymentTargetVerification struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Revision   string `json:"revision,omitempty"`
	SystemPath string `json:"systemPath,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type DeploymentVerificationSummary struct {
	Attempted int                            `json:"attempted"`
	Verified  int                            `json:"verified"`
	Recorded  int                            `json:"recorded"`
	Targets   []DeploymentTargetVerification `json:"targets"`
	Detail    string                         `json:"detail,omitempty"`
}

type DeploymentHistory struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	Repository    string                              `json:"repository"`
	Hosts         map[string]LastSuccessfulDeployment `json:"hosts"`
}

func (r DeploymentExecutionReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "partial" || len(r.Issues) > 0
}
