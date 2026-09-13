package domain

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
