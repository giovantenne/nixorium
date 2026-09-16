package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/giovantenne/nixorium/internal/domain"
)

type SettingsSource interface {
	ReadSettings(repository string) ([]byte, error)
	LabMeta(ctx context.Context, repository string) (domain.LabMeta, error)
	ValidateCandidate(ctx context.Context, repository string, settings domain.LabSettingsFile) error
	WriteSettingsIfUnchanged(repository string, expected []byte, settings domain.LabSettingsFile) error
}

func (m SettingsManager) Current(repository string) (domain.LabSettingsFile, error) {
	data, err := m.source.ReadSettings(repository)
	if err != nil {
		return domain.LabSettingsFile{}, err
	}
	settings, issues := domain.DecodeLabSettings(data)
	if len(issues) > 0 {
		return domain.LabSettingsFile{}, fmt.Errorf("%s: %s", issues[0].Field, issues[0].Message)
	}
	return settings, nil
}

func (m SettingsManager) PlanSettings(ctx context.Context, repository string, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
	data, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		return invalidConfigPlan(repository, err)
	}
	return m.Plan(ctx, repository, data)
}

func (m SettingsManager) ApplySettings(ctx context.Context, repository string, candidate domain.LabSettingsFile, expectedFingerprint string) domain.ConfigApplyReport {
	data, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		return domain.ConfigApplyReport{
			SchemaVersion: domain.SchemaVersion,
			Operation:     "config-apply",
			State:         "invalid",
			Repository:    repository,
			File:          "lab-settings.json",
			Changes:       []domain.SettingChange{},
			Issues:        []domain.ValidationIssue{{Field: "$candidate", Message: err.Error()}},
		}
	}
	return m.Apply(ctx, repository, data, expectedFingerprint)
}

// ApplyReviewedSettings applies a candidate already accepted by PlanSettings.
// The candidate and base fingerprints bind this write to that exact review, so
// the save path does not repeat the expensive Nix evaluation.
func (m SettingsManager) ApplyReviewedSettings(repository string, candidate domain.LabSettingsFile, reviewed domain.ConfigPlanReport) domain.ConfigApplyReport {
	report := domain.ConfigApplyReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "config-apply",
		State:         "invalid",
		Repository:    repository,
		File:          "lab-settings.json",
		Changes:       []domain.SettingChange{},
		Issues:        []domain.ValidationIssue{},
	}
	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$candidate", Message: err.Error()})
		return report
	}
	if reviewed.BaseFingerprint == "" || reviewed.CandidateFingerprint == "" || domain.SettingsFingerprint(candidateData) != reviewed.CandidateFingerprint {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$fingerprint", Message: "the candidate no longer matches the reviewed configuration; review it again"})
		return report
	}
	baseData, err := m.source.ReadSettings(repository)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$", Message: err.Error()})
		return report
	}
	base, issues := domain.DecodeLabSettings(baseData)
	if len(issues) > 0 {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$base", Message: "managed settings are invalid; repair them before applying a candidate"})
		return report
	}
	currentFingerprint := domain.SettingsFingerprint(baseData)
	if currentFingerprint == reviewed.CandidateFingerprint {
		report.State = "unchanged"
		return report
	}
	if currentFingerprint != reviewed.BaseFingerprint {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$fingerprint", Message: "the managed settings changed after review; create a new plan"})
		return report
	}
	report.Changes = domain.DiffLabSettings(base, candidate)
	if len(report.Changes) == 0 {
		report.State = "unchanged"
		return report
	}
	if err := m.source.WriteSettingsIfUnchanged(repository, baseData, candidate); err != nil {
		if errors.Is(err, domain.ErrSettingsConflict) {
			report.State = "conflict"
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$fingerprint", Message: "the managed settings changed while applying; create a new plan"})
			return report
		}
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$write", Message: err.Error()})
		return report
	}
	report.State = "applied"
	return report
}

func invalidConfigPlan(repository string, err error) domain.ConfigPlanReport {
	return domain.ConfigPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "config-plan",
		State:         "invalid",
		Repository:    repository,
		File:          "lab-settings.json",
		Changes:       []domain.SettingChange{},
		Issues:        []domain.ValidationIssue{{Field: "$candidate", Message: err.Error()}},
	}
}

func (m SettingsManager) Plan(ctx context.Context, repository string, candidateData []byte) domain.ConfigPlanReport {
	report, _, _ := m.planCandidate(ctx, repository, candidateData)
	return report
}

func (m SettingsManager) Apply(ctx context.Context, repository string, candidateData []byte, expectedFingerprint string) domain.ConfigApplyReport {
	plan, baseData, candidate := m.planCandidate(ctx, repository, candidateData)
	report := domain.ConfigApplyReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "config-apply",
		State:         plan.State,
		Repository:    repository,
		File:          "lab-settings.json",
		Changes:       plan.Changes,
		Issues:        plan.Issues,
	}
	if plan.HasErrors() {
		report.State = "invalid"
		return report
	}
	if expectedFingerprint == "" || expectedFingerprint != plan.BaseFingerprint {
		report.State = "conflict"
		report.Issues = append(report.Issues, domain.ValidationIssue{
			Field:   "$fingerprint",
			Message: "the managed settings changed after review; create a new plan",
		})
		return report
	}
	if len(plan.Changes) == 0 {
		report.State = "unchanged"
		return report
	}
	if err := m.source.WriteSettingsIfUnchanged(repository, baseData, candidate); err != nil {
		if errors.Is(err, domain.ErrSettingsConflict) {
			report.State = "conflict"
			report.Issues = append(report.Issues, domain.ValidationIssue{
				Field:   "$fingerprint",
				Message: "the managed settings changed while applying; create a new plan",
			})
			return report
		}
		report.State = "invalid"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$write", Message: err.Error()})
		return report
	}
	report.State = "applied"
	return report
}

func (m SettingsManager) planCandidate(ctx context.Context, repository string, candidateData []byte) (domain.ConfigPlanReport, []byte, domain.LabSettingsFile) {
	report := domain.ConfigPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "config-plan",
		State:         "invalid",
		Repository:    repository,
		File:          "lab-settings.json",
		Changes:       []domain.SettingChange{},
		Issues:        []domain.ValidationIssue{},
	}
	baseData, err := m.source.ReadSettings(repository)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$", Message: err.Error()})
		return report, nil, domain.LabSettingsFile{}
	}
	base, baseIssues := domain.DecodeLabSettings(baseData)
	if len(baseIssues) > 0 {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "$base", Message: "managed settings are invalid; repair them before applying a candidate"})
		return report, baseData, domain.LabSettingsFile{}
	}
	report.BaseFingerprint = domain.SettingsFingerprint(baseData)
	report.CandidateFingerprint = domain.SettingsFingerprint(candidateData)
	candidate, candidateIssues := domain.DecodeLabSettings(candidateData)
	report.Issues = append(report.Issues, candidateIssues...)
	if len(candidateIssues) > 0 {
		return report, baseData, candidate
	}
	if err := m.source.ValidateCandidate(ctx, repository, candidate); err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{
			Field:   "$nix",
			Message: fmt.Sprintf("Nix evaluation rejected the candidate: %v", err),
		})
		return report, baseData, candidate
	}
	report.Changes = domain.DiffLabSettings(base, candidate)
	if len(report.Changes) == 0 {
		report.State = "unchanged"
	} else {
		report.State = "valid"
	}
	return report, baseData, candidate
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
