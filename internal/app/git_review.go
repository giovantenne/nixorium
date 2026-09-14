package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type GitReviewSource interface {
	GitReview(context.Context, string) (domain.GitReviewSnapshot, error)
	GitRevision(context.Context, string) (string, error)
}

type GitReviewManager struct {
	source GitReviewSource
}

func NewGitReviewManager(source GitReviewSource) *GitReviewManager {
	return &GitReviewManager{source: source}
}

func (m *GitReviewManager) Review(ctx context.Context, repository string) domain.GitReviewReport {
	report := domain.GitReviewReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "git-review",
		GeneratedAt:   time.Now().UTC(),
		State:         "failed",
		Changes:       []domain.GitChange{},
		Diffs:         []domain.GitDiff{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return gitReviewIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	revision, err := m.source.GitRevision(ctx, root)
	if err != nil {
		return gitReviewIssue(report, "revision", err.Error())
	}
	report.Revision = revision
	snapshot, err := m.source.GitReview(ctx, root)
	if err != nil {
		return gitReviewIssue(report, "worktree", err.Error())
	}
	report.Changes = snapshot.Changes
	report.Diffs = snapshot.Diffs
	currentRevision, err := m.source.GitRevision(ctx, root)
	if err != nil {
		return gitReviewIssue(report, "revision", fmt.Sprintf("recheck HEAD: %v", err))
	}
	if currentRevision != report.Revision {
		return gitReviewIssue(report, "revision", "HEAD changed during review; retry against a stable revision")
	}
	for _, change := range report.Changes {
		if change.Staged != "" {
			report.Summary.Staged++
		}
		if change.Unstaged != "" {
			report.Summary.Unstaged++
		}
		if change.Untracked {
			report.Summary.Untracked++
		}
		if change.Managed {
			report.Summary.Managed++
		} else {
			report.Summary.Unexpected++
		}
		if change.Private {
			report.Summary.Private++
		}
	}
	if report.Summary.Private > 0 {
		report.State = "blocked"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "private-paths", Message: "private key paths are present in Git changes; their contents were not read and they must remain untracked"})
		return report
	}
	if len(report.Changes) == 0 {
		report.State = "clean"
	} else {
		report.State = "changes"
	}
	return report
}

func gitReviewIssue(report domain.GitReviewReport, field, message string) domain.GitReviewReport {
	report.State = "failed"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}
