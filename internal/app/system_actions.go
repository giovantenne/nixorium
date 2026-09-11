package app

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

const InstallSecretsUnit = "nixorium-install-secrets.service"
const ApplyControllerUnit = "nixorium-apply-controller.service"

type SystemActionSource interface {
	StartSystemUnit(ctx context.Context, unit string) error
}

type SystemActions struct {
	source SystemActionSource
}

func NewSystemActions(source SystemActionSource) SystemActions {
	return SystemActions{source: source}
}

func (a SystemActions) InstallSecrets(ctx context.Context) domain.ActionReport {
	report := domain.ActionReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "setup-install-secrets",
		State:         "completed",
		Unit:          InstallSecretsUnit,
		Message:       "verified controller key material installed",
	}
	if err := a.source.StartSystemUnit(ctx, InstallSecretsUnit); err != nil {
		report.State = "failed"
		report.Message = err.Error()
	}
	return report
}

func (a SystemActions) ApplyController(ctx context.Context) domain.ActionReport {
	report := domain.ActionReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "setup-apply-controller",
		State:         "completed",
		Unit:          ApplyControllerUnit,
		Message:       "reviewed controller configuration built and activated",
	}
	if err := a.source.StartSystemUnit(ctx, ApplyControllerUnit); err != nil {
		report.State = "failed"
		report.Message = err.Error()
	}
	return report
}
