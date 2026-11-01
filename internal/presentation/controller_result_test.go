package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestControllerResultIsShownBeforeTheStatusRefresh(t *testing.T) {
	refreshed := make(chan struct{})
	m := newTestModel(t, withActions(DashboardActions{
		ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Applied: true, Verified: true, Message: "Controller operation completed."}
		},
		Refresh: func(context.Context) (domain.StatusReport, error) {
			<-refreshed
			return domain.StatusReport{Repository: "/home/admin/nixorium-deployment"}, nil
		},
	}))
	m.screen = dashboardControllerReview
	m.controller.plan = domain.ControllerRebuildPlanReport{State: "ready"}
	next, cmd := m.startReviewedControllerApply()
	m = next.(dashboardModel)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("controller apply is not followed by a separate refresh")
	}
	operation, ok := batch[0]().(tea.BatchMsg)
	if !ok || len(operation) != 2 {
		t.Fatal("result and refresh are not separate commands")
	}
	result := operation[0]()
	next, _ = m.Update(result)
	m = next.(dashboardModel)
	if m.controller.applying || m.busy != "" || m.controller.result.Message != "Controller operation completed." {
		t.Fatalf("result waited for the refresh: %+v", m.controller)
	}
	close(refreshed)
	next, _ = m.Update(operation[1]())
	m = next.(dashboardModel)
	if m.report.Repository != "/home/admin/nixorium-deployment" {
		t.Fatal("refreshed status not kept")
	}
}

func TestReopenedInterfaceShowsTheUpdateOutcome(t *testing.T) {
	m := newTestModel(t)
	m.reopenNotice = "Nixorium and this controller are updated. Next: Computers → Update computers."
	next, _ := m.Update(dashboardInitialMsg{})
	m = next.(dashboardModel)
	if !strings.Contains(m.View().Content, "Nixorium and this controller are updated") || m.reopenNotice != "" {
		t.Fatalf("outcome not shown after reopening:\n%s", m.View().Content)
	}
	if plain := demoANSI.ReplaceAllString(m.View().Content, ""); m.screen != dashboardHome || !strings.Contains(plain, "✓ Nixorium and this controller are updated") {
		t.Fatalf("outcome is not a success notice on the overview:\n%s", plain)
	}
	// It stays while the overview is open, and goes when the administrator
	// leaves it.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(dashboardModel)
	if m.reopenOutcome == "" {
		t.Fatal("outcome cleared by a move inside the overview")
	}
	next, _ = m.Update(tea.KeyPressMsg{Text: "m"})
	m = next.(dashboardModel)
	if m.screen == dashboardHome || m.reopenOutcome != "" {
		t.Fatalf("outcome kept after leaving the overview: screen %v", m.screen)
	}
	m.screen = dashboardUpdate
	if outcome := m.reloadOutcome(); !strings.Contains(outcome, "Update computers") {
		t.Fatalf("outcome = %q", outcome)
	}
}

func TestLateControllerStatusIsDroppedAfterAnotherOperation(t *testing.T) {
	m := newTestModel(t, withReport(domain.StatusReport{Repository: "fresh"}))
	m.screen = dashboardController
	m.controller.progressID = 7
	next, _ := m.Update(controllerStatusRefreshMsg{status: domain.StatusReport{Repository: "late"}, id: 6})
	if next.(dashboardModel).report.Repository != "fresh" {
		t.Fatal("stale controller status replaced a newer one")
	}
	m.screen = dashboardPXE
	next, _ = m.Update(controllerStatusRefreshMsg{status: domain.StatusReport{Repository: "late"}, id: 7})
	if next.(dashboardModel).report.Repository != "fresh" {
		t.Fatal("controller status replaced the installation's state")
	}
}
