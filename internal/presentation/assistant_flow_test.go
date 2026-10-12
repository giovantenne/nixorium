package presentation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantNetworkFailuresPreserveDraft(t *testing.T) {
	for _, activation := range []bool{false, true} {
		t.Run(fmt.Sprint(activation), func(t *testing.T) {
			m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
				connection, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					connection.Close()
				}
			})
			if err := store.WriteToken(flowToken); err != nil {
				t.Fatal(err)
			}
			if err := store.WriteConsent(); err != nil {
				t.Fatal(err)
			}
			key := "enter"
			if activation {
				m.assistant.state = "activate"
				m.assistant.code.SetValue("synthetic-code")
				key = "enter"
			}
			updated, command := m.Update(keyPress(key))
			m = assistantWork(t, updated.(dashboardModel), command)
			if m.message != assistantNetworkNotice || m.assistant.input.Value() != "Synthetic private request" {
				t.Fatal("network failure lost draft or notice")
			}
		})
	}
}

func TestAssistantAllGateLayoutsFit(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("layout made HTTP request") })
		m.width, m.height = size[0], size[1]
		for _, state := range []string{"chat", "welcome", "activate", "consent", "running"} {
			m.assistant.state = state
			m.assistant.streamed = strings.Repeat("Synthetic streaming text ", 100)
			view := m.View().Content
			if (state == "chat" || state == "running") && !strings.Contains(view, "Synthetic private request") {
				t.Fatal("history obscured the draft")
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s exceeds %dx%d", state, size[0], size[1])
			}
		}
	}
}

func TestAssistantTokenIsNotInMessages(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	m.assistant.token = flowToken
	m.assistant.state = "running"
	m.assistant.events = make(chan tea.Msg)
	updated, _ := m.finishAssistantStream(assistantStreamMsg{id: m.assistant.id, event: domain.AssistantEvent{Type: "text", Delta: flowToken}})
	m = updated.(dashboardModel)
	updated, _ = m.finishAssistantStream(assistantStreamMsg{id: m.assistant.id, event: domain.AssistantEvent{Type: "final", Summary: flowToken}})
	m = updated.(dashboardModel)
	if strings.Contains(m.View().Content, flowToken) || strings.Contains(m.assistant.messages[0].Content, flowToken) {
		t.Fatal("token leaked to conversation")
	}
}

const flowTurnID = "12345678-1234-4234-8234-123456789abc"

var flowToken = strings.Repeat("t", 43)

func assistantFlowFixture(t *testing.T, handler http.HandlerFunc) (dashboardModel, adapters.AssistantStateStore) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store := adapters.AssistantStateStore{Directory: t.TempDir()}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	client := adapters.AssistantClient{Endpoint: server.URL + "/v1", Version: "3.2.0"}
	actions := DashboardActions{Assistant: domain.AssistantConfig{Enabled: true, Endpoint: server.URL + "/v1", SubscribeURL: "https://example.org/subscribe"}, AssistantService: &AssistantActions{
		ReadToken: store.ReadToken, WriteToken: store.WriteToken, DeleteToken: store.DeleteToken,
		ReadConsent: store.ReadConsent, WriteConsent: store.WriteConsent, Activate: client.Activate,
	}}
	fixture := &assistantFlowStore{AssistantStateStore: store}
	runner := app.AssistantTurnRunner{Client: client, Store: fixture, Tools: fixture, Version: client.Version}
	actions.AssistantService.Run = runner.Run
	actions.AssistantService.PrepareConversation = func(string) (domain.AssistantConversation, error) {
		return domain.AssistantConversation{ID: flowTurnID}, nil
	}
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, actions, false)
	m.width, m.height = 120, 30
	opened, _ := m.openAssistant()
	m = opened.(dashboardModel)
	m.assistant.state = "chat"
	m.assistant.input.SetValue("Synthetic private request")
	return m, store
}

func assistantWork(t *testing.T, m dashboardModel, command tea.Cmd) dashboardModel {
	t.Helper()
	for index := 0; command != nil && (m.assistant.state == "running" || m.assistant.state == "consent"); index++ {
		if index > 20 {
			t.Fatal("Assistant command loop did not finish")
		}
		message := command()
		updated, next := m.Update(message)
		m, command = updated.(dashboardModel), next
	}
	return m
}

