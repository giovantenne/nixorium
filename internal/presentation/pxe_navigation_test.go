package presentation

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestPXEHasOneEntryAndStateSpecificPrimaryAction(t *testing.T) {
	if len(installationAreaTasks) != 2 || installationAreaTasks[0].shortcut != "p" || installationAreaTasks[1].shortcut != "u" {
		t.Fatal("Installation must offer only PXE and USB methods")
	}
	for _, tc := range []struct {
		mode  string
		ready bool
		want  string
	}{
		{"stopped", false, "configure"}, {"ready", true, "start-review"},
		{"active", true, "stop"}, {"recovery-required", true, "recover"}, {"degraded", false, "recover"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			called := ""
			report := testDashboardReport(tc.mode)
			report.PXEPreparation.Ready = tc.ready
			m := experienceFixture(2)
			m.screen = dashboardInstallationArea
			m.actions = DashboardActions{
				LoadRemoteInstall: func() (domain.RemoteInstallResponse, error) {
					t.Fatal("PXE must not wait for the USB worker")
					return domain.RemoteInstallResponse{}, nil
				},
				Refresh:      func() (domain.StatusReport, error) { return report, nil },
				LoadSettings: func() (domain.LabSettingsFile, error) { called = "configure"; return wizardSettings(), nil },
				PlanPXEStart: func() domain.PXELifecycleReport {
					called = "start-review"
					return domain.PXELifecycleReport{State: "ready"}
				},
				StartPXE: func() domain.PXELifecycleReport {
					t.Fatal("entry/primary action bypassed typed start confirmation")
					return domain.PXELifecycleReport{}
				},
				StopPXE:    func() domain.PXELifecycleReport { called = "stop"; return domain.PXELifecycleReport{} },
				RecoverPXE: func() domain.PXELifecycleReport { called = "recover"; return domain.PXELifecycleReport{} },
			}
			next, command := m.Update(tea.KeyPressMsg{Text: "p"})
			m = next.(dashboardModel)
			if command == nil || called != "" {
				t.Fatal("entry must only observe current state")
			}
			next, _ = m.Update(command())
			m = next.(dashboardModel)
			if m.screen != dashboardPXE || !strings.Contains(m.View().Content, m.pxePrimaryAction().label) {
				t.Fatal("missing primary action")
			}
			next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if command == nil {
				t.Fatal("Enter did not schedule the visible action")
			}
			next, _ = next.Update(command())
			if called != tc.want {
				t.Fatalf("called %q, want %q", called, tc.want)
			}
			if tc.want == "start-review" && next.(dashboardModel).screen != dashboardPXEStartReview {
				t.Fatal("start must open the existing review")
			}
		})
	}
}

func TestPXEFailedObservationCannotActOnPreviousReadiness(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardInstallationArea
	m.report.PXEPreparation.Ready = true
	m.actions.Refresh = func() (domain.StatusReport, error) { return domain.StatusReport{}, errors.New("unavailable") }
	next, command := m.Update(tea.KeyPressMsg{Text: "p"})
	next, _ = next.Update(command())
	m = next.(dashboardModel)
	if !m.installation.stateError || !strings.Contains(m.View().Content, "Retry status") {
		t.Fatal("observation failure is not actionable")
	}
	for _, key := range []string{"p", "s", "x", "r"} {
		_, command = m.Update(tea.KeyPressMsg{Text: key})
		if command != nil {
			t.Fatalf("stale state authorized %s", key)
		}
	}
	_, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("retry missing")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next.(dashboardModel).screen != dashboardInstallationArea {
		t.Fatal("lost Installation parent")
	}
}
