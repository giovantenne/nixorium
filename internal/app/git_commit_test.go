package app

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeGitCommitSource struct {
	revision string
	review   domain.GitReviewSnapshot
	proposal domain.GitCommitProposal
	settings []byte
	software []byte
	commits  int
}

func (source *fakeGitCommitSource) GitRevision(context.Context, string) (string, error) {
	return source.revision, nil
}
func (source *fakeGitCommitSource) GitReview(context.Context, string) (domain.GitReviewSnapshot, error) {
	return source.review, nil
}
func (source *fakeGitCommitSource) GitCommitProposal(context.Context, string, []string) (domain.GitCommitProposal, error) {
	return source.proposal, nil
}
func (source *fakeGitCommitSource) CommitGitPaths(context.Context, string, []string, string, string, string) (string, error) {
	source.commits++
	source.revision = strings.Repeat("b", 40)
	return source.revision, nil
}
func (source *fakeGitCommitSource) ReadSettings(string) ([]byte, error) { return source.settings, nil }
func (source *fakeGitCommitSource) ReadSoftware(string) ([]byte, error) { return source.software, nil }

func TestGitCommitPlanAcceptsValidatedManagedSoftware(t *testing.T) {
	software, err := domain.MarshalLabSoftware(domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}}})
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "lab-software.json", Unstaged: "modified", Managed: true}}},
		proposal: domain.GitCommitProposal{TreeID: strings.Repeat("c", 40), Diff: domain.GitDiff{Scope: "proposed-commit", Content: "+vlc\n"}},
		software: software,
	}
	plan := NewGitCommitManager(source).Plan(context.Background(), ".", "lab-software.json")
	if plan.HasErrors() || plan.CommitMessage != "chore: update laboratory software" {
		t.Fatalf("software commit plan = %+v", plan)
	}
}

func TestGitCommitPlanAndApplyUseExactReviewToken(t *testing.T) {
	source := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review:   domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "module.nix", Unstaged: "modified"}}},
		proposal: domain.GitCommitProposal{TreeID: strings.Repeat("c", 40), Diff: domain.GitDiff{Scope: "proposed-commit", Content: "+safe\n"}},
	}
	manager := NewGitCommitManager(source)
	plan := manager.Plan(context.Background(), ".", "module.nix")
	if plan.State != "ready" || !strings.HasPrefix(plan.ReviewToken, "sha256:") || !strings.HasPrefix(plan.Confirmation, "COMMIT ") || plan.CommitMessage != "chore: update laboratory deployment" {
		t.Fatalf("plan = %+v", plan)
	}
	blocked := manager.Apply(context.Background(), ".", "module.nix", "sha256:stale")
	if blocked.State != "blocked" || source.commits != 0 {
		t.Fatalf("stale apply = %+v, commits = %d", blocked, source.commits)
	}
	report := manager.Apply(context.Background(), ".", "module.nix", plan.ReviewToken)
	if report.State != "completed" || !report.Committed || report.RetrySafe || source.commits != 1 || report.Revision == report.PreviousRevision {
		t.Fatalf("apply = %+v, commits = %d", report, source.commits)
	}
}

func TestParseGitCommitPathsRejectsTraversalDuplicatesAndGitMetadata(t *testing.T) {
	paths, err := parseGitCommitPaths("modules/site.nix,lab-settings.json")
	if err != nil || strings.Join(paths, ",") != "lab-settings.json,modules/site.nix" {
		t.Fatalf("paths = %v, error = %v", paths, err)
	}
	for _, input := range []string{"", "../secret", "/absolute", ".git/config", "same,same", "bad\npath"} {
		if _, err := parseGitCommitPaths(input); err == nil {
			t.Fatalf("unsafe path input accepted: %q", input)
		}
	}
}

func TestGeneratedGitCommitMessageDescribesNixoriumUpdate(t *testing.T) {
	if message := generatedGitCommitMessage([]string{"flake.lock", "flake.nix"}); message != "chore: update Nixorium" {
		t.Fatalf("update commit message = %q", message)
	}
}

func TestGitCommitPlanRejectsRenameWithoutBuildingProposal(t *testing.T) {
	source := &fakeGitCommitSource{
		revision: strings.Repeat("a", 40),
		review: domain.GitReviewSnapshot{Changes: []domain.GitChange{{
			Path:         "renamed.nix",
			OriginalPath: "original.nix",
			Staged:       "renamed",
		}}},
	}
	report := NewGitCommitManager(source).Plan(context.Background(), ".", "renamed.nix")
	if report.State != "blocked" || len(report.Issues) != 1 || !strings.Contains(report.Issues[0].Message, "both path identities") {
		t.Fatalf("rename plan = %+v", report)
	}
}
