package app

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeGitReviewSource struct {
	revision string
	snapshot domain.GitReviewSnapshot
	err      error
}

func (source fakeGitReviewSource) GitRevision(context.Context, string) (string, error) {
	if source.err != nil {
		return "", source.err
	}
	return source.revision, nil
}

func (source fakeGitReviewSource) GitReview(context.Context, string) (domain.GitReviewSnapshot, error) {
	return source.snapshot, source.err
}

func TestGitReviewSummarizesManagedAndUnexpectedScopes(t *testing.T) {
	source := fakeGitReviewSource{
		revision: "0123456789abcdef0123456789abcdef01234567",
		snapshot: domain.GitReviewSnapshot{Changes: []domain.GitChange{
			{Path: "lab-settings.json", Staged: "modified", Unstaged: "modified", Managed: true},
			{Path: "module.nix", Unstaged: "modified"},
			{Path: "note", Untracked: true},
		}},
	}
	report := NewGitReviewManager(source).Review(context.Background(), ".")
	if report.State != "changes" || report.Summary.Staged != 1 || report.Summary.Unstaged != 2 || report.Summary.Untracked != 1 || report.Summary.Managed != 1 || report.Summary.Unexpected != 2 {
		t.Fatalf("report = %+v", report)
	}
}

func TestGitReviewBlocksPrivatePathsAndFailsClosed(t *testing.T) {
	private := fakeGitReviewSource{revision: "abc", snapshot: domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "secret-key", Staged: "added", Private: true}}}}
	report := NewGitReviewManager(private).Review(context.Background(), ".")
	if report.State != "blocked" || report.Summary.Private != 1 || len(report.Issues) != 1 {
		t.Fatalf("private report = %+v", report)
	}
	failure := NewGitReviewManager(fakeGitReviewSource{err: errors.New("unavailable")}).Review(context.Background(), ".")
	if failure.State != "failed" || len(failure.Issues) != 1 {
		t.Fatalf("failure report = %+v", failure)
	}
}
