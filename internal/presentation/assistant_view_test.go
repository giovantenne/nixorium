package presentation

import (
	"net/http"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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

func TestAssistantCodeBlocksHideDelimitersAndIndentContent(t *testing.T) {
	for _, dark := range []bool{false, true} {
		got := ansi.Strip(assistantMarkdown("Before\n```nix\n{ course = 2; }\n  nested\n\n```\nAfter **ready** and `code`", dark))
		want := "Before\n  { course = 2; }\n    nested\n  \nAfter ready and code"
		if got != want {
			t.Fatalf("code block presentation: %q", got)
		}
	}
}

func TestAssistantResultBoxesHaveInnerSpace(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
	for _, kind := range []tuiStatusKind{tuiStatusSuccess, tuiStatusAttention} {
		box := ansi.Strip(m.assistantBox(strings.Repeat("word ", 40), 76, kind))
		if lipgloss.Width(box) != 76 {
			t.Fatal("box exceeded available width")
		}
		lines := strings.Split(box, "\n")
		for _, line := range lines[1 : len(lines)-1] {
			if !strings.HasPrefix(line, "│ ") || !strings.HasSuffix(line, " │") {
				t.Fatal("text touches a box border", line)
			}
		}
	}
	m.assistant.changedFiles = 1
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "│ Proposal ready · 1 file changed") || !strings.Contains(view, "Ctrl+R to review │") {
		t.Fatal("proposal spacing or singular lost", view)
	}
}

func TestAssistantGateIntroIsOneLine(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
	m.assistant.state = "welcome"
	m.assistant.account = domain.AssistantAccount{}
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	view = strings.Join(lines, "\n")
	if !strings.Contains(view, "Activate Assistant to send your request.\n\n  › Start a free trial") {
		t.Fatal("gate intro is not a single line above the choices", view)
	}
}
