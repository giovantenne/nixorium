package app

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSoftwarePresetSaveRecordsOneReviewedCandidate(t *testing.T) {
	softwareSource, software := softwarePresetFixture(t)
	softwareSource.definition.Controller = "pc99"
	plan := software.PlanPreset(context.Background(), ".", domain.SoftwarePresetRequest{
		Preset: "essential",
		Scope:  domain.SoftwareScope{Kind: domain.SoftwareScopeShared},
	})
	commitSource := softwareSaveCommitSource(t, plan.Candidate)
	manager := NewSoftwarePresetSaveManager(
		software,
		NewGitReviewManager(&fakeGitCommitSource{revision: strings.Repeat("a", 40)}),
		NewManagedConfigurationSaveManager(NewGitReviewManager(commitSource), NewGitCommitManager(commitSource)),
	)

	report := manager.Save(context.Background(), plan)
	if report.State != "saved" || report.HasErrors() || report.RecoveryRequired || softwareSource.writes != 1 || commitSource.commits != 1 {
		t.Fatalf("save = %+v, writes=%d commits=%d", report, softwareSource.writes, commitSource.commits)
	}
	if report.ManagedFile != "lab-software.json" || report.Revision == "" || report.AffectedController != "pc99" || len(report.Additions) != 2 {
		t.Fatalf("save evidence = %+v", report)
	}
}

func TestSoftwarePresetSaveRefusesPreExistingManagedFileChanges(t *testing.T) {
	softwareSource, software := softwarePresetFixture(t)
	plan := software.PlanPreset(context.Background(), ".", domain.SoftwarePresetRequest{
		Preset: "essential",
		Scope:  domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	dirty := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review: domain.GitReviewSnapshot{Changes: []domain.GitChange{{
			Path: "lab-software.json", Unstaged: "modified", Managed: true,
		}}},
	}
	commitSource := softwareSaveCommitSource(t, plan.Candidate)
	manager := NewSoftwarePresetSaveManager(
		software,
		NewGitReviewManager(dirty),
		NewManagedConfigurationSaveManager(NewGitReviewManager(commitSource), NewGitCommitManager(commitSource)),
	)

	report := manager.Save(context.Background(), plan)
	if report.State != "conflict" || !report.HasErrors() || softwareSource.writes != 0 || commitSource.commits != 0 || !strings.Contains(report.Message, "outside the current save") {
		t.Fatalf("blocked save = %+v, writes=%d commits=%d", report, softwareSource.writes, commitSource.commits)
	}
}

func TestSoftwarePresetSaveRecoversAfterCandidateWrite(t *testing.T) {
	softwareSource, software := softwarePresetFixture(t)
	plan := software.PlanPreset(context.Background(), ".", domain.SoftwarePresetRequest{
		Preset: "essential",
		Scope:  domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	candidateData, err := domain.MarshalLabSoftware(plan.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	softwareSource.data = candidateData
	dirty := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review: domain.GitReviewSnapshot{Changes: []domain.GitChange{{
			Path: "lab-software.json", Unstaged: "modified", Managed: true,
		}}},
	}
	commitSource := softwareSaveCommitSource(t, plan.Candidate)
	manager := NewSoftwarePresetSaveManager(
		software,
		NewGitReviewManager(dirty),
		NewManagedConfigurationSaveManager(NewGitReviewManager(commitSource), NewGitCommitManager(commitSource)),
	)

	report := manager.Save(context.Background(), plan)
	if report.State != "saved" || report.HasErrors() || softwareSource.writes != 0 || commitSource.commits != 1 {
		t.Fatalf("recovered save = %+v, writes=%d commits=%d", report, softwareSource.writes, commitSource.commits)
	}
}
