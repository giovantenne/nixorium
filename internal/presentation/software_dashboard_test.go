package presentation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSoftwareControllerScopesAndPendingReview(t *testing.T) {
	model := dashboardModel{}
	model.software.catalog.Controller = "pc99"
	options := model.software.scopeOptions()
	if options[0].scope.Kind != domain.SoftwareScopeShared || options[1].scope.Kind != domain.SoftwareScopeController {
		t.Fatalf("controller scopes missing: %+v", options)
	}
	for _, option := range options {
		if option.scope.Kind == domain.SoftwareScopeClients {
			t.Fatal("offered empty client selection")
		}
	}
	model.software.plan = domain.SoftwareChangePlanReport{
		Request:            domain.SoftwareChangeRequest{Package: "hello", Present: true, Scope: options[0].scope},
		AffectedController: "pc99",
	}
	review := strings.Join(model.software.reviewView(softwareViewContext{dark: model.isDark}), "\n")
	if !strings.Contains(review, "(pc99) is rebuilt") || !strings.Contains(review, "this controller and all clients, including future ones") {
		t.Fatalf("unclear review: %s", review)
	}
	model.software.result = domain.SoftwareChangeApplyReport{State: "saved", AffectedController: "pc99"}
	result := strings.Join(model.software.resultView(softwareViewContext{dark: model.isDark}), "\n")
	if !strings.Contains(result, "controller needs attention") || !strings.Contains(result, "retrying the controller") {
		t.Fatalf("pending activation lacks recovery: %s", result)
	}
}

func TestSoftwareSaveAutomaticallyAppliesAffectedController(t *testing.T) {
	planCalls, applyCalls := 0, 0
	model := dashboardModel{
		screen: dashboardSoftware,
		software: softwareModel{
			stage: softwareReview,
			plan: domain.SoftwareChangePlanReport{
				State: "ready", Repository: "/deployment", AffectedController: "pc99",
				Request: domain.SoftwareChangeRequest{Package: "hello", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}},
			},
		},
		actions: DashboardActions{
			SaveSoftware: func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
				return domain.SoftwareChangeApplyReport{Revision: strings.Repeat("a", 40), State: "saved", Repository: plan.Repository, AffectedController: plan.AffectedController}
			},
			PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
				planCalls++
				return domain.ControllerRebuildPlanReport{State: "ready", Repository: "/deployment", Controller: "pc99", Revision: strings.Repeat("a", 40)}
			},
			ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
				applyCalls++
				return domain.ControllerRebuildExecutionReport{Revision: strings.Repeat("a", 40), Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true}
			},
		},
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("software save did not start")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil || !model.software.applying {
		t.Fatal("controller apply did not follow the save")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planCalls != 1 || applyCalls != 1 || model.software.applying || !strings.Contains(model.View().Content, "Software is ready on this controller") {
		t.Fatalf("plan=%d apply=%d view=%s", planCalls, applyCalls, model.View().Content)
	}
}

