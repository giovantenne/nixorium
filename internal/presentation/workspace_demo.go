package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// This fixture renders the real state machine without operational adapters.
func demoWorkspacePlan() domain.WorkspacePlanReport {
	baseline, _ := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"desktop":{"favorites":["firefox.desktop","code.desktop"]}}`))
	candidate := domain.WorkspaceProfile{SchemaVersion: 1}
	return domain.WorkspacePlanReport{
		SchemaVersion: 1, Operation: "workspace-plan", State: "ready",
		Repository: "/demo/lab", ManagedFile: domain.WorkspaceFileName,
		Candidate: &candidate, ReviewToken: "sha256:synthetic-workspace", Confirmation: "SAVE",
		Message: "Only the profile declaration will be saved. No commit, deployment or reset is included.",
		Inspection: &domain.WorkspaceInspection{
			Snapshot: domain.WorkspaceSnapshot{BaseFingerprint: "sha256:synthetic-absent"},
			Resolution: domain.WorkspaceResolution{
				SchemaVersion: 1, State: "prepared", StudentUser: "student", Declared: candidate, Effective: baseline,
				Targets: []domain.WorkspaceTarget{{Name: "controller", Role: "controller"}, {Name: "pc01", Role: "client"}, {Name: "pc02", Role: "client"}},
				Catalog: domain.WorkspaceCatalog{
					SchemaVersion: 1, Baseline: baseline,
					Applications: []domain.WorkspaceApplication{
						{ID: "firefox.desktop", Package: "firefox", Browser: true},
						{ID: "code.desktop", Package: "vscode"},
						{ID: "org.gnome.Nautilus.desktop", Package: "nautilus"},
					},
					Extensions: []domain.WorkspaceExtension{{ID: "ritwickdey.liveserver", Package: "vscode-extensions.ritwickdey.liveserver", Version: "5.7.9"}},
				},
			},
		},
	}
}

func renderWorkspaceDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	actions.LoadSettings = func(ctx context.Context) (domain.LabSettingsFile, error) { return demoSettings(), nil }
	actions.LoadGitReview = func(ctx context.Context) domain.GitReviewReport {
		return domain.GitReviewReport{State: "changed"}
	}
	actions.LoadPackageBase = func(ctx context.Context) domain.PackageBaseStatus {
		return domain.PackageBaseStatus{Channel: "nixos-26.05", Revision: revision}
	}
	actions.PlanPackageBase = func(ctx context.Context, _ string, _ bool, _ func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
		return demoWorkspaceUpdatePlan()
	}
	actions.SavePackageBase = func(domain.UpdatePlanReport) domain.UpdateApplyReport {
		panic("workspace demo must not apply a system update")
	}
	actions.LoadWorkspace = func(context.Context) domain.WorkspacePlanReport { return demoWorkspacePlan() }
	actions.PlanWorkspace = func(_ context.Context, candidate domain.WorkspaceProfile) domain.WorkspacePlanReport {
		plan := demoWorkspacePlan()
		plan.Candidate = &candidate
		plan.Inspection.Snapshot.Revision = revision
		plan.Inspection.Resolution.Declared = candidate
		plan.Inspection.Resolution.Effective = candidate
		return plan
	}
	actions.SaveWorkspace = func(domain.WorkspacePlanReport) domain.WorkspaceApplyReport {
		return domain.WorkspaceApplyReport{Operation: "workspace-save", State: "saved", Recorded: true, Revision: revision, Message: "Profile saved and recorded locally. No computer or student home changed."}
	}
	actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: revision}
	}
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1000)
	r.key(demoText("a"))
	r.command(r.key(demoText("e")))
	load := r.key(demoText("w"))
	r.capture("Resolve the saved student profile and pinned catalog", 1000)
	r.command(load)
	r.capture("Choose one student workspace section", 1600)
	r.key(demoCode(tea.KeyEnter))
	r.key(demoCode(tea.KeyEnter))
	r.capture("Inherit the ordered favorites from the deployment baseline", 1600)
	r.key(demoText("c"))
	r.key(demoCode(tea.KeyDown))
	r.key(demoCode(tea.KeySpace))
	r.capture("Keep only the editor in the draft favorites", 1600)
	r.key(demoCode(tea.KeyEnter))
	r.command(r.key(demoText("v")))
	r.capture("Review the declaration without activating a system", 2200)
	r.typeAndCapture("SAVE", "Confirm the profile save and local record")
	save := r.key(demoCode(tea.KeyEnter))
	r.capture("Recheck before the atomic save", 900)
	r.command(save)
	r.capture("Saved and recorded does not mean deployed", 2500)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Review controller application separately after saving", 2000)
	r.key(demoCode(tea.KeyEsc))
	r.key(demoCode(tea.KeyEsc))
	r.key(demoCode(tea.KeyEsc))
	r.command(r.key(demoText("b")))
	// Inject a synthetic validated report: demo generation never checks a
	// channel, builds a system or claims these are current vendor versions.
	r.message(dashboardUpdatePlanMsg{report: demoWorkspaceUpdatePlan()})
	r.capture("Compare workspace versions during a system update", 2500)
	return DemoScenario{
		ID: "student-workspace", Title: "Customize the initial student workspace",
		Description: "Edit ordered favorites, review and save one laboratory-wide profile with its local commit. Controller application is offered separately; runtime opt-in, client deployment and boot reset remain separate.",
		Frames:      r.frames,
	}
}

func demoWorkspaceUpdatePlan() domain.UpdatePlanReport {
	before := demoWorkspacePlan().Inspection.Resolution
	after := demoWorkspacePlan().Inspection.Resolution
	before.Packages = []domain.WorkspacePackage{{Package: "vscode", Version: "1.0"}}
	after.Packages = []domain.WorkspacePackage{{Package: "vscode", Version: "2.0"}, {Package: "nodejs", Version: "24.0"}}
	before.Extensions = []domain.WorkspaceExtension{{ID: "example.extension", Version: "1.0"}}
	after.Extensions = []domain.WorkspaceExtension{{ID: "example.extension", Version: "2.0", RequiredPackages: []string{"nodejs"}}}
	return domain.UpdatePlanReport{Kind: "package-base", State: "ready", CurrentRef: "nixos-26.05", Target: "nixos-26.05", Diff: domain.GitDiff{Content: "+ reviewed pin\n"}, Workspace: &domain.WorkspaceUpdateImpact{Current: &before, Proposed: &after}}
}
