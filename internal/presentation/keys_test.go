package presentation

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// r refreshes observed state on every screen that offers a refresh; the old
// f key no longer acts.
func TestRefreshKeyIsROnEveryRefreshableScreen(t *testing.T) {
	loads := map[string]int{}
	actions := DashboardActions{
		LoadServices: func(context.Context) domain.ServicesReport {
			loads["services"]++
			return domain.ServicesReport{}
		},
		LoadLogs: func(context.Context) domain.OperationLogsReport {
			loads["logs"]++
			return domain.OperationLogsReport{}
		},
		LoadGitReview: func(context.Context) domain.GitReviewReport {
			loads["git"]++
			return domain.GitReviewReport{}
		},
		Refresh: func(context.Context) (domain.StatusReport, error) {
			loads["pxe"]++
			return testDashboardReport("stopped"), nil
		},
	}
	for name, screen := range map[string]dashboardScreen{"services": dashboardServices, "logs": dashboardLogs, "git": dashboardGitReview, "pxe": dashboardPXE} {
		model := dashboardModel{report: testDashboardReport("stopped"), actions: actions, screen: screen}
		if _, command := model.Update(tea.KeyPressMsg{Text: "f"}); command != nil {
			t.Fatalf("%s: f still schedules work", name)
		}
		_, command := model.Update(tea.KeyPressMsg{Text: "r"})
		if command == nil {
			t.Fatalf("%s: r does not refresh", name)
		}
		command()
		if loads[name] != 1 {
			t.Fatalf("%s: r loaded %d times", name, loads[name])
		}
	}
}
