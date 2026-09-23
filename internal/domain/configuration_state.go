package domain

import "time"

// ConfigurationStateReport is a point-in-time view of the desired deployment
// revision and the evidence observed for the controller and clients. It does
// not turn deployment history into current-state evidence.
type ConfigurationStateReport struct {
	SchemaVersion   int                         `json:"schemaVersion"`
	Operation       string                      `json:"operation"`
	GeneratedAt     time.Time                   `json:"generatedAt"`
	State           string                      `json:"state"`
	Repository      string                      `json:"repository,omitempty"`
	DesiredRevision string                      `json:"desiredRevision,omitempty"`
	Controller      ControllerRebuildPlanReport `json:"controller"`
	Clients         HostsReport                 `json:"clients"`
	Issues          []ValidationIssue           `json:"issues"`
}

func (r ConfigurationStateReport) HasErrors() bool {
	return len(r.Issues) > 0
}

func (r ConfigurationStateReport) ControllerVerified() bool {
	return r.Controller.Operation != "" &&
		r.Controller.State == "current" &&
		r.Controller.Current &&
		!r.Controller.HasErrors() &&
		r.DesiredRevision != "" &&
		r.Controller.Revision == r.DesiredRevision
}
