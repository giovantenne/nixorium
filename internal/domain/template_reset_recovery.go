package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Cases of an interrupted template reset, decided from Git state only.
const (
	ResetRecoveryFinished       = "finished"        // the branch already has the reset commit
	ResetRecoveryCompleteBranch = "complete-branch" // files match the reset; only the branch moves
	ResetRecoveryNotStarted     = "not-started"     // files still match the original
	ResetRecoveryRestore        = "restore"         // mixed state; restore the original files
)

// ResetRecoveryConfirmation finishes the reset; RestoreConfirmation returns to
// the configuration from before the reset.
const (
	ResetRecoveryConfirmation = "RECOVERED"
	ResetRestoreConfirmation  = "RESTORE"
)

type TemplateResetRecoveryPlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Case          string            `json:"case,omitempty"`
	Branch        string            `json:"branch,omitempty"`
	Original      string            `json:"original,omitempty"`
	Candidate     string            `json:"candidate,omitempty"`
	Head          string            `json:"head,omitempty"`
	BackupRef     string            `json:"backupRef,omitempty"`
	Changed       []string          `json:"changed,omitempty"`
	ExpiresAt     time.Time         `json:"expiresAt,omitempty"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Confirmation  string            `json:"confirmation,omitempty"`
	Message       string            `json:"message,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p TemplateResetRecoveryPlan) HasErrors() bool {
	return p.State == "blocked" || len(p.Issues) > 0
}

type TemplateResetRecoveryResult struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Case          string            `json:"case,omitempty"`
	Revision      string            `json:"revision,omitempty"`
	RecoveryRef   string            `json:"recoveryRef,omitempty"`
	Archive       string            `json:"archive,omitempty"`
	Message       string            `json:"message,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r TemplateResetRecoveryResult) HasErrors() bool {
	return r.State != "completed" || len(r.Issues) > 0
}

func TemplateResetRecoveryToken(plan TemplateResetRecoveryPlan) string {
	plan.ReviewToken, plan.Message, plan.State, plan.Confirmation = "", "", "", ""
	plan.ExpiresAt = plan.ExpiresAt.UTC()
	content, _ := json.Marshal(plan)
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}
