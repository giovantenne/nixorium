package presentation

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func finishResumeCommand(t *testing.T, m dashboardModel, command tea.Cmd) dashboardModel {
	t.Helper()
	if command == nil {
		t.Fatal("missing history command")
	}
	updated, _ := m.Update(command())
	return updated.(dashboardModel)
}

func TestAssistantResumeCurrentAndReadOnlyNeverOpenTurn(t *testing.T) {
	for _, state := range []string{"", "proposal not saved", "saved", "outdated"} {
		t.Run(state, func(t *testing.T) {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("resume called service") })
			m.assistant.input.Reset()
			m.assistant.conversation = domain.AssistantConversation{ID: "current"}
			answer := strings.Repeat("è", 601)
			h := domain.AssistantConversationHistory{State: state, ChangedFiles: 1, Conversation: domain.AssistantConversation{ID: "old", Title: "Previous topic", LastActivityAt: time.Now().Format(time.RFC3339), LastAnswer: &answer}, Messages: []domain.AssistantMessage{{Role: "user", Content: "Old question"}, {Role: "assistant", Content: "Old answer"}}}
			if state == "" {
				h.ChangedFiles = 0
			}
			prepared, runs, plans, cleanup := 0, 0, 0, 0
			m.actions.AssistantService.Cleanup = func(current string) error {
				if current != "current" {
					t.Fatal(current)
				}
				cleanup++
				return nil
			}
			m.actions.AssistantService.ListConversations = func(_ context.Context, current string) ([]domain.AssistantConversationHistory, error) {
				if current != "current" {
					t.Fatal(current)
				}
				return []domain.AssistantConversationHistory{h}, nil
			}
			m.actions.AssistantService.ConversationHistory = func(_ context.Context, id string) (domain.AssistantConversationHistory, error) {
				if id != "old" {
					t.Fatal(id)
				}
				return h, nil
			}
			m.actions.AssistantService.PrepareConversation = func(id string) (domain.AssistantConversation, error) {
				if id != "" {
					t.Fatal("reused old chat")
				}
				prepared++
				return domain.AssistantConversation{ID: "fresh", Base: "current-head"}, nil
			}
			m.actions.AssistantService.Run = func(context.Context, domain.AssistantConversation, string, func(domain.AssistantEvent)) (domain.AssistantTurnResult, error) {
				runs++
				return domain.AssistantTurnResult{}, nil
			}
			m.actions.AssistantService.PlanSave = func(context.Context, string) domain.AssistantSavePlan { plans++; return domain.AssistantSavePlan{} }
			m.actions.AssistantService.Save = func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult {
				return domain.TemplateResetResult{}
			}
			m.report.Meta.Controller.Name = "pc99"
			m.assistant.input.SetValue("/resume")
			updated, cmd := m.Update(keyPress("enter"))
			m = finishResumeCommand(t, updated.(dashboardModel), cmd)
			if m.assistant.state != "resume" || cleanup != 1 || !strings.Contains(ansi.Strip(m.View().Content), "Enter open · Esc back") {
				t.Fatal("resume list")
			}
			updated, cmd = m.Update(keyPress("enter"))
			m = finishResumeCommand(t, updated.(dashboardModel), cmd)
			if state == "saved" || state == "outdated" {
				if m.assistant.state != "history" || m.assistant.conversation.ID != "current" || !strings.Contains(ansi.Strip(m.View().Content), "Old answer") || !strings.Contains(ansi.Strip(m.View().Content), "This conversation is based on an older configuration.") {
					t.Fatal("readonly history")
				}
				for _, key := range []string{"ctrl+r", "ctrl+n", "ctrl+s", "x"} {
					updated, cmd = m.Update(keyPress(key))
					m = updated.(dashboardModel)
					if cmd != nil || plans != 0 || runs != 0 || m.assistant.input.Value() != "" {
						t.Fatal("readonly action", key)
					}
				}
				updated, _ = m.Update(keyPress("down"))
				m = updated.(dashboardModel)
				updated, cmd = m.Update(keyPress("enter"))
				m = updated.(dashboardModel)
				if cmd != nil || m.assistant.state != "resume" || prepared != 0 {
					t.Fatal("Back prepared chat")
				}
				updated, cmd = m.Update(keyPress("enter"))
				m = finishResumeCommand(t, updated.(dashboardModel), cmd)
				updated, cmd = m.Update(keyPress("enter"))
				m = finishResumeCommand(t, updated.(dashboardModel), cmd)
				want := "Continuing a previous chat: \"Previous topic\". Last answer: " + strings.Repeat("è", 599) + "…\n\n"
				if m.assistant.state != "chat" || m.assistant.conversation.ID != "fresh" || m.assistant.conversation.Base != "current-head" || m.assistant.input.Value() != want || prepared != 1 || len(m.assistant.messages) != 0 {
					t.Fatal("prefill", m.assistant.input.Value())
				}
			} else {
				if m.assistant.state != "chat" || m.assistant.conversation.ID != "old" || len(m.assistant.messages) != 2 || m.assistant.changedFiles != h.ChangedFiles || prepared != 0 {
					t.Fatal("current opening")
				}
				if state == "proposal not saved" {
					updated, cmd = m.Update(keyPress("ctrl+r"))
					m = updated.(dashboardModel)
					if cmd == nil || m.screen != dashboardAssistantReview {
						t.Fatal("proposal cannot be reviewed")
					}
				}
			}
			if runs != 0 {
				t.Fatal("opening sent a request")
			}
		})
	}
}

