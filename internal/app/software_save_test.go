package app

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSoftwareSaveRecordsOnlyTheReviewedManagedFile(t *testing.T) {
	softwareSource, software := softwareManagerFixture(t)
	plan := software.Plan(context.Background(), ".", domain.SoftwareChangeRequest{
		Package: "gimp",
		Present: true,
		Scope:   domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	commitSource := softwareSaveCommitSource(t, plan.Candidate)
	manager := NewSoftwareSaveManager(
		software,
		NewGitReviewManager(&fakeGitCommitSource{revision: strings.Repeat("a", 40)}),
		NewManagedConfigurationSaveManager(NewGitReviewManager(commitSource), NewGitCommitManager(commitSource)),
	)

	report := manager.Save(context.Background(), plan)
	if report.State != "saved" || report.HasErrors() || report.RecoveryRequired || softwareSource.writes != 1 || commitSource.commits != 1 {
		t.Fatalf("save = %+v, writes=%d commits=%d", report, softwareSource.writes, commitSource.commits)
	}
	if report.ManagedFile != "lab-software.json" || report.Revision == "" {
		t.Fatalf("save evidence = %+v", report)
	}
}

func TestSoftwareSaveRefusesPreExistingChangesToTheManagedFile(t *testing.T) {
	softwareSource, software := softwareManagerFixture(t)
	plan := software.Plan(context.Background(), ".", domain.SoftwareChangeRequest{
		Package: "gimp",
		Present: true,
		Scope:   domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	dirty := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-software.json", Unstaged: "modified", Managed: true}}},
	}
	commitSource := softwareSaveCommitSource(t, plan.Candidate)
	manager := NewSoftwareSaveManager(
		software,
		NewGitReviewManager(dirty),
		NewManagedConfigurationSaveManager(NewGitReviewManager(commitSource), NewGitCommitManager(commitSource)),
	)

	report := manager.Save(context.Background(), plan)
	if report.State != "conflict" || !report.HasErrors() || softwareSource.writes != 0 || commitSource.commits != 0 || !strings.Contains(report.Message, "outside the current save") {
		t.Fatalf("blocked save = %+v, writes=%d commits=%d", report, softwareSource.writes, commitSource.commits)
	}
}

func TestSoftwareSaveRecoversWriteCompletedBeforeLocalRecord(t *testing.T) {
	softwareSource, software := softwareManagerFixture(t)
	plan := software.Plan(context.Background(), ".", domain.SoftwareChangeRequest{
		Package: "gimp",
		Present: true,
		Scope:   domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	candidateData, err := domain.MarshalLabSoftware(plan.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	softwareSource.data = candidateData
	dirty := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-software.json", Unstaged: "modified", Managed: true}}},
	}
	commitSource := softwareSaveCommitSource(t, plan.Candidate)
	manager := NewSoftwareSaveManager(
		software,
		NewGitReviewManager(dirty),
		NewManagedConfigurationSaveManager(NewGitReviewManager(commitSource), NewGitCommitManager(commitSource)),
	)

	report := manager.Save(context.Background(), plan)
	if report.State != "saved" || report.HasErrors() || softwareSource.writes != 0 || commitSource.commits != 1 {
		t.Fatalf("recovered save = %+v, writes=%d commits=%d", report, softwareSource.writes, commitSource.commits)
	}
}

func softwareSaveCommitSource(t *testing.T, candidate domain.LabSoftwareFile) *fakeGitCommitSource {
	t.Helper()
	software, err := domain.MarshalLabSoftware(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-software.json", Unstaged: "modified", Managed: true}}},
		proposal: domain.GitCommitProposal{TreeID: strings.Repeat("c", 40), Diff: domain.GitDiff{Scope: "proposed-commit", Content: "+software\n"}},
		software: software,
	}
}
