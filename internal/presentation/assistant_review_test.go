package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func assistantReviewFixture() dashboardModel {
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{Assistant: domain.AssistantConfig{Enabled: true}}, false)
	m.assistant = newAssistantModel()
	m.screen = dashboardAssistantReview
	m.assistant.summary = "Prepare the course."
	m.assistant.review.plan = domain.AssistantSavePlan{State: "ready", Changed: []string{"course.nix"}, ChangedHosts: []string{"pc01"}, Evaluation: domain.AssistantEvaluation{OK: true}, Diff: strings.Repeat("-old\n+new\n", 30)}
	return m
}
func TestAssistantReviewLayoutsAndScroll(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		m := assistantReviewFixture()
		m.width, m.height = size[0], size[1]
		view := m.View().Content
		for _, want := range []string{"Proposal", "Summary from the assistant", "Checked by Nixorium", "Files: 0 added, 1 changed, 0 removed", "Computers whose system changes: pc01", "Nix evaluation: passed", "Type SAVE", "Enter", "Esc"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%v missing %s: %s", size, want, view)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("%v overflow: %s", size, view)
		}
		updated, _ := m.Update(keyPress("end"))
		m = updated.(dashboardModel)
		if m.assistant.review.scroll == 0 || !strings.Contains(m.View().Content, "+new") {
			t.Fatal("comparison did not scroll")
		}
		updated, _ = m.Update(keyPress("home"))
		m = updated.(dashboardModel)
		if m.assistant.review.scroll != 0 {
			t.Fatal("Home did not return to summary")
		}
		m.assistant.review.plan.Evaluation.OK = false
		view = m.View().Content
		if !strings.Contains(view, "Saving is not available") || strings.Contains(view, "Type SAVE") {
			t.Fatal(view)
		}
		if lipgloss.Height(view) > size[1] {
			t.Fatal("failure overflow", view)
		}
	}
}
func TestAssistantReviewSaveAndFollowup(t *testing.T) {
	m := assistantReviewFixture()
	saves := 0
	m.actions.AssistantService = &AssistantActions{Save: func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
		saves++
		return domain.TemplateResetResult{State: "saved", Revision: "new", BackupRef: "refs/nixorium/assistant-backups/example"}
	}}
	m.assistant.review.confirmation = "save"
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || saves != 0 || !strings.Contains(m.message, "Type SAVE exactly") {
		t.Fatal("invalid confirmation accepted")
	}
	m.assistant.review.confirmation = "SAVE"
	m.assistant.review.plan.Evaluation.OK = false
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || saves != 0 {
		t.Fatal("failed evaluation saved")
	}
	m.assistant.review.plan.Evaluation.OK = true
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd == nil || !m.assistant.review.saving {
		t.Fatal("SAVE did not start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if saves != 1 || m.assistant.review.result.State != "saved" || m.assistant.review.saving {
		t.Fatal("save result missing")
	}
	planned, applied := 0, 0
	m.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		planned++
		return domain.ControllerRebuildPlanReport{}
	}
	m.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		applied++
		return domain.ControllerRebuildExecutionReport{}
	}
	if !strings.Contains(m.View().Content, "Apply to this controller") || !strings.Contains(m.View().Content, "Backup:") {
		t.Fatal(m.View().Content)
	}
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd == nil || !m.controller.fromSave || m.controller.saveOrigin != dashboardAssistantReview {
		t.Fatal("followup did not review controller")
	}
	cmd()
	if planned != 1 || applied != 0 {
		t.Fatal("followup applied without review")
	}
}
func TestAssistantReviewOnlyForChangesAndCancellation(t *testing.T) {
	m := assistantReviewFixture()
	m.screen = dashboardAssistant
	calls := 0
	m.actions.AssistantService = &AssistantActions{PlanSave: func(context.Context, string) domain.AssistantSavePlan {
		calls++
		return domain.AssistantSavePlan{State: "ready"}
	}, Save: func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
		return domain.TemplateResetResult{}
	}}
	m.assistant.conversation.ID = "test-conversation"
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(dashboardModel)
	if cmd != nil || m.screen != dashboardAssistant {
		t.Fatal("empty workspace reviewed")
	}
	m.assistant.changedFiles = 1
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(dashboardModel)
	if cmd == nil || m.screen != dashboardAssistantReview || m.read.cancel == nil {
		t.Fatal("review did not start")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if calls != 0 || m.screen != dashboardAssistant || m.assistant.review.plan.State != "" {
		t.Fatal("cancelled read resumed")
	}
}
