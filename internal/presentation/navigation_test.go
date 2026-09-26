package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestEveryTaskHasVisibleUniqueShortcutAndMatchingEnterRoute(t *testing.T) {
	for _, menu := range []struct {
		screen dashboardScreen
		tasks  []dashboardTask
	}{
		{dashboardHome, dashboardTasks}, {dashboardComputersArea, computersAreaTasks}, {dashboardInstallationArea, installationAreaTasks}, {dashboardAdministration, administrationTasks},
	} {
		used := map[string]bool{"q": true, "j": true, "k": true}
		for index, task := range menu.tasks {
			if task.shortcut == "" || used[task.shortcut] {
				t.Fatalf("invalid shortcut %q in %v", task.shortcut, menu.screen)
			}
			used[task.shortcut] = true
			for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
				for _, dark := range []bool{false, true} {
					m := experienceFixture(2)
					m.screen = menu.screen
					m.width = size[0]
					m.height = size[1]
					m.isDark = dark
					m.homeMenu.list.Select(index)
					m.computers.areaCursor = index
					m.installationAreaCursor = index
					m.adminCursor = index
					view := m.View().Content
					if !strings.Contains(view, menuTitle(task.shortcut, task.title)) || !strings.Contains(view, "F1") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
						t.Fatalf("menu %v choice %s %v overflow or hidden action:\n%s", menu.screen, task.id, size, view)
					}
					shortcut, sc := m.Update(tea.KeyPressMsg{Text: task.shortcut})
					enter, ec := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					if shortcut.(dashboardModel).screen != enter.(dashboardModel).screen || (sc == nil) != (ec == nil) {
						t.Fatalf("shortcut and Enter differ: %s", task.id)
					}
				}
			}
		}
	}
}

func TestCanonicalDeploymentAndInstallationRoutes(t *testing.T) {
	m := experienceFixture(2)
	m = press(m, "c")
	m = press(m, "d")
	if m.screen != dashboardDeploy {
		t.Fatal("distribution missing")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(dashboardModel)
	if m.screen != dashboardComputersArea {
		t.Fatal("Esc lost the Computers parent")
	}
	if strings.Contains(m.View().Content, "Restore computers") {
		t.Fatal("duplicate restore route")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(dashboardModel)
	m = press(m, "n")
	if m.screen != dashboardInstallationArea {
		t.Fatal("canonical installation route missing")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next.(dashboardModel).screen != dashboardHome {
		t.Fatal("Installation did not return to Overview")
	}
}

func TestSettingsShortcutsDoNotStealNavigationOrSearch(t *testing.T) {
	for index, group := range routineSettingsGroups {
		m := experienceFixture(2)
		m.screen = dashboardSettings
		m.settings.current = wizardSettings()
		m.settings.menu = newRoutineSettingsMenu(false, 80, 24)
		if !strings.Contains(m.settings.menu.list.Items()[index].(routineSettingsGroupItem).Title(), "["+group.shortcut+"]") {
			t.Fatal("hidden category shortcut")
		}
		next, _ := m.Update(tea.KeyPressMsg{Text: group.shortcut})
		if next.(dashboardModel).screen != dashboardSettingsEdit {
			t.Fatalf("%s did not open", group.id)
		}
	}
	m := experienceFixture(2)
	m.screen = dashboardSettings
	m.settings.menu = newRoutineSettingsMenu(false, 80, 24)
	m.settings.menu.list.Select(2)
	m = press(m, "k")
	if m.screen != dashboardSettings || m.settings.menu.list.Index() != 1 {
		t.Fatal("k must move up, not open keys")
	}
	m = press(m, "/")
	m = press(m, "q")
	m = press(m, "n")
	if !m.settings.menu.filtering() || m.settings.menu.list.FilterValue() != "qn" || m.screen != dashboardSettings {
		t.Fatal("search text triggered navigation")
	}
}

func TestSoftwareTabsHaveDirectShortcutsIncludingDuringSearch(t *testing.T) {
	for index, key := range []rune{tea.KeyF2, tea.KeyF3, tea.KeyF4} {
		m := softwareModel{}.open()
		m.catalog = testSoftwareCatalogReport()
		m.searching = false
		direct, _ := m.update(tea.KeyPressMsg{Code: key})
		m.searching = true
		search, input := m.updateSearchInput(tea.KeyPressMsg{Code: key}, nil)
		if int(direct.mode) != index || int(search.mode) != index || !input.handled {
			t.Fatalf("tab %d not reachable", index)
		}
	}
}
