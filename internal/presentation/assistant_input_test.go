package presentation

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

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
