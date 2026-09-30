package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type workspaceSaveFixture struct {
	*fakeWorkspaceSource
	records   int
	recordErr error
	revision  string
	candidate domain.WorkspaceProfile
}

func (source *workspaceSaveFixture) RecordWorkspaceCandidate(_ context.Context, _ string, revision string, candidate domain.WorkspaceProfile) (string, error) {
	source.records++
	source.revision, source.candidate = revision, candidate
	return strings.Repeat("f", 40), source.recordErr
}

func TestWorkspaceSaveRecordsExactReviewedCandidate(t *testing.T) {
	source, workspace := workspaceManagerFixture()
	plan := workspace.Plan(t.Context(), "/deployment", minimalWorkspace)
	saveSource := &workspaceSaveFixture{fakeWorkspaceSource: source}
	review := NewGitReviewManager(&fakeGitCommitSource{revision: strings.Repeat("a", 40), review: domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "module.nix", Staged: "modified"}}}})
	result := NewWorkspaceSaveManager(saveSource, review).Save(t.Context(), plan)
	if result.HasErrors() || !result.Recorded || result.Revision != strings.Repeat("f", 40) || source.writes != 1 || saveSource.records != 1 || saveSource.revision != plan.Inspection.Snapshot.Revision || !reflect.DeepEqual(saveSource.candidate, *plan.Candidate) {
		t.Fatalf("save: %+v; source: %+v", result, saveSource)
	}
}

func TestWorkspaceSaveRejectsUnsafeOrIncompleteRecords(t *testing.T) {
	for _, mode := range []string{"dirty", "renamed", "tampered", "stale", "write", "durability", "record", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			source, workspace := workspaceManagerFixture()
			plan := workspace.Plan(t.Context(), "/deployment", minimalWorkspace)
			saveSource := &workspaceSaveFixture{fakeWorkspaceSource: source}
			git := &fakeGitCommitSource{revision: strings.Repeat("a", 40)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch mode {
			case "dirty":
				git.review.Changes = []domain.GitChange{{Path: domain.WorkspaceFileName, Untracked: true}}
			case "renamed":
				git.review.Changes = []domain.GitChange{{Path: "other.json", OriginalPath: domain.WorkspaceFileName}}
			case "tampered":
				plan.ReviewToken = "other"
			case "stale":
				source.inspection.Snapshot.Revision = strings.Repeat("e", 40)
			case "write":
				source.writeErr = errors.New("write failed")
			case "durability":
				source.writeErr = domain.ErrWorkspaceDurability
			case "record":
				saveSource.recordErr = errors.New("HEAD advanced but recording is unconfirmed")
			case "cancelled":
				cancel()
			}
			result := NewWorkspaceSaveManager(saveSource, NewGitReviewManager(git)).Save(ctx, plan)
			if !result.HasErrors() || result.Recorded || result.Revision != "" {
				t.Fatalf("unsafe success: %+v", result)
			}
			if mode == "record" || mode == "durability" {
				if !result.RecoveryRequired || result.State != "partial" {
					t.Fatalf("lost uncertainty: %+v", result)
				}
			}
			if mode != "record" && saveSource.records != 0 {
				t.Fatal("recorded after an unsuccessful save")
			}
			if (mode == "dirty" || mode == "renamed" || mode == "tampered" || mode == "stale") && source.writes != 0 {
				t.Fatal("wrote an unauthorized candidate")
			}
		})
	}
}