func TestDashboardGuidesReviewedSoftwareDeclarationWithoutDeploying(t *testing.T) {
	catalog := testSoftwareCatalogReport()
	plans := 0
	applies := 0
	actions := DashboardActions{
		LoadSoftware: func(ctx context.Context) domain.SoftwareCatalogReport { return catalog },
		PlanSoftware: func(ctx context.Context, request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
			plans++
			if request.Package != "gimp" || !request.Present || request.Scope.Kind != domain.SoftwareScopeAllClients {
				t.Fatalf("software request = %+v", request)
			}
			return domain.SoftwareChangePlanReport{
				SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-plan", State: "ready",
				Repository: "/deployment", ManagedFile: "lab-software.json", Request: request,
				AffectedClients: catalog.Clients, ReviewToken: "sha256:abcdef0123456789",
			}
		},
		SaveSoftware: func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
			applies++
			return domain.SoftwareChangeApplyReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-save", State: "saved", Repository: plan.Repository, ManagedFile: plan.ManagedFile, Request: plan.Request, AffectedClients: plan.AffectedClients}
		},
	}
	model := newTestModel(t, withReadyLab(), withActions(actions))
	model.width, model.height = 100, 30

	updated, command := model.Update(tea.KeyPressMsg{Text: "w"})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("software catalog was not loaded")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || !strings.Contains(model.View().Content, "Configured software") || !strings.Contains(model.View().Content, "not a check of what is installed") {
		t.Fatalf("software catalog missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Suggested software") {
		t.Fatalf("suggested software view missing:\n%s", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || model.software.stage != softwareScope || !strings.Contains(model.View().Content, "Choose which computers should receive this package") {
		t.Fatalf("software scope missing:\n%s", model.View().Content)
	}
	// Choose "All clients", after the two controller scopes.
	for range 2 {
		updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("software proposal was not requested")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	for _, expected := range []string{"Available in this laboratory’s software versions", "Installed on", "Client computers", "the software choices are saved", "Update computers installs it"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("software review omits %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "SAVE SOFTWARE") || strings.Contains(view, "abcdef012345") {
		t.Fatalf("software review exposed an internal confirmation token:\n%s", view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("enter did not start the reviewed software save")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if plans != 1 || applies != 1 || model.screen != dashboardSoftware || model.software.stage != softwareResult || !strings.Contains(model.View().Content, "Software configuration saved") || strings.Contains(model.View().Content, "Git") || strings.Contains(model.View().Content, "distribution targets") {
		t.Fatalf("software result is not self-contained:\n%s", model.View().Content)
	}
}

func TestDashboardSearchesPinnedPackagesAndIgnoresStaleResults(t *testing.T) {
	catalog := testSoftwareCatalogReport()
	searches := 0
	actions := DashboardActions{
		SearchSoftware: func(_ context.Context, query string) domain.SoftwareSearchReport {
			searches++
			return domain.SoftwareSearchReport{
				SchemaVersion: domain.SoftwareSchemaVersion,
				Operation:     "software-search",
				State:         "ready",
				Query:         query,
				Results: []domain.SoftwareCatalogItem{
					{ID: "hello", Label: "hello", Summary: "A friendly greeting program", Version: "2.12", Availability: "available"},
					{ID: "hello-unfree", Label: "example-unfree-package", Summary: "Policy test package", Version: "1.0", Availability: "blocked-unfree"},
				},
				Issues: []domain.ValidationIssue{},
			}
		},
	}
	model := dashboardModel{
		screen: dashboardSoftware, width: 100, height: 30, actions: actions,
		software: softwareModel{catalog: catalog, mode: softwareSuggested},
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "/"})
	model = updated.(dashboardModel)
	if !model.software.searching || model.software.mode != softwareSearch {
		t.Fatal("the slash shortcut did not open package search")
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "hell"})
	model = updated.(dashboardModel)
	if command == nil || !model.software.searching || model.software.mode != softwareSearch || !model.software.searchBusy {
		t.Fatalf("search was not scheduled: %+v", model)
	}
	id := model.software.searchID
	updated, command = model.Update(dashboardSoftwareSearchStartMsg{id: id, query: "hell", ctx: context.Background()})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("debounced search did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if searches != 1 || model.software.searchBusy || !strings.Contains(model.View().Content, "hello") || !strings.Contains(model.View().Content, "2.12") || !strings.Contains(model.View().Content, "license settings") {
		t.Fatalf("search result missing:\n%s", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	if model.software.searching || model.software.cursor != 1 {
		t.Fatalf("down did not leave search input and select the next result: searching=%t cursor=%d", model.software.searching, model.software.cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model = updated.(dashboardModel)
	if model.software.cursor != 0 {
		t.Fatalf("up did not select the previous result: cursor=%d", model.software.cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "/"})
	model = updated.(dashboardModel)

	updated, _ = model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	if model.software.searchID == id {
		t.Fatal("new query did not advance request identity")
	}
	updated, _ = model.Update(dashboardSoftwareSearchMsg{id: id, report: domain.SoftwareSearchReport{Operation: "software-search", State: "ready", Results: []domain.SoftwareCatalogItem{{ID: "stale", Label: "stale", Summary: "stale", Availability: "available"}}}})
	model = updated.(dashboardModel)
	if strings.Contains(model.View().Content, "stale") {
		t.Fatal("stale package search replaced the current query")
	}
}

func TestDashboardExplainsBlockedSearchResultBeforeScope(t *testing.T) {
	model := dashboardModel{
		screen: dashboardSoftware, width: 100, height: 30,
		software: softwareModel{
			mode:   softwareSearch,
			search: domain.SoftwareSearchReport{Operation: "software-search", State: "ready", Results: []domain.SoftwareCatalogItem{{ID: "hello-unfree", Label: "Example", Summary: "Policy test", Availability: "blocked-unfree"}}},
		},
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardSoftware || !strings.Contains(model.View().Content, "license settings") {
		t.Fatalf("blocked package was not explained:\n%s", model.View().Content)
	}
}

func TestDashboardCancelsSupersededPackageSearch(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan tea.Msg, 1)
	model := dashboardModel{
		screen: dashboardSoftware, width: 100, height: 30,
		software: softwareModel{
			mode: softwareSearch, searching: true,
			query: "he", searchID: 1,
		},
		actions: DashboardActions{SearchSoftware: func(ctx context.Context, query string) domain.SoftwareSearchReport {
			close(started)
			<-ctx.Done()
			return domain.SoftwareSearchReport{Operation: "software-search", State: "failed", Query: query, Issues: []domain.ValidationIssue{{Field: "search", Message: ctx.Err().Error()}}}
		}},
	}
	searchContext, cancel := context.WithTimeout(context.Background(), time.Second)
	model.software.searchCancel = cancel
	updated, command := model.Update(dashboardSoftwareSearchStartMsg{id: 1, query: "he", ctx: searchContext})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("package search did not start")
	}
	go func() { finished <- command() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("package search callback did not start")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "l"})
	model = updated.(dashboardModel)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("superseded package search was not cancelled")
	}
}

func TestDashboardSoftwareResultDistinguishesNoChangeAndUncertainSave(t *testing.T) {
	model := dashboardModel{screen: dashboardSoftware, width: 100, height: 30, software: softwareModel{stage: softwareResult}}
	model.software.result = domain.SoftwareChangeApplyReport{State: "unchanged", Message: "GIMP already has the requested declaration."}
	view := model.View().Content
	if !strings.Contains(view, "already saved") || !strings.Contains(view, "No file changed") || strings.Contains(view, "declaration saved") {
		t.Fatalf("unchanged software result is misleading:\n%s", view)
	}

	model.software.result = domain.SoftwareChangeApplyReport{
		State:   "partial",
		Message: "lab-software.json was replaced, but durable storage could not be confirmed.",
		Issues:  []domain.ValidationIssue{{Field: "durability", Message: "directory sync failed"}},
	}
	view = model.View().Content
	for _, expected := range []string{"needs attention", "could not be confirmed", "Technical detail: directory sync failed", "Retry completes the local save", "No system was built or deployed"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("partial software result omits %q:\n%s", expected, view)
		}
	}
}

func TestSoftwareResultOpensFreshDeploymentSelection(t *testing.T) {
	requested := ""
	report := domain.StatusReport{}
	report.Meta.Controller.Name = "pc99"
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc03", IP: "10.0.0.3"},
	}
	model := dashboardModel{
		screen: dashboardSoftware,
		report: report,
		software: softwareModel{
			stage: softwareResult,
			result: domain.SoftwareChangeApplyReport{
				State: "saved", AffectedClients: []string{"pc01", "pc03"},
			},
		},
		actions: DashboardActions{PlanDeployment: func(ctx context.Context, value string) domain.DeploymentPlanReport {
			requested = value
			return domain.DeploymentPlanReport{State: "ready"}
		}},
		width: 100, height: 30,
	}

	view := model.View().Content
	if !strings.Contains(view, "Update affected computers") || !strings.Contains(view, "Later") {
		t.Fatalf("software result omits contextual deployment action:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardDeploy || !model.deployment.chosen["pc01"] || model.deployment.chosen["pc02"] || !model.deployment.chosen["pc03"] {
		t.Fatalf("contextual selection: command=%v screen=%d chosen=%v", command != nil, model.screen, model.deployment.chosen)
	}
	if view := model.View().Content; !strings.Contains(view, "complete current system") || !strings.Contains(view, "not only that package") {
		t.Fatalf("deployment scope is unclear:\n%s", model.View().Content)
	}
	cancelled, cancelCommand := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cancelledModel := cancelled.(dashboardModel)
	if cancelCommand != nil || cancelledModel.screen != dashboardSoftware || cancelledModel.software.stage != softwareResult {
		t.Fatalf("contextual cancellation lost software result: command=%v screen=%d stage=%d", cancelCommand != nil, cancelledModel.screen, cancelledModel.software.stage)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || requested != "" {
		t.Fatalf("deployment planning was not deferred: command=%v requested=%q", command != nil, requested)
	}
	_ = command()
	if requested != "pc01,pc03" {
		t.Fatalf("fresh deployment requested %q", requested)
	}
}

func TestSoftwareDeploymentDoesNotExpandMissingTargets(t *testing.T) {
	report := domain.StatusReport{}
	report.Meta.Controller.Name = "pc99"
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc02", IP: "10.0.0.2"},
	}
	model := dashboardModel{
		screen: dashboardSoftware,
		report: report,
		software: softwareModel{
			stage: softwareResult,
			result: domain.SoftwareChangeApplyReport{
				State: "saved", AffectedClients: []string{"pc01", "pc09"},
			},
		},
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if !model.deployment.chosen["pc01"] || model.deployment.chosen["pc02"] || model.deployment.chosen["pc09"] {
		t.Fatalf("missing target expanded selection: %v", model.deployment.chosen)
	}
	if !strings.Contains(model.message, "pc09") || !strings.Contains(model.message, "no longer in the current inventory") {
		t.Fatalf("missing target was not explained: %q", model.message)
	}
}

func TestSoftwareDeploymentWaitsForControllerActivation(t *testing.T) {
	model := dashboardModel{
		screen: dashboardSoftware,
		software: softwareModel{
			stage: softwareResult,
			result: domain.SoftwareChangeApplyReport{
				State:              "saved",
				AffectedController: "controller",
				AffectedClients:    []string{"pc01"},
			},
		},
		controller: controllerModel{
			result: domain.ControllerRebuildExecutionReport{
				Operation: "controller apply", Applied: false, Verified: false,
			},
		},
	}

	if strings.Contains(model.View().Content, "Update affected computers") {
		t.Fatalf("client deployment was offered before controller activation:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardSoftware {
		t.Fatalf("blocked client deployment changed screen: command=%v screen=%d", command != nil, model.screen)
	}
}

func TestSoftwareStateReconstructsCurrentSnapshot(t *testing.T) {
	loads := 0
	firstRevision := "0123456789abcdef0123456789abcdef01234567"
	secondRevision := "89abcdef0123456789abcdef0123456789abcdef"
	actions := DashboardActions{
		LoadConfigurationState: func(ctx context.Context) domain.ConfigurationStateReport {
			loads++
			revision := firstRevision
			controller := domain.ControllerRebuildPlanReport{
				Operation: "controller-plan", State: "ready", Revision: revision,
				CurrentDetail: "the activation receipt belongs to an older revision",
			}
			if loads > 1 {
				revision = secondRevision
				controller.State = "current"
				controller.Revision = revision
				controller.Current = true
				controller.CurrentDetail = "active system and receipt match"
			}
			return domain.ConfigurationStateReport{
				Operation:       "configuration-state",
				State:           "partial",
				GeneratedAt:     time.Date(2026, 9, 23, 10, 30, 0, 0, time.UTC),
				DesiredRevision: revision,
				Controller:      controller,
				Clients: domain.HostsReport{
					Operation: "hosts", State: "partial", DesiredRevision: revision,
					Deployment: domain.HostDeploymentSummary{Current: 1, Unknown: 1},
					Hosts: []domain.HostStatus{
						{Name: "pc01", IP: "10.0.0.1", SSH: domain.SSHAvailable, Deployment: domain.DeploymentCurrent, CurrentRevision: revision, DesiredRevision: revision},
						{
							Name: "pc02", IP: "10.0.0.2", SSH: domain.SSHAvailable, Deployment: domain.DeploymentUnknown, DesiredRevision: revision,
							LastSuccessfulDeploy: &domain.LastSuccessfulDeployment{Revision: revision, VerifiedAt: time.Date(2026, 9, 22, 10, 30, 0, 0, time.UTC)},
						},
					},
				},
			}
		},
	}
	model := dashboardModel{
		screen: dashboardSoftware,
		software: softwareModel{
			stage:   softwareCatalog,
			catalog: domain.SoftwareCatalogReport{Operation: "software-catalog"},
		},
		actions: actions,
		width:   120,
		height:  30,
	}

	if !strings.Contains(model.View().Content, "Check systems") {
		t.Fatalf("reconstructible state action is missing:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "v"})
	model = updated.(dashboardModel)
	if command == nil || loads != 0 || !strings.Contains(model.View().Content, "Checking desired and observed") {
		t.Fatalf("state load was not explicit and deferred: command=%v loads=%d\n%s", command != nil, loads, model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	for _, expected := range []string{"Current system configuration", shortRevision(firstRevision), "Not verified for the desired revision", "1 up to date", "1 not verified", "Freshness", "2026-09-23", "Revision not verified"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("state view is missing %q:\n%s", expected, view)
		}
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command == nil || loads != 1 {
		t.Fatalf("state refresh was not deferred: command=%v loads=%d", command != nil, loads)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || !strings.Contains(model.View().Content, shortRevision(secondRevision)) || !strings.Contains(model.View().Content, "Verified at the desired revision") {
		t.Fatalf("state refresh did not use repository current state: loads=%d\n%s", loads, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || model.software.stage != softwareCatalog {
		t.Fatalf("state view did not return to software without replaying a result: screen=%d stage=%d", model.screen, model.software.stage)
	}
}

func TestDashboardSoftwareSupportsSearchRemovalAndBoundedClientSelection(t *testing.T) {
	catalog := testSoftwareCatalogReport()
	for index := 4; index <= 40; index++ {
		catalog.Clients = append(catalog.Clients, fmt.Sprintf("pc%02d", index))
	}
	var request domain.SoftwareChangeRequest
	model := dashboardModel{
		report: testDashboardReport("ready"), screen: dashboardSoftware,
		software: softwareModel{catalog: catalog, clients: map[string]bool{}},
		width:    90,
		height:   22,
		actions: DashboardActions{
			SearchSoftware: func(_ context.Context, query string) domain.SoftwareSearchReport {
				return domain.SoftwareSearchReport{Operation: "software-search", State: "ready", Query: query, Results: []domain.SoftwareCatalogItem{{ID: "vlc", Label: "VLC", Summary: "Play audio and video", Availability: "available"}}, Issues: []domain.ValidationIssue{}}
			},
			PlanSoftware: func(ctx context.Context, candidate domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
				request = candidate
				return domain.SoftwareChangePlanReport{State: "ready", Request: candidate}
			},
		},
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "/"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "vl"})
	model = updated.(dashboardModel)
	updated, command := model.Update(dashboardSoftwareSearchStartMsg{id: model.software.searchID, query: "vl", ctx: context.Background()})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if view := model.View().Content; !strings.Contains(view, "VLC") || strings.Contains(view, "GIMP") {
		t.Fatalf("software search did not use pinned results:\n%s", view)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	model = updated.(dashboardModel)
	// Removal is explicit: Enter opens details and x reviews removal; r does not remove.
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command != nil {
		t.Fatal("r still requests a software removal")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.software.stage != softwareDetails || !strings.Contains(model.View().Content, "Remove") {
		t.Fatalf("Enter did not open package details:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("managed software removal did not request a plan")
	}
	_ = command()
	if request.Package != "vlc" || request.Present || request.Scope.Kind != domain.SoftwareScopeAllClients {
		t.Fatalf("removal request = %+v", request)
	}

	model.busy = ""
	model.screen = dashboardSoftware
	model.software.stage = softwareScope
	model.software.selected = "gimp"
	model.software.scopeCursor = len(model.software.scopeOptions()) - 1
	view := model.View().Content
	if !strings.Contains(view, "pc01") || strings.Contains(view, "pc40") || strings.Count(view, "\npc") > 12 {
		t.Fatalf("client selection is not bounded at 90x22:\n%s", view)
	}
}

func TestDashboardSoftwareCatalogFailureBlocksManualPackageBypass(t *testing.T) {
	model := dashboardModel{
		screen: dashboardSoftware, width: 100, height: 30,
		software: softwareModel{catalog: domain.SoftwareCatalogReport{State: "failed", Message: "pinned evaluation failed", Issues: []domain.ValidationIssue{{Field: "catalog", Message: "failed"}}}},
	}
	view := model.View().Content
	if !strings.Contains(view, "Software information unavailable") || !strings.Contains(view, "deployment inputs are available") {
		t.Fatalf("catalog failure does not explain the safe boundary:\n%s", view)
	}
}

func TestCancelledSoftwareRemovalReturnsToSelectedPackage(t *testing.T) {
	model := experienceFixture(t, 2)
	model.screen = dashboardSoftware
	model.software.catalog = testSoftwareCatalogReport()
	model.software.cursor = 1
	model.actions.PlanSoftware = func(ctx context.Context, request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
		return domain.SoftwareChangePlanReport{State: "ready", Request: request}
	}

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if view := model.View().Content; !strings.Contains(view, "Remove VLC?") || !strings.Contains(view, "Enter") || !strings.Contains(view, "Remove") {
		t.Fatalf("software removal review lacks removal action:\n%s", view)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || model.software.stage != softwareDetails || model.software.selected != "vlc" || model.software.cursor != 1 {
		t.Fatalf("removal cancellation lost its origin: screen=%d selected=%q cursor=%d", model.screen, model.software.selected, model.software.cursor)
	}
	if strings.Contains(model.View().Content, "Add VLC") {
		t.Fatalf("removal cancellation opened an add flow:\n%s", model.View().Content)
	}
}
