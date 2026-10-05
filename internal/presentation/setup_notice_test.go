package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
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

func TestOverviewDoesNotAskForNetworkBootFilesAfterUSBInstallation(t *testing.T) {
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageArtifacts}, DashboardActions{}, false)
	model.screen = dashboardHome
	if view := model.View().Content; strings.Contains(view, "Installation files need preparing") {
		t.Fatalf("missing network boot files raised an alert: %s", view)
	}
}

func TestOverviewClearsSavedRevisionThatTheControllerAlreadyRuns(t *testing.T) {
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "ready"}, DashboardActions{}, false)
	model.screen = dashboardHome
	revision := "222c026dcb9046c902363ee5120d7c03e4854ea2"
	model.noteControllerSave(revision)
	if view := model.View().Content; !strings.Contains(view, "Saved configuration needs applying") {
		t.Fatalf("unapplied revision was not reported: %s", view)
	}
	updated, _ := model.Update(dashboardControllerPlanMsg{report: domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "current", Current: true, Revision: revision}})
	model = updated.(dashboardModel)
	model, _ = workspaceKey(model, demoCode(tea.KeyEscape))
	model, _ = workspaceKey(model, demoCode(tea.KeyEscape))
	if model.screen != dashboardHome {
		t.Fatal("did not return to Overview after controller review")
	}
	if view := model.View().Content; strings.Contains(view, "Saved configuration needs applying") {
		t.Fatalf("revision already running on the controller is still reported: %s", view)
	}
}
