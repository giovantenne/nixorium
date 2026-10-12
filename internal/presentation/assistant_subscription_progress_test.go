package presentation

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func assistantSubscriptionLines(m dashboardModel) int {
	lines, _, _, _ := m.assistantChatLayout()
	return strings.Count(ansi.Strip(strings.Join(lines, "\n")), "Checking Assistant subscription")
}

func TestAssistantSubscriptionProgressOncePerChat(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected HTTP") })
	actions := m.actions.AssistantService
	actions.ReadToken = func() (string, error) { return flowToken, nil }
	actions.ReadConsent = func() (bool, error) { return true, nil }
	checks, runs := 0, 0
	actions.Account = func(context.Context, string) (domain.AssistantAccount, error) {
		checks++
		return domain.AssistantAccount{Status: "active", TurnsRemaining: 5}, nil
	}
	actions.Run = func(_ context.Context, _ domain.AssistantConversation, prompt string, emit func(domain.AssistantEvent)) (domain.AssistantTurnResult, error) {
		runs++
		emit(domain.AssistantEvent{Type: "activity", Text: "Checking Assistant subscription"})
		emit(domain.AssistantEvent{Type: "activity", Text: "Reading modules/course.nix"})
		emit(domain.AssistantEvent{Type: "turn_started"})
		emit(domain.AssistantEvent{Type: "final", Summary: "Ready"})
		return domain.AssistantTurnResult{Messages: []domain.AssistantMessage{{Role: "user", Content: prompt}, {Role: "assistant", Content: "Ready"}}}, nil
	}
	for turn := 1; turn <= 2; turn++ {
		m.assistant.input.SetValue(fmt.Sprintf("Question %d", turn))
		updated, command := m.sendAssistant(false)
		m = assistantWork(t, updated.(dashboardModel), command)
		if checks != turn || runs != turn || assistantSubscriptionLines(m) != 1 {
			t.Fatalf("turn %d: checks=%d runs=%d progress lines=%d", turn, checks, runs, assistantSubscriptionLines(m))
		}
	}
	lines, _, _, _ := m.assistantChatLayout()
	if got := strings.Count(ansi.Strip(strings.Join(lines, "\n")), "Reading modules/course.nix"); got != 2 {
		t.Fatalf("other progress was suppressed: %d lines", got)
	}
	for _, fresh := range []string{"explicit", "after-save"} {
		t.Run(fresh, func(t *testing.T) {
			next := m
			if fresh == "explicit" {
				updated, _ := next.newAssistantChat()
				next = updated.(dashboardModel)
			} else {
				next.assistant.saved = true
			}
			next.assistant.input.SetValue("New topic")
			updated, command := next.sendAssistant(false)
			next = assistantWork(t, updated.(dashboardModel), command)
			if !next.assistant.subscriptionCheckShown {
				t.Fatal("new chat did not show subscription progress")
			}
			count := 0
			for _, entry := range next.assistant.entries {
				if strings.Contains(entry.text, "Checking Assistant subscription") {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("new chat has %d subscription progress lines", count)
			}
		})
	}
}

func TestAssistantSubscriptionProgressFailedFirstAttemptAndRetry(t *testing.T) {
	var admissions atomic.Int32
	m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/turns" {
			t.Errorf("unexpected content request %s", r.URL.Path)
		}
		admissions.Add(1)
		w.WriteHeader(http.StatusPaymentRequired)
		fmt.Fprint(w, `{"error":{"code":"subscription_inactive"}}`)
	})
	if err := store.WriteToken(flowToken); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	for attempt := int32(1); attempt <= 2; attempt++ {
		updated, command := m.sendAssistant(false)
		m = assistantWork(t, updated.(dashboardModel), command)
		if admissions.Load() != attempt || assistantSubscriptionLines(m) != 1 {
			t.Fatalf("attempt %d: admissions=%d progress lines=%d", attempt, admissions.Load(), assistantSubscriptionLines(m))
		}
		if m.message != m.assistantNotice(&domain.AssistantError{Code: "subscription_inactive"}) || m.assistant.input.Value() != "Synthetic private request" {
			t.Fatal("subscription failure or retry draft was hidden")
		}
	}
}

func TestAssistantSubscriptionProgressResumedHistory(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("resume sent HTTP") })
			m.assistant.resumeLoading = true
			h := domain.AssistantConversationHistory{Conversation: domain.AssistantConversation{ID: "resumed"}}
			if existing {
				h.Messages = []domain.AssistantMessage{{Role: "user", Content: "Old question"}, {Role: "assistant", Content: "Old answer"}}
			}
			updated, _ := m.finishAssistantResume(assistantResumeMsg{id: m.assistant.id, history: &h})
			m = updated.(dashboardModel)
			m.assistant.state = "running"
			updated, _ = m.finishAssistantStream(assistantStreamMsg{id: m.assistant.id, event: domain.AssistantEvent{Type: "activity", Text: "Checking Assistant subscription"}})
			m = updated.(dashboardModel)
			want := 1
			if existing {
				want = 0
			}
			if assistantSubscriptionLines(m) != want {
				t.Fatalf("existing=%t: %d subscription progress lines", existing, assistantSubscriptionLines(m))
			}
		})
	}
}
