package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func lockActions(t *testing.T, applied *int) DashboardActions {
	return DashboardActions{
		PlanLock: func(ctx context.Context, requested string, action domain.LockAction) domain.LockPlan {
			if requested != "pc01" || action != domain.LockOn {
				t.Fatalf("review %s %s", requested, action)
			}
			return domain.LockPlan{State: "ready", Action: action, ReviewToken: "test", Message: "1 of 1 computers available.", Targets: []domain.LockTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true, Reachable: true}}}
		},
		ApplyLock: func(plan domain.LockPlan) domain.LockReport {
			*applied++
			return domain.LockReport{State: "completed", Action: plan.Action, Message: "Locked 1 of 1 selected computers.", Targets: []domain.LockOutcome{{Name: "pc01", State: "verified", Detail: "Locked."}}}
		},
	}
}

func TestLockScreensReviewCancelAndApply(t *testing.T) {
	applied := 0
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), lockActions(t, &applied), false)
	opened, _ := model.openComputerTask("l")
	model = opened.(dashboardModel)
	if model.screen != dashboardLock {
		t.Fatal("Lock screens did not open")
	}
	key := func(code rune) {
		updated, cmd := model.updateLock(tea.KeyPressMsg{Code: code})
		model = updated.(dashboardModel)
		if cmd != nil {
			updated, _ = model.Update(cmd())
			model = updated.(dashboardModel)
		}
	}
	key(tea.KeyEnter)
	if model.lock.stage != 0 || !strings.Contains(model.message, "Select at least one") {
		t.Fatalf("empty selection: stage %d, %q", model.lock.stage, model.message)
	}
	key(' ')
	key(tea.KeyEnter)
	if model.lock.stage != 1 || applied != 0 {
		t.Fatal("did not stop for review")
	}
	key(tea.KeyEscape)
	if applied != 0 || model.lock.stage != 0 {
		t.Fatal("cancel applied the change")
	}
	key(tea.KeyEnter)
	key(tea.KeyEnter)
	if applied != 1 || model.lock.stage != 2 {
		t.Fatal("review not applied")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		model.width, model.height = size[0], size[1]
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overflow %dx%d:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "Lock screens") || !strings.Contains(view, "Locked 1 of 1") {
			t.Fatal(view)
		}
	}
}

func TestLockScreensEntryNeedsTheClassroomView(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), width: 120, height: 30, screen: dashboardComputersArea}
	for _, task := range model.availableComputerTasks() {
		if task.id == "lock" {
			t.Fatal("Lock screens listed without lock actions")
		}
	}
	if opened, _ := model.openComputerTask("l"); opened.(dashboardModel).screen == dashboardLock {
		t.Fatal("Lock screens opened without lock actions")
	}
	applied := 0
	model.actions = lockActions(t, &applied)
	found := false
	for _, task := range model.availableComputerTasks() {
		found = found || task.id == "lock"
	}
	if !found || !strings.Contains(model.View().Content, "Lock screens") {
		t.Fatalf("Lock screens missing:\n%s", model.View().Content)
	}
	model.actions.ClassroomMode = true
	found = false
	for _, task := range model.availableComputerTasks() {
		found = found || task.id == "lock"
	}
	if !found {
		t.Fatal("Lock screens missing for the teacher")
	}
}
