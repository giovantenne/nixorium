package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func savedFollowupFixture(screen dashboardScreen) dashboardModel {
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
	m.screen = screen
	revision := strings.Repeat("a", 40)
	m.settings.result = domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved", Revision: revision}
	m.templateReset.stage = "result"
	m.templateReset.result = domain.TemplateResetResult{State: "saved", Revision: revision}
	m.workspace.stage = workspaceResult
	m.workspace.loaded = demoWorkspacePlan()
	m.workspace.result = domain.WorkspaceApplyReport{Operation: "workspace-save", State: "saved", Recorded: true, Revision: revision}
	return m
}

func TestSavedFollowupsRequireIndependentControllerReview(t *testing.T) {
	for _, screen := range []dashboardScreen{dashboardSettings, dashboardTemplateReset, dashboardWorkspace} {
		m := savedFollowupFixture(screen)
		plans, applies := 0, 0
		m.actions.PlanController = func(ctx context.Context) domain.ControllerRebuildPlanReport {
			plans++
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("unbounded followup")
			}
			return domain.ControllerRebuildPlanReport{State: "ready", Revision: strings.Repeat("a", 40)}
		}
		m.actions.ApplyController = func(p domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			applies++
			return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Revision: p.Revision, Applied: true, Verified: true}
		}
		m, command := workspaceKey(m, demoCode(tea.KeyEnter))
		if command == nil || m.screen != screen || applies != 0 {
			t.Fatalf("followup skipped read or left result early: origin=%v screen=%v command=%t message=%s", screen, m.screen, command != nil, m.message)
		}
		m = workspaceComplete(t, m, command)
		if m.screen != dashboardControllerReview || plans != 1 || applies != 0 {
			t.Fatal("controller started without new review")
		}
		cancelled, command := workspaceKey(m, demoCode(tea.KeyEscape))
		if command != nil || cancelled.screen != screen || applies != 0 {
			t.Fatal("controller review did not cancel to the save")
		}
		m, command = workspaceKey(m, demoCode(tea.KeyEnter))
		if command == nil || !m.controller.applying {
			t.Fatal("reviewed controller did not start")
		}
		// A controller operation without progress polling returns a BatchMsg;
		// execute only its operation, not the unrelated delayed tick.
		batch := command().(tea.BatchMsg)
		updated, _ := m.Update(batch[0]())
		m = updated.(dashboardModel)
		if m.screen != screen || applies != 1 || !controllerVerifiedForSave(strings.Repeat("a", 40), m.controller.result) {
			t.Fatal("verified result lost its save context")
		}
		m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) {
			r := testDashboardReport("ready")
			r.Meta.Controller.Name = "pc99"
			return r, nil
		}
		m.actions.PlanDeployment = func(context.Context, string) domain.DeploymentPlanReport {
			t.Fatal("selection bypassed")
			return domain.DeploymentPlanReport{}
		}
		m, command = workspaceKey(m, demoCode(tea.KeyEnter))
		if command == nil {
			t.Fatal("missing fresh inventory")
		}
		m = workspaceComplete(t, m, command)
		if m.screen != dashboardDeploy || len(m.deployment.chosen) != 0 || m.deployment.confirmation != "" {
			t.Fatal("deployment was authorized or selected implicitly")
		}
	}
}

func TestSaveFollowupCancellationRecoveryAndStaleEvidence(t *testing.T) {
	m := savedFollowupFixture(dashboardSettings)
	m.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		return domain.ControllerRebuildPlanReport{State: "ready"}
	}
	m.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		t.Fatal("unreviewed apply")
		return domain.ControllerRebuildExecutionReport{}
	}
	m, command := workspaceKey(m, demoCode(tea.KeyEnter))
	reply := command()
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	updated, command := m.Update(reply)
	m = updated.(dashboardModel)
	if command != nil || m.screen != dashboardSettings || m.controller.fromSave {
		t.Fatal("cancelled followup consumed late reply")
	}
	m.settings.result.RecoveryRequired = true
	_, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil {
		t.Fatal("save recovery bypassed")
	}
	for _, revision := range []string{"", strings.Repeat("b", 40)} {
		if controllerVerifiedForSave(strings.Repeat("a", 40), domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: revision, State: "completed", Applied: true, Verified: true}) {
			t.Fatal("stale activation advertised as saved configuration")
		}
	}
}

func TestSaveResultsShareThreeRowsAndFit(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, screen := range []dashboardScreen{dashboardSettings, dashboardTemplateReset, dashboardWorkspace, dashboardSoftware, dashboardUpdate} {
			m := savedFollowupFixture(screen)
			m.width, m.height = size[0], size[1]
			m.software.stage = softwareResult
			m.software.result = domain.SoftwareChangeApplyReport{State: "saved", AffectedClients: []string{"pc01"}}
			m.updates.result = domain.UpdateApplyReport{Operation: "update-save", State: "saved", Updated: true}
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatal("save result overflow")
			}
			combined := view
			for range 20 {
				m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
				combined += m.View().Content
			}
			for _, text := range []string{"Configuration —", "This controller —", "Client computers —", "Esc", "Help"} {
				if !strings.Contains(combined, text) {
					t.Fatalf("screen %v size %v missing %s", screen, size, text)
				}
			}
		}
	}
}
