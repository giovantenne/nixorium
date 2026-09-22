package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

var packageBaseChannelPattern = regexp.MustCompile(`^nixos-[0-9]{2}\.(05|11)$`)

// NewPackageBaseManager reuses the reviewed storage boundary with a source that
// owns only the deployment's direct nixpkgs input. Plan's first policy boolean
// means allow-unverified for this manager; framework SemVer policy is separate.
func NewPackageBaseManager(source UpdateSource) *UpdateManager {
	return &UpdateManager{source: source, packageBase: true}
}

func (m *UpdateManager) PackageBaseStatus(repository string) domain.PackageBaseStatus {
	r := domain.PackageBaseStatus{Operation: "package-base-status", Repository: repository, Issues: []domain.ValidationIssue{}}
	s, err := m.source.InspectUpdateInput(repository)
	if err != nil {
		r.Issues = append(r.Issues, domain.ValidationIssue{Field: "input", Message: err.Error()})
		return r
	}
	r.Source, r.Channel, r.Revision = "github:"+s.SourcePrefix, s.CurrentRef, s.CurrentRev
	return r
}

func (m *UpdateManager) planPackageBase(ctx context.Context, repository, target string, allowUnverified bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
	r := domain.UpdatePlanReport{SchemaVersion: domain.SchemaVersion, Operation: "package-base-plan", Kind: "package-base", AllowUnverified: allowUnverified,
		GeneratedAt: time.Now().UTC(), State: "blocked", Target: target, Checks: []domain.UpdateCheck{}, Issues: []domain.ValidationIssue{}}
	root, err := filepath.Abs(repository)
	if err != nil {
		return updatePlanIssue(r, "repository", err.Error())
	}
	r.Repository = root
	emitUpdatePlanProgress(progress, domain.UpdatePlanPhaseInspect, "Checking the actual system/package pin", 0, 0)
	snapshot, err := m.source.InspectUpdateInput(root)
	if err != nil {
		return updatePlanIssue(r, "input", err.Error())
	}
	r.CurrentRef, r.CurrentRev = snapshot.CurrentRef, snapshot.CurrentRev
	if target == "" || target == "current" {
		target = snapshot.CurrentRef
		r.Target = target
	}
	if !packageBaseChannelPattern.MatchString(target) {
		return updatePlanIssue(r, "target", "select a stable NixOS channel such as nixos-26.05")
	}
	if target != snapshot.CurrentRef && !allowUnverified {
		return updatePlanIssue(r, "policy", "changing the NixOS channel requires --allow-unverified; builds do not certify runtime or hardware compatibility")
	}
	if target < snapshot.CurrentRef {
		return updatePlanIssue(r, "target", "channel downgrades are not supported; restore the previous reviewed configuration for recovery")
	}
	state, err := m.source.GitState(ctx, root)
	if err != nil || state.Dirty {
		return updatePlanIssue(r, "git", "commit or resolve deployment changes before planning a system update")
	}
	r.Revision, err = m.source.GitRevision(ctx, root)
	if err != nil || !fullGitRevisionPattern.MatchString(r.Revision) {
		return updatePlanIssue(r, "git", "deployment HEAD is not a full stable Git revision")
	}
	var proposal domain.UpdateProposal
	if source, ok := m.source.(UpdateProgressSource); ok {
		proposal, err = source.PrepareUpdateWithProgress(ctx, root, target, progress)
	} else {
		proposal, err = m.source.PrepareUpdate(ctx, root, target)
	}
	if err != nil {
		return updatePlanIssue(r, "proposal", err.Error())
	}
	emitUpdatePlanProgress(progress, domain.UpdatePlanPhaseVerify, "Rechecking the deployment before review", 0, 0)
	after, err := m.source.InspectUpdateInput(root)
	if err != nil || !bytes.Equal(after.FlakeContent, snapshot.FlakeContent) || !bytes.Equal(after.LockContent, snapshot.LockContent) {
		return updatePlanIssue(r, "input", "deployment inputs changed during validation")
	}
	if head, err := m.source.GitRevision(ctx, root); err != nil || head != r.Revision {
		return updatePlanIssue(r, "git", "HEAD changed during validation")
	}
	if state, err := m.source.GitState(ctx, root); err != nil || state.Dirty {
		return updatePlanIssue(r, "git", "worktree changed during validation")
	}
	r.Snapshot, r.Proposal, r.Diff, r.Checks, r.PackageBase = snapshot, proposal, proposal.Diff, proposal.Checks, proposal.PackageBase
	r.TargetChannel, r.CurrentChannel = domain.UpdateChannel(target), domain.UpdateChannel(snapshot.CurrentRef)
	r.Confirmation = "UPDATE"
	if target != snapshot.CurrentRef {
		r.Confirmation = "MIGRATE"
	}
	r.ReviewToken = updateReviewToken(r, snapshot, proposal)
	r.State = "ready"
	r.Checks = append(r.Checks, domain.UpdateCheck{ID: "runtime", State: "unverified", Message: fmt.Sprintf("%s: verify controller boot/services and a selected client before fleet distribution", target)})
	return r
}
