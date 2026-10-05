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
	model.pendingRevision = "222c026dcb9046c902363ee5120d7c03e4854ea2"
	if view := model.View().Content; !strings.Contains(view, "Saved configuration needs applying") {
		t.Fatalf("unapplied revision was not reported: %s", view)
	}
	model.controller.plan = domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "current", Current: true, Revision: model.pendingRevision}
	if view := model.View().Content; strings.Contains(view, "Saved configuration needs applying") {
		t.Fatalf("revision already running on the controller is still reported: %s", view)
	}
}
