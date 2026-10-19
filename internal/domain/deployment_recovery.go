package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// DeploymentActivity is one computer's read-only answer after an interrupted
// update: the running revision and whether activation is still in progress.
type DeploymentActivity struct {
	Reachable  bool
	Revision   string
	Activating bool
	Detail     string
	// The last activation result written on the computer by the deployment
	// engine (ADR 0031), if any.
	LastActivationID string
	LastExitCode     *int
}

// Recovery target states.
const (
	RecoveryTargetReviewed    = "finished-reviewed"
	RecoveryTargetOther       = "finished-other"
	RecoveryTargetActivating  = "activating"
	RecoveryTargetUnreachable = "unreachable"
	// RecoveryTargetNotStarted: the engine never started anything on it.
	RecoveryTargetNotStarted = "not-started"
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
	// PXEMode is the network installation mode when a session is still open
	// (active, degraded or recovery-required): a computer installed through
	// it may never have finished. It is advisory and not bound by the token.
	PXEMode      string            `json:"pxeMode,omitempty"`
	// Automatic: every computer's outcome is known from the engine's own
	// records (ADR 0031), so the record may be archived without asking.
	Automatic    bool              `json:"automatic,omitempty"`
	ExpiresAt    time.Time         `json:"expiresAt,omitempty"`
	ReviewToken  string            `json:"reviewToken,omitempty"`
	Confirmation string            `json:"confirmation,omitempty"`
	Message      string            `json:"message,omitempty"`
	Issues       []ValidationIssue `json:"issues"`
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
