package presentation

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func assistantReviewFixture() dashboardModel {
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{Assistant: domain.AssistantConfig{Enabled: true}}, false)
	m.assistant = newAssistantModel()
	m.report.Meta.Controller.Name = "pc99"
	m.screen = dashboardAssistantReview
	m.assistant.summary = "Prepare the course."
	m.assistant.review.plan = domain.AssistantSavePlan{State: "ready", Changed: []string{"course.nix"}, ChangedHosts: []string{"pc01"}, Evaluation: domain.AssistantEvaluation{OK: true}, Diff: strings.Repeat("-old\n+new\n", 30)}
	return m
}
func TestAssistantReviewLayoutsAndScroll(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		for _, profile := range []colorprofile.Profile{colorprofile.NoTTY, colorprofile.ANSI256} {
			m := assistantReviewFixture()
			m.width, m.height = size[0], size[1]
			captureView := func() string {
				var output bytes.Buffer
				writer := colorprofile.Writer{Forward: &output, Profile: profile}
				_, _ = writer.Write([]byte(m.View().Content))
				return ansi.Strip(output.String())
			}
			view := captureView()
			for _, want := range []string{"Review the proposal", "What changes", "course.nix changed", "Computers affected", "pc01", "✓ Checked by Nixorium: the configuration is valid", "Save this proposal", "Show technical details", "Back to the chat", "↑↓ choose · Enter confirm · Esc back"} {
				if !strings.Contains(view, want) {
					t.Fatalf("%v missing %s: %s", size, want, view)
				}
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%v overflow: %s", size, view)
			}
			if strings.Contains(view, "Full comparison") || strings.Contains(view, "Type SAVE") {
				t.Fatal("technical view in main review", view)
			}
			if dir := os.Getenv("NIXORIUM_T9_CAPTURES"); dir != "" && size[0] == 120 && profile == colorprofile.ANSI256 {
				if err := os.WriteFile(filepath.Join(dir, "review.txt"), []byte(view), 0600); err != nil {
					t.Fatal(err)
				}
			}
			updated, _ := m.Update(keyPress("down"))
			m = updated.(dashboardModel)
			updated, _ = m.Update(keyPress("enter"))
			m = updated.(dashboardModel)
			view = captureView()
			if !m.assistant.review.details || !strings.Contains(view, "Full comparison") || !strings.Contains(view, "Files: 0 added, 1 changed, 0 removed") || strings.Contains(view, "Type SAVE") {
				t.Fatal(view)
			}
			if dir := os.Getenv("NIXORIUM_T9_CAPTURES"); dir != "" && size[0] == 120 && profile == colorprofile.ANSI256 {
				if err := os.WriteFile(filepath.Join(dir, "details.txt"), []byte(view), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatal("details overflow", view)
			}
			updated, _ = m.Update(keyPress("end"))
			m = updated.(dashboardModel)
			if m.assistant.review.scroll == 0 || !strings.Contains(m.View().Content, "+new") {
				t.Fatal("comparison did not scroll")
			}
			updated, _ = m.Update(keyPress("home"))
			m = updated.(dashboardModel)
			if m.assistant.review.scroll != 0 {
				t.Fatal("Home did not return to summary")
			}
			updated, _ = m.Update(keyPress("esc"))
			m = updated.(dashboardModel)
			if m.screen != dashboardAssistantReview || m.assistant.review.details {
				t.Fatal("Esc did not return to main review")
			}
			m.assistant.review.plan.Evaluation.OK = false
			view = ansi.Strip(m.View().Content)
			if !strings.Contains(view, "✗ The configuration is not valid") || strings.Contains(view, "Type SAVE") {
				t.Fatal(view)
			}
			if lipgloss.Height(view) > size[1] {
				t.Fatal("failure overflow", view)
			}
		}
	}
}

func TestAssistantProposalChangesAndHosts(t *testing.T) {
	plan := domain.AssistantSavePlan{Added: []string{"a.nix"}, Changed: []string{"b.nix"}, Removed: []string{"c.nix"}}
	for _, test := range []struct {
		summary string
		want    []string
	}{
		{"Changes:\n- Install the drawing application\n* Keep the controller unchanged\n\nChecked.", []string{"Install the drawing application", "Keep the controller unchanged"}},
		{"Changes:\n\n- One\n\n- Two\n\nChecked.", []string{"One", "Two"}},
		{"Changes:\n- One\n- Two\n- Three\n- Four\n- Five", []string{"One", "Two", "Three", "Four"}},
		{"Summary without the changes block", []string{"a.nix added", "b.nix changed", "c.nix removed"}},
		{"Changes:\nNothing in bullets.", []string{"a.nix added", "b.nix changed", "c.nix removed"}},
	} {
		if got := assistantProposalChanges(test.summary, plan); !reflect.DeepEqual(got, test.want) {
			t.Fatal(got, test.want)
		}
	}
	m := assistantReviewFixture()
	m.report.Meta.Controller.Name = "pc99"
	m.assistant.review.plan.ChangedHosts = []string{"pc99", "pc03", "pc02", "pc01"}
	if got := m.assistantProposalHosts(); got != "pc01–pc03, the controller (pc99)" {
		t.Fatal(got)
	}
	m.assistant.review.plan.ChangedHosts = nil
	if got := m.assistantProposalHosts(); got != "No computer changes" {
		t.Fatal(got)
	}
}

