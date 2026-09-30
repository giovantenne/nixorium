package presentation

import (
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
	"strings"
	"testing"
)

func TestSetupPasswordCancellationNamesSetup(t *testing.T) {
	for _, installation := range []bool{false, true} {
		model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{}, false)
		model.screen = dashboardSettingsPasswords
		model.settings.returnScreen = dashboardSetup
		model.installation.flow = installation
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		model = updated.(dashboardModel)
		if installation != strings.Contains(model.message, "Computer installation cancelled") || !strings.Contains(model.message, "no setting was changed") {
			t.Fatalf("wrong cancellation context: %s", model.message)
		}
	}
}

func TestPXEBusyQuitDescribesContinuingWork(t *testing.T) {
	model := dashboardModel{busy: "Preparing", installation: installationModel{pxePreparing: true}}
	// Test actions directly so narrow responsive labels cannot hide the meaning.
	for _, action := range model.pxeActions() {
		if action.key == "q" && strings.Contains(action.label, "Quit Nixorium") && strings.Contains(action.label, "work continues") {
			return
		}
	}
	t.Fatal("PXE detach does not describe its effect")
}

func TestUpdateDiscoverySelectsStableOrCurrentTarget(t *testing.T) {
	for _, test := range []struct {
		name   string
		report domain.UpdateCheckReport
		target string
	}{
		{"stable", domain.UpdateCheckReport{State: "available", Development: []domain.UpdateRelease{{Tag: "master"}}, Stable: []domain.UpdateRelease{{Tag: "v2.0.1"}, {Tag: "v2.0.0"}}}, "v2.0.1"},
		{"current", domain.UpdateCheckReport{State: "available", CurrentRef: "v2.1.0-beta.1", CurrentChannel: domain.UpdateChannelPrerelease, Development: []domain.UpdateRelease{{Tag: "master"}}, Prerelease: []domain.UpdateRelease{{Tag: "v2.1.0-beta.1"}}}, "v2.1.0-beta.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := dashboardModel{}
			updated, _ := model.Update(dashboardUpdateCheckMsg{report: test.report})
			model = updated.(dashboardModel)
			releases := model.availableUpdateReleases()
			if releases[model.updates.cursor].Tag != test.target {
				t.Fatalf("selected %s, want %s", releases[model.updates.cursor].Tag, test.target)
			}
		})
	}
}
