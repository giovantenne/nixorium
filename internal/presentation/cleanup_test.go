package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestCleanupScreenRequiresTypedConfirmation(t *testing.T) {
	requested := ""
	applied := 0
	plan := domain.CleanupPlanReport{
		State: "ready", KeepGenerations: 10, Confirmation: "CLEAN", ReviewToken: "sha256:x", Eligible: 1,
		Message: "Old system versions can be removed on 1 of 2 selected computer(s).",
		Targets: []domain.CleanupTargetPlan{
			{Name: "pc99", Controller: true, Eligible: true, Remove: []int{1, 2, 3}, Keep: []domain.CleanupGeneration{{Number: 4, Reason: "newest"}, {Number: 13, Reason: "newest"}}, FreeBytes: 5 << 30},
			{Name: "pc01", Detail: "off or not reachable"},
		},
	}
	actions := DashboardActions{
		PlanCleanup: func(_ context.Context, value string) domain.CleanupPlanReport {
			requested = value
			return plan
		},
		ApplyCleanup: func(domain.CleanupPlanReport) domain.CleanupApplyReport {
			applied++
			return domain.CleanupApplyReport{State: "completed", Message: "Old versions removed on 1 computer(s).", Cleaned: 1,
				Targets: []domain.CleanupTargetOutcome{{Name: "pc99", State: "cleaned", Detail: "old versions removed", FreeBefore: 1_000_000_000, FreeAfter: 3_000_000_000}}}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 120, height: 30}
	model.report.Meta.Controller.Name = "pc99"
	model.report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	updated, _ := model.openCleanup()
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "pc99 (controller)") || !strings.Contains(model.View().Content, "newest 10") {
		t.Fatalf("selection view:\n%s", model.View().Content)
	}
	model = cleanupPress(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = cleanupPress(t, model, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if requested != "controller,pc01" {
		t.Fatalf("requested = %q", requested)
	}
	view := model.View().Content
	if !strings.Contains(view, "remove 3 old version(s) (1 2 3)") || !strings.Contains(view, "off or not reachable") || !strings.Contains(view, "Type CLEAN to continue") {
		t.Fatalf("review view:\n%s", view)
	}
	for _, key := range "clean" {
		model = cleanupPress(t, model, tea.KeyPressMsg{Code: key, Text: string(key)})
	}
	model = cleanupPress(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if applied != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatal("a lowercase confirmation started the cleanup")
	}
	for _, key := range "CLEAN" {
		model = cleanupPress(t, model, tea.KeyPressMsg{Code: key, Text: string(key)})
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || !strings.Contains(model.View().Content, "free space 1.0 GB → 3.0 GB") {
		t.Fatalf("result view (applied %d):\n%s", applied, model.View().Content)
	}
}

func cleanupPress(t *testing.T, model dashboardModel, key tea.KeyPressMsg) dashboardModel {
	t.Helper()
	updated, _ := model.Update(key)
	return updated.(dashboardModel)
}
