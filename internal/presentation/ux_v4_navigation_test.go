package presentation

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSetupRendersBusyActivity(t *testing.T) {
	model := experienceFixture(2)
	model.screen = dashboardSetup
	model.setupMode = true
	model.busy = "Refreshing first-run progress"

	if view := model.View().Content; !strings.Contains(view, model.busy) {
		t.Fatalf("setup hid its active operation:\n%s", view)
	}
}

func TestActivePXEExitUsesOneGlobalReview(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		screen dashboardScreen
		key    tea.KeyPressMsg
	}{
		{name: "home q", screen: dashboardHome, key: tea.KeyPressMsg{Text: "q"}},
		{name: "PXE q", screen: dashboardPXE, key: tea.KeyPressMsg{Text: "q"}},
		{name: "setup ctrl-c", screen: dashboardSetup, key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			model := experienceFixture(2)
			model.report.PXE.Mode = "active"
			model.screen = scenario.screen

			updated, command := model.Update(scenario.key)
			model = updated.(dashboardModel)
			if command != nil || model.screen != dashboardPXELeaveReview {
				t.Fatalf("exit bypassed active-PXE review: screen=%d", model.screen)
			}
			view := model.View().Content
			if !strings.Contains(view, "Stop installation mode") || !strings.Contains(view, "LEAVE PXE ACTIVE") {
				t.Fatalf("exit choices are incomplete:\n%s", view)
			}
		})
	}
}

func TestActivePXECanBeStoppedAndVerifiedBeforeExit(t *testing.T) {
	stops := 0
	model := experienceFixture(2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.actions.StopPXE = func() domain.PXELifecycleReport {
		stops++
		return domain.PXELifecycleReport{State: "stopped", Mode: "stopped", Message: "Installation mode stopped."}
	}
	model.actions.Refresh = func() (domain.StatusReport, error) {
		return testDashboardReport("stopped"), nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatal("stop-and-exit did not start")
	}
	updated, quit := model.Update(command())
	model = updated.(dashboardModel)
	if stops != 1 || quit == nil {
		t.Fatalf("stop-and-exit did not finish: stops=%d", stops)
	}
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("verified PXE stop did not quit")
	}
	if model.report.PXE.Mode != "stopped" {
		t.Fatalf("final PXE state = %q", model.report.PXE.Mode)
	}
}

func TestActivePXEStopFailureKeepsDashboardOpen(t *testing.T) {
	model := experienceFixture(2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.actions.StopPXE = func() domain.PXELifecycleReport {
		return domain.PXELifecycleReport{State: "failed", Mode: "active", Message: "network service did not stop"}
	}
	model.actions.Refresh = func() (domain.StatusReport, error) {
		return domain.StatusReport{}, errors.New("unavailable")
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	updated, quit := model.Update(command())
	model = updated.(dashboardModel)
	if quit != nil || model.screen != dashboardPXELeaveReview || !strings.Contains(model.message, "could not be stopped") {
		t.Fatalf("failed stop closed or lost recovery: %+v", model)
	}
}

func TestCancelledSoftwareRemovalReturnsToSelectedPackage(t *testing.T) {
	model := experienceFixture(2)
	model.screen = dashboardSoftware
	model.softwareCatalog = testSoftwareCatalogReport()
	model.softwareCursor = 1
	model.actions.PlanSoftware = func(request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
		return domain.SoftwareChangePlanReport{State: "ready", Request: request, Confirmation: "SAVE SOFTWARE token"}
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || model.softwareSelected != "vlc" || model.softwareCursor != 1 {
		t.Fatalf("removal cancellation lost its origin: screen=%d selected=%q cursor=%d", model.screen, model.softwareSelected, model.softwareCursor)
	}
	if strings.Contains(model.View().Content, "Add VLC") {
		t.Fatalf("removal cancellation opened an add flow:\n%s", model.View().Content)
	}
}

func TestRoutineFlowsStartWithoutStaleResults(t *testing.T) {
	model := experienceFixture(2)
	model.updateResult = domain.UpdateApplyReport{Operation: "update-apply", State: "applied"}
	model.actions.CheckUpdate = func() domain.UpdateCheckReport {
		return domain.UpdateCheckReport{Operation: "update-check", State: "current", CurrentRef: "v2.0.0"}
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	if model.updateResult.Operation != "" || command == nil {
		t.Fatal("Update retained a result from the previous session")
	}

	model.screen = dashboardHome
	model.busy = ""
	model.settingsResult = domain.ConfigApplyReport{Operation: "config-apply", State: "applied"}
	model.actions.LoadSettings = func() (domain.LabSettingsFile, error) { return domain.LabSettingsFile{}, nil }
	updated, command = model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if model.settingsResult.Operation != "" || command == nil {
		t.Fatal("Settings retained a result from the previous session")
	}
}
