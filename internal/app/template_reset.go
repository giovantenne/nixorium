package app

import (
	"context"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

type TemplateResetSource interface {
	TemplateResetCatalog(context.Context, string) domain.TemplateResetCatalog
	PrepareTemplateReset(context.Context, string, string, func(string)) domain.TemplateResetPlan
	ApplyTemplateReset(context.Context, domain.TemplateResetPlan) domain.TemplateResetResult
}

type TemplateResetManager struct{ source TemplateResetSource }

func NewTemplateResetManager(source TemplateResetSource) TemplateResetManager {
	return TemplateResetManager{source: source}
}

func (m TemplateResetManager) Catalog(ctx context.Context, repository string) domain.TemplateResetCatalog {
	root, err := filepath.Abs(repository)
	if err != nil {
		return domain.TemplateResetCatalog{Error: "Could not resolve the deployment directory."}
	}
	return m.source.TemplateResetCatalog(ctx, root)
}

func (m TemplateResetManager) Plan(ctx context.Context, repository, preset string, progress func(string)) domain.TemplateResetPlan {
	root, err := filepath.Abs(repository)
	if err != nil {
		return domain.TemplateResetPlan{State: "blocked", Message: "Could not resolve the deployment directory."}
	}
	plan := m.source.PrepareTemplateReset(ctx, root, preset, progress)
	plan.SchemaVersion = domain.TemplateResetSchemaVersion
	if plan.HasErrors() {
		return plan
	}
	token, err := domain.TemplateResetToken(plan)
	if err != nil {
		plan.State, plan.Message = "blocked", err.Error()
		return plan
	}
	plan.ReviewToken, plan.Confirmation = token, "RESET DEPLOYMENT"
	plan.Message = "Replace the local deployment with the pinned template and selected software profile. Enable the guided student home for the next applied system and boot. Back up first; do not activate, deploy, reboot or push."
	return plan
}

func (m TemplateResetManager) Apply(ctx context.Context, reviewed domain.TemplateResetPlan) domain.TemplateResetResult {
	invalid := domain.TemplateResetResult{State: "blocked", Message: "A fresh complete template reset review is required; no file was changed."}
	if reviewed.HasErrors() || reviewed.ReviewToken == "" || reviewed.Confirmation != "RESET DEPLOYMENT" {
		return invalid
	}
	token, err := domain.TemplateResetToken(reviewed)
	if err != nil || token != reviewed.ReviewToken {
		return invalid
	}
	// The adapter holds both operation and repository locks, rechecks every
	// source byte/path and creates a durable backup before the first mutation.
	return m.source.ApplyTemplateReset(ctx, reviewed)
}
