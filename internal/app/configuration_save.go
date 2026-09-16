package app

import (
	"context"
	"sort"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type ManagedConfigurationSaveManager struct {
	review  *GitReviewManager
	commits *GitCommitManager
}

func NewManagedConfigurationSaveManager(review *GitReviewManager, commits *GitCommitManager) ManagedConfigurationSaveManager {
	return ManagedConfigurationSaveManager{review: review, commits: commits}
}

// SaveChanged records only changed paths from an application-owned allowlist.
// It is intended for changes already produced by a typed Nixorium operation,
// such as generated public keys, and never broadens the selection to unrelated
// repository changes.
func (m ManagedConfigurationSaveManager) SaveChanged(ctx context.Context, repository string, allowedPaths []string) domain.ConfigurationSaveReport {
	report := domain.ConfigurationSaveReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "configuration-save",
		State:         "blocked",
		Repository:    repository,
		Paths:         []string{},
		Changes:       []domain.SettingChange{},
		Issues:        []domain.ValidationIssue{},
	}
	allowed := map[string]bool{}
	for _, path := range allowedPaths {
		allowed[path] = true
	}
	review := m.review.Review(ctx, repository)
	if review.HasErrors() {
		report.Issues = append(report.Issues, review.Issues...)
		report.Message = "Configuration storage needs attention before this save can continue."
		return report
	}
	for _, change := range review.Changes {
		if allowed[change.Path] {
			report.Paths = append(report.Paths, change.Path)
		}
	}
	sort.Strings(report.Paths)
	if len(report.Paths) == 0 {
		report.State = "unchanged"
		report.Message = "Configuration is already saved locally."
		return report
	}
	requested := strings.Join(report.Paths, ",")
	plan := m.commits.Plan(ctx, repository, requested)
	if plan.HasErrors() {
		report.State = "partial"
		report.RecoveryRequired = true
		report.Issues = append(report.Issues, plan.Issues...)
		report.Message = "Configuration files are ready, but saving needs recovery. No computer was changed."
		return report
	}
	committed := m.commits.Apply(ctx, repository, requested, plan.ReviewToken)
	report.Revision = committed.Revision
	if committed.HasErrors() || !committed.Committed {
		report.State = "partial"
		report.RecoveryRequired = true
		report.Issues = append(report.Issues, committed.Issues...)
		if committed.Message != "" && len(report.Issues) == 0 {
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "storage", Message: committed.Message})
		}
		report.Message = "Configuration files are ready, but saving needs recovery. No computer was changed."
		return report
	}
	report.State = "saved"
	report.Message = "Configuration saved locally. No computer was changed."
	return report
}

// SettingsSaveManager turns a reviewed settings candidate into one bounded
// application save. Git remains an internal storage detail: the presentation
// receives only a configuration result and never a commit token or Git plan.
type SettingsSaveManager struct {
	settings SettingsManager
	review   *GitReviewManager
	commits  *GitCommitManager
}

func NewSettingsSaveManager(settings SettingsManager, review *GitReviewManager, commits *GitCommitManager) SettingsSaveManager {
	return SettingsSaveManager{settings: settings, review: review, commits: commits}
}

