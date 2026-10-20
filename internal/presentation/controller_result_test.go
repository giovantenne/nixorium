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
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Applied: true, Verified: true, Message: "Controller operation completed."}
		},
		Refresh: func(context.Context) (domain.StatusReport, error) {
			<-refreshed
			return domain.StatusReport{Repository: "/home/admin/nixorium-deployment"}, nil
		},
	}, false)
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
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
	m.reopenNotice = "Nixorium and this controller are updated. Next: Computers → Update computers."
	next, _ := m.Update(dashboardInitialMsg{})
	m = next.(dashboardModel)
	if !strings.Contains(m.View().Content, "Nixorium and this controller are updated") || m.reopenNotice != "" {
		t.Fatalf("outcome not shown after reopening:\n%s", m.View().Content)
	}
	m.screen = dashboardUpdate
	if outcome := m.reloadOutcome(); !strings.Contains(outcome, "Update computers") {
		t.Fatalf("outcome = %q", outcome)
	}
}
