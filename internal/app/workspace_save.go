package app

import (
	"context"
	"errors"

	"github.com/giovantenne/nixorium/internal/domain"
)

// WorkspaceSaveSource records only the exact candidate just written by the
// workspace boundary, against the reviewed HEAD. It must preserve other index
// paths and reject replacement content, transforms, hooks and remote effects.
type WorkspaceSaveSource interface {
	WorkspaceSource
	RecordWorkspaceCandidate(context.Context, string, string, domain.WorkspaceProfile) (string, error)
}

// WorkspaceSaveManager is the ordinary TUI save. CLI Apply remains file-only.
// A partial write/record is inspection-only: never reuse a consumed workspace
// review token to adopt pre-existing edits or retry an uncertain commit.
type WorkspaceSaveManager struct {
	source WorkspaceSaveSource
	review *GitReviewManager
}

func NewWorkspaceSaveManager(source WorkspaceSaveSource, review *GitReviewManager) WorkspaceSaveManager {
	return WorkspaceSaveManager{source: source, review: review}
}

func (m WorkspaceSaveManager) Save(ctx context.Context, plan domain.WorkspacePlanReport) domain.WorkspaceApplyReport {
	report := domain.WorkspaceApplyReport{SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-save", State: "invalid", Repository: plan.Repository, ManagedFile: domain.WorkspaceFileName, Targets: []domain.WorkspaceTarget{}, Issues: []domain.ValidationIssue{}}
	if plan.HasErrors() || plan.Candidate == nil || plan.Inspection == nil {
		return workspaceApplyIssue(report, "invalid", "review", errors.New("review the workspace before saving"))
	}
	token, err := domain.WorkspaceReviewToken(plan.Repository, *plan.Candidate, *plan.Inspection)
	if err != nil || token != plan.ReviewToken {
		return workspaceApplyIssue(report, "conflict", "review", errors.New("the workspace proposal changed after review"))
	}
	review := m.review.Review(ctx, plan.Repository)
	if review.HasErrors() {
		return workspaceApplyIssue(report, "failed", "storage", errors.New("configuration storage needs attention before saving"))
	}
	for _, change := range review.Changes {
		if change.Path == domain.WorkspaceFileName || change.OriginalPath == domain.WorkspaceFileName {
			return workspaceApplyIssue(report, "conflict", "storage", errors.New("the workspace file already has local changes; inspect them in Git review before saving"))
		}
	}
	data, err := domain.MarshalWorkspaceProfile(*plan.Candidate)
	if err != nil {
		return workspaceApplyIssue(report, "invalid", "candidate", err)
	}
	report = NewWorkspaceManager(m.source).Apply(ctx, plan.Repository, data, plan.ReviewToken)
	report.Operation = "workspace-save"
	if report.HasErrors() || report.State != "saved" {
		return report
	}
	revision, err := m.source.RecordWorkspaceCandidate(ctx, plan.Repository, plan.Inspection.Snapshot.Revision, *plan.Candidate)
	if err != nil {
		report.State, report.RecoveryRequired = "partial", true
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "storage", Message: "The file was written but its local commit could not be confirmed."})
		report.Message = "Inspect the workspace file and Git state before continuing; do not retry the old save. No system application or runtime enablement was requested."
		return report
	}
	report.Recorded, report.Revision = true, revision
	report.Message = "Workspace configuration saved and recorded locally. Apply systems separately; runtime enablement and home reset were not requested."
	return report
}
