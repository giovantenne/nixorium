package presentation

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantExamplesAreHints(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("example sent HTTP") })
	m.assistant.input.Reset()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Try asking, for example:") {
		t.Fatal(view)
	}
	for _, example := range assistantExamples {
		if !strings.Contains(view, "  "+example) || strings.Contains(view, "› "+example) {
			t.Fatal(view)
		}
	}
	for _, key := range []string{"up", "down", "enter"} {
		updated, cmd := m.Update(keyPress(key))
		m = updated.(dashboardModel)
		if cmd != nil || m.assistant.input.Value() != "" || m.assistant.state != "chat" {
			t.Fatal("hint selected or submitted")
		}
	}
	m.assistant.state = "welcome"
	for i := 0; i < 4; i++ {
		view = ansi.Strip(m.View().Content)
		for _, example := range assistantExamples {
			if strings.Contains(view, "› "+example) {
				t.Fatal("welcome example selected")
			}
		}
		updated, _ := m.Update(keyPress("down"))
		m = updated.(dashboardModel)
	}
}

func TestAssistantDraftClearsOnlyAfterAdmission(t *testing.T) {
	for _, failAfterStart := range []bool{false, true} {
		t.Run(fmt.Sprint(failAfterStart), func(t *testing.T) {
			m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/turns":
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintf(w, `{"turnId":%q,"limits":{"steps":24,"seconds":900,"maxRequestBytes":2000000}}`, flowTurnID)
				case strings.HasSuffix(r.URL.Path, "/steps"):
					if failAfterStart {
						w.WriteHeader(503)
						fmt.Fprint(w, `{"error":{"code":"unavailable"}}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Prepared\"}\n\nevent: final\ndata: {\"summary\":\"Prepared\"}\n\n")
				case strings.HasSuffix(r.URL.Path, "/cancel"):
					w.WriteHeader(204)
				default:
					t.Error("unexpected route", r.URL.Path)
				}
			})
			if err := store.WriteToken(flowToken); err != nil {
				t.Fatal(err)
			}
			if err := store.WriteConsent(); err != nil {
				t.Fatal(err)
			}
			updated, cmd := m.Update(keyPress("enter"))
			m = updated.(dashboardModel)
			sawAdmission := false
			for i := 0; cmd != nil && m.assistant.state == "running"; i++ {
				if i > 20 {
					t.Fatal("turn did not finish")
				}
				msg := cmd()
				if event, ok := msg.(assistantStreamMsg); ok && event.event.Type == "turn_started" {
					sawAdmission = true
				}
				updated, next := m.Update(msg)
				m, cmd = updated.(dashboardModel), next
				want := "Synthetic private request"
				if sawAdmission {
					want = ""
				}
				if m.assistant.input.Value() != want {
					t.Fatalf("admitted=%v draft=%q", sawAdmission, m.assistant.input.Value())
				}
			}
			if !sawAdmission || m.assistant.state != "chat" {
				t.Fatal("missing admission or completion")
			}
			if !failAfterStart && strings.Count(ansi.Strip(m.View().Content), "Prepared") != 1 {
				t.Fatal("streamed final duplicated")
			}
		})
	}
	// Workspace preparation can fail after access checks and before admission.
	m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("preparation made HTTP") })
	if err := store.WriteToken(flowToken); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	m.actions.AssistantService.PrepareConversation = func(string) (domain.AssistantConversation, error) {
		return domain.AssistantConversation{}, context.DeadlineExceeded
	}
	updated, cmd := m.Update(keyPress("enter"))
	m = assistantWork(t, updated.(dashboardModel), cmd)
	if m.assistant.input.Value() != "Synthetic private request" {
		t.Fatal("pre-start failure lost draft")
	}
}

func TestAssistantFinalDoesNotRepeatEarlierStreaming(t *testing.T) {
	for _, summary := range []string{"Final answer", "Different handoff"} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
		m.assistant.input.Reset()
		m.assistant.state = "running"
		m.assistant.events = make(chan tea.Msg)
		m.assistant.entries = []assistantEntry{{"user", "Request"}}
		for _, event := range []domain.AssistantEvent{
			{Type: "text", Delta: "Investigating the configuration"},
			{Type: "activity", Text: "Reading modules/course.nix"},
			{Type: "text", Delta: "Final "}, {Type: "text", Delta: "answer"},
			{Type: "final", Summary: summary}, {Type: "final", Summary: summary},
		} {
			updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, event: event})
			m = updated.(dashboardModel)
		}
		updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, done: true, result: &domain.AssistantTurnResult{Summary: summary}})
		m = updated.(dashboardModel)
		view := ansi.Strip(m.View().Content)
		if strings.Count(view, "Final answer") != 1 || strings.Count(view, summary) != 1 {
			t.Fatal(view)
		}
	}
	// The done fallback also deduplicates when there was no final notification.
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	m.assistant.entries = []assistantEntry{{"user", "Request"}, {"assistant", "Final answer"}}
	updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, done: true, result: &domain.AssistantTurnResult{Summary: "Final answer"}})
	if len(updated.(dashboardModel).assistant.entries) != 2 {
		t.Fatal("done repeated text")
	}
}

func TestAssistantPreparingActivityOnlyKeepsSpinner(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	m.assistant.state = "running"
	m.assistant.events = make(chan tea.Msg)
	for _, text := range []string{"Preparing a proposal", "Reading modules/course.nix"} {
		updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, event: domain.AssistantEvent{Type: "activity", Text: text}})
		m = updated.(dashboardModel)
		if m.assistant.activity != text {
			t.Fatal("spinner activity lost")
		}
	}
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "Preparing a proposal") || !strings.Contains(view, "Reading modules/course.nix") || !strings.Contains(view, "Working…") {
		t.Fatal(view)
	}
}

func assistantVisibleTop(m dashboardModel) int {
	lines, _, _, capacity := m.assistantChatLayout()
	return max(0, len(lines)-capacity-m.assistant.historyScroll)
}
func TestAssistantScrollKeepsPositionDuringStreaming(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(fmt.Sprint(running), func(t *testing.T) {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
			m.assistant.input.Reset()
			if running {
				m.assistant.state = "running"
			}
			m.assistant.events = make(chan tea.Msg)
			for i := 0; i < 50; i++ {
				m.assistant.entries = append(m.assistant.entries, assistantEntry{"assistant", fmt.Sprintf("message %02d", i)})
			}
			for _, test := range []struct {
				msg    tea.Msg
				offset int
			}{
				{keyPress("pgup"), 4}, {keyPress("up"), 5}, {keyPress("down"), 4},
				{tea.MouseWheelMsg{Button: tea.MouseWheelUp}, 8}, {tea.MouseWheelMsg{Button: tea.MouseWheelDown}, 4},
			} {
				updated, _ := m.Update(test.msg)
				m = updated.(dashboardModel)
				if m.assistant.historyScroll != test.offset {
					t.Fatal("scroll", m.assistant.historyScroll, test.offset)
				}
			}
			top := assistantVisibleTop(m)
			for _, event := range []domain.AssistantEvent{{Type: "text", Delta: "\nnew streamed line\nnext line"}, {Type: "activity", Text: "Reading flake.nix"}, {Type: "final", Summary: "A different final summary"}} {
				updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, event: event})
				m = updated.(dashboardModel)
				if assistantVisibleTop(m) != top {
					t.Fatal("new content moved history")
				}
			}
			updated, _ := m.Update(keyPress("end"))
			m = updated.(dashboardModel)
			if m.assistant.historyScroll != 0 {
				t.Fatal("End did not follow bottom")
			}
			updated, _ = m.Update(assistantStreamMsg{id: m.assistant.id, event: domain.AssistantEvent{Type: "text", Delta: "\nNewest line"}})
			m = updated.(dashboardModel)
			if m.assistant.historyScroll != 0 || !strings.Contains(ansi.Strip(m.View().Content), "Newest line") {
				t.Fatal("bottom no longer follows content")
			}
			for i := 0; i < 100; i++ {
				updated, _ = m.Update(keyPress("pgup"))
				m = updated.(dashboardModel)
			}
			if assistantVisibleTop(m) != 0 {
				t.Fatal("cannot reach start")
			}
			for i := 0; i < 100; i++ {
				updated, _ = m.Update(keyPress("pgdown"))
				m = updated.(dashboardModel)
			}
			if m.assistant.historyScroll != 0 {
				t.Fatal("cannot reach bottom")
			}
			if m.View().MouseMode != tea.MouseModeCellMotion {
				t.Fatal("wheel not enabled")
			}
		})
	}
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	for i := 0; i < 30; i++ {
		m.assistant.entries = append(m.assistant.entries, assistantEntry{"assistant", "older message"})
	}
	m.assistant.historyScroll = 4
	// With a draft, arrows edit the text; Enter starts submission and returns to bottom.
	m.assistant.input.SetValue("one\ntwo")
	updated, _ := m.Update(keyPress("up"))
	m = updated.(dashboardModel)
	if m.assistant.historyScroll != 4 {
		t.Fatal("draft arrows scrolled history")
	}
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.historyScroll != 0 || m.assistant.input.Value() != "one\ntwo" {
		t.Fatal("submission did not return to bottom or cleared too soon")
	}
	if m.assistant.cancel != nil {
		m.assistant.cancel()
	}
}
