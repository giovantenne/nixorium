package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

var updateReleasePattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var numericReleaseIdentifier = regexp.MustCompile(`^[0-9]+$`)

type UpdateSource interface {
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
	InspectUpdateInput(string) (domain.UpdateInputSnapshot, error)
	PrepareUpdate(context.Context, string, string) (domain.UpdateProposal, error)
	ApplyPreparedUpdate(context.Context, string, string, domain.UpdateInputSnapshot, domain.UpdateProposal) (bool, error)
}

type UpdateManager struct {
	source UpdateSource
}

func NewUpdateManager(source UpdateSource) *UpdateManager {
	return &UpdateManager{source: source}
}

func (m *UpdateManager) Plan(ctx context.Context, repository, target string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
	report := domain.UpdatePlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "update-plan",
		GeneratedAt:   time.Now().UTC(),
		State:         "blocked",
		Target:        target,
		Checks:        []domain.UpdateCheck{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return updatePlanIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	targetRelease, err := parseUpdateRelease(target)
	if err != nil {
		return updatePlanIssue(report, "target", err.Error())
	}
	report.TargetChannel = targetRelease.Channel()
	if report.TargetChannel == domain.UpdateChannelPrerelease && !allowPrerelease {
		report = updatePlanIssue(report, "target", "prerelease targets require explicit --allow-prerelease")
	}
	state, err := m.source.GitState(ctx, root)
	if err != nil {
		report = updatePlanIssue(report, "git", fmt.Sprintf("inspect worktree: %v", err))
	} else if state.Dirty {
		report = updatePlanIssue(report, "git", fmt.Sprintf("deployment worktree has %d changed path(s); commit or stash them before planning an update", state.Changes))
	}
	revision, err := m.source.GitRevision(ctx, root)
	if err != nil || !fullGitRevisionPattern.MatchString(revision) {
		report = updatePlanIssue(report, "git", "deployment HEAD is not a full stable Git revision")
	} else {
		report.Revision = revision
	}
	snapshot, err := m.source.InspectUpdateInput(root)
	if err != nil {
		return updatePlanIssue(report, "input", err.Error())
	}
	report.CurrentRef = snapshot.CurrentRef
	report.CurrentRev = snapshot.CurrentRev
	if current, parseErr := parseUpdateRelease(snapshot.CurrentRef); parseErr == nil {
		report.CurrentChannel = current.Channel()
		comparison := compareUpdateReleases(targetRelease, current)
		if comparison == 0 {
			report = updatePlanIssue(report, "target", "target release is already configured")
		} else if comparison < 0 {
			report.Downgrade = true
			if !allowDowngrade {
				report = updatePlanIssue(report, "target", "downgrade targets require explicit --allow-downgrade")
			}
		}
	} else {
		report.CurrentChannel = domain.UpdateChannelMoving
	}
	if len(report.Issues) > 0 {
		return report
	}
	proposal, err := m.source.PrepareUpdate(ctx, root, target)
	if err != nil {
		return updatePlanIssue(report, "proposal", err.Error())
	}
	after, err := m.source.InspectUpdateInput(root)
	if err != nil || !bytes.Equal(after.FlakeContent, snapshot.FlakeContent) || !bytes.Equal(after.LockContent, snapshot.LockContent) || after.HasLock != snapshot.HasLock {
		return updatePlanIssue(report, "repository", "flake.nix or flake.lock changed while the update was being planned")
	}
	if current, revisionErr := m.source.GitRevision(ctx, root); revisionErr != nil || current != report.Revision {
		return updatePlanIssue(report, "git", "HEAD changed while the update was being planned")
	}
	if current, stateErr := m.source.GitState(ctx, root); stateErr != nil || current.Dirty {
		return updatePlanIssue(report, "git", "worktree changed while the update was being planned")
	}
	report.Diff = proposal.Diff
	report.Checks = append(report.Checks, proposal.Checks...)
	report.Snapshot = snapshot
	report.Proposal = proposal
	report.ReviewToken = updateReviewToken(report, snapshot, proposal)
	verb := "UPDATE"
	if report.Downgrade {
		verb = "DOWNGRADE"
	}
	report.Confirmation = verb + " NIXORIUM TO " + target
	report.State = "ready"
	return report
}

func (m *UpdateManager) Apply(ctx context.Context, repository, target, expectedToken string, allowPrerelease, allowDowngrade bool) domain.UpdateApplyReport {
	plan := m.Plan(ctx, repository, target, allowPrerelease, allowDowngrade)
	report := domain.UpdateApplyReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "update-apply",
		State:         "blocked",
		Repository:    plan.Repository,
		Revision:      plan.Revision,
		Target:        plan.Target,
		RetrySafe:     true,
		Issues:        append([]domain.ValidationIssue(nil), plan.Issues...),
	}
	if plan.HasErrors() {
		report.Message = "update preflight failed; flake.nix and flake.lock were not changed"
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "review token does not match the current validated update proposal"})
		report.Message = "update was not applied; run a fresh update plan"
		return report
	}
	partial, err := m.source.ApplyPreparedUpdate(ctx, plan.Repository, plan.Revision, plan.Snapshot, plan.Proposal)
	if err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("update write failed: %v", err)
		if partial {
			report.State = "partial"
			report.Updated = true
			report.RetrySafe = false
			report.Message += "; flake.nix and flake.lock may differ, so inspect them and create a fresh plan"
		}
		return report
	}
	report.State = "completed"
	report.Updated = true
	report.RetrySafe = false
	report.Message = "validated Nixorium release written to flake.nix and flake.lock; review and commit it separately before deployment"
	return report
}

