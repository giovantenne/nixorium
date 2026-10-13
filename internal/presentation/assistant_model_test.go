package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantMenuRequiresInstalledConfigAndAdministratorRole(t *testing.T) {
	for _, test := range []struct {
		name                        string
		enabled, classroom, visible bool
	}{
		{"missing-or-disabled-config", false, false, false}, {"enabled", true, false, true}, {"classroom", true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{
				Assistant: domain.AssistantConfig{Enabled: test.enabled}, ClassroomMode: test.classroom,
			}, false)
			areas := m.homeAreas()
			if (len(areas) == 5) != test.visible || len(dashboardTasks) != 4 {
				t.Fatalf("areas=%+v", areas)
			}
			view := m.View().Content
			if strings.Contains(view, "Assistant") != test.visible {
				t.Fatalf("visibility mismatch: %s", view)
			}
			if test.visible && areas[4] != assistantTask {
				t.Fatal("Assistant is not fifth and last")
			}
			updated, _ := m.Update(keyPress(assistantTask.shortcut))
			m = updated.(dashboardModel)
			if (m.screen == dashboardAssistant) != test.visible {
				t.Fatal("shortcut bypassed installed configuration or classroom boundary")
			}
			if test.visible {
				updated, _ = m.Update(keyPress("esc"))
				if updated.(dashboardModel).screen != dashboardHome {
					t.Fatal("Esc did not return home")
				}
			}
			m.screen = dashboardHome
			m.helpOpen = true
			if strings.Contains(m.View().Content, "Assistant") != test.visible {
				t.Fatal("help visibility mismatch")
			}
		})
	}
}

func TestAssistantMenuSelectionAndLayouts(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{Assistant: domain.AssistantConfig{Enabled: true}}, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(dashboardModel)
		m.syncHomeTasks()
		m.homeMenu.list.Select(4)
		updated, _ = m.Update(tea.BackgroundColorMsg{})
		m = updated.(dashboardModel)
		updated, _ = m.Update(keyPress("enter"))
		m = updated.(dashboardModel)
		if m.screen != dashboardAssistant {
			t.Fatal("Enter did not open selected Assistant")
		}
		if lipgloss.Height(m.View().Content) > size[1] || lipgloss.Width(m.View().Content) > size[0] {
			t.Fatal("Assistant exceeds terminal")
		}
	}
}