func (m SettingsSaveManager) Save(ctx context.Context, repository string, candidate domain.LabSettingsFile, reviewed domain.ConfigPlanReport) domain.ConfigurationSaveReport {
	const managedPath = "lab-settings.json"
	report := domain.ConfigurationSaveReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "configuration-save",
		State:         "blocked",
		Repository:    repository,
		Paths:         []string{managedPath},
		Changes:       []domain.SettingChange{},
		Issues:        []domain.ValidationIssue{},
	}

	if reviewed.HasErrors() || (reviewed.State != "valid" && reviewed.State != "unchanged") {
		return configurationSaveIssue(report, "review", "the configuration does not have a successful review; review the settings again")
	}
	if reviewed.BaseFingerprint == "" || reviewed.CandidateFingerprint == "" {
		return configurationSaveIssue(report, "review", "the reviewed configuration snapshot is missing; review the settings again")
	}
	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		return configurationSaveIssue(report, "candidate", err.Error())
	}
	if domain.SettingsFingerprint(candidateData) != reviewed.CandidateFingerprint {
		return configurationSaveIssue(report, "review", "the settings changed after review; review them again")
	}
	report.Changes = append(report.Changes, reviewed.Changes...)

	repositoryReview := m.review.Review(ctx, repository)
	if repositoryReview.HasErrors() {
		report.Issues = append(report.Issues, repositoryReview.Issues...)
		report.Message = "Configuration storage needs attention before this save can continue."
		return report
	}
	targetChanged := false
	for _, change := range repositoryReview.Changes {
		if change.Path == managedPath || change.OriginalPath == managedPath {
			targetChanged = true
			break
		}
	}

	currentData, err := m.settings.source.ReadSettings(repository)
	if err != nil {
		return configurationSaveIssue(report, managedPath, err.Error())
	}
	currentFingerprint := domain.SettingsFingerprint(currentData)
	if currentFingerprint == reviewed.CandidateFingerprint {
		if !targetChanged {
			if reviewed.BaseFingerprint != currentFingerprint {
				report.State = "saved"
				report.Message = "Configuration was already saved locally."
			} else {
				report.State = "unchanged"
				report.Message = "Configuration is already current."
			}
			return report
		}
		// A prior attempt may have written the exact reviewed candidate before
		// local recording failed. Only that fingerprint transition is eligible
		// for recovery; arbitrary pre-existing edits remain blocked.
		if reviewed.BaseFingerprint == currentFingerprint {
			return configurationSaveIssue(report, managedPath, "this file already contains changes outside the current save; resolve them in Advanced before continuing")
		}
		return m.recordSettings(ctx, repository, report)
	}

	if reviewed.BaseFingerprint != currentFingerprint {
		return configurationSaveIssue(report, "review", "the settings changed after review; review them again")
	}
	if targetChanged {
		return configurationSaveIssue(report, managedPath, "this file already contains changes outside the current save; resolve them in Advanced before continuing")
	}

	applied := m.settings.ApplyReviewedSettings(repository, candidate, reviewed)
	report.Changes = append(report.Changes[:0], applied.Changes...)
	if applied.HasErrors() || applied.State != "applied" {
		report.State = applied.State
		if report.State != "conflict" && report.State != "invalid" {
			report.State = "failed"
		}
		report.Issues = append(report.Issues, applied.Issues...)
		report.Message = "Configuration was not saved; review the settings and try again."
		return report
	}
	return m.recordSettings(ctx, repository, report)
}

func (m SettingsSaveManager) recordSettings(ctx context.Context, repository string, report domain.ConfigurationSaveReport) domain.ConfigurationSaveReport {
	plan := m.commits.Plan(ctx, repository, "lab-settings.json")
	if plan.HasErrors() {
		report.State = "partial"
		report.RecoveryRequired = true
		report.Issues = append(report.Issues, plan.Issues...)
		report.Message = "Configuration is stored locally, but saving needs recovery. No computer was changed."
		return report
	}
	committed := m.commits.Apply(ctx, repository, "lab-settings.json", plan.ReviewToken)
	report.Revision = committed.Revision
	if committed.HasErrors() || !committed.Committed {
		report.State = "partial"
		report.RecoveryRequired = true
		report.Issues = append(report.Issues, committed.Issues...)
		if committed.Message != "" && len(report.Issues) == 0 {
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "storage", Message: committed.Message})
		}
		report.Message = "Configuration is stored locally, but saving needs recovery. No computer was changed."
		return report
	}
	report.State = "saved"
	report.Message = "Configuration saved locally. No computer was changed."
	return report
}

func configurationSaveIssue(report domain.ConfigurationSaveReport, field, message string) domain.ConfigurationSaveReport {
	report.State = "blocked"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = "Configuration was not saved. " + message
	return report
}
