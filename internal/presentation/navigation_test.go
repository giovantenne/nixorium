package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
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
					if !strings.Contains(demoANSI.ReplaceAllString(view, ""), menuTitle(task.shortcut, task.title)) || !strings.Contains(view, "F1") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
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
		if group.id == "workspace" {
			continue // The workspace has a dedicated typed editor, tested separately.
		}
		m := experienceFixture(2)
		m.screen = dashboardSettings
		m.settings.current = wizardSettings()
		m.settings.menu = newRoutineSettingsMenu(false, 80, 24)
		if !strings.Contains(m.settings.menu.list.Items()[index].(routineSettingsGroupItem).Title(), "["+group.shortcut+"]") {
			t.Fatal("hidden category shortcut")
		}
		next, _ := m.Update(tea.KeyPressMsg{Text: group.shortcut})
		want := dashboardSettingsEdit
		if group.id == "accounts" {
			want = dashboardSettingsAccounts // Names and passwords submenu.
		}
		if next.(dashboardModel).screen != want {
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

func TestDashboardAreaNavigationUsesVisibleSelection(t *testing.T) {
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	model.width = 90
	model.height = 30
	model.homeMenu.setSize(model.width, model.height)
	view := model.View().Content
	for _, area := range []string{"Computers", "Installation", "Software", "Maintenance"} {
		if !strings.Contains(view, area) {
			t.Fatalf("home area menu lacks %q:\n%s", area, view)
		}
	}
	if strings.Contains(view, "Update Nixorium") || !strings.Contains(view, "\x1b[") {
		t.Fatalf("home area menu lacks hierarchy or color:\n%s", view)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardComputersArea || !strings.Contains(model.View().Content, "Computer inventory") {
		t.Fatalf("computers area did not open: screen=%d", model.screen)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy {
		t.Fatalf("selected dashboard task did not open: screen=%d", model.screen)
	}

	model = newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	for _, shortcut := range []string{"r", "d", "x", "h", "s", "l", "g", "u", "e", "p", "f", "i"} {
		updated, _ = model.Update(tea.KeyPressMsg{Text: shortcut})
		model = updated.(dashboardModel)
		if model.screen != dashboardHome {
			t.Fatalf("hidden %q shortcut still bypasses area navigation: screen=%d", shortcut, model.screen)
		}
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if model.screen != dashboardPXE {
		t.Fatalf("PXE task did not open from Installation: screen=%d", model.screen)
	}
}

func TestRoutineScreensShareVisualTitleHierarchy(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), isDark: true, width: 100, height: 30}
	screens := map[string]string{
		"computers":  model.computersView(),
		"deploy":     model.deployView(),
		"controller": model.controllerView(),
		"services":   model.servicesView(),
		"logs":       model.logsView(),
		"git":        model.gitReviewView(),
		"update":     model.updateView(),
		"pxe":        model.pxeView(),
	}
	for name, view := range screens {
		if !strings.Contains(view, "Nixorium") || !strings.Contains(view, "\x1b[") {
			t.Errorf("%s screen lacks shared visual title hierarchy:\n%s", name, view)
		}
	}
}

func TestBusyScreensUseAnimatedSharedSpinner(t *testing.T) {
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	model.busy = "Validating candidate configuration"
	initial := model.busyView()
	updated, command := model.Update(model.activitySpinner.Tick())
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(initial, "Validating candidate configuration") || !strings.Contains(model.busyView(), "Validating candidate configuration") {
		t.Fatalf("busy spinner did not remain active: initial=%q current=%q", initial, model.busyView())
	}
}

func TestRoutineFlowsStartWithoutStaleResults(t *testing.T) {
	model := experienceFixture(2)
	model.updates.result = domain.UpdateApplyReport{Operation: "update-apply", State: "applied"}
	model.actions.CheckUpdate = func(ctx context.Context) domain.UpdateCheckReport {
		return domain.UpdateCheckReport{Operation: "update-check", State: "current", CurrentRef: "v2.0.0"}
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	if model.updates.result.Operation != "" || command == nil {
		t.Fatal("Update retained a result from the previous session")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)

	model.screen = dashboardHome
	model.busy = ""
	model.settings.result = domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved"}
	model.actions.LoadSettings = func(ctx context.Context) (domain.LabSettingsFile, error) { return domain.LabSettingsFile{}, nil }
	updated, _ = model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if model.settings.result.Operation != "" || command == nil {
		t.Fatal("Settings retained a result from the previous session")
	}
}

func TestIncompleteInitialConfigurationOpensSetupAndRemainsReachable(t *testing.T) {
	setup := domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}
	model := newDashboardModel(testDashboardReport("stopped"), setup, DashboardActions{}, false)
	if model.screen != dashboardSetup || !model.setupMode {
		t.Fatalf("incomplete initial configuration opened screen=%d setupMode=%t", model.screen, model.setupMode)
	}

	ready := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "ready"}, DashboardActions{}, false)
	found := false
	for _, task := range dashboardTasks {
		if task.id == "installation" && task.shortcut == "n" {
			found = true
		}
	}
	if !found || !strings.Contains(ready.View().Content, "Installation") {
		t.Fatal("configured labs cannot reopen computer installation from the intervention menu")
	}

	maintenance := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}, DashboardActions{}, false)
	if maintenance.screen != dashboardHome || !strings.Contains(maintenance.View().Content, "Computer installation is not configured yet") {
		t.Fatalf("a later readiness issue hijacked navigation or became invisible: screen=%d\n%s", maintenance.screen, maintenance.View().Content)
	}
}

