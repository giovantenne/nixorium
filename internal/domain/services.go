package domain

type ManagedService struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Purpose  string         `json:"purpose"`
	Mode     string         `json:"mode"`
	State    string         `json:"state"`
	Healthy  bool           `json:"healthy"`
	Detail   string         `json:"detail,omitempty"`
	Actions  []string       `json:"actions"`
	Commands []string       `json:"commands,omitempty"`
	Units    []ServiceState `json:"units"`
}

type ServicesReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Services      []ManagedService  `json:"services"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r ServicesReport) HasErrors() bool {
	return r.State == "degraded" || len(r.Issues) > 0
}

type ServiceActionReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Action        string            `json:"action"`
	Service       string            `json:"service"`
	Unit          string            `json:"unit,omitempty"`
	Previous      ServiceState      `json:"previous"`
	Current       ServiceState      `json:"current"`
	Verified      bool              `json:"verified"`
	RetrySafe     bool              `json:"retrySafe"`
	Message       string            `json:"message,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r ServiceActionReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}
