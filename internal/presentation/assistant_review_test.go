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
			for _, want := range []string{"Review the proposal", "What changes", "course.nix changed", "Computers affected", "pc01", "✓ Checked by Nixorium: the configuration is valid", "Save and update the controller", "Save only — update computers later", "Technical details", "Back", "↑↓ choose · Enter confirm · Esc back to the chat"} {
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
			updated, _ = m.Update(keyPress("down"))
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

type assistantChain struct {
	saves, plans, applies, deploys, inventories int
	planReport                                  domain.ControllerRebuildPlanReport
	applyReport                                 domain.ControllerRebuildExecutionReport
	clients                                     []domain.HostMeta
}

func assistantChainFixture(c *assistantChain) dashboardModel {
	m := assistantReviewFixture()
	revision := strings.Repeat("b", 40)
	c.planReport = domain.ControllerRebuildPlanReport{State: "ready", Revision: revision}
	c.applyReport = domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Revision: revision, Applied: true, Verified: true}
	c.clients = []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}}
	m.actions.AssistantService = &AssistantActions{Save: func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
		c.saves++
		return domain.TemplateResetResult{State: "saved", Revision: revision, BackupRef: "refs/nixorium/assistant-backups/example"}
	}}
	m.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport { c.plans++; return c.planReport }
	m.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		c.applies++
		return c.applyReport
	}
	m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) {
		c.inventories++
		report := domain.StatusReport{}
		report.Meta.Controller.Name = "pc99"
		report.Meta.Clients.Hosts = c.clients
		return report, nil
	}
	m.actions.PlanDeployment = func(_ context.Context, requested string) domain.DeploymentPlanReport {
		c.deploys++
		return domain.DeploymentPlanReport{State: "ready", Requested: requested, Targets: []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc02"}}}
	}
	return m
}

// runChain presses Enter once and then only delivers the commands' messages.
func runChain(t *testing.T, m dashboardModel) dashboardModel {
	t.Helper()
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	for steps := 0; cmd != nil; steps++ {
		if steps > 30 {
			t.Fatal("chain does not settle")
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			msg = batch[0]()
		}
		updated, cmd = m.Update(msg)
		m = updated.(dashboardModel)
	}
	return m
}

func TestAssistantReviewOptionsAndNoConfirmationStep(t *testing.T) {
	c := &assistantChain{}
	m := assistantChainFixture(c)
	m.width, m.height = 100, 40
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Save and update the controller", "Save only — update computers later", "Technical details", "Back"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	m.assistant.review.plan.Evaluation.OK = false
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || c.saves != 0 || m.assistant.review.saving {
		t.Fatal("failed evaluation saved")
	}
}

func TestAssistantSaveAndUpdateControllerWithOneEnter(t *testing.T) {
	c := &assistantChain{}
	m := runChain(t, assistantChainFixture(c))
	if c.saves != 1 || c.plans != 1 || c.applies != 1 || c.inventories != 1 || c.deploys != 1 {
		t.Fatalf("calls: %+v", *c)
	}
	if m.screen != dashboardDeployReview || len(m.deployment.chosen) != 2 || !m.deployment.chosen["pc01"] || !m.deployment.chosen["pc02"] {
		t.Fatalf("screen=%v chosen=%v message=%s", m.screen, m.deployment.chosen, m.message)
	}
	if !m.assistant.saved || m.assistant.changedFiles != 0 || m.controller.autoApply || m.controller.fromSave {
		t.Fatal("assistant or controller flags not settled")
	}
}