func TestLoadingDashboardRendersBeforeInspectionAndThenRoutes(t *testing.T) {
	loads, settingsLoads := 0, 0
	setup := domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		LoadInitial: func(ctx context.Context) (domain.StatusReport, domain.SetupReport, error) {
			loads++
			return testDashboardReport("stopped"), setup, nil
		},
		LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) {
			settingsLoads++
			return wizardSettings(), nil
		},
	}, false)
	model.initializing = true
	model.busy = "Opening the laboratory and checking setup progress"
	if view := model.View().Content; !strings.Contains(view, model.busy) || strings.Contains(view, "Restore computers") || strings.Contains(view, "Laboratory overview") {
		t.Fatalf("startup activity is not visible before inspection:\n%s", view)
	}

	loading, load := model.loadInitial()
	updated, command := loading.(dashboardModel).Update(load())
	model = updated.(dashboardModel)
	if command == nil || loads != 1 || model.initializing || model.screen != dashboardSettings || !model.installation.flow {
		t.Fatalf("initial result did not route to laboratory settings: loads=%d model=%+v", loads, model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if settingsLoads != 1 || model.screen != dashboardSettingsEdit {
		t.Fatalf("startup settings did not open: settingsLoads=%d screen=%d", settingsLoads, model.screen)
	}
}

func TestLoadingDashboardFailureHasInPlaceRetry(t *testing.T) {
	attempts := 0
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		LoadInitial: func(ctx context.Context) (domain.StatusReport, domain.SetupReport, error) {
			attempts++
			if attempts == 1 {
				return domain.StatusReport{}, domain.SetupReport{}, errors.New("evaluation unavailable")
			}
			return testDashboardReport("stopped"), domain.SetupReport{State: "ready"}, nil
		},
	}, false)
	model.initializing = true
	model.busy = "Opening the laboratory and checking setup progress"

	loading, load := model.loadInitial()
	updated, _ := loading.(dashboardModel).Update(load())
	model = updated.(dashboardModel)
	failureView := model.View().Content
	if !model.initialError || !strings.Contains(failureView, "Enter") || !strings.Contains(failureView, "Try again") {
		t.Fatalf("startup failure has no recovery:\n%s", model.View().Content)
	}
	updated, retry := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if retry == nil || !model.initializing {
		t.Fatal("startup retry did not begin")
	}
	updated, _ = model.Update(retry())
	model = updated.(dashboardModel)
	if attempts != 2 || model.initialError || model.screen != dashboardHome || model.busy != "" {
		t.Fatalf("startup retry did not recover: attempts=%d model=%+v", attempts, model)
	}
}
