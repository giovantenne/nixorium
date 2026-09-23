package presentation

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func testSoftwarePresetCatalog() domain.SoftwarePresetCatalogReport {
	return domain.SoftwarePresetCatalogReport{
		SchemaVersion: domain.SoftwarePresetSchemaVersion,
		Operation:     "software-presets",
		State:         "ready",
		Repository:    "/deployment",
		Catalog: &domain.SoftwarePresetCatalog{
			SchemaVersion: domain.SoftwarePresetSchemaVersion,
			DefaultPreset: "essential",
			Presets: []domain.SoftwarePreset{
				{ID: "essential", Label: "Essential", Description: "Common classroom software", Packages: []string{"ghostty", "nodejs", "opencode", "pi-coding-agent"}},
				{ID: "programming", Label: "Programming", Description: "Editors and development tools", Packages: []string{"git", "nodejs", "opencode", "pi-coding-agent", "vscode"}},
			},
		},
		Fingerprint: "sha256:profiles",
		Issues:      []domain.ValidationIssue{},
	}
}

func TestSoftwareProfileModelBuildsOneTypedReview(t *testing.T) {
	model := softwareModel{
		stage: softwareCatalog,
		catalog: domain.SoftwareCatalogReport{
			State: "ready", ManagedFile: "lab-software.json", Controller: "pc99",
			Clients: []string{"pc01", "pc02"}, Groups: map[string][]string{},
			Packages: []domain.SoftwareDeclaration{{Package: "nodejs", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: []string{"pc01"}}}},
		},
	}
	model, loaded := model.loadProfiles(testSoftwarePresetCatalog())
	if !loaded.accepted || model.stage != softwareProfiles {
		t.Fatalf("profiles not opened: model=%+v result=%+v", model, loaded)
	}

	model, intent := model.update(keyPress("down"))
	if intent.kind != softwareNoIntent || model.selectedProfile().ID != "programming" {
		t.Fatalf("profile selection = %+v intent=%+v", model.selectedProfile(), intent)
	}
	model, _ = model.update(keyPress("enter"))
	if model.stage != softwareProfilePackages {
		t.Fatalf("package stage = %d", model.stage)
	}
	model.profilePackageCursor = 4
	model, _ = model.update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !model.profileExcluded["vscode"] {
		t.Fatalf("package exclusion = %+v", model.profileExcluded)
	}
	model, _ = model.update(keyPress("enter"))
	if model.stage != softwareProfileScope || model.currentScopeKind() != domain.SoftwareScopeShared {
		t.Fatalf("profile scope = stage %d kind %q", model.stage, model.currentScopeKind())
	}
	model, intent = model.update(keyPress("enter"))
	if intent.kind != softwarePresetPlanIntent || intent.presetRequest.Preset != "programming" || intent.presetRequest.Scope.Kind != domain.SoftwareScopeShared || strings.Join(intent.presetRequest.Exclude, ",") != "vscode" {
		t.Fatalf("profile request = %+v intent=%+v", intent.presetRequest, intent)
	}

	plan := domain.SoftwarePresetPlanReport{
		SchemaVersion: domain.SoftwarePresetSchemaVersion, Operation: "software-preset-plan", State: "ready",
		ManagedFile: "lab-software.json", Request: intent.presetRequest,
		Preset: model.selectedProfile(),
		SelectedPackages: []domain.SoftwareCatalogItem{
			{ID: "git", Label: "Git", Availability: "available"},
			{ID: "nodejs", Label: "Node.js", Availability: "available"},
			{ID: "opencode", Label: "OpenCode", Availability: "available"},
			{ID: "pi-coding-agent", Label: "Pi", Availability: "available"},
		},
		Existing: []domain.SoftwareDeclaration{{Package: "nodejs", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: []string{"pc01"}}}},
		Additions: []domain.SoftwareDeclaration{
			{Package: "git", Scope: intent.presetRequest.Scope},
			{Package: "opencode", Scope: intent.presetRequest.Scope},
			{Package: "pi-coding-agent", Scope: intent.presetRequest.Scope},
		},
		AffectedController: "pc99", AffectedClients: []string{"pc01", "pc02"},
		ReviewToken: "sha256:review", Issues: []domain.ValidationIssue{},
	}
	model, planned := model.finishProfilePlan(plan)
	view := strings.Join(model.profileReviewView(softwareViewContext{width: 80, height: 24}), "\n")
	for _, expected := range []string{"3 add", "1 keep scope", "1 excluded", "already present; keep pc01", "Validated together"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("profile review omits %q:\n%s", expected, view)
		}
	}
	if !planned.accepted || model.stage != softwareProfileReview {
		t.Fatalf("profile review transition = model %d result %+v", model.stage, planned)
	}
	_, intent = model.update(keyPress("enter"))
	if intent.kind != softwarePresetSaveIntent {
		t.Fatalf("profile save intent = %+v", intent)
	}
}

