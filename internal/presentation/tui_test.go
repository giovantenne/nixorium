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
	report.Meta.Controller.Name = "pc99"
	report.Meta.Controller.DHCPIP = "192.0.2.10"
	report.Meta.Network.Interface = "enp1s0"
	return report
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