func TestAssistantChainSkipsCurrentController(t *testing.T) {
	c := &assistantChain{}
	m := assistantChainFixture(c)
	m.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		c.plans++
		return domain.ControllerRebuildPlanReport{State: "current", Current: true, Revision: strings.Repeat("b", 40)}
	}
	m = runChain(t, m)
	if c.plans != 1 || c.applies != 0 || c.deploys != 1 || m.screen != dashboardDeployReview {
		t.Fatalf("calls: %+v screen=%v", *c, m.screen)
	}
	// A controller already verified for the saved revision skips the review.
	c2 := &assistantChain{}
	m = assistantChainFixture(c2)
	m.controller.result = domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: strings.Repeat("b", 40), Applied: true, Verified: true}
	m = runChain(t, m)
	if c2.plans != 0 || c2.applies != 0 || c2.deploys != 1 || m.screen != dashboardDeployReview {
		t.Fatalf("verified controller: %+v screen=%v", *c2, m.screen)
	}
}

func TestAssistantChainStopsOnControllerPlanIssues(t *testing.T) {
	c := &assistantChain{}
	m := assistantChainFixture(c)
	m.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		c.plans++
		return domain.ControllerRebuildPlanReport{State: "blocked", Issues: []domain.ValidationIssue{{Message: "dirty"}}}
	}
	m = runChain(t, m)
	if c.plans != 1 || c.applies != 0 || c.deploys != 0 || m.screen != dashboardAssistantReview || m.controller.autoApply || m.assistant.review.result.State != "saved" {
		t.Fatalf("calls: %+v screen=%v", *c, m.screen)
	}
}

func TestAssistantChainStopsOnControllerApplyFailure(t *testing.T) {
	for _, report := range []domain.ControllerRebuildExecutionReport{
		{Operation: "controller-apply", State: "failed", Revision: strings.Repeat("b", 40), Issues: []domain.ValidationIssue{{Message: "boom"}}},
		{Operation: "controller-apply", State: "recovery", Revision: strings.Repeat("b", 40), Applied: true},
	} {
		c := &assistantChain{}
		m := assistantChainFixture(c)
		m.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			c.applies++
			return report
		}
		m = runChain(t, m)
		if c.applies != 1 || c.inventories != 0 || c.deploys != 0 || m.screen != dashboardController || m.controller.autoApply || m.controller.fromSave {
			t.Fatalf("calls: %+v screen=%v", *c, m.screen)
		}
	}
}

func TestAssistantChainWithoutClientsShowsSavedResult(t *testing.T) {
	c := &assistantChain{}
	m := assistantChainFixture(c)
	c.clients = nil
	m = runChain(t, m)
	if c.applies != 1 || c.deploys != 0 || m.screen != dashboardAssistantReview || m.message != "No client computers are configured." {
		t.Fatalf("calls: %+v screen=%v message=%q", *c, m.screen, m.message)
	}
}

func TestAssistantChainEscapeDuringControllerReviewReturnsToResult(t *testing.T) {
	c := &assistantChain{}
	m := assistantChainFixture(c)
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd()) // save result starts the controller review read
	m = updated.(dashboardModel)
	if m.read.cancel == nil || !m.controller.autoApply {
		t.Fatal("controller review did not start")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.screen != dashboardAssistantReview || m.controller.autoApply || m.controller.fromSave || c.applies != 0 {
		t.Fatalf("cancel: screen=%v auto=%v", m.screen, m.controller.autoApply)
	}
}

func TestAssistantSaveOnlyReturnsToChat(t *testing.T) {
	c := &assistantChain{}
	m := assistantChainFixture(c)
	updated, _ := m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	m = runChain(t, m)
	if c.saves != 1 || c.plans+c.applies+c.deploys+c.inventories != 0 || m.screen != dashboardAssistantReview {
		t.Fatalf("calls: %+v screen=%v", *c, m.screen)
	}
	view := strings.Join(strings.Fields(ansi.Strip(m.View().Content)), " ")
	if !strings.Contains(view, "Computers were not updated") || !strings.Contains(view, "Computers → Update computers") {
		t.Fatal(view)
	}
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.screen != dashboardAssistant || !m.assistant.saved || m.assistant.changedFiles != 0 || c.plans != 0 {
		t.Fatalf("Enter did not return to the chat: %v", m.screen)
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
