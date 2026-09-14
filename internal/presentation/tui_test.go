package presentation

import (
	"strings"
	"testing"

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
			report := domain.HostsReport{State: "partial", Hosts: []domain.HostStatus{
				{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
				{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown},
			}}
			if loads > 1 {
				report.State = "available"
				report.Hosts[1].Reachability = domain.ReachabilityReachable
				report.Hosts[1].SSH = domain.SSHAvailable
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
	if model.screen != dashboardHosts || !strings.Contains(model.View(), "pc02") || !strings.Contains(model.View(), "unreachable  unknown") {
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
	if applied != 1 || model.deploying || model.screen != dashboardDeploy || !strings.Contains(model.View(), "Last result: completed") || !strings.Contains(model.View(), "/state/deploy.log") {
		t.Fatalf("deployment result missing: applied=%d\n%s", applied, model.View())
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
