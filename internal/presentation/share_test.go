package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSendDesktopReviewCancelAndApply(t *testing.T) {
	applied := 0
	actions := DashboardActions{
		PlanShare: func(ctx context.Context, requested string) domain.SharePlan {
			if requested != "pc01" {
				t.Fatalf("review %s", requested)
			}
			return domain.SharePlan{State: "ready", ReviewToken: "test", Message: "From /home/teacher/Desktop: 2 items (5 bytes) go to the desktop of 1 of 1 computers.", Targets: []domain.ShareTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true}}}
		},
		ApplyShare: func(plan domain.SharePlan) domain.ShareReport {
			applied++
			return domain.ShareReport{State: "completed", Message: "Delivered to 1 of 1 selected computers.", Targets: []domain.ShareOutcome{{Name: "pc01", State: "delivered", Detail: "On the desktop: notes.txt."}}}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	opened, _ := model.openComputerTask("s")
	model = opened.(dashboardModel)
	if model.screen != dashboardShare {
		t.Fatal("Send desktop did not open")
	}
	key := func(code rune) {
		updated, cmd := model.updateShare(tea.KeyPressMsg{Code: code})
		model = updated.(dashboardModel)
		if cmd != nil {
			updated, _ = model.Update(cmd())
			model = updated.(dashboardModel)
		}
	}
	key(' ')
	key(tea.KeyEnter)
	if model.share.stage != 1 || applied != 0 || !strings.Contains(model.View().Content, "2 items") {
		t.Fatalf("did not stop for review:\n%s", model.View().Content)
	}
	key(tea.KeyEscape)
	key(tea.KeyEnter)
	key(tea.KeyEnter)
	if applied != 1 || model.share.stage != 2 {
		t.Fatal("review not applied")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		model.width, model.height = size[0], size[1]
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overflow %dx%d:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "Send desktop") || !strings.Contains(view, "Delivered to 1 of 1") {
			t.Fatal(view)
		}
	}
	// Without the classroom view the task is neither listed nor opened.
	plain := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	for _, task := range plain.availableComputerTasks() {
		if task.id == "share" {
			t.Fatal("Send desktop listed without its actions")
		}
	}
	if opened, _ := plain.openComputerTask("s"); opened.(dashboardModel).screen == dashboardShare {
		t.Fatal("Send desktop opened without its actions")
	}
}