func TestSoftwareProfilesAbsentOrCancelledDoNotMutate(t *testing.T) {
	model := softwareModel{stage: softwareCatalog}
	model, result := model.loadProfiles(domain.SoftwarePresetCatalogReport{
		State: "unavailable", Message: "Individual software management remains available.", Issues: []domain.ValidationIssue{},
	})
	if model.stage != softwareCatalog || model.profilePlan.Operation != "" || model.profileResult.Operation != "" || result.message == "" {
		t.Fatalf("absent profile catalog changed workflow: model=%+v result=%+v", model, result)
	}

	model, _ = model.loadProfiles(testSoftwarePresetCatalog())
	model, _ = model.update(keyPress("enter"))
	model, _ = model.update(keyPress("esc"))
	if model.stage != softwareProfiles || model.profilePlan.Operation != "" || model.profileResult.Operation != "" {
		t.Fatalf("package cancellation changed state: %+v", model)
	}
	model, _ = model.update(keyPress("esc"))
	if model.stage != softwareCatalog {
		t.Fatalf("profile cancellation did not return to catalog: %+v", model)
	}
}

func TestDashboardSoftwareProfileUsesBatchCallbacksAndControllerFollowUp(t *testing.T) {
	catalog := testSoftwareCatalogReport()
	catalog.Controller = "pc99"
	requests := 0
	saves := 0
	controllerPlans := 0
	controllerApplies := 0
	actions := DashboardActions{
		LoadSoftwarePresets: func() domain.SoftwarePresetCatalogReport { return testSoftwarePresetCatalog() },
		PlanSoftwarePreset: func(request domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport {
			requests++
			preset := testSoftwarePresetCatalog().Catalog.Presets[0]
			return domain.SoftwarePresetPlanReport{
				SchemaVersion: domain.SoftwarePresetSchemaVersion, Operation: "software-preset-plan", State: "ready",
				Repository: "/deployment", ManagedFile: "lab-software.json", Request: request, Preset: preset,
				SelectedPackages:   []domain.SoftwareCatalogItem{{ID: "ghostty", Label: "Ghostty", Availability: "available"}},
				Additions:          []domain.SoftwareDeclaration{{Package: "ghostty", Scope: request.Scope}},
				AffectedController: "pc99", AffectedClients: []string{"pc01", "pc02"}, ReviewToken: "sha256:review", Issues: []domain.ValidationIssue{},
			}
		},
		SaveSoftwarePreset: func(plan domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
			saves++
			return domain.SoftwarePresetApplyReport{
				SchemaVersion: domain.SoftwarePresetSchemaVersion, Operation: "software-preset-save", State: "saved",
				Repository: plan.Repository, ManagedFile: plan.ManagedFile, Request: plan.Request, Preset: plan.Preset,
				Additions: plan.Additions, AffectedController: plan.AffectedController, AffectedClients: plan.AffectedClients,
				Revision: strings.Repeat("a", 40), Issues: []domain.ValidationIssue{},
			}
		},
		PlanController: func() domain.ControllerRebuildPlanReport {
			controllerPlans++
			return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: strings.Repeat("a", 40), Issues: []domain.ValidationIssue{}}
		},
		ApplyController: func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			controllerApplies++
			return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Controller: plan.Controller, Revision: plan.Revision, Applied: true, Verified: true, Issues: []domain.ValidationIssue{}}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	model.screen = dashboardSoftware
	model.software.catalog = catalog
	model.width, model.height = 100, 30

	updated, command := model.Update(keyPress("p"))
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("profile catalog was not requested")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	for _, key := range []tea.KeyPressMsg{keyPress("enter"), keyPress("enter"), keyPress("enter")} {
		updated, command = model.Update(key)
		model = updated.(dashboardModel)
	}
	if command == nil {
		t.Fatal("profile plan was not requested")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	updated, command = model.Update(keyPress("enter"))
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("profile save was not requested")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("controller follow-up did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if requests != 1 || saves != 1 || controllerPlans != 1 || controllerApplies != 1 || !strings.Contains(view, "ready on this controller") || !strings.Contains(view, "Distribute affected computers") {
		t.Fatalf("composed profile flow callbacks=%d/%d/%d/%d:\n%s", requests, saves, controllerPlans, controllerApplies, view)
	}
}

func TestSoftwareProfileListsRemainNavigableAtSupportedSizes(t *testing.T) {
	packages := make([]string, 30)
	items := make([]domain.SoftwareCatalogItem, 30)
	additions := make([]domain.SoftwareDeclaration, 30)
	for index := range packages {
		packages[index] = fmt.Sprintf("package-%02d", index+1)
		items[index] = domain.SoftwareCatalogItem{ID: packages[index], Label: packages[index], Availability: "available"}
		additions[index] = domain.SoftwareDeclaration{Package: packages[index], Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		model := dashboardModel{screen: dashboardSoftware, width: size[0], height: size[1], software: softwareModel{
			stage:                softwareProfilePackages,
			catalog:              domain.SoftwareCatalogReport{State: "ready", ManagedFile: "lab-software.json", Clients: []string{"pc01"}, Groups: map[string][]string{}},
			presets:              domain.SoftwarePresetCatalogReport{State: "ready", Catalog: &domain.SoftwarePresetCatalog{Presets: []domain.SoftwarePreset{{ID: "long", Label: "Long profile", Description: "Many packages", Packages: packages}}}},
			profilePackageCursor: len(packages) - 1,
			profileExcluded:      map[string]bool{},
		}}
		view := model.View().Content
		if !strings.Contains(view, "package-30") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("package list not navigable at %dx%d (%dx%d):\n%s", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view), view)
		}
		model.software.stage = softwareProfileReview
		model.software.profileReviewCursor = len(items) - 1
		model.software.profilePlan = domain.SoftwarePresetPlanReport{
			State: "ready", ManagedFile: "lab-software.json",
			Request:          domain.SoftwarePresetRequest{Preset: "long", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}},
			Preset:           domain.SoftwarePreset{ID: "long", Label: "Long profile", Packages: packages},
			SelectedPackages: items, Additions: additions, AffectedClients: []string{"pc01"},
		}
		view = model.View().Content
		if !strings.Contains(view, "package-30") || !strings.Contains(view, "Add profile") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("profile review not navigable at %dx%d (%dx%d):\n%s", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view), view)
		}
	}
}