func TestAssistantNoTokenSendsNoHTTPAndPreservesDraft(t *testing.T) {
	var requests atomic.Int32
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
	updated, command := m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if requests.Load() != 0 || command != nil || m.assistant.state != "welcome" || m.assistant.input.Value() != "Synthetic private request" || !m.assistant.pendingSend {
		t.Fatal("no-token CTA sent or lost content")
	}
	if !strings.Contains(m.View().Content, "Activate Assistant to send your request.") {
		t.Fatal("missing CTA")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.screen != dashboardAssistant || m.assistant.state != "chat" || m.assistant.input.Value() != "Synthetic private request" || m.assistant.pendingSend {
		t.Fatal("Esc lost draft")
	}
}

func TestAssistantOpensChatWithoutToken(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("open sent HTTP") })
	m.assistant = newAssistantModel()
	m.assistant.initialized = false
	opened, _ := m.openAssistant()
	m = opened.(dashboardModel)
	view := ansi.Strip(m.View().Content)
	if m.assistant.state != "chat" || !strings.Contains(view, "Not activated") || !strings.Contains(view, assistantExamples[0]) || strings.Contains(view, "requests left") {
		t.Fatal("first access is not the plain chat", view)
	}
}

func TestAssistantPendingPromptIsSentOnceAfterActivationAndConsent(t *testing.T) {
	var turns atomic.Int32
	m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/turns" {
			turns.Add(1)
		}
		w.WriteHeader(500)
	})
	m.actions.AssistantService.WriteConsentSharing = store.WriteConsentSharing
	m.actions.AssistantService.ReadSharing = store.ReadSharing
	m.actions.AssistantService.Activate = func(context.Context, string) (domain.AssistantActivation, error) {
		return domain.AssistantActivation{Token: flowToken, Account: domain.AssistantAccount{Plan: "trial", Status: "active", TurnsRemaining: 5}}, nil
	}
	updated, _ := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.state != "activate" {
		t.Fatal("gate did not open activation")
	}
	updated, _ = m.Update(tea.PasteMsg{Content: "synthetic"})
	m = updated.(dashboardModel)
	updated, command := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(command())
	m = updated.(dashboardModel)
	if m.assistant.state != "consent" || !m.assistant.pendingSend || turns.Load() != 0 {
		t.Fatal("activation skipped consent")
	}
	updated, command = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, command = m.Update(command())
	m = updated.(dashboardModel)
	if m.assistant.state != "running" || m.assistant.pendingSend || command == nil {
		t.Fatalf("pending prompt not sent after consent: %s", m.assistant.state)
	}
	if m.assistant.cancel != nil {
		m.assistant.cancel()
	}
}

func TestAssistantCancelledActivationKeepsDraft(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected HTTP") })
	updated, _ := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.assistant.state != "welcome" || !m.assistant.pendingSend {
		t.Fatal("Esc from activation should return to the gate")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || m.assistant.pendingSend || m.assistant.input.Value() != "Synthetic private request" {
		t.Fatal("cancel lost the draft")
	}
	m.assistant.state, m.assistant.pendingSend = "consent", true
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || m.assistant.pendingSend || m.assistant.input.Value() != "Synthetic private request" {
		t.Fatal("consent cancel lost the draft")
	}
}

func TestAssistantSubscriptionErrorsPreserveDraftAndSendNoContent(t *testing.T) {
	for _, test := range []struct {
		status      int
		code, state string
	}{
		{402, "subscription_inactive", "chat"}, {401, "invalid_token", "chat"},
		{409, "turn_active", "chat"}, {429, "quota_exhausted", "chat"}, {426, "client_upgrade_required", "chat"},
	} {
		t.Run(test.code, func(t *testing.T) {
			var requests atomic.Int32
			m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/v1/turns" {
					t.Error("content endpoint reached")
				}
				var body map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["client"] == nil {
					t.Error("admission leaked draft")
				}
				w.WriteHeader(test.status)
				fmt.Fprintf(w, `{"error":{"code":%q,"message":"Synthetic refusal"}}`, test.code)
			})
			if err := store.WriteToken(flowToken); err != nil {
				t.Fatal(err)
			}
			if err := store.WriteConsent(); err != nil {
				t.Fatal(err)
			}
			updated, command := m.Update(keyPress("enter"))
			m = assistantWork(t, updated.(dashboardModel), command)
			if requests.Load() != 1 || m.assistant.state != test.state || m.assistant.input.Value() != "Synthetic private request" {
				t.Fatalf("refusal state=%s, requests=%d", m.assistant.state, requests.Load())
			}
			if test.status == 401 {
				if token, _ := store.ReadToken(); token != "" {
					t.Fatal("invalid token retained")
				}
				if m.message != assistantActivationInvalidNotice || strings.Contains(m.message, "could not complete") {
					t.Fatalf("invalid token message = %q", m.message)
				}
				updated, command := m.Update(keyPress("enter"))
				m = assistantWork(t, updated.(dashboardModel), command)
				if requests.Load() != 1 || m.assistant.state != "welcome" || m.assistant.input.Value() != "Synthetic private request" {
					t.Fatal("next send after invalid token did not open the gate")
				}
			}
		})
	}
}

