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

func testDashboardReport(mode string) domain.StatusReport {
	report := domain.StatusReport{
		Deployment:     domain.DeploymentStatus{Ready: true},
		PXE:            domain.PXELifecycleState{Mode: mode},
		PXEPreparation: domain.PXEPreparationState{Present: true, Ready: true},
		Services: []domain.ServiceState{
			{Name: "nixorium-harmonia.service", State: "active", Active: true},
		},
	}
	report.Meta.Clients.Count = 2
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "192.0.2.11"},
		{Name: "pc02", IP: "192.0.2.12"},
	}
	report.Meta.Controller.Name = "pc99"
	report.Meta.Controller.DHCPIP = "192.0.2.10"
	report.Meta.Network.Interface = "enp1s0"
	return report
}

func TestSoftwareControllerScopesAndPendingReview(t *testing.T) {
	model := dashboardModel{}
	if first := model.software.scopeOptions()[0].scope.Kind; first != domain.SoftwareScopeAllClients {
		t.Fatalf("legacy default changed: %s", first)
	}
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
	if !strings.Contains(review, "rebuild pc99") || !strings.Contains(review, "this controller and all current or future clients") {
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
				return domain.SoftwareChangeApplyReport{State: "saved", Repository: plan.Repository, AffectedController: plan.AffectedController}
			},
			PlanController: func() domain.ControllerRebuildPlanReport {
				planCalls++
				return domain.ControllerRebuildPlanReport{State: "ready", Repository: "/deployment", Controller: "pc99", Revision: strings.Repeat("a", 40)}
			},
			ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
				applyCalls++
				return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true}
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

func testSetupReport(reviewed, applied, artifacts, _ bool) domain.SetupReport {
	complete := domain.SetupObservation{Complete: true}
	return domain.ReconcileSetup("/deployment", domain.SetupFacts{
		Environment: complete,
		Network:     complete,
		Identity:    complete,
		Credentials: complete,
		Keys:        complete,
		Validation:  complete,
		Review:      domain.SetupObservation{Complete: reviewed},
		Apply:       domain.SetupObservation{Complete: applied},
		Artifacts:   domain.SetupObservation{Complete: artifacts},
	})
}

func testSoftwareCatalogReport() domain.SoftwareCatalogReport {
	return domain.SoftwareCatalogReport{
		SchemaVersion: domain.SoftwareSchemaVersion,
		Operation:     "software-catalog",
		State:         "ready",
		Repository:    "/deployment",
		ManagedFile:   "lab-software.json",
		Clients:       []string{"pc01", "pc02", "pc03"},
		Groups:        map[string][]string{"graphics": {"pc01", "pc02"}},
		Catalog: []domain.SoftwareCatalogItem{
			{ID: "gimp", Label: "GIMP", Summary: "Edit bitmap images", Availability: "available"},
			{ID: "vlc", Label: "VLC", Summary: "Play audio and video", Availability: "available"},
		},
		Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"}},
		Issues:   []domain.ValidationIssue{},
	}
}

