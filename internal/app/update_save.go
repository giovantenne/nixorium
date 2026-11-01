package app

import (
	"bytes"
	"context"
	"slices"

	"github.com/giovantenne/nixorium/internal/domain"
)

// UpdateSaveManager turns a reviewed update proposal into one bounded local
// save. Repository mechanics remain an internal storage detail for the TUI;
// the explicit update apply and Git commands remain available to advanced CLI
// users.
type UpdateSaveManager struct {
	update *UpdateManager
	source UpdateSource
	review *GitReviewManager
	record ManagedConfigurationSaveManager
}

func NewUpdateSaveManager(update *UpdateManager, source UpdateSource, review *GitReviewManager, record ManagedConfigurationSaveManager) UpdateSaveManager {
	return UpdateSaveManager{update: update, source: source, review: review, record: record}
}

func (m UpdateSaveManager) Save(ctx context.Context, plan domain.UpdatePlanReport) domain.UpdateApplyReport {
	report := domain.UpdateApplyReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "update-save",
		State:         "blocked",
		Repository:    plan.Repository,
		Revision:      plan.Revision,
		Target:        plan.Target,
		RetrySafe:     true,
		Issues:        []domain.ValidationIssue{},
	}
	if plan.HasErrors() {
		report.Issues = append(report.Issues, plan.Issues...)
		report.Message = "The reviewed update is no longer ready; validate the release again."
		return report
	}
	if m.update.packageBase != (plan.Kind == "package-base") || plan.ReviewToken != updateReviewToken(plan, plan.Snapshot, plan.Proposal) {
		return updateSaveIssue(report, "review", "The proposal changed after review; validate it again.")
	}

	review := m.review.Review(ctx, plan.Repository)
	if review.HasErrors() {
		report.Issues = append(report.Issues, review.Issues...)
		report.Message = "Configuration storage needs attention before this update can be saved."
		return report
	}

	// The update saves flake.nix, flake.lock and the files the release's
	// migrations change (ADR 0034), in one commit.
	savePaths := []string{"flake.lock", "flake.nix"}
	for _, file := range plan.Proposal.MigratedFiles {
		savePaths = append(savePaths, file.Path)
	}
	slices.Sort(savePaths)
	savePaths = slices.Compact(savePaths)
	targetChanged := false
	for _, change := range review.Changes {
		if change.OriginalPath != "" || !slices.Contains(savePaths, change.Path) {
			return updateSaveIssue(report, "storage", "Other deployment files changed after review; resolve them in Advanced and validate the release again.")
		}
		targetChanged = true
	}

	if targetChanged {
		if head, err := m.source.GitRevision(ctx, plan.Repository); err != nil || head != plan.Revision {
			return updateSaveIssue(report, "review", "Deployment HEAD changed after review; inspect the pending save before retrying.")
		}
		current, err := m.source.InspectUpdateInput(plan.Repository)
		if err != nil || !updateProposalIsCurrent(current, plan.Proposal) || !m.migratedFilesSaved(plan) {
			return updateSaveIssue(report, "storage", "The update files differ from the reviewed proposal; resolve them in Advanced and validate the release again.")
		}
		report.Updated = true
	} else {
		applied := m.update.ApplyPlan(ctx, plan, plan.ReviewToken)
		if applied.HasErrors() || !applied.Updated {
			applied.Operation = "update-save"
			return applied
		}
		report.Updated = true
	}

	saved := m.record.SaveChanged(ctx, plan.Repository, savePaths)
	report.State = saved.State
	report.Revision = saved.Revision
	report.RecoveryRequired = saved.RecoveryRequired
	report.Issues = append(report.Issues, saved.Issues...)
	if saved.HasErrors() {
		report.State = "partial"
		report.RecoveryRequired = true
		report.Message = "The update files are ready, but saving needs recovery. Running systems were not changed; try again."
		return report
	}
	report.State = "saved"
	report.RetrySafe = false
	report.Message = "Nixorium update saved locally. Running systems were not changed."
	if plan.Kind == "package-base" {
		report.Message = "System and package update saved locally. Running systems were not changed."
	}
	return report
}

// migratedFilesSource checks, when a save is retried, that the migrated
// files on disk are exactly the reviewed result.
type migratedFilesSource interface {
	MigratedFilesSaved(repository string, files []domain.MigratedFile) bool
}

func (m UpdateSaveManager) migratedFilesSaved(plan domain.UpdatePlanReport) bool {
	if len(plan.Proposal.MigratedFiles) == 0 {
		return true
	}
	source, ok := m.source.(migratedFilesSource)
	return ok && source.MigratedFilesSaved(plan.Repository, plan.Proposal.MigratedFiles)
}

func updateProposalIsCurrent(current domain.UpdateInputSnapshot, proposal domain.UpdateProposal) bool {
	return current.HasLock &&
		bytes.Equal(current.FlakeContent, proposal.FlakeContent) &&
		bytes.Equal(current.LockContent, proposal.LockContent)
}

func updateSaveIssue(report domain.UpdateApplyReport, field, message string) domain.UpdateApplyReport {
	report.State = "blocked"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}