func TestAssistantResumePrefillDatesAndLayouts(t *testing.T) {
	if got := assistantResumePrefill(domain.AssistantConversation{Title: "Legacy without answer"}); got != "Continuing a previous chat: \"Legacy without answer\".\n\n" {
		t.Fatal(got)
	}
	now := time.Date(2026, 10, 11, 15, 30, 0, 0, time.FixedZone("test", 7200))
	for _, test := range [][2]string{{"2026-10-11T12:32:00Z", "Today 14:32"}, {"2026-10-10T20:00:00Z", "Yesterday"}, {"2026-10-09T20:00:00Z", "2026-10-09"}} {
		if assistantHistoryDate(test[0], now) != test[1] {
			t.Fatal(test)
		}
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render network") })
			m.width, m.height, m.isDark = size[0], size[1], dark
			m.assistant.state = "resume"
			m.assistant.resumeCursor = 29
			for i := 0; i < 30; i++ {
				m.assistant.resumeItems = append(m.assistant.resumeItems, domain.AssistantConversationHistory{State: "proposal not saved", Conversation: domain.AssistantConversation{Title: strings.Repeat("Long topic ", 6), LastActivityAt: now.Format(time.RFC3339)}})
			}
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(ansi.Strip(view), "Esc back") {
				t.Fatal("list overflow", size, lipgloss.Width(view), lipgloss.Height(view))
			}
			m.assistant.state = "history"
			m.assistant.readOnlyHistory.Messages = []domain.AssistantMessage{{Role: "assistant", Content: strings.Repeat("History line\n", 80)}}
			view = m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(ansi.Strip(view), "Continue in a new chat") {
				t.Fatal("history overflow", size)
			}
			updated, cmd := m.Update(keyPress("pgup"))
			m = updated.(dashboardModel)
			if cmd != nil || m.assistant.historyScroll == 0 {
				t.Fatal("history cannot scroll")
			}
		}
	}
}

func TestAssistantResumeEscDiscardsLateResults(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("network") })
	m.actions.AssistantService.ListConversations = func(context.Context, string) ([]domain.AssistantConversationHistory, error) { return nil, nil }
	updated, cmd := m.openAssistantResume()
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || m.assistant.resumeLoading {
		t.Fatal("late history opened")
	}
}
