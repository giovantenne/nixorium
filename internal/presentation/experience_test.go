package presentation

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestRenderingPreservesMeaningWithLimitedColor(t *testing.T) {
	for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
		m := experienceFixture(2)
		var output bytes.Buffer
		writer := colorprofile.Writer{Forward: &output, Profile: profile}
		if _, err := writer.Write([]byte(m.View().Content)); err != nil {
			t.Fatal(err)
		}
		text := output.String()
		if !strings.Contains(text, "Nixorium") || !strings.Contains(text, "Overview") || !strings.Contains(text, "Laboratory overview") || !strings.Contains(text, "Computers") || !strings.Contains(text, "Installation") || !strings.Contains(text, "Software") || !strings.Contains(text, "Maintenance") {
			t.Fatalf("meaning lost with %v", profile)
		}
		if strings.Contains(text, "38;2;") {
			t.Fatalf("truecolor leaked into %v", profile)
		}
	}
}

func TestOverviewAndMaintenanceUseStableShell(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		m := experienceFixture(2)
		m.width = size[0]
		m.height = size[1]
		view := m.View().Content
		for _, expected := range []string{"Nixorium", "Overview", "Laboratory overview", "Enter", "Open", "Help"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("overview %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overview overflow at %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}

		m.screen = dashboardAdministration
		view = m.View().Content
		for _, expected := range []string{"Nixorium", "Maintenance", "Enter", "Open", "Esc", "Overview"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("maintenance %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("maintenance overflow at %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}
	}

	m := experienceFixture(2)
	m.report.PXE.Mode = "recovery-required"
	view := m.View().Content
	if !strings.Contains(view, "NOTICE") || !strings.Contains(view, "Controller network recovery required") || !strings.Contains(view, "Enter") {
		t.Fatalf("overview warning is not separated from its action bar:\n%s", view)
	}
}

func TestNetworkInstallationShellKeepsPrimaryActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		states := []struct {
			name     string
			model    dashboardModel
			expected []string
		}{
			{
				name: "ready",
				model: dashboardModel{
					report: func() domain.StatusReport {
						report := testDashboardReport("ready")
						report.PXEPreparation.Ready = true
						return report
					}(),
					screen: dashboardPXE,
				},
				expected: []string{"Network installation", "Prepare", "Start PXE", "Esc", "Help"},
			},
			{
				name:     "setup installation",
				model:    dashboardModel{report: testDashboardReport("ready"), screen: dashboardPXE, setupMode: true},
				expected: []string{"Install computers", "Start PXE", "Esc", "Help"},
			},
			{
				name:     "active setup installation",
				model:    dashboardModel{report: testDashboardReport("active"), screen: dashboardPXE, setupMode: true},
				expected: []string{"Install computers", "Stop PXE", "Quit", "Help"},
			},
			{
				name: "start review",
				model: dashboardModel{
					report:    testDashboardReport("ready"),
					screen:    dashboardPXEStartReview,
					startPlan: domain.PXELifecycleReport{Interface: "eth0", StaticCIDR: "10.0.0.99/24", DHCPAddress: "192.168.1.10"},
				},
				expected: []string{"Type START to continue", "Enter", "Start PXE", "Esc", "Cancel", "Help"},
			},
			{
				name:     "recovery",
				model:    dashboardModel{report: testDashboardReport("recovery-required"), screen: dashboardPXE, setupMode: true},
				expected: []string{"recovery", "Recover", "Esc", "Installation", "Help"},
			},
		}

		for _, state := range states {
			state.model.width = size[0]
			state.model.height = size[1]
			view := state.model.View().Content
			for _, expected := range state.expected {
				if !strings.Contains(view, expected) {
					t.Fatalf("%s %dx%d lacks %q:\n%s", state.name, size[0], size[1], expected, view)
				}
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflows at %dx%d: %dx%d", state.name, size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestSoftwareShellKeepsContextAndActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		catalog := testSoftwareCatalogReport()
		states := []struct {
			name     string
			model    dashboardModel
			expected []string
		}{
			{
				name:     "configured",
				model:    dashboardModel{screen: dashboardSoftware, software: softwareModel{catalog: catalog}},
				expected: []string{"Software", "Selected", "Review removal", "Tab", "Change view", "/", "Search", "Esc", "Overview", "F1", "Help"},
			},
			{
				name: "search input",
				model: dashboardModel{
					screen: dashboardSoftware,
					software: softwareModel{
						catalog: catalog, mode: softwareSearch,
						searching: true, query: "gi",
					},
				},
				expected: []string{"Search packages", "Package name", "gi_", "Type", "Search", "Results", "Stop typing", "Help"},
			},
			{
				name: "scope",
				model: dashboardModel{
					screen: dashboardSoftware,
					software: softwareModel{
						stage:        softwareScope,
						catalog:      catalog,
						selected:     "gimp",
						scopeCursor:  len((softwareModel{catalog: catalog}).scopeOptions()) - 1,
						clientCursor: len(catalog.Clients) - 1,
						clients:      map[string]bool{"pc03": true},
					},
				},
				expected: []string{"Software  /  Scope", "pc03", "Space", "Toggle", "Enter", "Review", "Esc", "Catalog", "Help"},
			},
			{
				name: "review",
				model: dashboardModel{
					screen: dashboardSoftware,
					software: softwareModel{
						stage:   softwareReview,
						catalog: catalog,
						plan: domain.SoftwareChangePlanReport{
							Request:     domain.SoftwareChangeRequest{Package: "gimp", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}},
							ManagedFile: "lab-software.json", AffectedClients: catalog.Clients,
						},
					},
				},
				expected: []string{"Software  /  Review", "Validated against the pinned package set", "Enter", "Save", "Esc", "Scope", "Help"},
			},
			{
				name: "partial result",
				model: dashboardModel{
					screen: dashboardSoftware,
					software: softwareModel{
						stage: softwareResult,
						result: domain.SoftwareChangeApplyReport{
							State: "partial", Message: "Durability could not be confirmed.",
							Issues: []domain.ValidationIssue{{Field: "durability", Message: "directory sync failed"}},
						},
					},
				},
				expected: []string{"Software  /  Result", "needs attention", "Retry save", "Esc", "Overview", "Help"},
			},
		}

		for _, state := range states {
			state.model.width = size[0]
			state.model.height = size[1]
			view := state.model.View().Content
			for _, expected := range state.expected {
				if !strings.Contains(view, expected) {
					t.Fatalf("%s %dx%d lacks %q:\n%s", state.name, size[0], size[1], expected, view)
				}
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflows at %dx%d: %dx%d", state.name, size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestConfiguredSoftwareViewportKeepsFocusedItemVisible(t *testing.T) {
	catalog := domain.SoftwareCatalogReport{State: "ready", ManagedFile: "lab-software.json", Issues: []domain.ValidationIssue{}}
	for index := 1; index <= 24; index++ {
		id := fmt.Sprintf("package-%02d", index)
		label := fmt.Sprintf("Package %02d", index)
		catalog.Catalog = append(catalog.Catalog, domain.SoftwareCatalogItem{ID: id, Label: label, Summary: "Configured classroom software", Availability: "available"})
		catalog.Packages = append(catalog.Packages, domain.SoftwareDeclaration{Package: id, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"})
	}

	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		model := dashboardModel{
			screen: dashboardSoftware, width: size[0], height: size[1], isDark: true,
			software: softwareModel{catalog: catalog, mode: softwareConfigured},
		}
		for index := range catalog.Packages {
			model.software.cursor = index
			view := model.View().Content
			label := fmt.Sprintf("%-20s", fmt.Sprintf("Package %02d", index+1))
			if !strings.Contains(view, tuiSelection(label, true, true)) {
				t.Fatalf("focused software %d hidden at %dx%d:\n%s", index+1, size[0], size[1], view)
			}
			if !strings.Contains(view, "software selections") || !strings.Contains(view, "Enter") || !strings.Contains(view, "Review removal") {
				t.Fatalf("software viewport context or actions hidden at %dx%d:\n%s", size[0], size[1], view)
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("software viewport overflow at %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestConfiguredSoftwareListSummarizesExplicitClientScope(t *testing.T) {
	catalog := testSoftwareCatalogReport()
	clients := make([]string, 20)
	for index := range clients {
		clients[index] = fmt.Sprintf("pc%02d", index+1)
	}
	catalog.Packages[0].Scope = domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: clients}
	model := dashboardModel{
		screen: dashboardSoftware, width: 80, height: 24,
		software: softwareModel{catalog: catalog, mode: softwareConfigured},
	}
	view := model.View().Content
	if !strings.Contains(view, "20 selected clients") || strings.Contains(view, "pc01, pc02") {
		t.Fatalf("configured software scope is not compact:\n%s", view)
	}
}

func TestControllerMaintenanceShellKeepsValidActionsVisible(t *testing.T) {
	services := domain.ServicesReport{Services: []domain.ManagedService{{
		ID: "cache", Name: "Binary cache", State: "healthy", Detail: "HTTP-ready",
		Units: []domain.ServiceState{{Name: "nixorium-harmonia.service", Loaded: true, Active: true, State: "active"}},
	}}}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		states := []struct {
			name     string
			model    dashboardModel
			expected []string
		}{
			{
				name:     "controller idle",
				model:    dashboardModel{screen: dashboardController},
				expected: []string{"Maintenance  /  Controller", "Create review", "Esc", "Maintenance", "Help"},
			},
			{
				name: "controller progress",
				model: dashboardModel{
					screen: dashboardController, controllerApplying: true, busy: "Activating controller", controllerStarted: time.Now(),
					controllerProgress: domain.OperationProgress{Operation: "controller-apply", State: "running", Phase: "activate", Current: 2, Total: 4},
				},
				expected: []string{"Controller update is running", "Progress details", "Help"},
			},
			{
				name: "controller result",
				model: dashboardModel{
					screen:           dashboardController,
					controllerResult: domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true},
				},
				expected: []string{"Controller updated and verified", "Enter", "Maintenance", "Show details", "Logs", "New review", "Help"},
			},
			{
				name:     "services",
				model:    dashboardModel{screen: dashboardServices, services: services},
				expected: []string{"Maintenance  /  Services", "Binary cache", "Review cache restart", "Refresh", "Esc", "Maintenance", "Help"},
			},
			{
				name: "service result",
				model: dashboardModel{
					screen:        dashboardServices,
					serviceResult: domain.ServiceActionReport{Operation: "service-restart", State: "completed", Service: "cache", Verified: true},
				},
				expected: []string{"Binary cache restarted and verified", "Enter", "Maintenance", "Logs", "Restart again", "Help"},
			},
		}

		for _, state := range states {
			state.model.width = size[0]
			state.model.height = size[1]
			view := state.model.View().Content
			for _, expected := range state.expected {
				if !strings.Contains(view, expected) {
					t.Fatalf("%s %dx%d lacks %q:\n%s", state.name, size[0], size[1], expected, view)
				}
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflows at %dx%d: %dx%d", state.name, size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestEvidenceScreensKeepNavigationVisible(t *testing.T) {
	entry := domain.OperationLogEntry{ID: "deploy-example.log", Kind: "deployment", State: "completed", Available: true}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		states := []struct {
			name     string
			model    dashboardModel
			expected []string
		}{
			{
				name: "diagnostics",
				model: dashboardModel{
					screen: dashboardDiagnostics,
					doctor: domain.DoctorReport{Findings: []domain.Finding{{Level: domain.LevelWarning, Summary: "Check connection", Remediation: "Connect the network cable"}}},
				},
				expected: []string{"Maintenance  /  Diagnostics", "Check connection", "Enter", "Evidence", "Check again", "Esc", "Maintenance", "Help"},
			},
			{
				name: "history",
				model: dashboardModel{
					screen: dashboardLogs,
					logs:   domain.OperationLogsReport{Logs: []domain.OperationLogEntry{entry}},
				},
				expected: []string{"Maintenance  /  History", "deploy-example.log", "Enter", "View tail", "Refresh", "Esc", "Maintenance", "Help"},
			},
			{
				name: "log detail",
				model: dashboardModel{
					screen:    dashboardLogDetail,
					logDetail: domain.OperationLogReport{Log: &entry, Content: "first\nsecond\nthird\n"},
				},
				expected: []string{"History  /  Log", "deploy-example.log", "Scroll", "Esc", "History", "Help"},
			},
		}

		for _, state := range states {
			state.model.width = size[0]
			state.model.height = size[1]
			view := state.model.View().Content
			for _, expected := range state.expected {
				if !strings.Contains(view, expected) {
					t.Fatalf("%s %dx%d lacks %q:\n%s", state.name, size[0], size[1], expected, view)
				}
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflows at %dx%d: %dx%d", state.name, size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestConfigurationReviewHelpCannotApply(t *testing.T) {
	m := configReviewModel{}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	m = next.(configReviewModel)
	if !m.helpOpen || !m.View().AltScreen {
		t.Fatal("help must stay in the review screen")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Text: "y"})
	m = next.(configReviewModel)
	if m.accepted || cmd != nil {
		t.Fatal("help accepted a configuration change")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(configReviewModel)
	if m.helpOpen || m.cancelled {
		t.Fatal("escape should only close help")
	}
}

func experienceFixture(count int) dashboardModel {
	now := time.Now()
	status := testDashboardReport("ready")
	status.GeneratedAt = now
	status.Repository = "/lab"
	status.Git.Available = true
	status.Services = []domain.ServiceState{{Name: "nixorium-harmonia.service", Loaded: true, Active: true, State: "active"}}
	status.Meta.Clients.Count = count
	status.Meta.Clients.Hosts = nil
	hosts := domain.HostsReport{GeneratedAt: now, Repository: "/lab", Deployment: domain.HostDeploymentSummary{Current: count}}
	for i := 1; i <= count; i++ {
		name, ip := fmt.Sprintf("pc%02d", i), fmt.Sprintf("10.0.0.%d", i)
		status.Meta.Clients.Hosts = append(status.Meta.Clients.Hosts, domain.HostMeta{Name: name, IP: ip})
		hosts.Hosts = append(hosts.Hosts, domain.HostStatus{Name: name, IP: ip, SSH: domain.SSHAvailable, Reachability: domain.ReachabilityReachable, Deployment: domain.DeploymentCurrent})
	}
	m := newDashboardModel(status, domain.SetupReport{State: "ready"}, DashboardActions{}, false)
	m.hosts = hosts
	m.width = 120
	m.height = 30
	return m
}

func press(m dashboardModel, text string) dashboardModel {
	updated, _ := m.Update(tea.KeyPressMsg{Text: text})
	return updated.(dashboardModel)
}

func hostMetaNames(hosts []domain.HostMeta) []string {
	names := make([]string, 0, len(hosts))
	for _, host := range hosts {
		names = append(names, host.Name)
	}
	return names
}

func TestComputersSearchAndDetailsPreserveTargetIdentity(t *testing.T) {
	m := experienceFixture(200)
	m.screen = dashboardHosts
	m = press(m, "/")
	m = press(m, "pc17")
	if len(m.filteredHosts()) != 11 {
		t.Fatalf("search matches: %d", len(m.filteredHosts()))
	}
	// q is text while searching, never a quit operation.
	updated, command := m.Update(tea.KeyPressMsg{Text: "q"})
	m = updated.(dashboardModel)
	if command != nil || !strings.Contains(m.hostQuery, "q") {
		t.Fatal("search stole quit key")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(dashboardModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(dashboardModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(dashboardModel)
	if !m.hostDetail || !strings.Contains(m.View().Content, "pc17") {
		t.Fatal("detail not opened")
	}
	m = press(m, "d")
	if m.screen != dashboardDeploy || !m.deployChosen["pc17"] || len(m.deployChosen) != 1 {
		t.Fatalf("wrong deploy scope: %v", m.deployChosen)
	}
}

func TestComputerInventoryShellKeepsActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		m := experienceFixture(200)
		m.width = size[0]
		m.height = size[1]
		m.screen = dashboardHosts
		view := m.View().Content
		for _, expected := range []string{"Computers", "Inventory", "pc01", "Refresh", "Esc", "Help"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("inventory %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("inventory overflow at %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestInterventionEntryDoesNotScanTheFleet(t *testing.T) {
	m := experienceFixture(2)
	m = press(m, "c")
	updated, command := m.Update(tea.KeyPressMsg{Text: "r"})
	next := updated.(dashboardModel)
	if command != nil || next.screen != dashboardRestore {
		t.Fatal("restore entry started an implicit scan")
	}
	if strings.Contains(m.View().Content, "reachable") || strings.Contains(m.View().Content, "Everything looks good") {
		t.Fatal("intervention entry still presents fleet health")
	}
}

func TestHelpAndScrollingCannotConfirmMutation(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardDeployReview
	m.deployPlan = domain.DeploymentPlanReport{ColmenaSelector: "@lab"}
	m.confirmation = "DEPLOY"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	m = updated.(dashboardModel)
	if !m.helpOpen {
		t.Fatal("F1 help not opened")
	}
	updated, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(dashboardModel)
	if command != nil || m.deploying {
		t.Fatal("help confirmed deployment")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(dashboardModel)
	if m.helpOpen || m.screen != dashboardDeployReview {
		t.Fatal("help back lost review")
	}
	updated, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(dashboardModel)
	if command != nil || m.confirmation != "" || m.screen != dashboardDeploy {
		t.Fatal("cancel mutated")
	}
}

func TestLayoutKeepsFocusedComputerAndReviewVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, screen := range []dashboardScreen{dashboardHome, dashboardComputersArea, dashboardInstallationArea, dashboardRestore, dashboardSoftware, dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult, dashboardHosts, dashboardDeploy, dashboardDeployReview, dashboardServicesRestartReview, dashboardControllerReview, dashboardPXEStartReview, dashboardPXELeaveReview, dashboardSetup, dashboardSetupKeys, dashboardUpdate, dashboardAdministration} {
			m := experienceFixture(200)
			m.width = size[0]
			m.height = size[1]
			m.screen = screen
			m.hostCursor = 199
			m.deployCursor = 199
			m.deployPlan = domain.DeploymentPlanReport{Revision: strings.Repeat("a", 40), ColmenaSelector: "@lab", Targets: []domain.DeploymentTarget{{Name: "pc01"}}}
			m.software.catalog = domain.SoftwareCatalogReport{
				State: "ready", ManagedFile: "lab-software.json",
				Catalog: []domain.SoftwareCatalogItem{{ID: "gimp", Label: "GIMP", Summary: "Edit bitmap images", Availability: "available"}},
				Clients: hostMetaNames(m.report.Meta.Clients.Hosts), Groups: map[string][]string{}, Issues: []domain.ValidationIssue{},
			}
			m.software.selected = "gimp"
			m.software.scopeCursor = len(m.software.scopeOptions()) - 1
			m.software.clientCursor = 199
			m.software.plan = domain.SoftwareChangePlanReport{State: "ready", ManagedFile: "lab-software.json", Request: domain.SoftwareChangeRequest{Package: "gimp", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}, AffectedClients: m.software.catalog.Clients, Confirmation: "SAVE"}
			m.software.result = domain.SoftwareChangeApplyReport{State: "saved", ManagedFile: "lab-software.json"}
			m.shutdownCursor = 199
			m.shutdownChosen = map[string]bool{"pc200": true}
			m.shutdownPlan = domain.ShutdownPlanReport{State: "ready", Eligible: 1, Policy: domain.ShutdownProtectUnknown, Confirmation: "SHUTDOWN", Targets: []domain.ShutdownTargetPlan{{Name: "pc200", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionIdle, Eligible: true}}}
			m.shutdownResult = domain.ShutdownApplyReport{State: "completed", Accepted: 1, Targets: []domain.ShutdownTargetOutcome{{Name: "pc200", State: "accepted", Detail: "request accepted"}}}
			m.controllerPlan = domain.ControllerRebuildPlanReport{Controller: "pc99", Revision: strings.Repeat("a", 40), Confirmation: "REBUILD"}
			m.startPlan = domain.PXELifecycleReport{Interface: "eth0", StaticCIDR: "10.0.0.99/24", DHCPAddress: "192.168.1.10"}
			if screen == dashboardPXELeaveReview {
				m.setupMode = true
				m.report.PXE.Mode = "active"
			}
			m.updateCheck = domain.UpdateCheckReport{Operation: "update-check", State: "available", CurrentRef: "v2.2.0", Upstream: "github:giovantenne/nixorium", Stable: []domain.UpdateRelease{{Tag: "v2.3.0", Channel: domain.UpdateChannelStable}, {Tag: "v2.2.0", Channel: domain.UpdateChannelStable}}}
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%dx%d screen %d overflow: %dx%d", size[0], size[1], screen, lipgloss.Width(view), lipgloss.Height(view))
			}
			if screen == dashboardHosts || screen == dashboardDeploy || screen == dashboardShutdown {
				if !strings.Contains(view, "pc200") {
					t.Fatalf("focused row hidden screen %d size %v", screen, size)
				}
			}
			if screen == dashboardShutdown && (!strings.Contains(view, "No request is queued") || !strings.Contains(view, "Space") || !strings.Contains(view, "Select")) {
				t.Fatalf("shutdown guidance/footer hidden at size %v:\n%s", size, view)
			}
			if screen == dashboardDeployReview {
				if !strings.Contains(view, "to continue:") || !strings.Contains(view, "Esc") || !strings.Contains(view, "Selection") {
					t.Fatalf("deployment confirmation hidden screen %d size %v:\n%s", screen, size, view)
				}
			} else if screen == dashboardShutdownReview {
				if !strings.Contains(view, "to continue:") || !strings.Contains(view, "Esc") || !strings.Contains(view, "Cancel") {
					t.Fatalf("shutdown confirmation hidden screen %d size %v:\n%s", screen, size, view)
				}
			} else if screen == dashboardPXEStartReview || screen == dashboardPXELeaveReview {
				if !strings.Contains(view, "to continue:") || !strings.Contains(view, "Esc") || !strings.Contains(view, "Cancel") {
					t.Fatalf("PXE confirmation hidden screen %d size %v:\n%s", screen, size, view)
				}
			} else if screen == dashboardServicesRestartReview {
				if !strings.Contains(view, "to continue:") || !strings.Contains(view, "Esc") || !strings.Contains(view, "Cancel") {
					t.Fatalf("confirmation hidden screen %d size %v:\n%s", screen, size, view)
				}
			} else if screen == dashboardControllerReview {
				if !strings.Contains(view, "Press Enter") || !strings.Contains(view, "Esc") || !strings.Contains(view, "Cancel") || strings.Contains(view, "REBUILD pc99") {
					t.Fatalf("controller review actions hidden screen %d size %v:\n%s", screen, size, view)
				}
			}
		}
	}
}

func TestUpdatePlanningProgressFitsSupportedTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		model := experienceFixture(2)
		model.width = size[0]
		model.height = size[1]
		model.screen = dashboardUpdate
		model.busy = "Building the controller"
		model.updateTarget = "master"
		model.updatePlanning = true
		model.updatePlanStarted = time.Now().Add(-2 * time.Minute)
		model.updatePlanProgress = domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseBuild, Detail: "Building the controller", Current: 2, Total: 5}
		view := model.View().Content
		for _, expected := range []string{"Target: master", "Test systems before saving", "Testing the controller system", "Safety check 2/5", "elapsed", "current deployment remains unchanged", "Help"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("update progress %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}
		for _, jargon := range []string{"representative output", "candidate output", "local store", "flake.lock"} {
			if strings.Contains(strings.ToLower(view), jargon) {
				t.Fatalf("update progress %dx%d exposes implementation term %q:\n%s", size[0], size[1], jargon, view)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("update progress overflow at %dx%d: %dx%d\n%s", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view), view)
		}
	}
}

func TestRestoreSelectionUsesSharedNavigationAccent(t *testing.T) {
	model := dashboardModel{screen: dashboardRestore, isDark: true, width: 100, height: 30}
	view := model.View().Content
	if !strings.Contains(view, tuiSelection("Reapply the intended system", true, true)) ||
		strings.Contains(view, tuiSelection("Reinstall from scratch", true, true)) {
		t.Fatalf("restore did not accent only the focused choice:\n%s", view)
	}

	model.restoreCursor = 1
	view = model.View().Content
	if !strings.Contains(view, tuiSelection("Reinstall from scratch", true, true)) ||
		strings.Contains(view, tuiSelection("Reapply the intended system", true, true)) {
		t.Fatalf("restore accent did not follow navigation:\n%s", view)
	}
}

func TestUpdateProgressExplainsSystemChecksInOperatorLanguage(t *testing.T) {
	tests := []struct {
		detail string
		want   string
	}{
		{detail: "Building the representative client", want: "Testing a client computer system"},
		{detail: "Building the controller", want: "Testing the controller system"},
		{detail: "Building the netboot environment", want: "Testing the network installer"},
		{detail: "Building the PXE firmware", want: "Testing computer network boot"},
		{detail: "Building the offline installer bundle", want: "Testing the offline installer"},
	}
	for _, test := range tests {
		progress := domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseBuild, Detail: test.detail}
		if got := updatePlanProgressDescription(progress); got != test.want {
			t.Errorf("update progress %q = %q, want %q", test.detail, got, test.want)
		}
	}
}

func TestAdministrationBackAndDiagnosticsUseTypedCallback(t *testing.T) {
	m := experienceFixture(2)
	m = press(m, "a")
	m.actions.LoadDoctor = func() (domain.DoctorReport, error) {
		return domain.DoctorReport{Findings: []domain.Finding{{Level: domain.LevelWarning, Summary: "Check connection", Remediation: "Connect the network cable"}}}, nil
	}
	updated, command := m.Update(tea.KeyPressMsg{Text: "i"})
	m = updated.(dashboardModel)
	if command == nil || m.screen != dashboardDiagnostics {
		t.Fatal("diagnostics did not start")
	}
	updated, _ = m.Update(command())
	m = updated.(dashboardModel)
	if !strings.Contains(m.View().Content, "Connect the network cable") {
		t.Fatal("missing remediation")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(dashboardModel)
	if m.screen != dashboardAdministration {
		t.Fatal("lost administration context")
	}
}

func TestNavigationRespectsSelectedTaskDuringSetup(t *testing.T) {
	m := experienceFixture(2)
	m.setup.State = "incomplete"
	m = press(m, "enter")
	m = press(m, "down")
	m = press(m, "enter")
	if m.screen != dashboardDeploy {
		t.Fatal("selected task was replaced by setup")
	}
	m.screen = dashboardAdministration
	m = press(m, "z")
	if m.screen != dashboardAdministration {
		t.Fatal("unbound key navigated away")
	}
}

func TestPrimaryAreasPreserveContext(t *testing.T) {
	m := experienceFixture(2)
	m.actions.LoadSettings = func() (domain.LabSettingsFile, error) { return wizardSettings(), nil }
	m = press(m, "enter")
	if m.screen != dashboardComputersArea {
		t.Fatalf("computers area did not open: screen=%d", m.screen)
	}
	m = press(m, "down")
	m = press(m, "enter")
	if m.screen != dashboardDeploy || m.areaReturn != dashboardComputersArea {
		t.Fatalf("computer task lost its area: screen=%d return=%d", m.screen, m.areaReturn)
	}
	m = press(m, "esc")
	if m.screen != dashboardComputersArea {
		t.Fatalf("computer task returned to %d", m.screen)
	}
	m = press(m, "esc")
	if m.screen != dashboardHome || m.areaReturn != dashboardHome {
		t.Fatalf("computers area did not return to overview: screen=%d return=%d", m.screen, m.areaReturn)
	}

	m = press(m, "down")
	m = press(m, "enter")
	if m.screen != dashboardInstallationArea {
		t.Fatalf("installation area did not open: screen=%d", m.screen)
	}
	m = press(m, "enter")
	if m.screen != dashboardSettings || m.areaReturn != dashboardHome || !m.installationFlow {
		t.Fatalf("installation task did not start the direct flow: screen=%d return=%d", m.screen, m.areaReturn)
	}
	updated, command := m.Update(dashboardSettingsMsg{settings: wizardSettings()})
	m = updated.(dashboardModel)
	if command != nil || m.screen != dashboardSettingsEdit {
		t.Fatalf("installation settings did not open: screen=%d", m.screen)
	}
	m = press(m, "esc")
	if m.screen != dashboardHome || m.installationFlow {
		t.Fatalf("cancelled installation returned to %d", m.screen)
	}
}

func TestSetupShellKeepsPrimaryActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		m := experienceFixture(2)
		m.width = size[0]
		m.height = size[1]
		m.screen = dashboardSetup
		view := m.View().Content
		for _, expected := range []string{"Installation", "Setup", "Continue", "Technical steps", "Esc", "Help"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("setup %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}

		m.screen = dashboardSetupKeys
		m.setupKeys = domain.KeyReconcileReport{State: "action-required"}
		view = m.View().Content
		for _, expected := range []string{"Controller keys", "Create missing", "Esc", "Help"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("setup keys %dx%d lacks %q:\n%s", size[0], size[1], expected, view)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("setup keys overflow at %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestTypedConfirmationReviewsRejectWrongInputAndCancel(t *testing.T) {
	for _, screen := range []dashboardScreen{dashboardDeployReview, dashboardServicesRestartReview, dashboardPXEStartReview, dashboardGitCommitReview} {
		m := experienceFixture(2)
		m.screen = screen
		m.confirmation = "wrong"
		m.controllerPlan.Confirmation = "REBUILD"
		m.gitCommitPlan.Confirmation = "COMMIT"
		m.updatePlan.Confirmation = "UPDATE"
		m.deployPlan.ColmenaSelector = "@lab"
		updated, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = updated.(dashboardModel)
		if command != nil || m.confirmation != "" || m.screen != screen || m.busy != "" {
			t.Fatalf("screen %d accepted incorrect input", screen)
		}
		updated, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		m = updated.(dashboardModel)
		if command != nil || m.screen == screen || m.busy != "" {
			t.Fatalf("screen %d did not cancel safely", screen)
		}
	}
}

func TestExperienceStatesAndDisclosure(t *testing.T) {
	m := experienceFixture(24)
	if !strings.Contains(m.View().Content, "Laboratory overview") || !strings.Contains(m.View().Content, "Computers") || strings.Contains(m.View().Content, "24 computers") {
		t.Fatal("entry is not intervention-oriented")
	}
	m.busy = "Checking laboratory and computers"
	if !strings.Contains(m.View().Content, m.busy) {
		t.Fatal("loading not visible")
	}
	m = experienceFixture(0)
	if strings.Contains(m.View().Content, "No computers configured") || strings.Contains(m.View().Content, "0 / 0") {
		t.Fatal("empty inventory leaked into intervention entry")
	}
	m = experienceFixture(2)
	m.report.PXE.Mode = "recovery-required"
	if !strings.Contains(m.View().Content, "Controller network recovery required") {
		t.Fatal("error not prominent")
	}
	m = experienceFixture(2)
	m.screen = dashboardHosts
	m.hosts.Hosts[0].CurrentSystem = "/nix/store/technical-system"
	if strings.Contains(m.View().Content, "/nix/store/") {
		t.Fatal("technical data dominates default list")
	}
	m = press(m, "t")
	if !strings.Contains(m.View().Content, "/nix/store/technical-system") {
		t.Fatal("technical detail unavailable")
	}
}

// These are actual View() outputs from deterministic synthetic lab scenarios,
// useful for review without connecting to or changing a real laboratory.
func TestExperienceRenderGallery(t *testing.T) {
	m := experienceFixture(24)
	m.hosts.GeneratedAt = time.Now().Truncate(time.Minute)
	for _, name := range []string{"interventions", "setup", "restore", "software", "software-scope", "software-confirmation", "software-result", "software-partial", "shutdown", "shutdown-confirmation", "shutdown-result", "computers", "deploy", "update", "confirmation", "progress", "recovery"} {
		m.screen = dashboardHome
		m.busy = ""
		m.deploying = false
		switch name {
		case "restore":
			m.screen = dashboardRestore
		case "computers":
			m.screen = dashboardHosts
			m.hostCursor = 6
			m.hosts.Hosts[6].SSH = domain.SSHUnknown
			m.hosts.Hosts[6].Reachability = domain.ReachabilityUnreachable
			m.hosts.Hosts[6].Deployment = domain.DeploymentUnknown
			m.hosts.Deployment.Current = 23
			m.hosts.Deployment.Unknown = 1
		case "deploy":
			m.screen = dashboardDeploy
			m.deployChosen = map[string]bool{"pc01": true, "pc02": true}
		case "software":
			m.screen = dashboardSoftware
			m.software.catalog = testSoftwareCatalogReport()
		case "software-scope":
			m.screen = dashboardSoftware
			m.software.stage = softwareScope
			m.software.catalog = testSoftwareCatalogReport()
			m.software.selected = "gimp"
			m.software.clients = map[string]bool{}
		case "software-confirmation":
			m.screen = dashboardSoftware
			m.software.stage = softwareReview
			m.software.plan = domain.SoftwareChangePlanReport{
				State: "ready", ManagedFile: "lab-software.json",
				Request:         domain.SoftwareChangeRequest{Package: "gimp", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}},
				AffectedClients: []string{"pc01", "pc02", "pc03"}, Confirmation: "SAVE",
			}
		case "software-result":
			m.screen = dashboardSoftware
			m.software.stage = softwareResult
			m.software.result = domain.SoftwareChangeApplyReport{State: "saved", ManagedFile: "lab-software.json"}
		case "software-partial":
			m.screen = dashboardSoftware
			m.software.stage = softwareResult
			m.software.result = domain.SoftwareChangeApplyReport{
				State:   "partial",
				Message: "lab-software.json was replaced, but durable storage could not be confirmed.",
				Issues:  []domain.ValidationIssue{{Field: "durability", Message: "directory sync failed"}},
			}
		case "shutdown":
			m.screen = dashboardShutdown
			m.shutdownChosen = map[string]bool{"pc01": true, "pc02": true}
		case "shutdown-confirmation":
			m.screen = dashboardShutdownReview
			m.shutdownPlan = domain.ShutdownPlanReport{State: "ready", Eligible: 2, Policy: domain.ShutdownAcknowledgeUnknown, Confirmation: "SHUTDOWN", Targets: []domain.ShutdownTargetPlan{{Name: "pc01", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive, Eligible: true}, {Name: "pc02", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnknown, Eligible: true}, {Name: "pc07", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Session: domain.ShutdownSessionUnknown}}}
		case "shutdown-result":
			m.screen = dashboardShutdownResult
			m.shutdownResult = domain.ShutdownApplyReport{State: "partial", Accepted: 1, NotSent: 1, Unconfirmed: 1, Targets: []domain.ShutdownTargetOutcome{{Name: "pc01", State: "accepted", Detail: "the operating system accepted the power-off request"}, {Name: "pc02", State: "unconfirmed", Detail: "request result could not be confirmed; inspect the computer before retrying", TechnicalDetail: "connection closed during dispatch"}, {Name: "pc07", State: "not-sent", Detail: "not reachable"}}, Message: "Requests accepted for 1 computer; 1 not sent and 1 unconfirmed. Do not retry blindly."}
		case "update":
			m.screen = dashboardUpdate
			m.updateCheck = domain.UpdateCheckReport{Operation: "update-check", State: "available", CurrentRef: "v2.2.0", Upstream: "github:giovantenne/nixorium", Stable: []domain.UpdateRelease{{Tag: "v2.3.0", Channel: domain.UpdateChannelStable}, {Tag: "v2.2.1", Channel: domain.UpdateChannelStable}, {Tag: "v2.2.0", Channel: domain.UpdateChannelStable}}}
		case "confirmation":
			m.screen = dashboardPXEStartReview
			m.startPlan = domain.PXELifecycleReport{Interface: "eth0", StaticCIDR: "10.0.0.99/24", DHCPAddress: "192.168.1.10"}
		case "setup":
			m.screen = dashboardSetup
			m.setup = testSetupReport(true, false, false, false)
		case "progress":
			m.screen = dashboardDeploy
			m.deploying = true
			m.busy = "Updating selected computers"
			m.deployStarted = time.Now()
			m.deployProgress = domain.DeploymentProgress{Phase: domain.DeploymentPhaseApply, Completed: 2, Total: 4}
		case "recovery":
			m.report.PXE.Mode = "recovery-required"
		}
		fmt.Printf("CAPTURE %s\n%s\nEND CAPTURE\n", name, m.View().Content)
	}
}

func TestShutdownResultDisclosesTechnicalDetailOnlyOnRequest(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardShutdownResult
	m.shutdownResult = domain.ShutdownApplyReport{
		State:       "partial",
		Unconfirmed: 1,
		Targets: []domain.ShutdownTargetOutcome{{
			Name:            "pc01",
			State:           "unconfirmed",
			Detail:          "request result could not be confirmed; inspect the computer before retrying",
			TechnicalDetail: "ssh: connection closed by remote host",
		}},
	}
	if strings.Contains(m.View().Content, "connection closed by remote host") {
		t.Fatal("technical SSH detail shown in the primary result")
	}
	m = press(m, "t")
	if !strings.Contains(m.View().Content, "Technical: ssh: connection closed by remote host") {
		t.Fatal("technical SSH detail was not available on request")
	}
}