func TestAssistantActivationConsentAndStreaming(t *testing.T) {
	var contentRequests atomic.Int32
	m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/activate":
			var body struct {
				Code   string `json:"code"`
				Client any    `json:"client"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Code == "invalid" {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"error":{"code":"invalid_code","message":"Invalid code."}}`)
				return
			}
			fmt.Fprintf(w, `{"token":%q,"account":{"name":"Synthetic account","plan":"test","periodEnd":"2099-01-01"}}`, flowToken)
		case "/v1/turns":
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"turnId":%q,"limits":{"steps":24,"seconds":900,"maxRequestBytes":2000000}}`, flowTurnID)
		case "/v1/turns/" + flowTurnID + "/steps":
			contentRequests.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Preparing course\"}\n\nevent: final\ndata: {\"summary\":\"Synthetic summary\"}\n\n")
		case "/v1/turns/" + flowTurnID + "/cancel":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	})
	updated, command := m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	m.assistant.code.SetValue("invalid")
	updated, command = m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if m.assistant.state != "activate" || m.message != "This code is not valid or was already used." {
		t.Fatal("invalid activation missing")
	}
	m.assistant.code.SetValue("synthetic-code")
	updated, command = m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if m.assistant.state != "consent" || m.message != "" || contentRequests.Load() != 0 {
		t.Fatal("activation bypassed consent")
	}
	if accepted, _ := store.ReadConsent(); accepted {
		t.Fatal("activation saved consent automatically")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	updated, command = m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if m.assistant.state != "consent" || contentRequests.Load() != 0 {
		t.Fatal("missing first-send consent")
	}
	updated, command = m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if contentRequests.Load() != 0 || m.assistant.state != "chat" {
		t.Fatal("consent sent a prompt")
	}
	updated, command = m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if m.assistant.state != "chat" || m.assistant.streamed != "Preparing course" || m.assistant.summary != "Synthetic summary" || contentRequests.Load() != 1 {
		t.Fatal("stream not displayed")
	}
	if accepted, _ := store.ReadConsent(); !accepted {
		t.Fatal("consent not saved")
	}
	if m.assistant.input.Value() != "" {
		t.Fatal("successful request did not clear input")
	}
	m.assistant.input.SetValue("Another synthetic request")
	updated, command = m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if contentRequests.Load() != 2 || m.assistant.state != "chat" {
		t.Fatal("consent asked again")
	}
}

func TestAssistantEscCancelsContextAndRemoteTurn(t *testing.T) {
	var cancelled atomic.Int32
	m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/turns":
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"turnId":%q,"limits":{"steps":24,"seconds":900,"maxRequestBytes":2000000}}`, flowTurnID)
		case "/v1/turns/" + flowTurnID + "/steps":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Working\"}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/v1/turns/" + flowTurnID + "/cancel":
			cancelled.Add(1)
			w.WriteHeader(204)
		}
	})
	store.WriteToken(flowToken)
	store.WriteConsent()
	updated, command := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	for i := 0; m.assistant.streamed != "Working"; i++ {
		if i > 10 || command == nil {
			t.Fatal("stream did not start")
		}
		updated, command = m.Update(command())
		m = updated.(dashboardModel)
	}

	if m.assistant.state != "running" || m.assistant.streamed != "Working" {
		t.Fatal("stream did not start")
	}
	updated, command = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if command != nil {
		command()
	}
	deadline := time.Now().Add(2 * time.Second)
	for cancelled.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cancelled.Load() != 1 || m.message != "Stopped. Your deployment is unchanged." || m.assistant.input.Value() != "" {
		t.Fatal("Esc did not cancel safely")
	}
}

