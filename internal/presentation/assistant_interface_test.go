package presentation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantOutcomeLines(t *testing.T) {
	tests := []struct{ name, data, want string }{
		{"evaluate", `{"ok":true,"changedHosts":["pc01","pc02","pc03","pc04","pc05","pc06","pc99"]}`, "✓ Evaluation passed · changes pc01–pc06, pc99"},
		{"evaluate", `{"ok":true,"changedHosts":[]}`, "✓ Evaluation passed · no computer changes"},
		{"evaluate", `{"ok":false,"error":"evaluation_failed","message":"private detail"}`, "✗ Evaluation failed"},
		{"evaluate", `{"ok":false,"error":"timeout"}`, "✗ Evaluation timed out"},
		{"build", `{"ok":true,"built":3,"blocked":2,"skipped":[{}]}`, "✓ Build check passed · 3 parts built · 3 not checked"},
		{"build", `{"ok":true,"built":0,"blocked":0,"skipped":[]}`, "✓ Build check passed · 0 parts built"},
		{"build", `{"ok":false,"failed":[{"name":"a"},{"name":"b"}]}`, "✗ Build check failed · a +1 more"},
		{"build", `{"ok":false,"failed":[{"name":"a"}]}`, "✗ Build check failed · a"},
		{"build", `{"ok":false,"error":"evaluate_first"}`, "● Build check needs a new evaluation"},
		{"build", `{"ok":false,"error":"timeout"}`, "✗ Build check timed out"},
		{"build", `{"ok":false,"error":"too_large"}`, "● Build check not available"},
		{"build", `{"ok":false,"error":"build_unavailable"}`, "● Build check not available"},
		{"build", `{"ok":false,"error":"unknown"}`, "● Build check finished"},
		{"build", `broken`, "● Build check finished"},
	}
	for _, test := range tests {
		if got := assistantToolResultLine(test.name, test.data, "pc99"); got != test.want {
			t.Fatalf("%s %s: %q", test.name, test.data, got)
		}
	}
	hosts := []string{}
	for i := 1; i <= 12; i++ {
		hosts = append(hosts, fmt.Sprintf("pc%02d", i))
	}
	hosts = append(hosts, "pc99")
	if got := assistantCompactHosts(hosts, "pc99"); got != "pc01–pc12, pc99" {
		t.Fatal(got)
	}
	// The controller is never compressed into a consecutive client range.
	if got := assistantCompactHosts([]string{"pc97", "pc98", "pc99"}, "pc99"); got != "pc97–pc98, pc99" {
		t.Fatal(got)
	}
	long := []string{}
	for i := 0; i < 20; i++ {
		long = append(long, fmt.Sprintf("computer-%02d", i))
	}
	got := assistantCompactHosts(long, "")
	if len([]rune(got)) > 60 || !strings.HasSuffix(got, ", +16 more") {
		t.Fatal(got)
	}
}

