package app

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

// SoftwarePresetSaveManager turns one reviewed profile candidate into one
// bounded local save. The CLI intentionally keeps using ApplyPresetPlan
// directly; this composed boundary is for the ordinary management workflow.
type SoftwarePresetSaveManager struct {
	software SoftwareManager
	review   *GitReviewManager
	record   ManagedConfigurationSaveManager
}

func NewSoftwarePresetSaveManager(software SoftwareManager, review *GitReviewManager, record ManagedConfigurationSaveManager) SoftwarePresetSaveManager {
	return SoftwarePresetSaveManager{software: software, review: review, record: record}
}

func (m SoftwarePresetSaveManager) Save(ctx context.Context, plan domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
	fresh := m.software.PlanPreset(ctx, plan.Repository, plan.Request)
	review := m.review.Review(ctx, plan.Repository)
	targetChanged := false
	for _, change := range review.Changes {
		if change.Path == "lab-software.json" || change.OriginalPath == "lab-software.json" {
			targetChanged = true
			break
		}
	}
	if review.HasErrors() {
		return softwarePresetSaveFailure(plan, "failed", "storage", "Configuration storage needs attention before this save can continue.", review.Issues)
	}
	if fresh.State == "unchanged" {
		if !targetChanged {
			return domain.SoftwarePresetApplyReport{
				SchemaVersion: domain.SoftwarePresetSchemaVersion,
				Operation:     "software-preset-save", State: "unchanged",
				Repository: plan.Repository, ManagedFile: plan.ManagedFile,
				Request: plan.Request, Preset: plan.Preset,
				Existing:           append([]domain.SoftwareDeclaration{}, fresh.Existing...),
				Additions:          append([]domain.SoftwareDeclaration{}, plan.Additions...),
				AffectedController: plan.AffectedController,
				AffectedClients:    append([]string{}, plan.AffectedClients...),
				Issues:             []domain.ValidationIssue{},
				Message:            "Every selected package is already saved locally; existing scopes were preserved.",
			}
		}
		if plan.BaseFingerprint == "" || plan.BaseFingerprint == fresh.BaseFingerprint {
			return softwarePresetSaveFailure(plan, "conflict", "lab-software.json", "This file already contains changes outside the current save; resolve them in Advanced before continuing.", nil)
		}
		return m.recordPreset(ctx, plan)
	}
	if fresh.HasErrors() {
		return softwarePresetSaveFailure(plan, "failed", "proposal", fresh.Message, fresh.Issues)
	}
	if targetChanged {
		return softwarePresetSaveFailure(plan, "conflict", "lab-software.json", "This file already contains changes outside the current save; resolve them in Advanced before continuing.", nil)
	}
	applied := m.software.ApplyPresetPlan(ctx, plan, plan.ReviewToken)
	if applied.State != "applied" || applied.HasErrors() {
		applied.Operation = "software-preset-save"
		if applied.State == "partial" {
			applied.RecoveryRequired = true
		}
		return applied
	}
	return m.recordPreset(ctx, plan)
}

func (m SoftwarePresetSaveManager) recordPreset(ctx context.Context, plan domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
	saved := m.record.SaveChanged(ctx, plan.Repository, []string{"lab-software.json"})
	return domain.SoftwarePresetApplyReport{
		SchemaVersion: domain.SoftwarePresetSchemaVersion,
		Operation:     "software-preset-save", State: saved.State,
		Repository: plan.Repository, ManagedFile: plan.ManagedFile,
		Request: plan.Request, Preset: plan.Preset,
		Existing:           append([]domain.SoftwareDeclaration{}, plan.Existing...),
		Additions:          append([]domain.SoftwareDeclaration{}, plan.Additions...),
		AffectedController: plan.AffectedController,
		AffectedClients:    append([]string{}, plan.AffectedClients...),
		Revision:           saved.Revision, RecoveryRequired: saved.RecoveryRequired,
		Issues:  append([]domain.ValidationIssue{}, saved.Issues...),
		Message: saved.Message,
	}
}

func softwarePresetSaveFailure(plan domain.SoftwarePresetPlanReport, state, field, message string, issues []domain.ValidationIssue) domain.SoftwarePresetApplyReport {
	report := domain.SoftwarePresetApplyReport{
		SchemaVersion: domain.SoftwarePresetSchemaVersion,
		Operation:     "software-preset-save", State: state,
		Repository: plan.Repository, ManagedFile: plan.ManagedFile,
		Request: plan.Request, Preset: plan.Preset,
		Existing:           append([]domain.SoftwareDeclaration{}, plan.Existing...),
		Additions:          append([]domain.SoftwareDeclaration{}, plan.Additions...),
		AffectedController: plan.AffectedController,
		AffectedClients:    append([]string{}, plan.AffectedClients...),
		Issues:             append([]domain.ValidationIssue{}, issues...), Message: message,
	}
	if len(report.Issues) == 0 {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	}
	return report
}
