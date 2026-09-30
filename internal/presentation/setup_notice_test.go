package presentation

import (
	"github.com/giovantenne/nixorium/internal/domain"
	"strings"
	"testing"
)

func TestOverviewDistinguishesConfigurationFromPendingApplication(t *testing.T) {
	for _, test := range []struct {
		stage      string
		want       string
		incomplete bool
	}{
		{domain.SetupStageNetwork, "Computer installation is not configured yet", true},
		{domain.SetupStageKeys, "Computer installation is not configured yet", true},
		{domain.SetupStageValidate, "Computer installation is not configured yet", true},
		{domain.SetupStageApply, "Saved configuration needs applying to this controller", false},
		{domain.SetupStageArtifacts, "Installation files need preparing", false},
		{domain.SetupStageReview, "Managed configuration has uncommitted changes", false},
	} {
		t.Run(test.stage, func(t *testing.T) {
			model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "action-required", CurrentStage: test.stage}, DashboardActions{}, false)
			model.screen = dashboardHome
			view := model.View().Content
			if !strings.Contains(view, test.want) || strings.Contains(view, "Computer installation is not configured yet") != test.incomplete {
				t.Fatalf("misleading overview: %s", view)
			}
		})
	}
}
