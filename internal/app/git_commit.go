package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
)

type GitCommitSource interface {
	GitReview(context.Context, string) (domain.GitReviewSnapshot, error)
	GitRevision(context.Context, string) (string, error)
	GitCommitProposal(context.Context, string, []string) (domain.GitCommitProposal, error)
	CommitGitPaths(context.Context, string, []string, string, string, string) (string, error)
	ReadSettings(string) ([]byte, error)
}

type GitCommitManager struct {
	source GitCommitSource
}

func NewGitCommitManager(source GitCommitSource) *GitCommitManager {
	return &GitCommitManager{source: source}
}

func (m *GitCommitManager) Plan(ctx context.Context, repository, requestedPaths string) domain.GitCommitPlanReport {
	report := domain.GitCommitPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "git-commit-plan",
		GeneratedAt:   time.Now().UTC(),
		State:         "blocked",
		Paths:         []string{},
		Issues:        []domain.ValidationIssue{},
	}
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
	changed := make(map[string]domain.GitChange, len(review.Changes))
	for _, change := range review.Changes {
		changed[change.Path] = change
		if change.Staged == "conflict" || change.Unstaged == "conflict" {
			report = gitCommitPlanIssue(report, "worktree", "Git conflicts must be resolved before using the commit workflow")
		}
	}
	for _, path := range paths {
		change, ok := changed[path]
		if !ok {
			report = gitCommitPlanIssue(report, path, "selected path does not differ from HEAD")
			continue
		}
		if change.Private {
			report = gitCommitPlanIssue(report, path, "private key paths must remain untracked")
		}
		if change.OriginalPath != "" {
			report = gitCommitPlanIssue(report, path, "renamed or copied paths must be committed manually after reviewing both path identities")
		}
	}
	if containsString(paths, "lab-settings.json") {
		content, readErr := m.source.ReadSettings(root)
		if readErr != nil {
			report = gitCommitPlanIssue(report, "lab-settings.json", readErr.Error())
		} else if _, issues := domain.DecodeLabSettings(content); len(issues) > 0 {
			for _, issue := range issues {
				report = gitCommitPlanIssue(report, issue.Field, issue.Message)
			}
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
	report.CommitMessage = generatedGitCommitMessage(paths)
	report.ReviewToken = gitCommitReviewToken(report.Revision, report.TreeID, report.Paths, report.CommitMessage)
	report.Confirmation = "COMMIT " + strings.TrimPrefix(report.ReviewToken, "sha256:")[:12]
	report.State = "ready"
	return report
}

func (m *GitCommitManager) Apply(ctx context.Context, repository, requestedPaths, expectedToken string) domain.GitCommitReport {
	plan := m.Plan(ctx, repository, requestedPaths)
	report := domain.GitCommitReport{
		SchemaVersion:    domain.SchemaVersion,
		Operation:        "git-commit",
		State:            "blocked",
		Repository:       plan.Repository,
		PreviousRevision: plan.Revision,
		Paths:            append([]string{}, plan.Paths...),
		CommitMessage:    plan.CommitMessage,
		RetrySafe:        true,
		Issues:           append([]domain.ValidationIssue{}, plan.Issues...),
	}
	if plan.HasErrors() {
		report.Message = "Git commit preflight failed; no commit was created"
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "review token does not match the current selected changes"})
		report.Message = "Git commit was not created; run a fresh commit plan"
		return report
	}
	revision, err := m.source.CommitGitPaths(ctx, plan.Repository, plan.Paths, plan.CommitMessage, plan.Revision, plan.TreeID)
	if err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("Git commit failed: %v", err)
		if current, revisionErr := m.source.GitRevision(ctx, plan.Repository); revisionErr == nil && current != plan.Revision {
			report.State = "partial"
			report.Revision = current
			report.Committed = true
			report.RetrySafe = false
			report.Message += "; HEAD advanced, so inspect Git state instead of retrying blindly"
		}
		return report
	}
	report.State = "completed"
	report.Revision = revision
	report.Committed = true
	report.RetrySafe = false
	report.Message = "reviewed paths committed locally; no remote push was attempted"
	return report
}

func parseGitCommitPaths(requested string) ([]string, error) {
	if requested == "" {
		return nil, fmt.Errorf("at least one comma-separated path is required")
	}
	parts := strings.Split(requested, ",")
	if len(parts) > 100 {
		return nil, fmt.Errorf("at most 100 paths may be committed together")
	}
	seen := map[string]bool{}
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		path := strings.TrimSpace(part)
		if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || path == ".git" || strings.HasPrefix(path, ".git/") || strings.HasPrefix(path, "../") {
			return nil, fmt.Errorf("path %q must be a clean repository-relative file path outside .git", part)
		}
		for _, character := range path {
			if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
				return nil, fmt.Errorf("path %q contains terminal control characters", part)
			}
		}
		if seen[path] {
			return nil, fmt.Errorf("path %q is duplicated", path)
		}
		seen[path] = true
		paths = append(paths, filepath.ToSlash(path))
	}
	sort.Strings(paths)
	return paths, nil
}

func generatedGitCommitMessage(paths []string) string {
	if len(paths) == 1 && paths[0] == "lab-settings.json" {
		return "chore: update laboratory settings"
	}
	publicKeys := true
	managed := true
	for _, path := range paths {
		if path != "keys/cache-public-key" && path != "keys/admin-ssh.pub" && path != "keys/veyon-public-key.pem" {
			publicKeys = false
		}
		if path != "lab-settings.json" && path != "keys/cache-public-key" && path != "keys/admin-ssh.pub" && path != "keys/veyon-public-key.pem" {
			managed = false
		}
	}
	if publicKeys {
		return "chore: update laboratory public keys"
	}
	if managed {
		return "chore: update laboratory configuration"
	}
	return "chore: update laboratory deployment"
}

func gitCommitReviewToken(revision, tree string, paths []string, message string) string {
	digest := sha256.New()
	for _, value := range append([]string{revision, tree, message}, paths...) {
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", digest.Sum(nil))
}

func gitCommitPlanIssue(report domain.GitCommitPlanReport, field, message string) domain.GitCommitPlanReport {
	report.State = "blocked"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