func updateReviewToken(report domain.UpdatePlanReport, snapshot domain.UpdateInputSnapshot, proposal domain.UpdateProposal) string {
	digest := sha256.New()
	for _, content := range [][]byte{[]byte(report.Revision), []byte(report.Target), snapshot.FlakeContent, snapshot.LockContent, proposal.FlakeContent, proposal.LockContent} {
		_, _ = digest.Write(content)
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", digest.Sum(nil))
}

func updatePlanIssue(report domain.UpdatePlanReport, field, message string) domain.UpdatePlanReport {
	report.State = "blocked"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}

type updateRelease struct {
	Tag        string
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease string
	Build      string
}

func parseUpdateRelease(tag string) (updateRelease, error) {
	match := updateReleasePattern.FindStringSubmatch(tag)
	if match == nil {
		return updateRelease{}, fmt.Errorf("target %q must be a v-prefixed Semantic Version release tag", tag)
	}
	values := make([]uint64, 3)
	for index := range values {
		value, err := strconv.ParseUint(match[index+1], 10, 64)
		if err != nil {
			return updateRelease{}, fmt.Errorf("target %q has an invalid numeric version", tag)
		}
		values[index] = value
	}
	for _, identifier := range strings.Split(match[4], ".") {
		if len(identifier) > 1 && identifier[0] == '0' && numericReleaseIdentifier.MatchString(identifier) {
			return updateRelease{}, fmt.Errorf("target %q has a numeric prerelease identifier with a leading zero", tag)
		}
	}
	return updateRelease{Tag: tag, Major: values[0], Minor: values[1], Patch: values[2], Prerelease: match[4], Build: match[5]}, nil
}

func (release updateRelease) Channel() domain.UpdateChannel {
	if release.Prerelease != "" {
		return domain.UpdateChannelPrerelease
	}
	return domain.UpdateChannelStable
}

func compareUpdateReleases(left, right updateRelease) int {
	for _, pair := range [][2]uint64{{left.Major, right.Major}, {left.Minor, right.Minor}, {left.Patch, right.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if left.Prerelease == right.Prerelease {
		return 0
	}
	if left.Prerelease == "" {
		return 1
	}
	if right.Prerelease == "" {
		return -1
	}
	leftParts := strings.Split(left.Prerelease, ".")
	rightParts := strings.Split(right.Prerelease, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		if leftParts[index] == rightParts[index] {
			continue
		}
		leftNumeric := numericReleaseIdentifier.MatchString(leftParts[index])
		rightNumeric := numericReleaseIdentifier.MatchString(rightParts[index])
		if leftNumeric && rightNumeric {
			if len(leftParts[index]) < len(rightParts[index]) {
				return -1
			}
			if len(leftParts[index]) > len(rightParts[index]) {
				return 1
			}
			return strings.Compare(leftParts[index], rightParts[index])
		}
		if leftNumeric {
			return -1
		}
		if rightNumeric {
			return 1
		}
		return strings.Compare(leftParts[index], rightParts[index])
	}
	if len(leftParts) < len(rightParts) {
		return -1
	}
	return 1
}
