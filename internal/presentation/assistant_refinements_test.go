package presentation

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

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

func TestAssistantInputGrowsThenScrollsWithoutLosingContent(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {160, 50}} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("editing sent HTTP") })
		m.width, m.height = size[0], size[1]
		m.assistant.input.SetWidth(size[0] - 6)
		m.assistant.input.Reset()
		if m.assistant.input.Height() != 1 || strings.Count(ansi.Strip(m.assistant.input.View()), "> ") != 1 {
			t.Fatal("empty input is not one row with one prompt")
		}
		for rows := 1; rows <= 9; rows++ {
			lines := make([]string, rows)
			for i := range lines {
				lines[i] = fmt.Sprintf("row-%d", i+1)
			}
			draft := strings.Join(lines, "\n")
			m.assistant.input.Reset()
			updated, _ := m.Update(tea.PasteMsg{Content: draft})
			m = updated.(dashboardModel)
			input := ansi.Strip(m.assistant.input.View())
			if m.assistant.input.Value() != draft || m.assistant.input.Height() != min(rows, 6) {
				t.Fatalf("%d rows did not grow or preserve paste", rows)
			}
			if rows <= 6 {
				if strings.Count(input, "> ") != 1 || (rows > 1 && !strings.Contains(input, "\n  row-2")) {
					t.Fatal("continuation prompt or indentation", input)
				}
			} else if m.assistant.input.ScrollYOffset() == 0 || !strings.Contains(input, lines[rows-1]) || strings.Contains(input, "> ") {
				t.Fatal("long draft did not scroll to the cursor", input)
			}
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(view, "Enter send") {
				t.Fatal("grown input obscures the actions")
			}
		}
		updated, _ := m.Update(keyPress("ctrl+home"))
		m = updated.(dashboardModel)
		if m.assistant.input.ScrollYOffset() != 0 || !strings.Contains(ansi.Strip(m.assistant.input.View()), "> row-1") {
			t.Fatal("cannot return to the first input row")
		}
		m.assistant.input.Reset()
		for range 8 {
			updated, _ = m.Update(keyPress("ctrl+j"))
			m = updated.(dashboardModel)
		}
		if m.assistant.input.Value() != strings.Repeat("\n", 8) || m.assistant.input.Height() != 6 {
			t.Fatal("newline input was blocked at the viewport limit")
		}
	}
}

func TestAssistantInputHeightFollowsSoftWrapAndResize(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("resize sent HTTP") })
	m.assistant.input.SetValue(strings.Repeat("x", 130))
	for _, width := range []int{160, 80, 160} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m = updated.(dashboardModel)
		want := 1
		if width == 80 {
			want = 2
		}
		if m.assistant.input.Height() != want || m.assistant.input.Value() != strings.Repeat("x", 130) {
			t.Fatal("soft wrap did not resize the input", width, m.assistant.input.Height())
		}
	}
}

func TestAssistantQuotaHeaderUsesServiceStateAndRecovers(t *testing.T) {
	for _, test := range []struct{ plan, code, want string }{
		{"trial", "subscription_inactive", "Free trial ended"},
		{"trial", "quota_exhausted", "Free trial ended"},
		{"annual", "quota_exhausted", "No requests left"},
	} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("state update sent HTTP") })
		m.assistant.account = domain.AssistantAccount{Plan: test.plan, Status: "active", TurnsRemaining: 3}
		updated, _ := m.finishAssistantAccess(assistantAccessMsg{id: m.assistant.id, err: &domain.AssistantError{Code: test.code}})
		m = updated.(dashboardModel)
		header := ansi.Strip(m.assistantHeader(76))
		if !strings.Contains(header, test.want) || strings.Contains(header, "3") || m.assistant.input.Value() != "Synthetic private request" {
			t.Fatal("header contradicts the unavailable service state", header)
		}
		updated, _ = m.newAssistantChat()
		m = updated.(dashboardModel)
		if !strings.Contains(m.assistantHeader(76), test.want) {
			t.Fatal("new chat forgot the known quota state")
		}
		account := domain.AssistantAccount{Plan: "annual", Status: "active", TurnsRemaining: 1000}
		updated, _ = m.finishAssistantAccount(assistantAccountMsg{id: m.assistant.id, token: flowToken, account: &account})
		m = updated.(dashboardModel)
		if !strings.Contains(m.assistantHeader(76), "1000 requests left") || m.assistant.accessError != "" {
			t.Fatal("fresh account did not restore the quota")
		}
	}
}

func TestAssistantWelcomeExamplesAreSeparateFromChoices(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
	m.assistant.state = "welcome"
	m.assistant.account = domain.AssistantAccount{}
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	view = strings.Join(lines, "\n")
	if !strings.Contains(view, "    e.g. "+assistantExamples[0]) || !strings.Contains(view, "\n\n  › Start a free trial") {
		t.Fatal("welcome examples are not indented and separate from choices", view)
	}
}