func TestAssistantInputCommandsExamplesAndPaste(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("input sent HTTP") })
	m.assistant.input.Reset()
	updated, _ := m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "" || m.assistant.state != "chat" {
		t.Fatal("example selected or sent")
	}
	for _, key := range []string{"alt+enter", "ctrl+j"} {
		m.assistant.input.SetValue("one")
		updated, _ = m.Update(keyPress(key))
		m = updated.(dashboardModel)
		if m.assistant.input.Value() != "one\n" {
			t.Fatalf("%s: %q", key, m.assistant.input.Value())
		}
	}
	m.assistant.input.Reset()
	updated, _ = m.Update(tea.PasteMsg{Content: "first\nsecond\nthird"})
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "first\nsecond\nthird" {
		t.Fatal("multiline paste changed")
	}
	for _, key := range []string{"ctrl+s", "ctrl+p", "ctrl+f"} {
		updated, cmd := m.Update(keyPress(key))
		m = updated.(dashboardModel)
		if cmd != nil || m.assistant.state != "chat" {
			t.Fatal("retired shortcut active")
		}
	}
	assistantEnableCommands(&m)
	m.assistant.input.SetValue("/")
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "" || m.assistant.state != "chat" {
		t.Fatal("Enter should run the command chosen with the arrows")
	}
	assistantEnableCommands(&m)
	m.assistant.input.SetValue("/")
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("tab"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "/new" {
		t.Fatal("slash completion")
	}
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "" || m.assistant.state != "chat" {
		t.Fatal("new command")
	}
	m.assistant.input.SetValue("/unknown")
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd != nil || m.message != "Unknown command. Type / to see the commands." || m.assistant.input.Value() != "/unknown" {
		t.Fatal("unknown command reached model")
	}
	m.assistant.input.SetValue("/he")
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "" || !m.helpOpen {
		t.Fatal("Enter should run the highlighted command")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	m.assistant.input.SetValue("/code")
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.state != "activate" {
		t.Fatal("code command")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	m.assistant.input.SetValue("draft")
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if cmd == nil || m.assistant.state != "running" || m.assistant.input.Value() != "draft" {
		t.Fatal("Enter did not initiate send")
	}
	if m.assistant.cancel != nil {
		m.assistant.cancel()
	}
}

func TestAssistantWelcomeOfferActivationConsentAndReopen(t *testing.T) {
	m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected external call") })
	m.actions.AssistantService.WriteConsentSharing = store.WriteConsentSharing
	m.actions.AssistantService.ReadSharing = store.ReadSharing
	m.assistant = newAssistantModel()
	m.assistant.initialized = false
	calls := 0
	m.actions.AssistantService.Offer = func(context.Context) (domain.AssistantOffer, error) {
		calls++
		return domain.AssistantOffer{PriceText: "360 € + VAT per lab / year", TrialRequests: 10, TrialURL: "https://assistant.example/trial", SubscribeURL: "https://example.org/assistant/"}, nil
	}
	updated, _ := m.openAssistant()
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || !strings.Contains(m.View().Content, "Not activated") {
		t.Fatal("unactivated lab did not open the chat")
	}
	m.assistant.input.SetValue("Synthetic private request")
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.state != "welcome" {
		t.Fatal("sending without activation did not open the gate")
	}
	updated, _ = m.Update(m.loadAssistantOffer()())
	m = updated.(dashboardModel)
	if calls != 1 || !strings.Contains(m.View().Content, "Subscribe · 360 € + VAT per lab / year") {
		t.Fatal("offer missing")
	}
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	view := m.View().Content
	if m.assistant.state != "activate" || !strings.Contains(view, "https://assistant.example/trial") || strings.Contains(view, "Activation code") {
		t.Fatal("trial flow")
	}
	m.actions.AssistantService.Activate = func(context.Context, string) (domain.AssistantActivation, error) {
		return domain.AssistantActivation{Token: flowToken, Account: domain.AssistantAccount{Name: "Synthetic lab", Plan: "trial", Status: "active", TurnsRemaining: 5, PeriodEnd: "2099-01-01"}}, nil
	}
	// The code arrives by email: a terminal paste fills the field, trimmed.
	updated, _ = m.Update(tea.PasteMsg{Content: "  synthetic\n"})
	m = updated.(dashboardModel)
	if got := m.assistant.code.Value(); got != "synthetic" {
		t.Fatalf("pasted activation code = %q", got)
	}
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if m.assistant.state != "consent" {
		t.Fatal("activation bypassed consent")
	}
	if accepted, _ := store.ReadConsent(); accepted {
		t.Fatal("consent implicit")
	}
	updated, _ = m.Update(keyPress("space"))
	m = updated.(dashboardModel)
	m.assistant.input.Reset() // this test checks consent alone, not the pending prompt
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if accepted, _ := store.ReadConsent(); !accepted || m.assistant.state != "chat" || len(m.assistant.entries) != 0 {
		t.Fatal("consent did not continue to empty chat")
	}
	m.assistant.initialized = false
	updated, _ = m.openAssistant()
	m = updated.(dashboardModel)
	updated, _ = m.Update(m.refreshAssistantAccount()())
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || !m.assistant.sharing {
		t.Fatal("persisted consent or sharing lost on reopen")
	}
	// Failed offer requests use only the installed links, with no price.
	m.actions.AssistantService.Offer = func(context.Context) (domain.AssistantOffer, error) {
		return domain.AssistantOffer{}, errors.New("offline")
	}
	updated, _ = m.Update(m.loadAssistantOffer()())
	m = updated.(dashboardModel)
	if m.assistant.offer.PriceText != "" || m.assistant.offer.SubscribeURL != m.actions.Assistant.SubscribeURL || m.assistant.offer.TrialURL != strings.TrimSuffix(m.actions.Assistant.Endpoint, "/v1")+"/trial" {
		t.Fatal("fallback")
	}
	old := m.assistant.offer
	updated, _ = m.Update(assistantOfferMsg{id: m.assistant.id + 1, offer: domain.AssistantOffer{PriceText: "stale"}})
	if updated.(dashboardModel).assistant.offer != old {
		t.Fatal("late offer changed screen")
	}
}

func TestAssistantEventHistoryAndScroll(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	m.assistant.state = "running"
	m.assistant.started = time.Now()
	m.assistant.events = make(chan tea.Msg)
	events := []domain.AssistantEvent{{Type: "activity", Text: "Reading modules/course.nix"}, {Type: "text", Delta: "**Prepared**\n- `course`"}, {Type: "activity", Text: "Reading Nixorium source"}, {Type: "tool_result", ToolName: "evaluate", ToolResult: `{"ok":true,"changedHosts":["pc01"]}`}, {Type: "tool_result", ToolName: "build", ToolResult: `{"ok":true,"built":2,"blocked":1,"skipped":[]}`}}
	for _, event := range events {
		updated, _ := m.finishAssistantStream(assistantStreamMsg{id: m.assistant.id, event: event})
		m = updated.(dashboardModel)
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"● Reading modules/course.nix", "● Reading Nixorium source", "✓ Evaluation passed · changes pc01", "✓ Build check passed · 2 parts built · 1 not checked", "Working… 0s · Esc to stop", "Assistant", "Prepared"} {
		if !strings.Contains(view, want) {
			t.Fatal(want, view)
		}
	}
	for i := 0; i < 50; i++ {
		m.assistant.entries = append(m.assistant.entries, assistantEntry{role: "assistant", text: fmt.Sprintf("message %02d", i)})
	}
	latest := m.View().Content
	updated, _ := m.Update(keyPress("pgup"))
	m = updated.(dashboardModel)
	if m.assistant.historyScroll != 4 || m.View().Content == latest {
		t.Fatal("running history did not scroll")
	}
	updated, _ = m.Update(keyPress("pgdown"))
	m = updated.(dashboardModel)
	if m.assistant.historyScroll != 0 {
		t.Fatal("history scroll")
	}
}

func TestAssistantInterfaceGallery(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {160, 50}} {
		for _, dark := range []bool{false, true} {
			for _, state := range []string{"welcome", "activate", "consent", "chat", "running", "proposal", "warning", "commands"} {
				m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
				m.width, m.height, m.isDark = size[0], size[1], dark
				m.assistant.state = state
				m.assistant.offer = domain.AssistantOffer{PriceText: "360 € + VAT per lab / year", TrialRequests: 10, TrialURL: "https://assistant.example/trial", SubscribeURL: "https://example.org/assistant/"}
				m.assistant.account = domain.AssistantAccount{Name: "Synthetic laboratory", Plan: "trial", Status: "active", TurnsRemaining: 1}
				m.assistant.input.Reset()
				required := []string{"Assistant", "Esc"}
				switch state {
				case "welcome":
					m.assistant.account = domain.AssistantAccount{}
					required = append(required, "Activate Assistant to send your request.", "Start a free trial", "I have an activation code", "Subscribe ·", "Enter confirm")
				case "activate":
					m.assistant.account = domain.AssistantAccount{}
					m.assistant.activationLink = m.assistant.offer.TrialURL
					m.assistant.activationKind, m.assistant.codeStep = "free trial", true
					required = append(required, "Paste the code you receive by email.", "Activation code")
				case "consent":
					required = append(required, "Share my conversations", "Enter continue")
				case "running":
					m.assistant.started = time.Now()
					m.assistant.entries = []assistantEntry{{role: "activity", text: "● Reading modules/course.nix"}, {role: "activity", text: "✗ Evaluation failed"}}
					required = append(required, "● Reading", "✗ Evaluation failed", "Working…")
				case "proposal":
					m.assistant.state = "chat"
					m.assistant.entries = []assistantEntry{{role: "user", text: assistantExamples[0]}, {role: "assistant", text: "# Proposal\n**Ready**\n- Install `gimp`\n```nix\n{ course = 2; }\n```"}}
					m.assistant.changedFiles = 1
					required = append(required, "Proposal ready · 1 file changed", "Ctrl+R to review")
					if size[0] >= 120 {
						required = append(required, "● Proposal")
					}
				case "warning":
					m.assistant.state = "chat"
					m.assistant.input.SetValue("Keep this draft")
					m.assistant.account.UpgradeURL = "https://assistant.example/upgrade?account=synthetic"
					m.assistant.account.Status = "inactive"
					m.message = m.assistantNotice(&domain.AssistantError{Code: "quota_exhausted"})
					required = append(required, "Free trial ended", "Your free trial has ended", "Keep this draft", "/code")
				case "commands":
					m.assistant.state = "chat"
					assistantEnableCommands(&m)
					m.assistant.input.SetValue("/")
					required = append(required, "/share", "/report", "/code")
				case "chat":
					required = append(required, assistantExamples[0], "Free trial · 1 of 10 left", assistantInputHint)
				}
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%s %dx%d overflow %dx%d\n%s", state, size[0], size[1], lipgloss.Width(view), lipgloss.Height(view), view)
				}
				for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
					var output bytes.Buffer
					writer := colorprofile.Writer{Forward: &output, Profile: profile}
					writer.Write([]byte(view))
					for _, want := range required {
						if !strings.Contains(ansi.Strip(output.String()), want) {
							t.Fatalf("%s %dx%d %v missing %q\n%s", state, size[0], size[1], profile, want, output.String())
						}
					}
				}
				if dir := os.Getenv("NIXORIUM_T7_CAPTURES"); dir != "" && !dark && size[0] == 120 && (state == "welcome" || state == "proposal" || state == "warning") {
					if err := os.WriteFile(filepath.Join(dir, state+".txt"), []byte(ansi.Strip(view)), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestAssistantSlashOpensExistingReviewSharingAndReport(t *testing.T) {
	for _, command := range []string{"/review", "/save", "/share", "/report"} {
		m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("slash command sent HTTP") })
		m.report.Meta.Controller.Name = "pc99"
		m.actions.AssistantService.ReadSharing = store.ReadSharing
		m.actions.AssistantService.WriteSharing = store.WriteSharing
		m.actions.AssistantService.PlanSave = func(context.Context, string) domain.AssistantSavePlan {
			return domain.AssistantSavePlan{State: "ready"}
		}
		m.actions.AssistantService.Save = func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
			t.Fatal("slash saved without review")
			return domain.TemplateResetResult{}
		}
		m.assistant.canReport = true
		m.assistant.changedFiles = 1
		m.assistant.conversation = domain.AssistantConversation{ID: flowTurnID}
		m.assistant.input.SetValue(command)
		updated, _ := m.Update(keyPress("enter"))
		m = updated.(dashboardModel)
		switch command {
		case "/review", "/save":
			if m.screen != dashboardAssistantReview || m.read.cancel == nil {
				t.Fatal("review not opened")
			}
			m.read.cancel()
		case "/share":
			if m.assistant.state != "sharing" {
				t.Fatal("sharing not opened")
			}
		case "/report":
			if m.assistant.state != "report" {
				t.Fatal("report not opened")
			}
		}
	}
}

func TestAssistantInlineAccountAndNetworkWarningsKeepDraft(t *testing.T) {
	for _, test := range []struct{ plan, code, want string }{
		{"trial", "quota_exhausted", "Your free trial has ended. Subscribe to continue:"},
		{"annual", "subscription_inactive", "Your subscription is not active. Renew at"},
		{"annual", "quota_exhausted", "All requests for this year are used. They renew on 2099-01-01. Questions: hello@nixorium.org"},
		{"annual", "model_error", assistantFailedNotice},
		{"annual", "model_unavailable", "The AI provider is busy right now. Your request was not completed; try again in a minute."},
	} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
		m.assistant.account = domain.AssistantAccount{Plan: test.plan, Status: "inactive", PeriodEnd: "2099-01-01", UpgradeURL: "https://assistant.example/upgrade"}
		updated, _ := m.finishAssistantAccess(assistantAccessMsg{id: m.assistant.id, err: &domain.AssistantError{Code: test.code}})
		m = updated.(dashboardModel)
		if m.assistant.state != "chat" || m.assistant.input.Value() != "Synthetic private request" || !strings.Contains(m.message, test.want) || !strings.Contains(m.View().Content, "Synthetic private request") {
			t.Fatal("blocking warning", test, m.message)
		}
	}
	m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	store.WriteToken(flowToken)
	updated, _ := m.finishAssistantAccount(assistantAccountMsg{id: m.assistant.id, token: flowToken, err: &domain.AssistantError{Code: "invalid_token"}})
	m = updated.(dashboardModel)
	if token, _ := store.ReadToken(); token != "" || m.assistant.state != "chat" || m.assistant.input.Value() != "Synthetic private request" {
		t.Fatal("invalid activation not removed")
	}
	view := m.View().Content
	if m.message != assistantActivationInvalidNotice || !strings.Contains(view, "Assistant activation is no longer valid") || strings.Contains(view, "could not complete") {
		t.Fatalf("invalid activation is not explained: %q\n%s", m.message, view)
	}
}

// assistantEnableCommands puts the chat in a state where every command applies.
func assistantEnableCommands(m *dashboardModel) {
	m.assistant.changedFiles = 1
	m.assistant.canReport = true
	m.assistant.token = flowToken
	m.assistant.account.Plan = "annual"
	if len(m.assistant.entries) == 0 {
		m.assistant.entries = []assistantEntry{{role: "user", text: "Question"}}
	}
}

func TestAssistantNoticeSeparatesProviderOutageFromConnectionFailure(t *testing.T) {
	if got := assistantServiceNotice(&domain.AssistantError{Status: 503, Code: "model_unavailable"}, ""); got != "The AI provider is busy right now. Your request was not completed; try again in a minute." {
		t.Fatal(got)
	}
	for _, code := range []string{"model_error", "some_future_code", "service_unavailable"} {
		if got := assistantServiceNotice(&domain.AssistantError{Status: 500, Code: code}, ""); got == assistantNetworkNotice || strings.Contains(got, "Internet") {
			t.Fatal("service response reported as connection failure", code)
		}
	}
	wrapped := fmt.Errorf("stream: %w", &domain.AssistantError{Code: "model_unavailable"})
	if assistantServiceNotice(wrapped, "") != assistantProviderBusyNotice {
		t.Fatal("wrapped error")
	}
	for _, err := range []error{errors.New("dial tcp: lookup assistant: no such host"), context.DeadlineExceeded, nil} {
		if assistantServiceNotice(err, "") != assistantNetworkNotice {
			t.Fatal("connection failure lost its notice", err)
		}
	}
}
