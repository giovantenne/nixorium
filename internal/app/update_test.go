package app

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeUpdateSource struct {
	revision string
	dirty    bool
	snapshot domain.UpdateInputSnapshot
	proposal domain.UpdateProposal
	prepared int
}

func (source *fakeUpdateSource) GitState(context.Context, string) (domain.GitState, error) {
	return domain.GitState{Available: true, Dirty: source.dirty, Changes: 1}, nil
}

func (source *fakeUpdateSource) GitRevision(context.Context, string) (string, error) {
	return source.revision, nil
}

func (source *fakeUpdateSource) InspectUpdateInput(string) (domain.UpdateInputSnapshot, error) {
	return source.snapshot, nil
}

func (source *fakeUpdateSource) PrepareUpdate(context.Context, string, string) (domain.UpdateProposal, error) {
	source.prepared++
	return source.proposal, nil
}

func TestParseUpdateReleaseClassifiesAndComparesTargets(t *testing.T) {
	stable, err := parseUpdateRelease("v2.1.0")
	if err != nil || stable.Channel() != domain.UpdateChannelStable {
		t.Fatalf("stable = %+v, error = %v", stable, err)
	}
	prerelease, err := parseUpdateRelease("v2.1.0-beta.3")
	if err != nil || prerelease.Channel() != domain.UpdateChannelPrerelease || compareUpdateReleases(prerelease, stable) >= 0 {
		t.Fatalf("prerelease = %+v, error = %v", prerelease, err)
	}
	older, _ := parseUpdateRelease("v1.9.9")
	if compareUpdateReleases(older, prerelease) >= 0 {
		t.Fatal("older release did not sort below prerelease")
	}
	for _, value := range []string{"2.1.0", "v2.1", "master", "v02.1.0", "v2.1.0-"} {
		if _, err := parseUpdateRelease(value); err == nil {
			t.Fatalf("invalid release accepted: %q", value)
		}
	}
	beta2, _ := parseUpdateRelease("v2.1.0-beta.2+build.7")
	beta10, _ := parseUpdateRelease("v2.1.0-beta.10")
	if compareUpdateReleases(beta2, beta10) >= 0 {
		t.Fatal("numeric prerelease identifiers were not compared numerically")
	}
	if _, err := parseUpdateRelease("v2.1.0-beta.01"); err == nil {
		t.Fatal("leading-zero numeric prerelease identifier was accepted")
	}
}

func TestUpdatePlanRequiresOptInsAndBindsValidatedProposal(t *testing.T) {
	source := &fakeUpdateSource{
		revision: strings.Repeat("a", 40),
		snapshot: domain.UpdateInputSnapshot{
			SourceURL: "github:owner/repo/v2.0.0", CurrentRef: "v2.0.0",
			CurrentRev: strings.Repeat("b", 40), FlakeContent: []byte("old flake"),
			LockContent: []byte("old lock"), HasLock: true,
		},
		proposal: domain.UpdateProposal{
			FlakeContent: []byte("new flake"), LockContent: []byte("new lock"),
			Diff:   domain.GitDiff{Scope: "nixorium-update", Content: "+target"},
			Checks: []domain.UpdateCheck{{ID: "client", State: "passed"}},
		},
	}
	manager := NewUpdateManager(source)
	blocked := manager.Plan(context.Background(), ".", "v1.9.0-beta.1", false, false)
	if blocked.State != "blocked" || len(blocked.Issues) != 2 || source.prepared != 0 {
		t.Fatalf("blocked plan = %+v, prepared = %d", blocked, source.prepared)
	}
	plan := manager.Plan(context.Background(), ".", "v1.9.0-beta.1", true, true)
	if plan.State != "ready" || !plan.Downgrade || plan.TargetChannel != domain.UpdateChannelPrerelease || plan.Confirmation != "DOWNGRADE NIXORIUM TO v1.9.0-beta.1" || !strings.HasPrefix(plan.ReviewToken, "sha256:") || len(plan.Checks) != 1 {
		t.Fatalf("ready plan = %+v", plan)
	}
}

func TestUpdatePlanBlocksDirtyMovingAndUnchangedTargets(t *testing.T) {
	source := &fakeUpdateSource{
		revision: strings.Repeat("a", 40), dirty: true,
		snapshot: domain.UpdateInputSnapshot{CurrentRef: "master"},
	}
	report := NewUpdateManager(source).Plan(context.Background(), ".", "v2.0.0", false, false)
	if report.State != "blocked" || report.CurrentChannel != domain.UpdateChannelMoving || source.prepared != 0 {
		t.Fatalf("dirty moving plan = %+v", report)
	}
	source.dirty = false
	source.snapshot.CurrentRef = "v2.0.0"
	report = NewUpdateManager(source).Plan(context.Background(), ".", "v2.0.0", false, false)
	if report.State != "blocked" || source.prepared != 0 {
		t.Fatalf("unchanged plan = %+v", report)
	}
}