func TestAssistantSaveWordsOpenLocalReview(t *testing.T) {
	for _, word := range []string{"save", "SAVE", "salva", "SalVa", "/save", "/SAVE"} {
		m := assistantReviewFixture()
		m.screen = dashboardAssistant
		m.assistant.conversation.ID = "synthetic-proposal"
		m.assistant.changedFiles = 1
		turns := 0
		m.actions.AssistantService = &AssistantActions{
			Run: func(context.Context, domain.AssistantConversation, string, func(domain.AssistantEvent)) (domain.AssistantTurnResult, error) {
				turns++
				return domain.AssistantTurnResult{}, nil
			},
			PlanSave: func(context.Context, string) domain.AssistantSavePlan {
				return domain.AssistantSavePlan{State: "ready"}
			},
			Save: func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
				return domain.TemplateResetResult{}
			},
		}
		m.assistant.input.SetValue(word)
		updated, cmd := m.Update(keyPress("enter"))
		m = updated.(dashboardModel)
		if m.screen != dashboardAssistantReview || cmd == nil || turns != 0 || m.assistant.input.Value() != "" {
			t.Fatal(word, "did not open local review")
		}
		cmd()
		if turns != 0 {
			t.Fatal("save word sent a model turn")
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
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || saves != 0 || !m.assistant.review.confirming || !strings.Contains(ansi.Strip(m.View().Content), assistantSavePrompt) {
		t.Fatal("confirmation did not open")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.assistant.review.confirming || m.screen != dashboardAssistantReview {
		t.Fatal("confirmation cancellation failed")
	}
	m.assistant.review.plan.Evaluation.OK = false
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || saves != 0 || m.assistant.review.confirming {
		t.Fatal("failed evaluation saved")
	}
	m.assistant.review.plan.Evaluation.OK = true
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || !m.assistant.review.confirming {
		t.Fatal("confirmation missing")
	}
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd == nil || !m.assistant.review.saving {
		t.Fatal("Enter did not start save")
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
	view := strings.Join(strings.Fields(demoANSI.ReplaceAllString(m.View().Content, "")), " ")
	for _, want := range []string{"Nothing was built or activated.", "Try it on one computer first: Computers → Update computers, select a single computer, check it, then update the others."} {
		if !strings.Contains(view, want) {
			t.Fatalf("saved proposal lacks field verification guidance %q: %s", want, view)
		}
	}
	help := strings.Join(strings.Fields(demoANSI.ReplaceAllString(m.helpView(), "")), " ")
	if !strings.Contains(help, "After saving, update one computer first and check it before the others.") {
		t.Fatal("proposal help lacks field verification guidance", help)
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

func TestAssistantReviewLoadsControllerIdentity(t *testing.T) {
	m := assistantReviewFixture()
	m.screen = dashboardAssistant
	m.assistant.conversation.ID = "synthetic"
	m.assistant.changedFiles = 1
	observed := m.report
	m.report.Meta = domain.LabMeta{}
	loads, plans := 0, 0
	m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) { loads++; return observed, nil }
	m.actions.AssistantService = &AssistantActions{PlanSave: func(context.Context, string) domain.AssistantSavePlan {
		plans++
		return domain.AssistantSavePlan{State: "ready", ChangedHosts: []string{"pc99"}, Evaluation: domain.AssistantEvaluation{OK: true}}
	}, Save: func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
		return domain.TemplateResetResult{}
	}}
	updated, cmd := m.openAssistantReview()
	m = updated.(dashboardModel)
	if cmd == nil || m.screen != dashboardAssistant || plans != 0 {
		t.Fatal("review bypassed evaluated inventory")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(dashboardModel)
	if loads != 1 || cmd == nil || m.screen != dashboardAssistantReview {
		t.Fatal("review did not resume after inventory")
	}
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if plans != 1 || m.assistantProposalHosts() != "the controller (pc99)" {
		t.Fatal("controller identity unavailable")
	}
}
