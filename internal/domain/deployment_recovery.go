package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// DeploymentRecoveryConfirmation acknowledges an interrupted client update
// after its computers were checked. It never declares that update successful.
const DeploymentRecoveryConfirmation = "RECOVERED"

// DeploymentActivity is one computer's read-only answer after an interrupted
// update: the running revision and whether activation is still in progress.
type DeploymentActivity struct {
	Reachable  bool
	Revision   string
	Activating bool
	Detail     string
}

// Recovery target states.
const (
	RecoveryTargetReviewed    = "finished-reviewed"
	RecoveryTargetOther       = "finished-other"
	RecoveryTargetActivating  = "activating"
	RecoveryTargetUnreachable = "unreachable"
)

type DeploymentRecoveryTarget struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	State    string `json:"state"`
	Revision string `json:"revision,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type DeploymentRecoveryPlan struct {
	SchemaVersion          int                        `json:"schemaVersion"`
	Operation              string                     `json:"operation"`
	State                  string                     `json:"state"`
	Repository             string                     `json:"repository"`
	Pending                PendingDeployment          `json:"pending"`
	Targets                []DeploymentRecoveryTarget `json:"targets"`
	Unreachable            int                        `json:"unreachable"`
	AcknowledgeUnreachable bool                       `json:"acknowledgeUnreachable"`
	ExpiresAt              time.Time                  `json:"expiresAt,omitempty"`
	ReviewToken            string                     `json:"reviewToken,omitempty"`
	Confirmation           string                     `json:"confirmation,omitempty"`
	Message                string                     `json:"message,omitempty"`
	Issues                 []ValidationIssue          `json:"issues"`
}

func (p DeploymentRecoveryPlan) HasErrors() bool {
	return p.State == "blocked" || p.State == "failed" || len(p.Issues) > 0
}

type DeploymentRecoveryResult struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Archive       string            `json:"archive,omitempty"`
	Targets       []string          `json:"targets,omitempty"`
	Message       string            `json:"message,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r DeploymentRecoveryResult) HasErrors() bool {
	return r.State != "completed" || len(r.Issues) > 0
}

func DeploymentRecoveryToken(plan DeploymentRecoveryPlan) string {
	bound := struct {
		Repository  string
		Pending     PendingDeployment
		Targets     []DeploymentRecoveryTarget
		Acknowledge bool
		ExpiresAt   time.Time
	}{plan.Repository, plan.Pending, plan.Targets, plan.AcknowledgeUnreachable, plan.ExpiresAt.UTC()}
	content, _ := json.Marshal(bound)
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}
