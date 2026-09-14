package presentation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	report.Meta.Controller.Name = "pc99"
	report.Meta.Controller.DHCPIP = "192.0.2.10"
	report.Meta.Network.Interface = "enp1s0"
	return report
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
	if !strings.Contains(model.View(), "Computers            2 configured") || !strings.Contains(model.View(), "View computers") {
		t.Fatalf("home omits computer summary:\n%s", model.View())
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View(), "Checking configured computers") {
		t.Fatalf("opening inventory did not start explicit probe:\n%s", model.View())
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardHosts || !strings.Contains(model.View(), "pc02") || !strings.Contains(model.View(), "unreachable  unknown") || !strings.Contains(model.View(), "1 current, 0 outdated, 1 unknown") || !strings.Contains(model.View(), "2026-09-14 10:30Z") || !strings.Contains(model.View(), "never") {
		t.Fatalf("computer inventory is incomplete:\n%s", model.View())
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View(), "Refreshing computer status") {
		t.Fatalf("refresh did not enter busy state:\n%s", model.View())
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.screen != dashboardHosts || !strings.Contains(model.View(), "SSH available: 2/2") {
		t.Fatalf("load count = %d, screen = %d:\n%s", loads, model.screen, model.View())
	}
}

func TestDashboardOffersPXEWorkflowFromReconciledState(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready")}
	view := model.View()
	if !strings.Contains(view, "Installation mode    ready") || !strings.Contains(view, "Install computers over network") {
		t.Fatalf("dashboard omits PXE workflow:\n%s", view)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(dashboardModel)
	view = model.View()
	if model.screen != dashboardPXE || !strings.Contains(view, "Prepared artifacts: ready") || !strings.Contains(view, "Start installation mode") {
		t.Fatalf("PXE screen is incomplete:\n%s", view)
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
		ApplyDeployment: func(plan domain.DeploymentPlanReport) domain.DeploymentExecutionReport {
			applied++
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
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View(), "[ ] pc01") {
		t.Fatalf("deployment selection not shown:\n%s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != "@lab" || model.screen != dashboardDeployReview || !strings.Contains(model.View(), "Revision: 0123456789abcdef") {
		t.Fatalf("planned = %q, screen = %d:\n%s", planned, model.screen, model.View())
	}

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("DEPLOY")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("@lab")},
	} {
		updated, _ = model.Update(key)
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.deploying || !strings.Contains(model.View(), "Closing is disabled") {
		t.Fatalf("confirmed deployment did not enter protected busy state:\n%s", model.View())
	}
	updated, quitCommand := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "wait for its result") {
		t.Fatal("dashboard allowed quit while deployment was running")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.deploying || model.screen != dashboardDeploy || !strings.Contains(model.View(), "Last result: completed") || !strings.Contains(model.View(), "Authenticated: 2/2   Recorded: 2") || !strings.Contains(model.View(), "/state/deploy.log") {
		t.Fatalf("deployment result missing: applied=%d\n%s", applied, model.View())
	}
}

func TestDashboardReviewsAndRunsControllerRebuild(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	applied := 0
	actions := DashboardActions{
		PlanController: func() domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-plan",
				State:         "ready",
				Controller:    "pc99",
				Revision:      revision,
				Confirmation:  "REBUILD pc99",
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
	}
	model := dashboardModel{actions: actions}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardControllerReview || !strings.Contains(model.View(), revision) || !strings.Contains(model.View(), "REBUILD pc99") {
		t.Fatalf("controller review missing:\n%s", model.View())
	}
	for _, character := range "REBUILD pc99" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller apply did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.screen != dashboardController || !strings.Contains(model.View(), "Applied: true   Verified: true") {
		t.Fatalf("controller result missing: applied=%d\n%s", applied, model.View())
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
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardServices || !strings.Contains(model.View(), "Binary cache — healthy") || !strings.Contains(model.View(), "Managed through the Install computers workflow") {
		t.Fatalf("services screen missing:\n%s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(dashboardModel)
	if model.screen != dashboardServicesRestartReview || !strings.Contains(model.View(), "RESTART CACHE") {
		t.Fatalf("restart review missing:\n%s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("restart cache")})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || restarts != 0 || !strings.Contains(model.View(), "did not match") {
		t.Fatalf("inexact restart was accepted: restarts=%d", restarts)
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("RESTART")}, {Type: tea.KeySpace}, {Type: tea.KeyRunes, Runes: []rune("CACHE")}} {
		updated, _ = model.Update(key)
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if restarts != 1 || model.screen != dashboardServices || !strings.Contains(model.View(), "verified=true") || !strings.Contains(model.View(), "cache healthy") {
		t.Fatalf("verified restart result missing: restarts=%d\n%s", restarts, model.View())
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
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardLogs || !strings.Contains(model.View(), "Recent actions") || !strings.Contains(model.View(), "pxe-start") || !strings.Contains(model.View(), id) || !strings.Contains(model.View(), "partial") {
		t.Fatalf("operation log list missing:\n%s", model.View())
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loaded != id || model.screen != dashboardLogDetail || !strings.Contains(model.View(), "earlier bytes omitted") || !strings.Contains(model.View(), "line-20") || strings.Contains(model.View(), "line-01") {
		t.Fatalf("bounded tail detail missing: loaded=%q\n%s", loaded, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View(), "line-01") {
		t.Fatalf("log detail did not scroll to the beginning:\n%s", model.View())
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
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("Git review did not load: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View(), "Git change review") || !strings.Contains(model.View(), "lab-settings.json") {
		t.Fatalf("Git review missing: loads=%d\n%s", loads, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View(), "line-07") || strings.Contains(model.View(), "lab-settings.json") {
		t.Fatalf("Git review did not scroll:\n%s", model.View())
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.gitScroll != 0 {
		t.Fatalf("Git review refresh = loads %d, scroll %d", loads, model.gitScroll)
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
	model := dashboardModel{report: report, actions: actions, screen: dashboardDeploy, deployChosen: map[string]bool{}}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.View(), "Select at least one") {
		t.Fatalf("empty selection was planned:\n%s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View(), "git: worktree is dirty") {
		t.Fatalf("blocked plan was not surfaced:\n%s", model.View())
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

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardPXEStartReview || !strings.Contains(model.View(), "Temporarily remove 10.0.0.99/24") {
		t.Fatalf("start review not shown:\n%s", model.View())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("start pxe")})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || starts != 0 || !strings.Contains(model.View(), "did not match") {
		t.Fatalf("inexact confirmation started PXE: starts=%d", starts)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("START PXE")})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if starts != 1 || model.report.PXE.Mode != "active" || !strings.Contains(model.View(), "PXE active") {
		t.Fatalf("confirmed start did not refresh state: starts=%d view=%s", starts, model.View())
	}
}

func TestDashboardPXEConfirmationAcceptsTerminalSpaceEvent(t *testing.T) {
	model := dashboardModel{screen: dashboardPXEStartReview}
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("START")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("PXE")},
	} {
		updated, _ := model.Update(key)
		model = updated.(dashboardModel)
	}
	if model.confirmation != "START PXE" {
		t.Fatalf("terminal confirmation = %q, want START PXE", model.confirmation)
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
		model := dashboardModel{report: testDashboardReport("active"), actions: actions, screen: dashboardPXE}
		updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		model = updated.(dashboardModel)
		if command == nil {
			t.Fatalf("%s did not schedule an operation", key)
		}
		updated, _ = model.Update(command())
		if called != expected {
			t.Errorf("%s called %q, want %q", key, called, expected)
		}
	}
}