func TestAssistantNewChatAndInputKeys(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("input key made HTTP request") })
	m.assistant.input.SetValue("q?")
	updated, _ := m.Update(keyPress("alt+enter"))
	m = updated.(dashboardModel)
	if !strings.Contains(m.assistant.input.Value(), "\n") {
		t.Fatal("Alt+Enter is not newline")
	}
	updated, _ = m.Update(keyPress("ctrl+n"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "" || !m.assistant.input.Focused() {
		t.Fatal("new chat not empty and focused")
	}
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.input.Value() != "" || m.assistant.state != "chat" {
		t.Fatal("example was selected")
	}
}

func TestAssistantRunnerFlowAndStaleChatResults(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("presentation bypassed runner") })
	actions := m.actions.AssistantService
	actions.ReadToken = func() (string, error) { return flowToken, nil }
	actions.ReadConsent = func() (bool, error) { return true, nil }
	actions.PrepareConversation = func(id string) (domain.AssistantConversation, error) {
		return domain.AssistantConversation{ID: flowTurnID}, nil
	}
	calls := 0
	actions.Run = func(ctx context.Context, c domain.AssistantConversation, prompt string, emit func(domain.AssistantEvent)) (domain.AssistantTurnResult, error) {
		calls++
		emit(domain.AssistantEvent{Type: "text", Delta: "Prepared"})
		emit(domain.AssistantEvent{Type: "final", Summary: "Ready"})
		return domain.AssistantTurnResult{Summary: "Ready", Messages: []domain.AssistantMessage{{Role: "user", Content: prompt}, {Role: "assistant", Content: "Prepared"}}}, nil
	}
	updated, command := m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), command)
	if calls != 1 || m.assistant.conversation.ID != flowTurnID || m.assistant.summary != "Ready" || len(m.assistant.messages) != 2 {
		t.Fatal("runner flow missing")
	}
	oldID := m.assistant.id
	updated, _ = m.Update(keyPress("ctrl+n"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(assistantStreamMsg{id: oldID, event: domain.AssistantEvent{Type: "text", Delta: "stale"}})
	m = updated.(dashboardModel)
	if m.assistant.streamed != "" || m.assistant.conversation.ID != "" {
		t.Fatal("old chat updated new chat")
	}
}

// The real turn runner uses a small in-memory conversation in HTTP flow tests.
// File confinement and Git state are covered by adapter and VM integration tests.
type assistantFlowStore struct {
	adapters.AssistantStateStore
	messages []domain.AssistantMessage
}

func (s *assistantFlowStore) ReadTranscript(domain.AssistantConversation) ([]domain.AssistantMessage, error) {
	return s.messages, nil
}
func (s *assistantFlowStore) WriteTranscript(_ domain.AssistantConversation, m []domain.AssistantMessage) error {
	s.messages = m
	return nil
}
func (s *assistantFlowStore) CompleteTurn(domain.AssistantConversation, []domain.AssistantMessage, *string) error {
	return nil
}
func (s *assistantFlowStore) VisibleFiles(domain.AssistantConversation) ([]string, error) {
	return []string{}, nil
}
func (s *assistantFlowStore) CoreIdentity(domain.AssistantConversation) (*domain.AssistantCore, error) {
	return nil, nil
}
func (s *assistantFlowStore) Execute(context.Context, domain.AssistantConversation, domain.AssistantToolCall) string {
	panic("unexpected tool call in HTTP-only flow test")
}

func TestAssistantProposalReadyUsesActualChangeCount(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("render made HTTP request") })
		m.assistant.state = "running"
		updated, _ := m.finishAssistantStream(assistantStreamMsg{id: m.assistant.id, done: true, changedFiles: count, result: &domain.AssistantTurnResult{Summary: "Prepared"}})
		m = updated.(dashboardModel)
		if count == 0 {
			if strings.Contains(m.View().Content, "Proposal ready") {
				t.Fatal("empty workspace advertised")
			}
		} else {
			want := "Proposal ready · " + countNoun(count, "file") + " changed"
			if !strings.Contains(m.View().Content, want) || !strings.Contains(m.View().Content, "Ctrl+R to review") {
				t.Fatal(m.View().Content)
			}
		}
	}
}
