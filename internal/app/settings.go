package app

import (
	"context"
	"fmt"

	"github.com/giovantenne/nixorium/internal/domain"
)

type SettingsSource interface {
	ReadSettings(repository string) ([]byte, error)
	LabMeta(ctx context.Context, repository string) (domain.LabMeta, error)
}

type SettingsManager struct {
	source SettingsSource
}

func NewSettingsManager(source SettingsSource) SettingsManager {
	return SettingsManager{source: source}
}

func (m SettingsManager) Validate(ctx context.Context, repository string) domain.ConfigValidationReport {
	report := domain.ConfigValidationReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "config-validate",
		State:         "invalid",
		Repository:    repository,
		File:          "lab-settings.json",
		Issues:        []domain.ValidationIssue{},
	}
	data, err := m.source.ReadSettings(repository)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{
			Field:   "$",
			Message: err.Error(),
		})
		return report
	}
	_, issues := domain.DecodeLabSettings(data)
	report.Issues = append(report.Issues, issues...)
	if len(report.Issues) > 0 {
		return report
	}
	if _, err := m.source.LabMeta(ctx, repository); err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{
			Field:   "$nix",
			Message: fmt.Sprintf("Nix evaluation rejected the settings: %v", err),
		})
		return report
	}
	report.State = "valid"
	return report
}
