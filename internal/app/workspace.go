package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

// WorkspaceSource must inspect a coherent Git-filtered source without saving
// the candidate, composing both deployment validation and resolution hooks.
// The writer locks the deployment root and rechecks the entire snapshot before
// replacing only WorkspaceFileName, with no-follow, atomic, durable semantics.
type WorkspaceSource interface {
	ReadWorkspace(string) ([]byte, error)
	InspectWorkspace(context.Context, string, domain.WorkspaceProfile) (domain.WorkspaceInspection, error)
	WriteWorkspaceIfUnchanged(context.Context, string, domain.WorkspaceSnapshot, domain.WorkspaceProfile) error
}

type WorkspaceManager struct{ source WorkspaceSource }

func NewWorkspaceManager(source WorkspaceSource) WorkspaceManager {
	return WorkspaceManager{source: source}
}

// Load reviews the actual current profile, or an explicit empty proposal for
// a new draft. It never imports the example or applies a system.
func (m WorkspaceManager) Load(ctx context.Context, repository string) domain.WorkspacePlanReport {
	data, err := m.source.ReadWorkspace(repository)
	exists := !errors.Is(err, fs.ErrNotExist)
	if err != nil && exists {
		return workspacePlanIssue(domain.WorkspacePlanReport{
			SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-plan", State: "invalid",
			Repository: repository, ManagedFile: domain.WorkspaceFileName, Issues: []domain.ValidationIssue{},
		}, "file", err)
	}
	if !exists {
		data = []byte(`{"schemaVersion":1}`)
	}
	plan := m.Plan(ctx, repository, data)
	if plan.HasErrors() {
		return plan
	}
	if plan.Inspection.Snapshot.BaseExists != exists {
		return workspacePlanIssue(plan, "source", domain.ErrWorkspaceConflict)
	}
	if exists {
		current, _ := domain.MarshalWorkspaceProfile(*plan.Inspection.Base)
		requested, _ := domain.MarshalWorkspaceProfile(*plan.Candidate)
		if !bytes.Equal(current, requested) {
			return workspacePlanIssue(plan, "source", domain.ErrWorkspaceConflict)
		}
	}
	return plan
}

func (m WorkspaceManager) Plan(ctx context.Context, repository string, data []byte) domain.WorkspacePlanReport {
	report := domain.WorkspacePlanReport{
		SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-plan", State: "invalid",
		Repository: repository, ManagedFile: domain.WorkspaceFileName, Issues: []domain.ValidationIssue{},
	}
	candidate, issues := domain.DecodeWorkspaceProfile(data)
	if len(issues) != 0 {
		report.Issues = issues
		report.Message = "The workspace proposal is invalid; no file was changed."
		return report
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return workspacePlanIssue(report, "repository", err)
	}
	report.Repository, report.Candidate = root, &candidate
	inspection, err := m.source.InspectWorkspace(ctx, root, candidate)
	if err != nil {
		return workspacePlanIssue(report, "inspection", err)
	}
	token, err := domain.WorkspaceReviewToken(root, candidate, inspection)
	if err != nil {
		return workspacePlanIssue(report, "inspection", err)
	}
	report.Inspection, report.ReviewToken = &inspection, token
	report.State, report.Confirmation = "ready", "SAVE"
	report.Message = "Save initial student preferences for the controller and all clients; no commit, build, deployment or home reset is included."
	if inspection.Base != nil {
		base, _ := domain.MarshalWorkspaceProfile(*inspection.Base)
		normalized, _ := domain.MarshalWorkspaceProfile(candidate)
		if bytes.Equal(base, normalized) {
			report.State, report.Confirmation = "unchanged", ""
			report.Message = "The profile already declares these preferences; this does not establish deployed or active home state."
		}
	}
	return report
}

// Apply always resolves a fresh plan, even for an unchanged candidate. A stale
// token is never treated as success merely because somebody else saved it.
func (m WorkspaceManager) Apply(ctx context.Context, repository string, data []byte, expectedToken string) domain.WorkspaceApplyReport {
	report := domain.WorkspaceApplyReport{
		SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-apply", State: "invalid",
		Repository: repository, ManagedFile: domain.WorkspaceFileName,
		Targets: []domain.WorkspaceTarget{}, Issues: []domain.ValidationIssue{},
	}
	if expectedToken == "" {
		return workspaceApplyIssue(report, "conflict", "reviewToken", errors.New("a reviewed workspace token is required"))
	}
	fresh := m.Plan(ctx, repository, data)
	report.Repository = fresh.Repository
	if fresh.HasErrors() {
		report.State, report.Issues, report.Message = fresh.State, fresh.Issues, fresh.Message
		return report
	}
	if expectedToken != fresh.ReviewToken {
		return workspaceApplyIssue(report, "conflict", "reviewToken", errors.New("the workspace proposal changed after review; create a new plan"))
	}
	report.StudentUser = fresh.Inspection.Resolution.StudentUser
	report.Targets = append(report.Targets, fresh.Inspection.Resolution.Targets...)
	if fresh.State == "unchanged" {
		report.State, report.Message = "unchanged", fresh.Message
		return report
	}
	if err := ctx.Err(); err != nil {
		return workspaceApplyIssue(report, "failed", "save", err)
	}
	if err := m.source.WriteWorkspaceIfUnchanged(ctx, fresh.Repository, fresh.Inspection.Snapshot, *fresh.Candidate); err != nil {
		switch {
		case errors.Is(err, domain.ErrWorkspaceConflict):
			return workspaceApplyIssue(report, "conflict", "source", err)
		case errors.Is(err, domain.ErrWorkspaceDurability):
			report = workspaceApplyIssue(report, "partial", "durability", err)
			report.RecoveryRequired = true
			report.Message = "The profile was replaced, but durable storage is unconfirmed. Inspect the file and Git state before reviewing another save; no deployment or home reset was requested."
			return report
		default:
			return workspaceApplyIssue(report, "failed", "save", err)
		}
	}
	report.State = "saved"
	report.Message = "Saved workspace-profile.json only. No commit, build, deployment, runtime enablement or home reset was requested."
	return report
}

func workspacePlanIssue(report domain.WorkspacePlanReport, field string, err error) domain.WorkspacePlanReport {
	report.ReviewToken, report.Confirmation = "", ""
	report.State = "invalid"
	if errors.Is(err, domain.ErrWorkspaceConflict) {
		report.State = "conflict"
	}
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: err.Error()})
	report.Message = "The workspace proposal could not be reviewed; no file was changed."
	return report
}

func workspaceApplyIssue(report domain.WorkspaceApplyReport, state, field string, err error) domain.WorkspaceApplyReport {
	report.State = state
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: err.Error()})
	report.Message = "The workspace profile was not saved."
	return report
}
