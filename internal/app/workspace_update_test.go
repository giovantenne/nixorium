package app

import (
	"context"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWorkspaceUpdateMetadataIsDisplayedAndTokenBound(t *testing.T) {
	for _, base := range []bool{false, true} {
		for _, mutation := range []string{"version", "dependency", "target", "seed", "omit-report", "omit-proposal"} {
			source := updateSaveSource()
			manager, target := NewUpdateManager(source), "v1.1.0"
			if base {
				source = baseSourceFixture()
				manager, target = NewPackageBaseManager(source), "current"
			}
			source.proposal.Workspace = &domain.WorkspaceUpdateImpact{
				Current:  &domain.WorkspaceResolution{Packages: []domain.WorkspacePackage{{Package: "vscode", Version: "1.0"}}},
				Proposed: &domain.WorkspaceResolution{Extensions: []domain.WorkspaceExtension{{ID: "example.extension", Version: "2.0"}}},
			}
			plan := manager.Plan(context.Background(), t.TempDir(), target, false, false)
			if plan.HasErrors() || plan.Workspace == nil || plan.Workspace.Proposed.Extensions[0].Version != "2.0" {
				t.Fatalf("metadata missing: %+v", plan)
			}
			switch mutation {
			case "version":
				plan.Workspace.Proposed.Extensions[0].Version = "3.0"
			case "dependency":
				plan.Workspace.Proposed.Extensions[0].RequiredPackages = []string{"nodejs"}
			case "target":
				plan.Workspace.Proposed.Targets = []domain.WorkspaceTarget{{Name: "other", Role: "controller"}}
			case "seed":
				plan.Workspace.Proposed.Seed = "/nix/store/11111111111111111111111111111111-home"
			case "omit-report":
				plan.Workspace = nil
			case "omit-proposal":
				plan.Proposal.Workspace = nil
			}
			result := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
			if result.State != "blocked" || source.applied != 0 {
				t.Fatalf("%s bypassed token binding", mutation)
			}
			review := NewGitReviewManager(source)
			save := NewUpdateSaveManager(manager, source, review, NewManagedConfigurationSaveManager(review, NewGitCommitManager(source)))
			if result := save.Save(context.Background(), plan); result.State != "blocked" || source.applied != 0 || source.commits != 0 {
				t.Fatalf("TUI save accepted %s", mutation)
			}
		}
	}
}
