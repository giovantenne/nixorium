package app

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

type SoftwareSaveManager struct {
	software SoftwareManager
	review   *GitReviewManager
	record   ManagedConfigurationSaveManager
}

func NewSoftwareSaveManager(software SoftwareManager, review *GitReviewManager, record ManagedConfigurationSaveManager) SoftwareSaveManager {
	return SoftwareSaveManager{software: software, review: review, record: record}
}

func (m SoftwareSaveManager) Save(ctx context.Context, plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
	fresh := m.software.Plan(ctx, plan.Repository, plan.Request)
	review := m.review.Review(ctx, plan.Repository)
	targetChanged := false
	for _, change := range review.Changes {
		if change.Path == "lab-software.json" || change.OriginalPath == "lab-software.json" {
			targetChanged = true
			break
		}
	}
	if review.HasErrors() {
		return softwareSaveFailure(plan, "failed", "storage", "Configuration storage needs attention before this save can continue.", review.Issues)
	}
	if fresh.State == "unchanged" {
		if !targetChanged {
			return domain.SoftwareChangeApplyReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-save", State: "unchanged", Repository: plan.Repository, ManagedFile: plan.ManagedFile, Request: plan.Request, AffectedController: plan.AffectedController, AffectedClients: append([]string{}, plan.AffectedClients...), Issues: []domain.ValidationIssue{}, Message: "Software configuration is already saved locally."}
		}
		if plan.BaseFingerprint == "" || plan.BaseFingerprint == fresh.BaseFingerprint {
			return softwareSaveFailure(plan, "conflict", "lab-software.json", "This file already contains changes outside the current save; resolve them in Advanced before continuing.", nil)
		}
		return m.recordSoftware(ctx, plan)
	}
	if fresh.HasErrors() {
		return softwareSaveFailure(plan, "failed", "proposal", fresh.Message, fresh.Issues)
	}
	if targetChanged {
		return softwareSaveFailure(plan, "conflict", "lab-software.json", "This file already contains changes outside the current save; resolve them in Advanced before continuing.", nil)
	}
	applied := m.software.ApplyPlan(ctx, plan, plan.ReviewToken)
	if applied.State != "applied" || applied.HasErrors() {
		applied.Operation = "software-change-save"
		return applied
	}
	return m.recordSoftware(ctx, plan)
}

func (m SoftwareSaveManager) recordSoftware(ctx context.Context, plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
	saved := m.record.SaveChanged(ctx, plan.Repository, []string{"lab-software.json"})
	report := domain.SoftwareChangeApplyReport{
		SchemaVersion:      domain.SoftwareSchemaVersion,
		Operation:          "software-change-save",
		State:              saved.State,
		Repository:         plan.Repository,
		ManagedFile:        plan.ManagedFile,
		Request:            plan.Request,
		AffectedController: plan.AffectedController,
		AffectedClients:    append([]string{}, plan.AffectedClients...),
		Revision:           saved.Revision,
		RecoveryRequired:   saved.RecoveryRequired,
		Issues:             append([]domain.ValidationIssue{}, saved.Issues...),
		Message:            saved.Message,
	}
	return report
}

func softwareSaveFailure(plan domain.SoftwareChangePlanReport, state, field, message string, issues []domain.ValidationIssue) domain.SoftwareChangeApplyReport {
	result := domain.SoftwareChangeApplyReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-save", State: state, Repository: plan.Repository, ManagedFile: plan.ManagedFile, Request: plan.Request, AffectedController: plan.AffectedController, AffectedClients: append([]string{}, plan.AffectedClients...), Issues: append([]domain.ValidationIssue{}, issues...), Message: message}
	if len(result.Issues) == 0 {
		result.Issues = append(result.Issues, domain.ValidationIssue{Field: field, Message: message})
	}
	return result
}
