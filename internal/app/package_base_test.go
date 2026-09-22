package app

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func baseSourceFixture() *fakeUpdateSource {
	return &fakeUpdateSource{
		revision: strings.Repeat("a", 40),
		snapshot: domain.UpdateInputSnapshot{CurrentRef: "nixos-26.05", SourcePrefix: "NixOS/nixpkgs", CurrentRev: strings.Repeat("b", 40), FlakeContent: []byte("before"), LockContent: []byte("lock-before"), HasLock: true},
		proposal: domain.UpdateProposal{FlakeContent: []byte("after"), LockContent: []byte("lock-after"), PackageBase: &domain.PackageBaseChange{TargetRevision: strings.Repeat("c", 40)}},
	}
}

func TestPackageBaseChannelPolicy(t *testing.T) {
	for _, test := range []struct {
		target       string
		allow, ready bool
	}{
		{"", false, true}, {"current", false, true}, {"nixos-26.05", false, true},
		{"nixos-26.11", false, false}, {"nixos-26.11", true, true},
		{"nixos-25.11", true, false}, {"nixos-26.06", true, false}, {"master", true, false},
	} {
		t.Run(test.target, func(t *testing.T) {
			source := baseSourceFixture()
			manager := NewPackageBaseManager(source)
			plan := manager.Plan(context.Background(), t.TempDir(), test.target, test.allow, false)
			if (!plan.HasErrors()) != test.ready || source.discover != 0 || source.applied != 0 {
				t.Fatalf("%+v source=%+v", plan, source)
			}
			if test.ready {
				if source.prepared != 1 || plan.Kind != "package-base" || plan.ReviewToken == "" {
					t.Fatalf("%+v", plan)
				}
				if test.target == "nixos-26.11" && plan.Confirmation != "MIGRATE" {
					t.Fatal("missing migration acknowledgement")
				}
			} else if source.prepared != 0 {
				t.Fatal("blocked plan built a candidate")
			}
		})
	}
}

func TestPackageBaseReviewedProposalCannotChange(t *testing.T) {
	for _, mutate := range []func(*domain.UpdatePlanReport){
		func(p *domain.UpdatePlanReport) { p.Proposal.LockContent = []byte("unreviewed") },
		func(p *domain.UpdatePlanReport) { p.AllowUnverified = !p.AllowUnverified },
		func(p *domain.UpdatePlanReport) { p.Repository += "-different" },
		func(p *domain.UpdatePlanReport) { p.Kind = "" },
	} {
		source := baseSourceFixture()
		manager := NewPackageBaseManager(source)
		plan := manager.Plan(context.Background(), t.TempDir(), "", false, false)
		mutate(&plan)
		report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
		if !report.HasErrors() || source.applied != 0 {
			t.Fatalf("tampered proposal applied: %+v", report)
		}
	}
	source := baseSourceFixture()
	manager := NewPackageBaseManager(source)
	plan := manager.Plan(context.Background(), t.TempDir(), "", false, false)
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if report.HasErrors() || report.Operation != "package-base-apply" || source.applied != 1 {
		t.Fatalf("%+v", report)
	}
}

func TestPackageBaseSaveRecoveryRejectsChangedHead(t *testing.T) {
	for _, drift := range []bool{false, true} {
		source := updateSaveSource()
		source.snapshot.CurrentRef = "nixos-26.05"
		source.snapshot.SourcePrefix = "NixOS/nixpkgs"
		manager := NewPackageBaseManager(source)
		plan := manager.Plan(context.Background(), t.TempDir(), "", false, false)
		source.snapshot.FlakeContent = append([]byte(nil), source.proposal.FlakeContent...)
		source.snapshot.LockContent = append([]byte(nil), source.proposal.LockContent...)
		source.dirty = true
		source.review = domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "flake.lock", Unstaged: "modified", Managed: true}, {Path: "flake.nix", Unstaged: "modified", Managed: true}}}
		if drift {
			source.revision = strings.Repeat("f", 40)
		}
		review := NewGitReviewManager(source)
		save := NewUpdateSaveManager(manager, source, review, NewManagedConfigurationSaveManager(review, NewGitCommitManager(source)))
		report := save.Save(context.Background(), plan)
		if report.HasErrors() != drift || source.applied != 0 || (source.commits == 1) == drift {
			t.Fatalf("drift=%t report=%+v writes=%d commits=%d", drift, report, source.applied, source.commits)
		}
	}
}
