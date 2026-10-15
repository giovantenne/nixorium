package presentation

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantNoticeSeparatesInterruptionFromInvalidResponse(t *testing.T) {
	if got := assistantServiceNotice(&domain.AssistantError{Code: "service_interrupted"}, ""); got != "The Assistant service interrupted the answer. Send the message again." {
		t.Fatal(got)
	}
	if got := assistantServiceNotice(&domain.AssistantError{Code: "protocol_error", Detail: "unknown tool"}, ""); got != "The Assistant service returned an invalid response. Try again after updating Nixorium." {
		t.Fatal(got)
	}
	if strings.Contains(assistantInterruptedNotice, "updating Nixorium") {
		t.Fatal("an interruption must not ask for an update")
	}
}

func TestAssistantFailureShowsShortDetailOnly(t *testing.T) {
	for _, test := range []struct {
		name, detail string
		shown        bool
	}{
		{"short", "stream ended before a terminal event", true},
		{"limit", strings.Repeat("x", assistantNoticeDetailLimit), true},
		{"too long", strings.Repeat("x", assistantNoticeDetailLimit+1), false},
		{"empty", "", false},
		{"multiline", "first\nsecond", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
			m.assistant.state = "running"
			m.assistant.events = make(chan tea.Msg)
			updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, done: true, err: &domain.AssistantError{Code: "service_interrupted", Detail: test.detail}})
			m = updated.(dashboardModel)
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, "The Assistant service interrupted the answer.") {
				t.Fatal(view)
			}
			if shown := test.detail != "" && strings.Contains(view, test.detail); shown != test.shown {
				t.Fatalf("detail shown = %v, want %v\n%s", shown, test.shown, view)
			}
			// The line belongs to this notice only.
			m.message = "Something else happened."
			if test.detail != "" && strings.Contains(ansi.Strip(m.View().Content), test.detail) {
				t.Fatal("detail outlived its notice")
			}
		})
	}
}