func TestDashboardGuidesReviewedSoftwareDeclarationWithoutDeploying(t *testing.T) {
	catalog := testSoftwareCatalogReport()
	plans := 0
	applies := 0
	actions := DashboardActions{
		LoadSoftware: func() domain.SoftwareCatalogReport { return catalog },
		PlanSoftware: func(request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
			plans++
			if request.Package != "gimp" || !request.Present || request.Scope.Kind != domain.SoftwareScopeAllClients {
				t.Fatalf("software request = %+v", request)
			}
			return domain.SoftwareChangePlanReport{
				SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-plan", State: "ready",
				Repository: "/deployment", ManagedFile: "lab-software.json", Request: request,
				AffectedClients: catalog.Clients, ReviewToken: "sha256:abcdef0123456789", Confirmation: "SAVE",
			}
		},
		SaveSoftware: func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
			applies++
			return domain.SoftwareChangeApplyReport{SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-save", State: "saved", Repository: plan.Repository, ManagedFile: plan.ManagedFile, Request: plan.Request, AffectedClients: plan.AffectedClients}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	model.width, model.height = 100, 30

	updated, command := model.Update(tea.KeyPressMsg{Text: "w"})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("software catalog was not loaded")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || !strings.Contains(model.View().Content, "Selected software") || !strings.Contains(model.View().Content, "desired configuration") {
		t.Fatalf("software catalog missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Suggestions") {
		t.Fatalf("suggested software view missing:\n%s", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || model.software.stage != softwareScope || !strings.Contains(model.View().Content, "This is not the set of computers deployed today") {
		t.Fatalf("software scope missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("software proposal was not requested")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	for _, expected := range []string{"Validated against the pinned package set", "Destination", "Clients", "Update lab-software.json locally", "Deploy clients"} {
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
	if searches != 1 || model.software.searchBusy || !strings.Contains(model.View().Content, "hello") || !strings.Contains(model.View().Content, "2.12") || !strings.Contains(model.View().Content, "blocked-unfree") {
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
	if command != nil || model.screen != dashboardSoftware || !strings.Contains(model.View().Content, "licensing policy") {
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
	if !strings.Contains(view, "already current") || !strings.Contains(view, "No file changed") || strings.Contains(view, "declaration saved") {
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
		actions: DashboardActions{PlanDeployment: func(value string) domain.DeploymentPlanReport {
			requested = value
			return domain.DeploymentPlanReport{State: "ready"}
		}},
		width: 100, height: 30,
	}

	view := model.View().Content
	if !strings.Contains(view, "Distribute affected computers") || !strings.Contains(view, "Later") {
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

	if strings.Contains(model.View().Content, "Distribute affected computers") {
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
		LoadConfigurationState: func() domain.ConfigurationStateReport {
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

func TestDashboardShutdownIncludesActiveSessionAndAcknowledgesUnknownSession(t *testing.T) {
	plans := 0
	applies := 0
	actions := DashboardActions{
		PlanShutdown: func(requested string, policy domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
			plans++
			if requested != "@lab" {
				t.Fatalf("shutdown requested = %q", requested)
			}
			eligible := 1
			secondEligible := false
			if policy == domain.ShutdownAcknowledgeUnknown {
				eligible = 2
				secondEligible = true
			}
			return domain.ShutdownPlanReport{
				State: "ready", Requested: requested, Policy: policy, Eligible: eligible,
				Targets: []domain.ShutdownTargetPlan{
					{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive, Eligible: true},
					{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnknown, Eligible: secondEligible},
				},
				ReviewToken: "sha256:abcdef0123456789", Confirmation: "SHUTDOWN",
			}
		},
		ApplyShutdown: func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
			applies++
			if plan.Policy != domain.ShutdownAcknowledgeUnknown || plan.Eligible != 2 {
				t.Fatalf("applied shutdown plan = %+v", plan)
			}
			return domain.ShutdownApplyReport{State: "completed", Accepted: 2, Targets: []domain.ShutdownTargetOutcome{{Name: "pc01", State: "accepted"}, {Name: "pc02", State: "accepted"}}, Message: "Requests accepted; physical state is not inferred."}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	model.width, model.height = 100, 30
	updated, _ := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	if model.screen != dashboardShutdown || strings.Contains(model.View().Content, "pc99") {
		t.Fatalf("shutdown selection includes controller or did not open:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardShutdownReview || !strings.Contains(model.View().Content, "Active user session · will shut down") || !strings.Contains(model.View().Content, "Session unknown · not sent") || !strings.Contains(model.View().Content, "Controller  excluded") || !strings.Contains(model.View().Content, "Type SHUTDOWN to confirm shutdown of active sessions") {
		t.Fatalf("shutdown review is incomplete:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if plans != 2 || !strings.Contains(model.View().Content, "risk acknowledged") {
		t.Fatalf("unknown-session policy was not replanned:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "SHUTDOWN"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("exact shutdown confirmation did not dispatch")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applies != 1 || model.screen != dashboardShutdownResult || !strings.Contains(model.View().Content, "Shutdown requests accepted") || !strings.Contains(model.View().Content, "not evidence") {
		t.Fatalf("shutdown result is misleading:\n%s", model.View().Content)
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
			PlanSoftware: func(candidate domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
				request = candidate
				return domain.SoftwareChangePlanReport{State: "ready", Request: candidate, Confirmation: "SAVE"}
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
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
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

func TestDashboardGuidesAndResumesFirstSetup(t *testing.T) {
	setup := testSetupReport(false, false, false, false)
	loads := 0
	model := dashboardModel{
		report:    testDashboardReport("ready"),
		setup:     setup,
		screen:    dashboardSetup,
		setupMode: true,
		actions: DashboardActions{
			SaveSetupConfiguration: func() domain.ConfigurationSaveReport {
				return domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved", Message: "Configuration saved locally."}
			},
			LoadSetup: func() domain.SetupReport {
				loads++
				return testSetupReport(true, false, false, false)
			},
		},
	}
	if view := model.View().Content; !strings.Contains(view, "Step 1 of 3") || !strings.Contains(view, "Laboratory settings") || !strings.Contains(view, "Save the generated configuration locally") || strings.Contains(view, "› ●") || strings.Contains(view, "! Next step") || !strings.Contains(view, "Continue setup") {
		t.Fatalf("setup progress screen is incomplete:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("setup did not start local save: screen=%d", model.screen)
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSetup {
		t.Fatalf("local save did not refresh setup: screen=%d", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.setup.CurrentStage != domain.SetupStageApply || !strings.Contains(model.View().Content, "Review and activate the controller configuration") {
		t.Fatalf("setup did not resume from refreshed state: loads=%d setup=%+v\n%s", loads, model.setup, model.View().Content)
	}
}

func TestCompletedSetupOpensGenericNetworkInstallation(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), setup: testSetupReport(true, true, true, true), setupMode: true, screen: dashboardSetup}
	if !strings.Contains(model.View().Content, "Controller and client systems are ready") || !strings.Contains(model.View().Content, "Any configured computer") {
		t.Fatalf("completed setup omits first installation action:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardPXE || !strings.Contains(model.View().Content, "Install computers") || strings.Contains(strings.ToLower(model.View().Content), "pilot") {
		t.Fatalf("completed setup did not open generic installation:\n%s", model.View().Content)
	}
}

func TestSetupRequiresExplicitConfirmationToLeavePXEActive(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("active"), setupMode: true, screen: dashboardPXE}
	updated, command := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXELeaveReview || !strings.Contains(model.View().Content, "Closing Nixorium will not stop installation mode") {
		t.Fatalf("active PXE quit did not open consequence review:\n%s", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "leave pxe active"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.View().Content, "Confirmation did not match") {
		t.Fatal("inexact leave confirmation quit the TUI")
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "LEAVE"})
	model = updated.(dashboardModel)
	_, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("exact leave confirmation did not quit")
	}
}

func TestDashboardAreaNavigationUsesVisibleSelection(t *testing.T) {
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	model.width = 90
	model.height = 30
	model.homeMenu.setSize(model.width, model.height)
	view := model.View().Content
	for _, area := range []string{"Computers", "Installation", "Software", "Maintenance"} {
		if !strings.Contains(view, area) {
			t.Fatalf("home area menu lacks %q:\n%s", area, view)
		}
	}
	if strings.Contains(view, "Update Nixorium") || !strings.Contains(view, "\x1b[") {
		t.Fatalf("home area menu lacks hierarchy or color:\n%s", view)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardComputersArea || !strings.Contains(model.View().Content, "Computer inventory") {
		t.Fatalf("computers area did not open: screen=%d", model.screen)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy {
		t.Fatalf("selected dashboard task did not open: screen=%d", model.screen)
	}

	model = newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	for _, shortcut := range []string{"r", "d", "x", "h", "s", "l", "g", "u", "e", "p", "f", "i"} {
		updated, _ = model.Update(tea.KeyPressMsg{Text: shortcut})
		model = updated.(dashboardModel)
		if model.screen != dashboardHome {
			t.Fatalf("hidden %q shortcut still bypasses area navigation: screen=%d", shortcut, model.screen)
		}
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if model.screen != dashboardPXE {
		t.Fatalf("PXE task did not open from Installation: screen=%d", model.screen)
	}
}

func TestRoutineScreensShareVisualTitleHierarchy(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), isDark: true, width: 100, height: 30}
	screens := map[string]string{
		"computers":  model.hostsView(),
		"deploy":     model.deployView(),
		"controller": model.controllerView(),
		"services":   model.servicesView(),
		"logs":       model.logsView(),
		"git":        model.gitReviewView(),
		"update":     model.updateView(),
		"pxe":        model.pxeView(),
	}
	for name, view := range screens {
		if !strings.Contains(view, "Nixorium") || !strings.Contains(view, "\x1b[") {
			t.Errorf("%s screen lacks shared visual title hierarchy:\n%s", name, view)
		}
	}
}

func TestBusyScreensUseAnimatedSharedSpinner(t *testing.T) {
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	model.busy = "Validating candidate configuration"
	initial := model.busyView()
	updated, command := model.Update(model.activitySpinner.Tick())
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(initial, "Validating candidate configuration") || !strings.Contains(model.busyView(), "Validating candidate configuration") {
		t.Fatalf("busy spinner did not remain active: initial=%q current=%q", initial, model.busyView())
	}
}

func TestDashboardLoadsAndRefreshesComputerInventory(t *testing.T) {
	loads := 0
	actions := DashboardActions{
		LoadHosts: func() (domain.HostsReport, error) {
			loads++
			report := domain.HostsReport{State: "partial", Deployment: domain.HostDeploymentSummary{Current: 1, Unknown: 1}, Hosts: []domain.HostStatus{
				{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Deployment: domain.DeploymentCurrent, LastSuccessfulDeploy: &domain.LastSuccessfulDeployment{Revision: "0123456789abcdef", VerifiedAt: time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC)}},
				{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Deployment: domain.DeploymentUnknown},
			}}
			if loads > 1 {
				report.State = "available"
				report.Deployment = domain.HostDeploymentSummary{Current: 2}
				report.Hosts[1].Reachability = domain.ReachabilityReachable
				report.Hosts[1].SSH = domain.SSHAvailable
				report.Hosts[1].Deployment = domain.DeploymentCurrent
			}
			return report, nil
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	if strings.Contains(model.View().Content, "2 computers configured") || !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("home should not scan or summarise computers:\n%s", model.View().Content)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "h"})
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View().Content, "Checking configured computers") {
		t.Fatalf("opening inventory did not start explicit probe:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardHosts || !strings.Contains(model.View().Content, "pc02") || !strings.Contains(model.View().Content, "Could not be reached") || !strings.Contains(model.View().Content, "1 up to date") {
		t.Fatalf("computer inventory is incomplete:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View().Content, "Refreshing computer status") {
		t.Fatalf("refresh did not enter busy state:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.screen != dashboardHosts || !strings.Contains(model.View().Content, "2 / 2 reachable") {
		t.Fatalf("load count = %d, screen = %d:\n%s", loads, model.screen, model.View().Content)
	}
}

func TestDashboardOffersPXEWorkflowFromReconciledState(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready")}
	view := model.View().Content
	if !strings.Contains(view, "Installation") || !strings.Contains(view, "Laboratory overview") {
		t.Fatalf("dashboard omits PXE workflow:\n%s", view)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	view = model.View().Content
	if model.screen != dashboardPXE || !strings.Contains(view, "Prepared artifacts: ready") || !strings.Contains(view, "Next: start network installation") {
		t.Fatalf("PXE screen is incomplete:\n%s", view)
	}
}

func TestRestoreKeepsReapplyAndReinstallDistinct(t *testing.T) {
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	updated, _ := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	view := model.View().Content
	if command != nil || model.screen != dashboardRestore || !strings.Contains(view, "Keeps the disk") || !strings.Contains(view, "target disk locally") || !strings.Contains(view, "does not erase or reserve") {
		t.Fatalf("restore choice is ambiguous:\n%s", view)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !model.computers.restoreMode {
		t.Fatalf("reapply did not route to reviewed deployment: %+v", model)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	if model.screen != dashboardRestore || model.computers.restoreMode {
		t.Fatal("deployment did not return to the restoration choice")
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	view = model.View().Content
	if model.screen != dashboardPXE || !model.computers.restoreMode || !strings.Contains(view, "Reinstall computers") || strings.Contains(view, "Choose a computer") {
		t.Fatalf("reinstall did not open generic network installation: %s", view)
	}
}

func TestCompletedRestoreContextDoesNotLeakIntoLaterInstallation(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), computers: computersModel{restoreMode: true}, screen: dashboardHome}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if model.computers.restoreMode || model.screen != dashboardPXE || strings.Contains(model.View().Content, "Reinstall computers") {
		t.Fatalf("stale restore context changed a later installation:\n%s", model.View().Content)
	}
}

func TestReinstallReviewsConsequencesBeforeLeavingPXEActive(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("active"), computers: computersModel{restoreMode: true}, screen: dashboardPXE}
	updated, command := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXELeaveReview || !strings.Contains(model.View().Content, "Type LEAVE to continue") {
		t.Fatalf("reinstall quit bypassed the active-PXE review:\n%s", model.View().Content)
	}
}

func TestPXEScreenRecommendsOnlyTheObservedNextStage(t *testing.T) {
	report := testDashboardReport("ready")
	report.PXEPreparation = domain.PXEPreparationState{}
	view := (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	if !strings.Contains(view, "Next: prepare installation files") || strings.Contains(view, "start PXE") {
		t.Fatalf("unprepared PXE guidance is ambiguous:\n%s", view)
	}

	report = testDashboardReport("active")
	view = (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	for _, expected := range []string{"Next: install computers", "/installer/setup.sh", "x", "stop PXE"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("active PXE guidance omits %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "p prepare") || strings.Contains(view, "s start PXE") {
		t.Fatalf("active PXE guidance offers invalid setup actions:\n%s", view)
	}

	report.PXE.Mode = "recovery-required"
	view = (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	if !strings.Contains(view, "Next: recover normal controller networking") || strings.Contains(view, "start PXE") {
		t.Fatalf("recovery guidance is ambiguous:\n%s", view)
	}
}

func TestDashboardReviewsAndRunsAllClientDeployment(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc02", IP: "10.0.0.2"},
	}
	planned := ""
	applied := 0
	actions := DashboardActions{
		PlanDeployment: func(requested string) domain.DeploymentPlanReport {
			planned = requested
			return domain.DeploymentPlanReport{
				Operation:       "deploy-plan",
				State:           "ready",
				Requested:       requested,
				Revision:        "0123456789abcdef",
				ColmenaSelector: "@lab",
				Targets:         []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc02"}},
			}
		},
		ApplyDeployment: func(plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			applied++
			observe(domain.DeploymentProgress{Phase: domain.DeploymentPhaseBuild, Total: 4, Activity: "Building configurations for 2 selected computer(s)"})
			observe(domain.DeploymentProgress{Phase: domain.DeploymentPhaseVerify, Completed: 3, Total: 4, TargetCurrent: 2, TargetTotal: 2, Activity: "Checked authenticated state for 2/2 computer(s)"})
			return domain.DeploymentExecutionReport{
				Operation:       "deploy-apply",
				State:           "completed",
				Phase:           domain.DeploymentPhaseComplete,
				ColmenaSelector: plan.ColmenaSelector,
				BuildCompleted:  true,
				ApplyCompleted:  true,
				Verification:    domain.DeploymentVerificationSummary{Attempted: 2, Verified: 2, Recorded: 2},
				LogPath:         "/state/deploy.log",
				Message:         "all selected targets were built and applied successfully",
			}
		},
	}
	model := dashboardModel{report: report, actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "[ ] pc01") {
		t.Fatalf("deployment selection not shown:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != "@lab" || model.screen != dashboardDeployReview || !strings.Contains(model.View().Content, "Reviewed revision  0123456789abcdef") {
		t.Fatalf("planned = %q, screen = %d:\n%s", planned, model.screen, model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "DEPLOY"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.deployment.applying || !strings.Contains(model.View().Content, "Closing is disabled") {
		t.Fatalf("confirmed deployment did not enter protected busy state:\n%s", model.View().Content)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "wait for its result") {
		t.Fatal("dashboard allowed quit while deployment was running")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Building configurations") || !strings.Contains(model.View().Content, "Progress details") || !strings.Contains(model.View().Content, "private log") {
		t.Fatalf("deployment progress missing:\n%s", model.View().Content)
	}
	for model.deployment.applying && command != nil {
		updated, command = model.Update(command())
		model = updated.(dashboardModel)
	}
	if applied != 1 || model.deployment.applying || model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "Deployment completed and verified") || !strings.Contains(model.View().Content, "Authenticated: 2/2   Recorded: 2") || !strings.Contains(model.View().Content, "/state/deploy.log") || !strings.Contains(model.View().Content, "New review") {
		t.Fatalf("deployment result missing: applied=%d\n%s", applied, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardComputersArea {
		t.Fatalf("deployment result did not return to Computers: screen=%d", model.screen)
	}
}

func TestDeploymentProgressKeepsOnlyFiveAuthoredActivities(t *testing.T) {
	recent := []string{}
	for _, activity := range []string{"one", "two", "three", "four", "five", "six"} {
		recent = appendBoundedActivity(recent, activity, 5)
	}
	if got := strings.Join(recent, ","); got != "two,three,four,five,six" {
		t.Fatalf("bounded deployment activity = %q", got)
	}

	model := dashboardModel{
		deployment: deploymentModel{
			progress: domain.DeploymentProgress{
				Phase: domain.DeploymentPhaseVerify, Completed: 3, Total: 4,
				TargetCurrent: 2, TargetTotal: 3,
			},
			recent: recent,
		},
		progressDetails: true,
		width:           80,
	}
	view := strings.Join(model.deploymentProgressView(), "\n")
	for _, expected := range []string{"Verifying computers", "3/4", "Computers checked: 2/3", "six"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("deployment progress omits %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "one") {
		t.Fatalf("deployment progress retained more than five activities:\n%s", view)
	}
}

func TestDeploymentProgressDoesNotRegressAfterBuild(t *testing.T) {
	model := dashboardModel{deployment: deploymentModel{progress: domain.DeploymentProgress{Phase: domain.DeploymentPhasePreflight}}}
	view := strings.Join(model.deploymentProgressView(), "\n")
	if !strings.Contains(view, "✓ Building configurations") || !strings.Contains(view, "● Revalidating reviewed configuration · Running") {
		t.Fatalf("post-build revalidation regressed to an initial step:\n%s", view)
	}
}

func TestDashboardReviewsAndRunsControllerRebuild(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	applied := 0
	refreshed := 0
	actions := DashboardActions{
		Refresh: func() (domain.StatusReport, error) {
			refreshed++
			return domain.StatusReport{Deployment: domain.DeploymentStatus{Ready: true}}, nil
		},
		PlanController: func() domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-plan",
				State:         "ready",
				Controller:    "pc99",
				Revision:      revision,
				Confirmation:  "REBUILD",
				Issues:        []domain.ValidationIssue{},
			}
		},
		ApplyController: func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			applied++
			return domain.ControllerRebuildExecutionReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-apply",
				State:         "completed",
				Controller:    plan.Controller,
				Revision:      plan.Revision,
				Phase:         domain.ControllerRebuildPhaseComplete,
				Applied:       true,
				Verified:      true,
			}
		},
		LoadControllerProgress: func() (domain.OperationProgress, error) {
			return domain.OperationProgress{}, nil
		},
	}
	model := dashboardModel{actions: actions}
	updated, command := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardAdministration {
		t.Fatalf("maintenance did not open: screen=%d", model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardControllerReview || !strings.Contains(model.View().Content, revision) || strings.Contains(model.View().Content, "REBUILD pc99") || !strings.Contains(model.View().Content, "Press Enter") {
		t.Fatalf("controller review missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller apply did not start: %+v", model)
	}
	if !strings.Contains(model.View().Content, "Waiting for managed progress") {
		t.Fatalf("controller apply omits managed progress feedback:\n%s", model.View().Content)
	}
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("controller apply did not schedule action and progress polling")
	}
	updated, _ = model.Update(batch[0]())
	model = updated.(dashboardModel)
	if applied != 1 || refreshed != 1 || !model.report.Deployment.Ready || model.screen != dashboardController || !strings.Contains(model.View().Content, "Controller updated and verified") || !strings.Contains(model.View().Content, "Enter") || !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("controller result missing: applied=%d refreshed=%d\n%s", applied, refreshed, model.View().Content)
	}
	model.controller.progress = domain.OperationProgress{
		Operation: "controller-apply", State: "completed", Phase: "complete",
		Current: 4, Total: 4, Recent: []string{"Controller revision activated and verified"},
	}
	if strings.Contains(model.View().Content, "progress details") {
		t.Fatalf("completed controller details should start collapsed:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Recent activity") || !strings.Contains(model.View().Content, "Hide details") {
		t.Fatalf("controller details did not expand:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("controller completion did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestDashboardControllerProgressShowsTypedBuildState(t *testing.T) {
	started := time.Now().UTC().Add(-2 * time.Second)
	model := dashboardModel{
		screen: dashboardController,
		busy:   "Building and activating the reviewed controller revision",
		controller: controllerModel{
			applying:   true,
			started:    started,
			progressID: 3,
		},
		width: 90,
		actions: DashboardActions{LoadControllerProgress: func() (domain.OperationProgress, error) {
			return domain.OperationProgress{}, nil
		}},
	}
	updated, command := model.Update(dashboardControllerProgressMsg{
		id: 3,
		progress: domain.OperationProgress{
			Operation: "controller-apply", State: "running", Phase: "build",
			StartedAt: started, UpdatedAt: time.Now().UTC(), Current: 1, Total: 4,
			Recent: []string{"Validated configuration and installed keys", "Building the reviewed controller system"},
		},
	})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("controller apply did not schedule another progress poll")
	}
	view := model.View().Content
	for _, expected := range []string{"Building system", "1/4", "Progress details", "Building the reviewed controller system", "elapsed"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("controller progress omits %q:\n%s", expected, view)
		}
	}
	if strings.Count(view, "Progress details") != 1 || strings.Contains(view, "l progress details") {
		t.Fatalf("controller progress duplicates the contextual action bar:\n%s", view)
	}
}

func TestDashboardReviewsAndAppliesValidatedNixoriumUpdate(t *testing.T) {
	target := "v2.3.0"
	confirmation := "UPDATE"
	token := "sha256:" + strings.Repeat("d", 64)
	checked := 0
	planned := 0
	applied := 0
	actions := DashboardActions{
		CheckUpdate: func() domain.UpdateCheckReport {
			checked++
			return domain.UpdateCheckReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "update-check",
				State:         "available",
				Upstream:      "github:giovantenne/nixorium",
				CurrentRef:    "v2.2.0",
				Development:   []domain.UpdateRelease{{Tag: "master", Channel: domain.UpdateChannelMoving}},
				Stable: []domain.UpdateRelease{
					{Tag: target, Channel: domain.UpdateChannelStable},
					{Tag: "v2.2.0", Channel: domain.UpdateChannelStable},
				},
				Prerelease: []domain.UpdateRelease{{Tag: "v2.4.0-beta.1", Channel: domain.UpdateChannelPrerelease}},
			}
		},
		PlanUpdate: func(received string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			planned++
			if received != target || allowPrerelease || allowDowngrade {
				t.Fatalf("update plan input = %q, prerelease=%t, downgrade=%t", received, allowPrerelease, allowDowngrade)
			}
			return domain.UpdatePlanReport{
				SchemaVersion:  domain.SchemaVersion,
				Operation:      "update-plan",
				State:          "ready",
				Revision:       strings.Repeat("a", 40),
				CurrentRef:     "v2.2.0",
				CurrentChannel: domain.UpdateChannelStable,
				Target:         received,
				TargetChannel:  domain.UpdateChannelStable,
				ReviewToken:    token,
				Confirmation:   confirmation,
				Checks:         []domain.UpdateCheck{{ID: "controller", State: "passed", Message: "candidate controller built"}},
				Diff:           domain.GitDiff{Content: strings.Repeat("+ reviewed update\n", 20)},
			}
		},
		SaveUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			applied++
			if plan.ReviewToken != token || plan.Target != target {
				t.Fatalf("applied unexpected update plan: %+v", plan)
			}
			return domain.UpdateApplyReport{Operation: "update-save", State: "saved", Target: target, Updated: true, Message: "Nixorium update saved locally. Running systems were not changed."}
		},
		PlanController: func() domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: strings.Repeat("b", 40)}
		},
		ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true}
		},
	}
	actions.RunningVersion = "2.0.0-test"
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	if strings.Contains(model.View().Content, "Update Nixorium") || !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("home omits navigable task menu:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardUpdate || !strings.Contains(model.View().Content, "Fetching available Nixorium updates") {
		t.Fatalf("release discovery did not start:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if checked != 1 || !strings.Contains(model.View().Content, target) || !strings.Contains(model.View().Content, "master") || !strings.Contains(model.View().Content, "Development branch") || !strings.Contains(model.View().Content, "Running interface   2.0.0-test") || strings.Contains(model.View().Content, "v2.4.0-beta.1") || strings.Contains(model.View().Content, "Target: >") {
		t.Fatalf("fetched stable release list is incorrect:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "v2.4.0-beta.1") {
		t.Fatalf("prerelease disclosure missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("update plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardUpdateReview || !strings.Contains(model.View().Content, "Validated release") || !strings.Contains(model.View().Content, "candidate controller built") || strings.Contains(model.View().Content, confirmation) || !strings.Contains(model.View().Content, "Press Enter") || !strings.Contains(model.View().Content, "No push, PXE action") {
		t.Fatalf("update review missing: planned=%d\n%s", planned, model.View().Content)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Height: 40})
	model = updated.(dashboardModel)
	if lines := strings.Count(model.View().Content, "\n"); lines > 40 {
		t.Fatalf("update review exceeds terminal height: got %d lines\n%s", lines, model.View().Content)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Height: 24})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	model = updated.(dashboardModel)
	if model.updates.scroll == 0 {
		t.Fatal("update diff did not scroll")
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.updates.applying || !strings.Contains(model.View().Content, "Wait for the atomic two-file result") {
		t.Fatalf("update apply did not enter protected busy state:\n%s", model.View().Content)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "wait for its result") {
		t.Fatal("dashboard allowed quit while update apply was running")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil || !model.updates.applying {
		t.Fatal("saved update did not start controller activation")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.updates.applying || model.screen != dashboardUpdate || !strings.Contains(model.View().Content, "Nixorium and this controller are updated") || strings.Contains(model.View().Content, "review Git changes") || !strings.Contains(model.View().Content, "Running interface: 2.0.0-test") || !strings.Contains(model.View().Content, "Reopen Nixorium") || !strings.Contains(model.View().Content, "New update") {
		t.Fatalf("update result missing: applied=%d\n%s", applied, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("update result did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestUpdatePlanningShowsRealCandidatePhasesAndBuildCount(t *testing.T) {
	target := "v2.3.0"
	model := dashboardModel{
		screen: dashboardUpdate,
		updates: updateModel{
			check: domain.UpdateCheckReport{
				Operation:  "update-check",
				State:      "available",
				CurrentRef: "v2.2.0",
				Stable:     []domain.UpdateRelease{{Tag: target, Channel: domain.UpdateChannelStable}},
			},
		},
		actions: DashboardActions{
			PlanUpdateWithProgress: func(received string, allowPrerelease, allowDowngrade bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
				if received != target || allowPrerelease || allowDowngrade {
					t.Fatalf("update plan input = %q, prerelease=%t, downgrade=%t", received, allowPrerelease, allowDowngrade)
				}
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseInspect, Detail: "Checking the deployment repository and selected release"})
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseLock, Detail: "Resolving the selected release and generating its candidate lock"})
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseEvaluate, Detail: "Evaluating candidate deployment readiness"})
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseBuild, Detail: "Building the controller", Current: 2, Total: 5})
				return domain.UpdatePlanReport{Operation: "update-plan", State: "ready", Target: target, TargetChannel: domain.UpdateChannelStable}
			},
		},
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.updates.planning || !strings.Contains(model.View().Content, "Target: "+target) {
		t.Fatalf("update planning did not start with visible context:\n%s", model.View().Content)
	}
	foundBuild := false
	for command != nil && model.screen != dashboardUpdateReview {
		message := command()
		updated, command = model.Update(message)
		model = updated.(dashboardModel)
		if progress, ok := message.(dashboardUpdatePlanProgressMsg); ok && progress.progress.Phase == domain.UpdatePlanPhaseBuild {
			view := model.View().Content
			foundBuild = strings.Contains(view, "Test systems before saving") && strings.Contains(view, "Testing the controller system") && strings.Contains(view, "Safety check 2/5") && strings.Contains(view, "elapsed") && strings.Contains(view, "current deployment remains unchanged")
		}
	}
	if !foundBuild || model.updates.planning || model.screen != dashboardUpdateReview {
		t.Fatalf("update progress did not reach a visible representative build: found=%t planning=%t screen=%d\n%s", foundBuild, model.updates.planning, model.screen, model.View().Content)
	}
}

func TestNixoriumUpdateDiscoveryFailureHasNoEditableFallback(t *testing.T) {
	model := dashboardModel{
		report: testDashboardReport("ready"),
		actions: DashboardActions{CheckUpdate: func() domain.UpdateCheckReport {
			return domain.UpdateCheckReport{
				Operation: "update-check",
				State:     "failed",
				Issues:    []domain.ValidationIssue{{Field: "network", Message: "upstream unavailable"}},
			}
		}},
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if !strings.Contains(view, "Updates could not be fetched") || !strings.Contains(view, "No candidate can be selected") || strings.Contains(view, "Target: >") {
		t.Fatalf("failed discovery exposed an unsafe fallback:\n%s", view)
	}
	updated, planCommand := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if planCommand != nil || model.busy != "" {
		t.Fatal("failed discovery could start planning")
	}
}

func TestNixoriumMovingMasterCanAdvanceToNewRevision(t *testing.T) {
	currentRevision := strings.Repeat("a", 40)
	remoteRevision := strings.Repeat("b", 40)
	planned := 0
	model := dashboardModel{
		screen: dashboardUpdate,
		report: testDashboardReport("ready"),
		updates: updateModel{check: domain.UpdateCheckReport{
			Operation:  "update-check",
			State:      "available",
			CurrentRef: "master",
			CurrentRev: currentRevision,
			Development: []domain.UpdateRelease{{
				Tag: "master", ObjectID: remoteRevision, Channel: domain.UpdateChannelMoving,
			}},
		}},
		actions: DashboardActions{PlanUpdate: func(target string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			planned++
			if target != "master" || allowPrerelease || allowDowngrade {
				t.Fatalf("moving update input = %q, prerelease=%t, downgrade=%t", target, allowPrerelease, allowDowngrade)
			}
			return domain.UpdatePlanReport{Operation: "update-plan", State: "ready", Target: target, TargetChannel: domain.UpdateChannelMoving}
		}},
	}
	if view := model.View().Content; !strings.Contains(view, "New revision available") {
		t.Fatalf("moving update is not visible:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("new master revision did not start validation: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardUpdateReview {
		t.Fatalf("new master revision was not planned: calls=%d screen=%d", planned, model.screen)
	}
}

func TestNixoriumMovingMasterBlocksSameRevision(t *testing.T) {
	revision := strings.Repeat("a", 40)
	model := dashboardModel{
		screen: dashboardUpdate,
		report: testDashboardReport("ready"),
		updates: updateModel{check: domain.UpdateCheckReport{
			Operation:  "update-check",
			State:      "available",
			CurrentRef: "master",
			CurrentRev: revision,
			Development: []domain.UpdateRelease{{
				Tag: "master", ObjectID: revision, Channel: domain.UpdateChannelMoving,
			}},
		}},
	}
	if view := model.View().Content; !strings.Contains(view, "Current revision") {
		t.Fatalf("current master revision is not identified:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.message, "current upstream revision") {
		t.Fatalf("current master revision was not blocked clearly: command=%v message=%q", command, model.message)
	}
}

func TestNixoriumUpdateInputFailureIsNotPresentedAsNetworkFailure(t *testing.T) {
	model := dashboardModel{
		report: testDashboardReport("ready"),
		actions: DashboardActions{CheckUpdate: func() domain.UpdateCheckReport {
			return domain.UpdateCheckReport{
				Operation: "update-check",
				State:     "failed",
				Issues:    []domain.ValidationIssue{{Field: "input", Message: "unsupported flake input"}},
			}
		}},
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	view := updated.(dashboardModel).View().Content
	if !strings.Contains(view, "Update setup needs attention") ||
		!strings.Contains(view, "supported update source from this deployment") ||
		strings.Contains(view, "could not be fetched") {
		t.Fatalf("input failure was presented as a network error:\n%s", view)
	}
}

func TestNixoriumUpdatePartialSaveOffersInPlaceRecovery(t *testing.T) {
	calls := 0
	model := dashboardModel{
		report: testDashboardReport("ready"),
		screen: dashboardUpdate,
		updates: updateModel{
			plan: domain.UpdatePlanReport{
				Operation: "update-plan", State: "ready", Target: "v2.3.0", ReviewToken: "sha256:review",
			},
			result: domain.UpdateApplyReport{
				Operation: "update-save", State: "partial", Target: "v2.3.0", Updated: true,
				RecoveryRequired: true, Message: "The update files are ready, but saving needs recovery.",
			},
		},
		actions: DashboardActions{SaveUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			calls++
			return domain.UpdateApplyReport{Operation: "update-save", State: "saved", Target: plan.Target, Updated: true, Message: "Nixorium update saved locally."}
		}, PlanController: func() domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: strings.Repeat("c", 40)}
		}, ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true}
		}},
	}
	if !strings.Contains(model.View().Content, "Complete save") || strings.Contains(model.View().Content, "review Git changes") {
		t.Fatalf("partial save does not expose bounded recovery:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command == nil || !model.updates.applying || !strings.Contains(model.View().Content, "Completing the local update save") {
		t.Fatalf("recovery did not start in place: %+v", model)
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("recovered update did not start controller activation")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if calls != 1 || model.updates.applying || model.updates.result.State != "saved" || !strings.Contains(model.View().Content, "Nixorium and this controller are updated") {
		t.Fatalf("recovery result missing: calls=%d\n%s", calls, model.View().Content)
	}
}

func TestDashboardEditsReviewsAndAppliesManagedSettings(t *testing.T) {
	current := wizardSettings()
	planned := 0
	applied := 0
	actions := DashboardActions{
		LoadSettings: func() (domain.LabSettingsFile, error) {
			return current, nil
		},
		PlanSettings: func(candidate domain.LabSettingsFile) domain.ConfigPlanReport {
			planned++
			if candidate.Lab.StudentGitName != "Lab Student" {
				t.Fatalf("unexpected settings candidate: %+v", candidate.Lab)
			}
			return domain.ConfigPlanReport{
				Operation:       "config-plan",
				State:           "valid",
				BaseFingerprint: "sha256:reviewed",
				Changes: []domain.SettingChange{{
					Field:  "lab.studentGitName",
					Before: current.Lab.StudentGitName,
					After:  candidate.Lab.StudentGitName,
				}},
			}
		},
		SaveSettings: func(candidate domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			applied++
			if candidate.Lab.StudentGitName != "Lab Student" || plan.BaseFingerprint != "sha256:reviewed" {
				t.Fatalf("unexpected reviewed apply: candidate=%+v plan=%+v", candidate.Lab, plan)
			}
			return domain.ConfigurationSaveReport{
				Operation: "configuration-save",
				State:     "saved",
				Changes:   plan.Changes,
				Message:   "Configuration saved locally. No computer was changed.",
			}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 100, height: 30}
	if !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("home omits settings task:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("settings load did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettings || len(model.settings.menu.list.Items()) != len(routineSettingsGroups) || !strings.Contains(model.View().Content, "Regional") {
		t.Fatalf("settings categories missing:\n%s", model.View().Content)
	}
	model.settings.menu.list.Select(5)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || !strings.Contains(model.View().Content, "Edit Git") {
		t.Fatalf("Git settings editor missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "Lab Student"})
	model = updated.(dashboardModel)
	for range 3 {
		updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("settings plan did not start: %+v", model)
	}
	if !strings.Contains(model.View().Content, "Validating the complete settings candidate") {
		t.Fatalf("settings editor could not render while validation starts:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardSettingsReview || !strings.Contains(model.View().Content, "lab.studentGitName: Student → Lab Student") || !strings.Contains(model.View().Content, "Only lab-settings.json") {
		t.Fatalf("settings review missing: planned=%d\n%s", planned, model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "y"})
	model = updated.(dashboardModel)
	if command == nil || !model.settings.applying {
		t.Fatalf("settings apply did not start: %+v", model)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "wait for its result") {
		t.Fatal("dashboard allowed quit while settings apply was running")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.settings.applying || model.screen != dashboardSettings || model.settings.current.Lab.StudentGitName != "Lab Student" || !strings.Contains(model.View().Content, "Configuration saved") || strings.Contains(model.View().Content, "Git") {
		t.Fatalf("settings result missing: applied=%d model=%+v\n%s", applied, model, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if model.settings.result.Operation != "" || !strings.Contains(model.View().Content, "Choose one area to edit") {
		t.Fatalf("settings result did not open another edit:\n%s", model.View().Content)
	}
}

func TestDashboardReviewsAndRestartsOnlyCacheService(t *testing.T) {
	restarts := 0
	serviceReport := domain.ServicesReport{
		Operation: "services",
		State:     "healthy",
		Services: []domain.ManagedService{
			{ID: "cache", Name: "Binary cache", State: "healthy", Detail: "HTTP-ready", Units: []domain.ServiceState{{Name: "nixorium-harmonia.service", Loaded: true, Active: true, State: "active"}}},
			{ID: "pxe", Name: "PXE installation mode", State: "ready", Detail: "prepared", Units: []domain.ServiceState{{Name: "nixorium-pxe.service", Loaded: true, State: "inactive"}}},
		},
	}
	actions := DashboardActions{
		LoadServices: func() domain.ServicesReport { return serviceReport },
		RestartService: func(service string) domain.ServiceActionReport {
			restarts++
			if service != "cache" {
				t.Fatalf("restarted unexpected service %q", service)
			}
			return domain.ServiceActionReport{Operation: "service-restart", State: "completed", Service: service, Verified: true, Message: "cache healthy"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardServices || !strings.Contains(model.View().Content, "healthy") || !strings.Contains(model.View().Content, "Installation → PXE mode and network recovery") {
		t.Fatalf("services screen missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if model.screen != dashboardServicesRestartReview || !strings.Contains(model.View().Content, "Type RESTART to continue") {
		t.Fatalf("restart review missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "restart cache"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || restarts != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatalf("inexact restart was accepted: restarts=%d", restarts)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "RESTART"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if restarts != 1 || model.screen != dashboardServices || !strings.Contains(model.View().Content, "Binary cache restarted and verified") || !strings.Contains(model.View().Content, "cache healthy") || !strings.Contains(model.View().Content, "Restart again") {
		t.Fatalf("verified restart result missing: restarts=%d\n%s", restarts, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("service result did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestDashboardBrowsesBoundedOperationLogTail(t *testing.T) {
	id := "deploy-20260914T113000.000000000Z-11.log"
	loaded := ""
	actions := DashboardActions{
		LoadLogs: func() domain.OperationLogsReport {
			return domain.OperationLogsReport{Operation: "logs-list", State: "available", Records: []domain.OperationRecord{
				{RecordedAt: time.Date(2026, 9, 14, 11, 31, 0, 0, time.UTC), Operation: "pxe-start", State: "completed", Subject: "active", Summary: "PXE lifecycle transition finished"},
			}, Logs: []domain.OperationLogEntry{
				{ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC), SizeBytes: 70000, State: "partial", Available: true},
			}}
		},
		LoadLog: func(selected string) domain.OperationLogReport {
			loaded = selected
			lines := make([]string, 20)
			for index := range lines {
				lines[index] = fmt.Sprintf("line-%02d", index+1)
			}
			entry := domain.OperationLogEntry{ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC), SizeBytes: 70000, State: "partial", Available: true}
			return domain.OperationLogReport{Operation: "logs-show", State: "available", Log: &entry, Content: strings.Join(lines, "\n") + "\n", Truncated: true}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "l"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardLogs || !strings.Contains(model.View().Content, "Recent actions") || !strings.Contains(model.View().Content, "pxe-start") || !strings.Contains(model.View().Content, id) || !strings.Contains(model.View().Content, "partial") {
		t.Fatalf("operation log list missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loaded != id || model.screen != dashboardLogDetail || !strings.Contains(model.View().Content, "earlier bytes omitted") || !strings.Contains(model.View().Content, "line-20") || strings.Contains(model.View().Content, "line-01") {
		t.Fatalf("bounded tail detail missing: loaded=%q\n%s", loaded, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "line-01") {
		t.Fatalf("log detail did not scroll to the beginning:\n%s", model.View().Content)
	}
}

func TestDashboardShowsScrollableReadOnlyGitReview(t *testing.T) {
	loads := 0
	actions := DashboardActions{
		LoadGitReview: func() domain.GitReviewReport {
			loads++
			return domain.GitReviewReport{
				Operation: "git-review",
				State:     "changes",
				Summary:   domain.GitChangeSummary{Staged: 1, Untracked: 1, Managed: 1, Unexpected: 1},
				Changes: []domain.GitChange{
					{Path: "lab-settings.json", Staged: "modified", Managed: true},
					{Path: "notes", Untracked: true},
				},
				Diffs: []domain.GitDiff{{Scope: "staged", Content: "line-01\nline-02\nline-03\nline-04\nline-05\nline-06\nline-07\n"}},
			}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("Git review did not load: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View().Content, "Repository changes") || !strings.Contains(model.View().Content, "lab-settings.json") {
		t.Fatalf("Git review missing: loads=%d\n%s", loads, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "line-07") || strings.Contains(model.View().Content, "lab-settings.json") {
		t.Fatalf("Git review did not scroll:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "f"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.maintenance.gitScroll != 0 {
		t.Fatalf("Git review refresh = loads %d, scroll %d", loads, model.maintenance.gitScroll)
	}
}

func TestDashboardPlansAndCreatesExactLocalGitCommit(t *testing.T) {
	revision := strings.Repeat("a", 40)
	token := "sha256:" + strings.Repeat("b", 64)
	confirmation := "COMMIT"
	applied := 0
	review := domain.GitReviewReport{
		Operation: "git-review",
		State:     "changes",
		Summary:   domain.GitChangeSummary{Unstaged: 2, Managed: 1, Unexpected: 1},
		Changes: []domain.GitChange{
			{Path: "lab-settings.json", Unstaged: "modified", Managed: true},
			{Path: "modules/site.nix", Unstaged: "modified"},
		},
	}
	actions := DashboardActions{
		LoadGitReview: func() domain.GitReviewReport { return review },
		PlanGitCommit: func(paths string) domain.GitCommitPlanReport {
			if paths != "lab-settings.json" {
				t.Fatalf("planned paths = %q", paths)
			}
			return domain.GitCommitPlanReport{Operation: "git-commit-plan", State: "ready", Revision: revision, Paths: []string{"lab-settings.json"}, ReviewToken: token, CommitMessage: "chore: update laboratory settings", Confirmation: confirmation, Diff: domain.GitDiff{Content: "+settings\n"}}
		},
		ApplyGitCommit: func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
			applied++
			if plan.ReviewToken != token {
				t.Fatalf("applied token = %q", plan.ReviewToken)
			}
			return domain.GitCommitReport{Operation: "git-commit", State: "completed", Paths: plan.Paths, PreviousRevision: revision, Revision: strings.Repeat("c", 40), Committed: true, Message: "no remote push was attempted"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if model.screen != dashboardGitCommitSelect || !strings.Contains(model.View().Content, "Select paths for the local commit") {
		t.Fatalf("commit selection missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardGitCommitReview || !strings.Contains(model.View().Content, confirmation) || !strings.Contains(model.View().Content, "No hooks, signing actions, remote operations, or push") {
		t.Fatalf("commit review missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "COMMIT"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("commit apply did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View().Content, "Git changes committed locally") || !strings.Contains(model.View().Content, "no remote push was attempted") || !strings.Contains(model.View().Content, "Refresh review") {
		t.Fatalf("commit result missing: applied=%d\n%s", applied, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("Git result did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestFirstSetupCommitResultReturnsToObservedChecklist(t *testing.T) {
	loads := 0
	model := dashboardModel{
		setupMode: true,
		screen:    dashboardGitReview,
		maintenance: maintenanceModel{
			gitCommitResult: domain.GitCommitReport{
				Operation: "git-commit", State: "completed", Committed: true,
			},
		},
		actions: DashboardActions{LoadSetup: func() domain.SetupReport {
			loads++
			return testSetupReport(true, false, false, false)
		}},
	}
	if !strings.Contains(model.View().Content, "Enter") || !strings.Contains(model.View().Content, "Setup") {
		t.Fatalf("setup return action missing:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSetup {
		t.Fatalf("commit result did not begin setup refresh: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.screen != dashboardSetup || model.setup.CurrentStage != domain.SetupStageApply {
		t.Fatalf("setup result was not reconciled: loads=%d model=%+v", loads, model.setup)
	}
}

func TestGitCommitSelectionPreservesReviewOrderAndSkipsPrivatePaths(t *testing.T) {
	changes := []domain.GitChange{{Path: "z"}, {Path: "secret-key", Private: true}, {Path: "a"}}
	chosen := toggleAllGitCommitPaths(changes, nil)
	if chosen["secret-key"] || selectedGitCommitPaths(changes, chosen) != "z,a" {
		t.Fatalf("unsafe or reordered selection = %v / %q", chosen, selectedGitCommitPaths(changes, chosen))
	}
}

func TestDashboardDeploymentRejectsEmptySelectionAndBlockedPlan(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	actions := DashboardActions{
		PlanDeployment: func(string) domain.DeploymentPlanReport {
			return domain.DeploymentPlanReport{State: "blocked", Issues: []domain.ValidationIssue{{Field: "git", Message: "worktree is dirty"}}}
		},
	}
	model := dashboardModel{report: report, actions: actions, screen: dashboardDeploy, deployment: deploymentModel{chosen: map[string]bool{}}}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.View().Content, "Select at least one") {
		t.Fatalf("empty selection was planned:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "git: worktree is dirty") {
		t.Fatalf("blocked plan was not surfaced:\n%s", model.View().Content)
	}
}

func TestSelectedDeploymentTargetsPreservesInventoryOrder(t *testing.T) {
	hosts := []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}, {Name: "pc03"}}
	if got := selectedDeploymentTargets(hosts, map[string]bool{"pc03": true, "pc01": true}); got != "pc01,pc03" {
		t.Fatalf("selected targets = %q, want pc01,pc03", got)
	}
	if got := selectedDeploymentTargets(hosts, map[string]bool{"pc01": true, "pc02": true, "pc03": true}); got != "@lab" {
		t.Fatalf("all targets = %q, want @lab", got)
	}
	if got := strings.Join(selectedDeploymentTargetNames(hosts, map[string]bool{"pc01": true, "pc02": true, "pc03": true}), ","); got != "pc01,pc02,pc03" {
		t.Fatalf("explicit targets = %q, want pc01,pc02,pc03", got)
	}
}

func TestDashboardPXEStartRequiresExactTypedConfirmation(t *testing.T) {
	starts := 0
	actions := DashboardActions{
		Refresh: func() (domain.StatusReport, error) { return testDashboardReport("active"), nil },
		PlanPXEStart: func() domain.PXELifecycleReport {
			return domain.PXELifecycleReport{
				State:       "ready",
				Mode:        "ready",
				Interface:   "enp1s0",
				DHCPAddress: "192.0.2.10",
				StaticCIDR:  "10.0.0.99/24",
			}
		},
		StartPXE: func() domain.PXELifecycleReport {
			starts++
			return domain.PXELifecycleReport{State: "completed", Mode: "active", Message: "PXE active"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, screen: dashboardPXE}

	updated, command := model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardPXEStartReview || !strings.Contains(model.View().Content, "Temporarily remove 10.0.0.99/24") {
		t.Fatalf("start review not shown:\n%s", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "start pxe"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || starts != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatalf("inexact confirmation started PXE: starts=%d", starts)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "START"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if starts != 1 || model.report.PXE.Mode != "active" || !strings.Contains(model.View().Content, "PXE active") {
		t.Fatalf("confirmed start did not refresh state: starts=%d view=%s", starts, model.View().Content)
	}
}

func TestDashboardPXEConfirmationRejectsLegacyMultiwordInput(t *testing.T) {
	model := dashboardModel{screen: dashboardPXEStartReview}
	for _, key := range []tea.KeyPressMsg{
		{Text: "START"},
		{Code: tea.KeySpace},
		{Text: "PXE"},
	} {
		updated, _ := model.Update(key)
		model = updated.(dashboardModel)
	}
	if model.confirmation != "START PXE" {
		t.Fatalf("terminal confirmation = %q, want legacy phrase to remain unaccepted", model.confirmation)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.confirmation != "" || !strings.Contains(model.message, "did not match") {
		t.Fatal("legacy multiword PXE confirmation was accepted")
	}
}

func TestDashboardPXEPrepareStopAndRecoverUseCallbacks(t *testing.T) {
	called := ""
	actions := DashboardActions{
		Refresh: func() (domain.StatusReport, error) { return testDashboardReport("ready"), nil },
		PreparePXE: func() domain.ActionReport {
			called = "prepare"
			return domain.ActionReport{Message: "prepared"}
		},
		StopPXE: func() domain.PXELifecycleReport {
			called = "stop"
			return domain.PXELifecycleReport{Message: "stopped"}
		},
		RecoverPXE: func() domain.PXELifecycleReport {
			called = "recover"
			return domain.PXELifecycleReport{Message: "recovered"}
		},
	}
	for key, expected := range map[string]string{"p": "prepare", "x": "stop", "r": "recover"} {
		called = ""
		mode := "active"
		if key == "r" {
			mode = "recovery-required"
		}
		model := dashboardModel{report: testDashboardReport(mode), actions: actions, screen: dashboardPXE}
		updated, command := model.Update(tea.KeyPressMsg{Text: key})
		model = updated.(dashboardModel)
		if command == nil {
			t.Fatalf("%s did not schedule an operation", key)
		}
		if key == "p" && !strings.Contains(model.View().Content, "Waiting for managed progress") {
			t.Fatalf("PXE preparation omits managed progress feedback:\n%s", model.View().Content)
		}
		message := command()
		if key == "p" {
			batch, ok := message.(tea.BatchMsg)
			if !ok || len(batch) != 2 {
				t.Fatalf("PXE preparation command = %#v, want action and progress poll", message)
			}
			message = batch[0]()
		}
		_, _ = model.Update(message)
		if called != expected {
			t.Errorf("%s called %q, want %q", key, called, expected)
		}
	}
}

func TestDashboardPXEHidesAndIgnoresRecoveryInNormalStates(t *testing.T) {
	called := false
	model := dashboardModel{
		report: testDashboardReport("ready"),
		screen: dashboardPXE,
		actions: DashboardActions{RecoverPXE: func() domain.PXELifecycleReport {
			called = true
			return domain.PXELifecycleReport{}
		}},
	}
	if strings.Contains(model.View().Content, "Recover") {
		t.Fatalf("normal PXE state offers recovery:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command != nil || called || model.busy != "" {
		t.Fatalf("hidden recovery shortcut ran in a normal state")
	}
}

func TestDashboardPXEProgressShowsPhaseBarAndRecentActivity(t *testing.T) {
	started := time.Now().UTC().Add(-3 * time.Second)
	model := dashboardModel{
		report: testDashboardReport("ready"),
		screen: dashboardPXE,
		busy:   "Preparing netboot artifacts and client closures",
		installation: installationModel{
			pxePreparing:  true,
			pxeStarted:    started,
			pxeProgressID: 7,
		},
		width: 100,
		actions: DashboardActions{
			LoadPXEProgress: func() (domain.OperationProgress, error) {
				return domain.OperationProgress{}, nil
			},
		},
	}
	updated, command := model.Update(dashboardPXEProgressMsg{
		id: 7,
		progress: domain.OperationProgress{
			SchemaVersion: domain.OperationProgressSchemaVersion,
			Operation:     "pxe-prepare",
			State:         "running",
			Phase:         "clients",
			StartedAt:     started,
			UpdatedAt:     time.Now().UTC(),
			Current:       2,
			Total:         10,
			Recent:        []string{"Built netboot kernel (1/4)", "Built client pc02 (2/10)"},
		},
	})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("running preparation did not schedule the next progress poll")
	}
	view := model.View().Content
	for _, expected := range []string{"Building client systems", "2/10", "Progress details", "Built client pc02 (2/10)", "elapsed"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("PXE progress omits %q:\n%s", expected, view)
		}
	}
	if strings.Count(view, "Progress details") != 1 || strings.Contains(view, "l progress details") {
		t.Fatalf("PXE progress duplicates the contextual action bar:\n%s", view)
	}
	if strings.Contains(view, "journalctl") {
		t.Fatalf("PXE progress exposes journal access instead of typed progress:\n%s", view)
	}
}

func TestDashboardPXEProgressIgnoresRecordFromPreviousRun(t *testing.T) {
	started := time.Now().UTC()
	model := dashboardModel{
		installation: installationModel{
			pxePreparing:  true,
			pxeStarted:    started,
			pxeProgressID: 9,
		},
		actions: DashboardActions{LoadPXEProgress: func() (domain.OperationProgress, error) {
			return domain.OperationProgress{}, nil
		}},
	}
	updated, command := model.Update(dashboardPXEProgressMsg{
		id: 9,
		progress: domain.OperationProgress{
			Operation: "pxe-prepare",
			StartedAt: started.Add(-time.Second),
			UpdatedAt: started.Add(-500 * time.Millisecond),
			Recent:    []string{"Old preparation"},
		},
	})
	model = updated.(dashboardModel)
	if model.installation.pxeProgress.Operation != "" {
		t.Fatalf("stale progress was rendered: %+v", model.installation.pxeProgress)
	}
	if command == nil {
		t.Fatal("stale progress stopped polling for the current run")
	}
}
