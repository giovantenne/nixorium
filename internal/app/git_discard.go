package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// GitDiscardSource restores selected paths to HEAD after saving their current
// content under a private reference.
type GitDiscardSource interface {
	GitReview(context.Context, string) (domain.GitReviewSnapshot, error)
	GitRevision(context.Context, string) (string, error)
	GitCommitProposal(context.Context, string, []string) (domain.GitCommitProposal, error)
	DiscardGitPaths(context.Context, string, []string, string, string) (string, error)
}

type GitDiscardManager struct {
	source GitDiscardSource
}

func NewGitDiscardManager(source GitDiscardSource) *GitDiscardManager {
	return &GitDiscardManager{source: source}
}

// Plan reviews which uncommitted changes would be lost. The proposal tree is
// the current content, kept as the backup when the discard is applied.
func (m *GitDiscardManager) Plan(ctx context.Context, repository, requestedPaths string) domain.GitCommitPlanReport {
	report := domain.GitCommitPlanReport{SchemaVersion: domain.SchemaVersion, Operation: "git-discard-plan", GeneratedAt: time.Now().UTC(), State: "blocked", Paths: []string{}, Issues: []domain.ValidationIssue{}}
	root, err := filepath.Abs(repository)
	if err != nil {
		return gitCommitPlanIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	paths, err := parseGitCommitPaths(requestedPaths)
	if err != nil {
		return gitCommitPlanIssue(report, "paths", err.Error())
	}
	report.Paths = paths
	review := NewGitReviewManager(m.source).Review(ctx, root)
	report.Revision = review.Revision
	if review.HasErrors() {
		report.Issues = append(report.Issues, review.Issues...)
		if len(report.Issues) == 0 {
			report = gitCommitPlanIssue(report, "review", "Git change review is unavailable")
		}
		return report
	}
	changed := map[string]domain.GitChange{}
	for _, change := range review.Changes {
		changed[change.Path] = change
	}
	for _, path := range paths {
		change, ok := changed[path]
		switch {
		case !ok:
			report = gitCommitPlanIssue(report, path, "selected path does not differ from HEAD")
		case change.Private:
			report = gitCommitPlanIssue(report, path, "private key files are never changed by Nixorium")
		case change.Untracked:
			report = gitCommitPlanIssue(report, path, "this file was never saved in Git; delete it yourself if it is not needed")
		case change.Staged == "added":
			report = gitCommitPlanIssue(report, path, "this file is new and not in the last saved version; commit it or remove it from Git manually")
		case change.Staged == "conflict" || change.Unstaged == "conflict":
			report = gitCommitPlanIssue(report, path, "Git conflicts must be resolved manually")
		case change.OriginalPath != "":
			report = gitCommitPlanIssue(report, path, "renamed or copied paths must be restored manually")
		}
	}
	if len(report.Issues) > 0 {
		return report
	}
	proposal, err := m.source.GitCommitProposal(ctx, root, paths)
	if err != nil {
		return gitCommitPlanIssue(report, "proposal", err.Error())
	}
	report.TreeID = proposal.TreeID
	report.Diff = proposal.Diff
	report.Diff.Scope = "discarded-changes"
	report.CommitMessage = "Changes discarded by Nixorium"
	report.ReviewToken = gitCommitReviewToken(report.Revision, report.TreeID, report.Paths, "discard")
	report.Confirmation = domain.GitDiscardConfirmation
	report.State = "ready"
	return report
}

func (m *GitDiscardManager) Apply(ctx context.Context, repository, requestedPaths, expectedToken string) domain.GitCommitReport {
	plan := m.Plan(ctx, repository, requestedPaths)
	report := domain.GitCommitReport{SchemaVersion: domain.SchemaVersion, Operation: "git-discard", State: "blocked", Repository: plan.Repository, PreviousRevision: plan.Revision, Revision: plan.Revision, Paths: append([]string{}, plan.Paths...), RetrySafe: true, Issues: append([]domain.ValidationIssue{}, plan.Issues...)}
	if plan.HasErrors() {
		report.Message = "Nothing was discarded; the review found a problem."
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "review token does not match the current selected changes"})
		report.Message = "Nothing was discarded; create a fresh review."
		return report
	}
	backup, err := m.source.DiscardGitPaths(ctx, plan.Repository, plan.Paths, plan.Revision, plan.TreeID)
	if err != nil {
		report.State = "failed"
		report.BackupRef = backup
		report.Message = "Discard failed: " + err.Error()
		if backup != "" {
			report.Message += "; the content before the discard is kept under " + backup
		}
		report.RetrySafe = false
		return report
	}
	report.State, report.BackupRef = "completed", backup
	report.Message = "The selected files are back to the last saved version. Their discarded content is kept under " + backup + "."
	return report
}
