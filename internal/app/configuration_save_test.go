package app

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type mutableSettingsSource struct {
	data        []byte
	writes      int
	validations int
}

func (source *mutableSettingsSource) ReadSettings(string) ([]byte, error) {
	return append([]byte{}, source.data...), nil
}

func (*mutableSettingsSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return domain.LabMeta{}, nil
}

func (source *mutableSettingsSource) ValidateCandidate(context.Context, string, domain.LabSettingsFile) error {
	source.validations++
	return nil
}

func (source *mutableSettingsSource) WriteSettingsIfUnchanged(_ string, expected []byte, settings domain.LabSettingsFile) error {
	if !bytes.Equal(expected, source.data) {
		return domain.ErrSettingsConflict
	}
	data, err := domain.MarshalLabSettings(settings)
	if err != nil {
		return err
	}
	source.data = data
	source.writes++
	return nil
}

func TestSettingsSaveRecordsOnlyTheReviewedManagedFile(t *testing.T) {
	baseData, candidate, candidateData := settingsSaveFixture(t)
	settingsSource := &mutableSettingsSource{data: baseData}
	reviewSource := &fakeGitCommitSource{revision: strings.Repeat("a", 40)}
	commitSource := settingsSaveCommitSource(candidateData)
	manager := NewSettingsSaveManager(
		NewSettingsManager(settingsSource),
		NewGitReviewManager(reviewSource),
		NewGitCommitManager(commitSource),
	)

	reviewed := manager.settings.PlanSettings(context.Background(), ".", candidate)
	report := manager.Save(context.Background(), ".", candidate, reviewed)
	if report.State != "saved" || report.HasErrors() || report.RecoveryRequired || settingsSource.writes != 1 || settingsSource.validations != 1 || commitSource.commits != 1 {
		t.Fatalf("save = %+v, writes=%d commits=%d", report, settingsSource.writes, commitSource.commits)
	}
	if len(report.Paths) != 1 || report.Paths[0] != "lab-settings.json" || len(report.Changes) != 1 || report.Revision == "" {
		t.Fatalf("save evidence = %+v", report)
	}
}

func TestSettingsSaveRefusesPreExistingChangesToTheManagedFile(t *testing.T) {
	baseData, candidate, candidateData := settingsSaveFixture(t)
	settingsSource := &mutableSettingsSource{data: baseData}
	reviewSource := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-settings.json", Unstaged: "modified", Managed: true}}},
	}
	commitSource := settingsSaveCommitSource(candidateData)
	manager := NewSettingsSaveManager(NewSettingsManager(settingsSource), NewGitReviewManager(reviewSource), NewGitCommitManager(commitSource))

	report := manager.Save(context.Background(), ".", candidate, reviewedSettingsPlan(baseData, candidateData, candidate))
	if report.State != "blocked" || !report.HasErrors() || settingsSource.writes != 0 || commitSource.commits != 0 || !strings.Contains(report.Message, "outside the current save") {
		t.Fatalf("blocked save = %+v, writes=%d commits=%d", report, settingsSource.writes, commitSource.commits)
	}
}

func TestSettingsSaveRecoversWriteCompletedBeforeLocalRecord(t *testing.T) {
	baseData, candidate, candidateData := settingsSaveFixture(t)
	settingsSource := &mutableSettingsSource{data: candidateData}
	reviewSource := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-settings.json", Unstaged: "modified", Managed: true}}},
	}
	commitSource := settingsSaveCommitSource(candidateData)
	manager := NewSettingsSaveManager(NewSettingsManager(settingsSource), NewGitReviewManager(reviewSource), NewGitCommitManager(commitSource))

	report := manager.Save(context.Background(), ".", candidate, reviewedSettingsPlan(baseData, candidateData, candidate))
	if report.State != "saved" || report.HasErrors() || settingsSource.writes != 0 || commitSource.commits != 1 {
		t.Fatalf("recovered save = %+v, writes=%d commits=%d", report, settingsSource.writes, commitSource.commits)
	}
}

func reviewedSettingsPlan(baseData, candidateData []byte, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
	base, _ := domain.DecodeLabSettings(baseData)
	return domain.ConfigPlanReport{
		State:                "valid",
		BaseFingerprint:      domain.SettingsFingerprint(baseData),
		CandidateFingerprint: domain.SettingsFingerprint(candidateData),
		Changes:              domain.DiffLabSettings(base, candidate),
		Issues:               []domain.ValidationIssue{},
	}
}

func settingsSaveFixture(t *testing.T) ([]byte, domain.LabSettingsFile, []byte) {
	t.Helper()
	baseData, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate, issues := domain.DecodeLabSettings(baseData)
	if len(issues) != 0 {
		t.Fatalf("template issues = %+v", issues)
	}
	candidate.Lab.PCCount++
	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return baseData, candidate, candidateData
}

func settingsSaveCommitSource(settings []byte) *fakeGitCommitSource {
	return &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-settings.json", Unstaged: "modified", Managed: true}}},
		proposal: domain.GitCommitProposal{TreeID: strings.Repeat("c", 40), Diff: domain.GitDiff{Scope: "proposed-commit", Content: "+settings\n"}},
		settings: settings,
	}
}