func TestControllerOnlyProfileResultDoesNotOfferClientDistribution(t *testing.T) {
	model := dashboardModel{screen: dashboardSoftware, width: 100, height: 30, software: softwareModel{stage: softwareResult}}
	software, _ := model.software.finishProfileApply(domain.SoftwarePresetApplyReport{
		Operation: "software-preset-save", State: "saved", Preset: domain.SoftwarePreset{ID: "essential", Label: "Essential"},
		AffectedController: "pc99", AffectedClients: []string{}, Issues: []domain.ValidationIssue{},
	})
	model.software = software
	model.controllerResult = domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Applied: true, Verified: true}
	view := model.View().Content
	if strings.Contains(view, "Distribute affected computers") || !strings.Contains(view, "No client deployment is required") {
		t.Fatalf("controller-only result offers client deployment:\n%s", view)
	}
}

func TestSoftwareProfilePartialResultRetriesTheBatchSave(t *testing.T) {
	model := softwareModel{stage: softwareResult}
	model, _ = model.finishProfileApply(domain.SoftwarePresetApplyReport{
		Operation: "software-preset-save", State: "partial", RecoveryRequired: true,
		Preset: domain.SoftwarePreset{ID: "essential", Label: "Essential"},
		Issues: []domain.ValidationIssue{{Field: "storage", Message: "recording failed"}},
	})
	_, intent := model.update(keyPress("r"))
	if intent.kind != softwarePresetSaveIntent {
		t.Fatalf("profile recovery intent = %+v", intent)
	}
}
