package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

func reloadTestActions(reload bool, checks *int) DashboardActions {
	return DashboardActions{
		RunningVersion: "1.0.0",
		InstalledInterface: func() (InterfaceReload, bool) {
			*checks++
			return InterfaceReload{Version: "1.1.0"}, reload
		},
	}
}

func updatedControllerModel(t testing.TB, actions DashboardActions) (dashboardModel, tea.Cmd) {
	model := newTestModel(t, withActions(actions))
	model.screen = dashboardUpdate
	model.updates.result = domain.UpdateApplyReport{Operation: "update-apply", State: "saved", Revision: "abc", Target: "v1.1.0", Updated: true}
	report := domain.ControllerRebuildExecutionReport{Operation: "controller-rebuild", State: "applied", Revision: "abc", Applied: true, Verified: true}
	updated, command := model.Update(dashboardUpdateControllerMsg{report: report})
	return updated.(dashboardModel), command
}

const reloadOfferLine = "Nixorium 1.1.0 is active on this controller. Enter reopens the interface with it."

func TestUpdateResultStaysUntilTheAdministratorChooses(t *testing.T) {
	checks := 0
	model, command := updatedControllerModel(t, reloadTestActions(true, &checks))
	view := model.View().Content
	if command != nil || checks != 1 || !strings.Contains(view, reloadOfferLine) || strings.Contains(view, reopenSentence) ||
		!strings.Contains(view, "Running interface: 1.0.0") || !strings.Contains(view, "Reopen with the update") || !strings.Contains(view, "Keep this interface") {
		t.Fatalf("reopen offer missing or timed (checks=%d):\n%s", checks, view)
	}
	if model.reload.state != interfaceReloadOffered {
		t.Fatalf("reload = %+v", model.reload)
	}
}

func TestUpdateResultEnterReopensAtOnce(t *testing.T) {
	checks := 0
	model, _ := updatedControllerModel(t, reloadTestActions(true, &checks))
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.reload.state != interfaceReloadRequested || command == nil {
		t.Fatalf("Enter did not request a reload: %+v", model.reload)
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("Enter did not quit the program")
	}
}

func TestUpdateResultEscapeKeepsRunningInterface(t *testing.T) {
	checks := 0
	model, _ := updatedControllerModel(t, reloadTestActions(true, &checks))
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	view := model.View().Content
	if command != nil || model.screen != dashboardUpdate || model.reload.state != interfaceReloadCancelled || !strings.Contains(view, reopenSentence) || strings.Contains(view, "is active") {
		t.Fatalf("Esc did not keep the running interface:\n%s", view)
	}
	// The next Esc leaves the result as before.
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.(dashboardModel).screen != dashboardHome {
		t.Fatal("second Esc did not leave the result")
	}
}

func TestUpdateResultWithoutDifferentBinaryKeepsOldSentence(t *testing.T) {
	checks := 0
	model, command := updatedControllerModel(t, reloadTestActions(false, &checks))
	view := model.View().Content
	if command != nil || checks != 1 || !strings.Contains(view, reopenSentence) || strings.Contains(view, "is active") {
		t.Fatalf("unexpected reload offer:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil && updated.(dashboardModel).reload.state == interfaceReloadRequested {
		t.Fatal("Enter requested a reload without an installed update")
	}
}

func TestReloadOfferWithoutVersion(t *testing.T) {
	model := newTestModel(t, withActions(DashboardActions{
		InstalledInterface: func() (InterfaceReload, bool) { return InterfaceReload{}, true },
	}))
	model.screen = dashboardUpdate
	model.updates.result = domain.UpdateApplyReport{Operation: "update-apply", State: "saved", Revision: "abc", Updated: true}
	updated, _ := model.Update(dashboardUpdateControllerMsg{report: domain.ControllerRebuildExecutionReport{Operation: "controller-rebuild", Revision: "abc", Applied: true, Verified: true}})
	if view := updated.(dashboardModel).View().Content; !strings.Contains(view, "The updated Nixorium is active on this controller. Enter reopens") {
		t.Fatalf("versionless offer missing:\n%s", view)
	}
}

func TestControllerResultOffersReload(t *testing.T) {
	checks := 0
	model := newTestModel(t, withActions(reloadTestActions(true, &checks)))
	model.screen = dashboardController
	model.controller.applying = true
	report := domain.ControllerRebuildExecutionReport{Operation: "controller-rebuild", State: "applied", Revision: "abc", Applied: true, Verified: true}
	updated, command := model.Update(dashboardControllerResultMsg{report: report})
	model = updated.(dashboardModel)
	view := model.View().Content
	if command != nil || !strings.Contains(view, reloadOfferLine) || !strings.Contains(view, "Applied to this controller and verified") {
		t.Fatalf("controller result has no reopen offer:\n%s", view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.reload.state != interfaceReloadRequested || command == nil || model.screen != dashboardController {
		t.Fatalf("Enter did not reopen from the controller result: %+v", model.reload)
	}

	model = newTestModel(t, withActions(reloadTestActions(true, &checks)))
	model.screen = dashboardController
	updated, _ = model.Update(dashboardControllerResultMsg{report: report})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardController || !strings.Contains(model.View().Content, reopenSentence) {
		t.Fatalf("Esc did not keep the controller result:\n%s", model.View().Content)
	}
}

func TestControllerFailureDoesNotOfferReload(t *testing.T) {
	checks := 0
	model := newTestModel(t, withActions(reloadTestActions(true, &checks)))
	model.screen = dashboardController
	report := domain.ControllerRebuildExecutionReport{Operation: "controller-rebuild", State: "failed", Revision: "abc"}
	updated, command := model.Update(dashboardControllerResultMsg{report: report})
	model = updated.(dashboardModel)
	if command != nil || checks != 0 || model.reload.state != interfaceReloadNone || strings.Contains(model.View().Content, "is active") {
		t.Fatalf("failed activation offered a reload (checks=%d):\n%s", checks, model.View().Content)
	}
}
