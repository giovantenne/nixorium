package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type ShutdownSessionPolicy string

const (
	ShutdownProtectUnknown     ShutdownSessionPolicy = "protect-unknown"
	ShutdownAcknowledgeUnknown ShutdownSessionPolicy = "acknowledge-unknown"
	// ShutdownRequireIdle is retained as a source-compatibility alias. Active
	// sessions are reviewed warnings, not an eligibility block.
	ShutdownRequireIdle = ShutdownProtectUnknown
)

type ShutdownSessionState string

const (
	ShutdownSessionIdle    ShutdownSessionState = "idle"
	ShutdownSessionActive  ShutdownSessionState = "active"
	ShutdownSessionUnknown ShutdownSessionState = "unknown"
)

type ShutdownObservation struct {
	Reachability Reachability         `json:"reachability"`
	SSH          SSHAvailability      `json:"ssh"`
	Session      ShutdownSessionState `json:"session"`
	Detail       string               `json:"detail,omitempty"`
}

type ShutdownTargetPlan struct {
	Name         string               `json:"name"`
	IP           string               `json:"ip"`
	Reachability Reachability         `json:"reachability"`
	SSH          SSHAvailability      `json:"ssh"`
	Session      ShutdownSessionState `json:"session"`
	Eligible     bool                 `json:"eligible"`
	Detail       string               `json:"detail,omitempty"`
}

type ShutdownPlanReport struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Operation     string                `json:"operation"`
	State         string                `json:"state"`
	Repository    string                `json:"repository"`
	Requested     string                `json:"requested"`
	Policy        ShutdownSessionPolicy `json:"policy"`
	Targets       []ShutdownTargetPlan  `json:"targets"`
	Eligible      int                   `json:"eligible"`
	ExpiresAt     time.Time             `json:"expiresAt,omitempty"`
	ReviewToken   string                `json:"reviewToken,omitempty"`
	Confirmation  string                `json:"confirmation,omitempty"`
	Message       string                `json:"message,omitempty"`
	Issues        []ValidationIssue     `json:"issues"`
}

func (r ShutdownPlanReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}

type ShutdownTargetOutcome struct {
	Name            string `json:"name"`
	State           string `json:"state"`
	Detail          string `json:"detail,omitempty"`
	TechnicalDetail string `json:"technicalDetail,omitempty"`
}

type ShutdownDispatchResult struct {
	Accepted bool
	Detail   string
}

type ShutdownApplyReport struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Operation     string                  `json:"operation"`
	State         string                  `json:"state"`
	Repository    string                  `json:"repository"`
	Requested     string                  `json:"requested"`
	Policy        ShutdownSessionPolicy   `json:"policy"`
	Targets       []ShutdownTargetOutcome `json:"targets"`
	Accepted      int                     `json:"accepted"`
	NotSent       int                     `json:"notSent"`
	Unconfirmed   int                     `json:"unconfirmed"`
	RetrySafe     bool                    `json:"retrySafe"`
	Message       string                  `json:"message,omitempty"`
	Issues        []ValidationIssue       `json:"issues"`
}

func (r ShutdownApplyReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "partial" || len(r.Issues) > 0
}

func ShutdownReviewToken(report ShutdownPlanReport) string {
	type boundTarget struct {
		Name         string
		IP           string
		Reachability Reachability
		SSH          SSHAvailability
		Session      ShutdownSessionState
		Eligible     bool
	}
	targets := make([]boundTarget, 0, len(report.Targets))
	for _, target := range report.Targets {
		targets = append(targets, boundTarget{target.Name, target.IP, target.Reachability, target.SSH, target.Session, target.Eligible})
	}
	bound := struct {
		Repository string
		Requested  string
		Policy     ShutdownSessionPolicy
		Targets    []boundTarget
		ExpiresAt  time.Time
	}{report.Repository, report.Requested, report.Policy, targets, report.ExpiresAt.UTC()}
	content, _ := json.Marshal(bound)
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}
