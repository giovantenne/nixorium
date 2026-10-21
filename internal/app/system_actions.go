package app

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

const InstallSecretsUnit = "nixorium-install-secrets.service"
const ApplyControllerUnit = "nixorium-apply-controller.service"
const PreparePXEUnit = "nixorium-prepare-pxe.service"

type SystemActionSource interface {
	StartSystemUnit(ctx context.Context, unit string) error
}

type SystemActions struct {
	source SystemActionSource
}

func NewSystemActions(source SystemActionSource) SystemActions {
	return SystemActions{source: source}
}

func (a SystemActions) unitAction(ctx context.Context, operation, unit, message string) domain.ActionReport {
	report := domain.ActionReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     operation,
		State:         "completed",
		Unit:          unit,
		Message:       message,
	}
	if err := a.source.StartSystemUnit(ctx, unit); err != nil {
		report.State = "failed"
		report.Message = err.Error()
	}
	return report
}

func (a SystemActions) InstallSecrets(ctx context.Context) domain.ActionReport {
	return a.unitAction(ctx, "setup-install-secrets", InstallSecretsUnit, "verified controller key material installed")
}

func (a SystemActions) ApplyController(ctx context.Context) domain.ActionReport {
	return a.unitAction(ctx, "setup-apply-controller", ApplyControllerUnit, "reviewed controller configuration built and activated")
}

func (a SystemActions) PreparePXE(ctx context.Context) domain.ActionReport {
	return a.unitAction(ctx, "pxe-prepare", PreparePXEUnit, "PXE artifacts and client closures prepared")
}
