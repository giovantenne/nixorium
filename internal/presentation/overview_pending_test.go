package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestOverviewPendingRowsAndEmptyState(t *testing.T) {
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "unchecked"}, DashboardActions{}, false)
	m.screen = dashboardHome
	if view := m.View().Content; len(m.pendingTasks()) != 0 || strings.Contains(view, "No pending work") || strings.Contains(view, "Needs attention") {
		t.Fatal("empty overview shows a pending-work line")
	}
	m.jobs.items = []domain.ManagedJob{{Operation: "controller-apply", State: "running"}}
	m.report.PXE.Mode = "recovery-required"
	m.report.Git.Dirty = true
	free := uint64(1 << 30)
	m.report.StoreFreeBytes, m.report.StoreSpaceLow = &free, true
	m.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply}
	m.computers.hosts = domain.HostsReport{GeneratedAt: time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC), Deployment: domain.HostDeploymentSummary{Outdated: 2}}
	seen := map[string]bool{}
	for _, task := range m.pendingTasks() {
		seen[task.id] = true
	}
	for _, id := range []string{"pending-jobs", "pending-pxe", "pending-git", "pending-controller"} {
		if !seen[id] {
			t.Fatalf("missing %s", id)
		}
	}
	if seen["pending-clients"] {
		t.Fatal("the last client check is listed as pending work")
	}
	next, cmd := m.updatePrimaryScreenKey(tea.KeyPressMsg{Code: '1', Text: "1"})
	if cmd != nil || next.(dashboardModel).screen != dashboardManagedJobs {
		t.Fatal("pending job started mutation instead of attachment")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		m.width, m.height = size[0], size[1]
		m.syncHomeTasks()
		for range len(m.overviewTasks()) {
			m, _ = workspaceKey(m, demoCode(tea.KeyDown))
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatal("overview overflow")
			}
		}
	}
}

func TestOverviewRefreshIsLocalAndPreservesClientObservation(t *testing.T) {
	m := savedFollowupFixture(dashboardHome)
	m.computers.hosts = domain.HostsReport{GeneratedAt: time.Now().UTC(), Hosts: []domain.HostStatus{{Name: "pc01"}}}
	m.actions.LoadInitial = func(context.Context) (domain.StatusReport, domain.SetupReport, error) {
		return domain.StatusReport{}, domain.SetupReport{State: "unchecked"}, nil
	}
	m.actions.LoadHosts = func(context.Context) (domain.HostsReport, error) {
		t.Fatal("refresh probed clients")
		return domain.HostsReport{}, nil
	}
	m, cmd := workspaceKey(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = workspaceComplete(t, m, cmd)
	if len(m.computers.hosts.Hosts) != 1 || m.screen != dashboardHome {
		t.Fatal("lost dated client evidence or navigated")
	}
	m.pendingRevision = strings.Repeat("a", 40)
	if len(m.pendingTasks()) != 1 {
		t.Fatal("unapplied save not visible")
	}
	m.controller.result = domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: m.pendingRevision, Applied: true, Verified: true}
	if len(m.pendingTasks()) != 0 {
		t.Fatal("verified saved revision remains pending")
	}
}
