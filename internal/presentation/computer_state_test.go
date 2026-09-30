package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func stateFixture() dashboardModel {
	m := experienceFixture(3)
	now := time.Date(2026, 9, 30, 10, 15, 0, 0, time.Local)
	m.computers.hosts = domain.HostsReport{GeneratedAt: now, Deployment: domain.HostDeploymentSummary{Outdated: 1}, Hosts: []domain.HostStatus{
		{Name: "pc01", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Deployment: domain.DeploymentOutdated},
		{Name: "pc02", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Deployment: domain.DeploymentCurrent},
		{Name: "pc03", Reachability: domain.ReachabilityUnreachable, Deployment: domain.DeploymentUnknown},
	}}
	return m
}

func TestDeploySelectionShowsStateAndSelectsThoseNeedingUpdate(t *testing.T) {
	m := stateFixture()
	m.screen = dashboardDeploy
	m.deployment.chosen = map[string]bool{"pc02": true}
	view := m.View().Content
	for _, want := range []string{"Last checked 10:15:00", "pc01", "Needs update", "Up to date", "Off or not reachable", "Those needing update", "Check computers"} {
		if !strings.Contains(view, want) {
			t.Fatalf("deploy selection lacks %q:\n%s", want, view)
		}
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(dashboardModel)
	if len(m.deployment.chosen) != 1 || !m.deployment.chosen["pc01"] {
		t.Fatalf("n selected %v", m.deployment.chosen)
	}
	loads := 0
	m.actions.LoadHosts = func(context.Context) (domain.HostsReport, error) {
		loads++
		report := stateFixture().computers.hosts
		report.Hosts[1].Deployment = domain.DeploymentOutdated
		return report, nil
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(dashboardModel)
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(dashboardModel)
	}
	if loads != 1 || m.screen != dashboardDeploy {
		t.Fatalf("check did not return to the selection: loads=%d screen=%d", loads, m.screen)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if chosen := next.(dashboardModel).deployment.chosen; len(chosen) != 2 {
		t.Fatalf("fresh observation not used: %v", chosen)
	}
	empty := experienceFixture(2)
	empty.computers.hosts = domain.HostsReport{}
	empty.screen = dashboardDeploy
	next, _ = empty.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if msg := next.(dashboardModel).message; !strings.Contains(msg, "press r") {
		t.Fatalf("selection without observation: %q", msg)
	}
}

func TestPowerSelectionShowsStateAndSelectsThoseOn(t *testing.T) {
	m := stateFixture()
	m.screen = dashboardShutdown
	view := m.View().Content
	if !strings.Contains(view, "On at last check") || !strings.Contains(view, "Off or not reachable at last check") || !strings.Contains(view, "Those that are on") {
		t.Fatalf("power selection lacks states:\n%s", view)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	chosen := next.(dashboardModel).shutdown.chosen
	if len(chosen) != 2 || !chosen["pc01"] || !chosen["pc02"] || chosen["pc03"] {
		t.Fatalf("n selected %v", chosen)
	}
}

func TestInternetSelectionChecksStateAndSelectsThoseToChange(t *testing.T) {
	m := experienceFixture(3)
	m.computers.hosts = domain.HostsReport{}
	m.screen = dashboardInternet
	m.internet.chosen = map[string]bool{}
	m.internet.action = domain.InternetUnblock
	m.actions.PlanInternet = func(_ context.Context, requested string, action domain.InternetAction) domain.InternetPlan {
		if requested != "pc01,pc02,pc03" {
			t.Fatalf("check requested %q", requested)
		}
		observation := func(state string) domain.InternetObservation {
			return domain.InternetObservation{SchemaVersion: domain.SchemaVersion, BootID: "12345678-1234-1234-1234-123456789abc", State: state}
		}
		return domain.InternetPlan{State: "ready", Action: action, Targets: []domain.InternetTarget{
			{HostMeta: domain.HostMeta{Name: "pc01"}, Observed: observation("blocked"), Eligible: true},
			{HostMeta: domain.HostMeta{Name: "pc02"}, Observed: observation("enabled"), Eligible: true},
			{HostMeta: domain.HostMeta{Name: "pc03"}},
		}}
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(dashboardModel)
	if !strings.Contains(m.message, "press r") {
		t.Fatalf("n without observation: %q", m.message)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(dashboardModel)
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(dashboardModel)
	}
	if m.internet.stage != 0 {
		t.Fatal("a state check must not open a review")
	}
	view := m.View().Content
	for _, want := range []string{"Internet blocked", "Internet on", "Not reachable at last check", "Last checked"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Internet list lacks %q:\n%s", want, view)
		}
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if chosen := next.(dashboardModel).internet.chosen; len(chosen) != 1 || !chosen["pc01"] {
		t.Fatalf("unblock selection = %v", chosen)
	}
}

func TestInventoryUpdatesComputersThatNeedIt(t *testing.T) {
	m := stateFixture()
	m.screen = dashboardHosts
	if !strings.Contains(m.View().Content, "Update those that need it") {
		t.Fatalf("inventory lacks the update action:\n%s", m.View().Content)
	}
	var requested string
	m.actions.PlanDeployment = func(_ context.Context, target string) domain.DeploymentPlanReport {
		requested = target
		return domain.DeploymentPlanReport{State: "ready"}
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = next.(dashboardModel)
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(dashboardModel)
	}
	if !strings.Contains(requested, "pc01") || strings.Contains(requested, "pc02") || m.screen != dashboardDeployReview {
		t.Fatalf("update opened %q on screen %d", requested, m.screen)
	}
}
