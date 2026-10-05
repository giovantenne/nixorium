package presentation

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSendFilesReviewCancelAndApply(t *testing.T) {
	applied := 0
	actions := DashboardActions{
		PlanShare: func(ctx context.Context, requested, path string) domain.SharePlan {
			if requested != "pc01" || path != "/srv/lesson" {
				t.Fatalf("review %s %s", requested, path)
			}
			return domain.SharePlan{State: "ready", ReviewToken: "test", Message: "Sending lesson: 2 items (5 bytes) go to the desktop of 1 of 1 computers.", Targets: []domain.ShareTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true}}}
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
		t.Fatal("Send files did not open")
	}
	if home, _ := os.UserHomeDir(); model.share.path != home+"/" {
		t.Fatalf("path starts at %q, not the home folder", model.share.path)
	}
	model.share.path = "/srv/lesso"
	updated, _ := model.updateShare(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = updated.(dashboardModel)
	updated, _ = model.updateShare(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
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
	if model.share.stage != shareStageReview || applied != 0 || !strings.Contains(model.View().Content, "2 items") {
		t.Fatalf("did not stop for review:\n%s", model.View().Content)
	}
	key(tea.KeyEscape)
	key(tea.KeyEnter)
	key(tea.KeyEnter)
	if applied != 1 || model.share.stage != shareStageResult {
		t.Fatal("review not applied")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		model.width, model.height = size[0], size[1]
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overflow %dx%d:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "Send files") || !strings.Contains(view, "Delivered to 1 of 1") {
			t.Fatal(view)
		}
	}
	// Without the classroom view the task is neither listed nor opened.
	plain := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	for _, task := range plain.availableComputerTasks() {
		if task.id == "share" {
			t.Fatal("Send files listed without its actions")
		}
	}
	if opened, _ := plain.openComputerTask("s"); opened.(dashboardModel).screen == dashboardShare {
		t.Fatal("Send files opened without its actions")
	}
}
