package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeUpdateSource struct {
	revision    string
	dirty       bool
	snapshot    domain.UpdateInputSnapshot
	proposal    domain.UpdateProposal
	releases    []domain.UpdateReleaseRef
	discover    int
	discoverErr error
	prepared    int
	applied     int
	partial     bool
	applyErr    error
	mutate      bool
	review      domain.GitReviewSnapshot
	commitPlan  domain.GitCommitProposal
	commits     int
	lastTarget  string
}

func (source *fakeUpdateSource) DiscoverUpdateReleases(context.Context, string) ([]domain.UpdateReleaseRef, error) {
	source.discover++
	return source.releases, source.discoverErr
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

func (source *fakeUpdateSource) PrepareUpdate(_ context.Context, _, target string) (domain.UpdateProposal, error) {
	source.prepared++
	source.lastTarget = target
	return source.proposal, nil
}

func (source *fakeUpdateSource) ApplyPreparedUpdate(context.Context, string, string, domain.UpdateInputSnapshot, domain.UpdateProposal) (bool, error) {
	source.applied++
	if source.mutate && source.applyErr == nil {
		source.snapshot.FlakeContent = append([]byte(nil), source.proposal.FlakeContent...)
		source.snapshot.LockContent = append([]byte(nil), source.proposal.LockContent...)
		source.snapshot.HasLock = true
		source.dirty = true
		source.review = domain.GitReviewSnapshot{Changes: []domain.GitChange{
			{Path: "flake.lock", Unstaged: "modified", Managed: true},
			{Path: "flake.nix", Unstaged: "modified", Managed: true},
		}}
	}
	return source.partial, source.applyErr
}

func (source *fakeUpdateSource) GitReview(context.Context, string) (domain.GitReviewSnapshot, error) {
	return source.review, nil
}

func (source *fakeUpdateSource) GitCommitProposal(context.Context, string, []string) (domain.GitCommitProposal, error) {
	return source.commitPlan, nil
}

func (source *fakeUpdateSource) CommitGitPaths(context.Context, string, []string, string, string, string) (string, error) {
	source.commits++
	source.revision = strings.Repeat("b", 40)
	source.dirty = false
	source.review = domain.GitReviewSnapshot{}
	return source.revision, nil
}

func (source *fakeUpdateSource) ReadSettings(string) ([]byte, error) { return nil, nil }
func (source *fakeUpdateSource) ReadSoftware(string) ([]byte, error) { return nil, nil }

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

func TestUpdateCheckSeparatesSortsAndBoundsReleaseChannels(t *testing.T) {
	source := &fakeUpdateSource{
		snapshot: domain.UpdateInputSnapshot{
			SourcePrefix: "owner/repo",
			CurrentRef:   "v2.0.0",
			CurrentRev:   strings.Repeat("a", 40),
		},
		releases: []domain.UpdateReleaseRef{
			{Tag: "master", ObjectID: strings.Repeat("a", 40)},
			{Tag: "documentation", ObjectID: strings.Repeat("d", 40)},
			{Tag: "v1.9.0", ObjectID: strings.Repeat("1", 40)},
			{Tag: "v2.1.0-beta.2", ObjectID: strings.Repeat("2", 40)},
			{Tag: "v2.1.0", ObjectID: strings.Repeat("3", 40)},
			{Tag: "v2.1.0-beta.10", ObjectID: strings.Repeat("4", 40)},
		},
	}
	for index := 0; index < maximumUpdateReleasesPerChannel; index++ {
		source.releases = append(source.releases, domain.UpdateReleaseRef{Tag: fmt.Sprintf("v1.%d.0", index), ObjectID: strings.Repeat("5", 40)})
	}
	report := NewUpdateManager(source).Check(context.Background(), ".")
	if report.State != "available" || report.Upstream != "github:owner/repo" || report.CurrentChannel != domain.UpdateChannelStable || source.discover != 1 {
		t.Fatalf("check report = %+v, discovery calls = %d", report, source.discover)
	}
	if len(report.Stable) != maximumUpdateReleasesPerChannel || !report.Truncated || report.Stable[0].Tag != "v2.1.0" {
		t.Fatalf("stable releases = %+v, truncated = %t", report.Stable, report.Truncated)
	}
	if len(report.Prerelease) != 2 || report.Prerelease[0].Tag != "v2.1.0-beta.10" || report.Prerelease[1].Tag != "v2.1.0-beta.2" {
		t.Fatalf("prerelease releases = %+v", report.Prerelease)
	}
	if len(report.Development) != 1 || report.Development[0].Tag != "master" || report.Development[0].Channel != domain.UpdateChannelMoving {
		t.Fatalf("development releases = %+v", report.Development)
	}
}

func TestUpdatePlanAcceptsReviewedMasterTarget(t *testing.T) {
	source := &fakeUpdateSource{
		revision: strings.Repeat("a", 40),
		snapshot: domain.UpdateInputSnapshot{
			SourceURL: "github:owner/repo/v2.0.0", CurrentRef: "v2.0.0",
			FlakeContent: []byte("old flake"), LockContent: []byte("old lock"), HasLock: true,
		},
		proposal: domain.UpdateProposal{FlakeContent: []byte("master flake"), LockContent: []byte("master lock"), Checks: []domain.UpdateCheck{{ID: "controller", State: "passed"}}},
	}
	plan := NewUpdateManager(source).Plan(context.Background(), ".", "master", false, false)
	if plan.State != "ready" || plan.TargetChannel != domain.UpdateChannelMoving || plan.Downgrade || plan.Confirmation != "UPDATE NIXORIUM TO master" || source.lastTarget != "master" {
		t.Fatalf("master plan = %+v, prepared target = %q", plan, source.lastTarget)
	}
}

func TestUpdateCheckReportsDiscoveryFailureWithoutReleaseData(t *testing.T) {
	source := &fakeUpdateSource{
		snapshot:    domain.UpdateInputSnapshot{SourcePrefix: "owner/repo", CurrentRef: "master"},
		discoverErr: errors.New("offline"),
	}
	report := NewUpdateManager(source).Check(context.Background(), ".")
	if report.State != "failed" || report.CurrentChannel != domain.UpdateChannelMoving || len(report.Issues) != 1 || report.Issues[0].Field != "network" || len(report.Stable) != 0 || len(report.Prerelease) != 0 {
		t.Fatalf("failed check = %+v", report)
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

func TestUpdateApplyRequiresCurrentPlanToken(t *testing.T) {
	source := &fakeUpdateSource{
		revision: strings.Repeat("a", 40),
		snapshot: domain.UpdateInputSnapshot{CurrentRef: "v1.0.0", FlakeContent: []byte("old"), LockContent: []byte("old lock"), HasLock: true},
		proposal: domain.UpdateProposal{FlakeContent: []byte("new"), LockContent: []byte("new lock"), Diff: domain.GitDiff{Content: "+new"}},
	}
	manager := NewUpdateManager(source)
	plan := manager.Plan(context.Background(), ".", "v1.1.0", false, false)
	blocked := manager.Apply(context.Background(), ".", "v1.1.0", "sha256:stale", false, false)
	if blocked.State != "blocked" || source.applied != 0 {
		t.Fatalf("blocked apply = %+v, calls = %d", blocked, source.applied)
	}
	report := manager.Apply(context.Background(), ".", "v1.1.0", plan.ReviewToken, false, false)
	if report.State != "completed" || !report.Updated || report.RetrySafe || source.applied != 1 || source.prepared != 3 {
		t.Fatalf("apply = %+v, calls = %d", report, source.applied)
	}
	prepared := source.prepared
	report = manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if report.State != "completed" || source.prepared != prepared {
		t.Fatalf("apply existing plan rebuilt proposal: report=%+v prepared=%d->%d", report, prepared, source.prepared)
	}
}

func TestUpdateSaveWritesAndRecordsOnlyReviewedUpdateFiles(t *testing.T) {
	source := updateSaveSource()
	update := NewUpdateManager(source)
	plan := update.Plan(context.Background(), ".", "v1.1.0", false, false)
	source.mutate = true
	review := NewGitReviewManager(source)
	manager := NewUpdateSaveManager(update, source, review, NewManagedConfigurationSaveManager(review, NewGitCommitManager(source)))

	report := manager.Save(context.Background(), plan)
	if report.State != "saved" || report.HasErrors() || !report.Updated || report.RecoveryRequired || source.applied != 1 || source.commits != 1 {
		t.Fatalf("save = %+v, applied=%d commits=%d", report, source.applied, source.commits)
	}
	if report.Operation != "update-save" || report.Revision != strings.Repeat("b", 40) {
		t.Fatalf("save evidence = %+v", report)
	}
}

func TestUpdateSaveRecoversFilesWrittenBeforeLocalRecord(t *testing.T) {
	source := updateSaveSource()
	update := NewUpdateManager(source)
	plan := update.Plan(context.Background(), ".", "v1.1.0", false, false)
	source.snapshot.FlakeContent = append([]byte(nil), source.proposal.FlakeContent...)
	source.snapshot.LockContent = append([]byte(nil), source.proposal.LockContent...)
	source.snapshot.HasLock = true
	source.dirty = true
	source.review = domain.GitReviewSnapshot{Changes: []domain.GitChange{
		{Path: "flake.lock", Unstaged: "modified", Managed: true},
		{Path: "flake.nix", Unstaged: "modified", Managed: true},
	}}
	review := NewGitReviewManager(source)
	manager := NewUpdateSaveManager(update, source, review, NewManagedConfigurationSaveManager(review, NewGitCommitManager(source)))

	report := manager.Save(context.Background(), plan)
	if report.State != "saved" || report.HasErrors() || source.applied != 0 || source.commits != 1 {
		t.Fatalf("recovered save = %+v, applied=%d commits=%d", report, source.applied, source.commits)
	}
}

func TestUpdateSaveRefusesUnreviewedDeploymentChanges(t *testing.T) {
	source := updateSaveSource()
	update := NewUpdateManager(source)
	plan := update.Plan(context.Background(), ".", "v1.1.0", false, false)
	source.dirty = true
	source.review = domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "hosts.nix", Unstaged: "modified", Managed: false}}}
	review := NewGitReviewManager(source)
	manager := NewUpdateSaveManager(update, source, review, NewManagedConfigurationSaveManager(review, NewGitCommitManager(source)))

	report := manager.Save(context.Background(), plan)
	if report.State != "blocked" || !report.HasErrors() || source.applied != 0 || source.commits != 0 || !strings.Contains(report.Message, "Other deployment files") {
		t.Fatalf("blocked save = %+v, applied=%d commits=%d", report, source.applied, source.commits)
	}
}

func updateSaveSource() *fakeUpdateSource {
	return &fakeUpdateSource{
		revision: strings.Repeat("a", 40),
		snapshot: domain.UpdateInputSnapshot{
			SourceURL: "github:owner/repo/v1.0.0", SourcePrefix: "owner/repo", CurrentRef: "v1.0.0",
			FlakeContent: []byte("old flake"), LockContent: []byte("old lock"), HasLock: true,
		},
		proposal: domain.UpdateProposal{
			FlakeContent: []byte("new flake"), LockContent: []byte("new lock"),
			Diff: domain.GitDiff{Content: "+new"}, Checks: []domain.UpdateCheck{{ID: "controller", State: "passed"}},
		},
		commitPlan: domain.GitCommitProposal{TreeID: strings.Repeat("c", 40), Diff: domain.GitDiff{Scope: "proposed-commit", Content: "+update"}},
	}
}
