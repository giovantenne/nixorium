package presentation

import (
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func overviewFitFixture(width, height int) dashboardModel {
	m := experienceFixture(2)
	m.actions.Assistant = domain.AssistantConfig{Enabled: true}
	m.recovery.Conditions = []domain.BlockingCondition{
		{Kind: domain.RecoveryBackupDue, Title: "A backup of this laboratory is due", Detail: "No backup has been made yet."},
		{Kind: domain.RecoveryDeferredUpdates, Title: "Queued updates: 1 computer needs attention", Detail: "Their configuration changed after queueing, or the last attempt failed; open the queue to review them."},
	}
	m.width, m.height = width, height
	return m
}

func TestOverviewShowsEveryAreaWhenItFits(t *testing.T) {
	for _, size := range [][2]int{{90, 30}, {80, 24}} {
		view := demoANSI.ReplaceAllString(overviewFitFixture(size[0], size[1]).View().Content, "")
		for _, expected := range []string{"Laboratory overview", "Items in yellow need your attention", "[c] Computers", "[n] Installation", "[w] Software", "Maintenance", "Assistant"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("overview %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}
		if strings.Contains(view, " of 7") || strings.Contains(view, "scroll") {
			t.Fatalf("overview %dx%d windows its menu although everything fits:\n%s", size[0], size[1], view)
		}
	}
	m := overviewFitFixture(90, 30)
	m.recovery.Conditions = nil
	view := demoANSI.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(view, "Choose an area with the arrows or its letter.") {
		t.Fatalf("overview lacks the generic subtitle:\n%s", view)
	}
}

func TestOverviewWindowKeepsCursorVisibleWhenSmall(t *testing.T) {
	m := overviewFitFixture(80, 18)
	for index := 0; index < 6; index++ {
		m = press(m, "j")
	}
	selected, _ := m.homeMenu.selected()
	view := demoANSI.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(view, " of 7") {
		t.Fatalf("a 18-row terminal must window the menu:\n%s", view)
	}
	if !strings.Contains(view, "] "+selected.title) {
		t.Fatalf("cursor row %q is not visible:\n%s", selected.title, view)
	}
	if strings.Contains(view, "scroll · ? help") {
		t.Fatalf("menu window still overflows the shell:\n%s", view)
	}
}

func TestOverviewCursorStaysVisibleInTinyTerminal(t *testing.T) {
	m := overviewFitFixture(80, 14)
	for index := 0; index < 6; index++ {
		m = press(m, "j")
	}
	selected, _ := m.homeMenu.selected()
	if view := demoANSI.ReplaceAllString(m.View().Content, ""); !strings.Contains(view, "] "+selected.title) {
		t.Fatalf("cursor row %q is not visible:\n%s", selected.title, view)
	}
}
