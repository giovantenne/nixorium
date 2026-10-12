package presentation

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func assistantMenu(t *testing.T, m dashboardModel) []string {
	t.Helper()
	m.assistant.input.SetValue("/")
	return m.assistant.commandChoices()
}

func TestAssistantMenuListsOnlyAvailableCommands(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("HTTP") })
	m.assistant.entries, m.assistant.messages, m.assistant.changedFiles, m.assistant.canReport = nil, nil, 0, false
	m.assistant.token, m.assistant.account.Plan = flowToken, "trial"
	if got := assistantMenu(t, m); !reflect.DeepEqual(got, []string{"/resume", "/share", "/help", "/name", "/code"}) {
		t.Fatalf("fresh chat: %v", got)
	}
	m.assistant.entries = []assistantEntry{{role: "user", text: "Q"}, {role: "assistant", text: "A"}}
	m.assistant.canReport, m.assistant.changedFiles = true, 1
	want := []string{"/review", "/save", "/new", "/resume", "/report", "/share", "/help", "/name", "/code"}
	if got := assistantMenu(t, m); !reflect.DeepEqual(got, want) {
		t.Fatalf("after proposal: %v", got)
	}
	m.assistant.account.Plan = "annual"
	want = []string{"/review", "/save", "/new", "/resume", "/billing", "/report", "/share", "/help", "/name", "/code"}
	if got := assistantMenu(t, m); !reflect.DeepEqual(got, want) {
		t.Fatalf("annual: %v", got)
	}
	m.assistant.token = ""
	if got := assistantMenu(t, m); strings.Contains(strings.Join(got, " "), "/billing") {
		t.Fatalf("billing without token: %v", got)
	}
}

func TestAssistantUnavailableCommandsExplainThemselves(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("HTTP") })
	m.assistant.entries, m.assistant.messages, m.assistant.changedFiles, m.assistant.canReport = nil, nil, 0, false
	m.assistant.account.Plan = "trial"
	for command, text := range map[string]string{
		"/review":  "No proposal to review yet. Ask Assistant for a change first.",
		"/save":    "No proposal to review yet. Ask Assistant for a change first.",
		"/report":  "/report becomes available after Assistant answers.",
		"/new":     "This chat is already empty.",
		"/billing": "/billing becomes available after subscribing.",
	} {
		next := m
		next.message = ""
		next.assistant.input.SetValue(command)
		updated, cmd := next.Update(keyPress("enter"))
		next = updated.(dashboardModel)
		if cmd != nil || next.message != text || next.screen != dashboardAssistant || next.assistant.state == "billing-loading" {
			t.Fatalf("%s: message=%q", command, next.message)
		}
	}
	m.message = ""
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = updated.(dashboardModel)
	if m.message != "No proposal to review yet. Ask Assistant for a change first." || m.screen != dashboardAssistant {
		t.Fatalf("Ctrl+R: %q", m.message)
	}
}

func TestAssistantHelpStillListsEveryCommand(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("HTTP") })
	m.assistant.entries, m.assistant.changedFiles, m.assistant.canReport = nil, 0, false
	m.assistant.input.SetValue("/help")
	updated, _ := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	view := strings.Join(strings.Fields(ansi.Strip(m.helpView())), " ")
	for _, command := range assistantCommands {
		if !strings.Contains(view, command+" ") {
			t.Fatalf("help lacks %s", command)
		}
	}
}
