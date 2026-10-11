package presentation

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func TestAssistantSpeakerRendering(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		for _, dark := range []bool{false, true} {
			for _, profile := range []colorprofile.Profile{colorprofile.NoTTY, colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
				t.Run(fmt.Sprintf("%dx%d/dark=%v/%v", size[0], size[1], dark, profile), func(t *testing.T) {
					m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
					m.width, m.height, m.isDark = size[0], size[1], dark
					updated, _ := m.Update(tea.ColorProfileMsg{Profile: profile})
					m = updated.(dashboardModel)
					m.assistant.state = "chat"
					m.assistant.input.Reset()
					user := assistantEntry{role: "user", text: strings.Repeat("Course materials ", 10) + "\nKeep this second line."}
					reply := assistantEntry{role: "assistant", text: "**Prepared** " + strings.Repeat("course materials ", 10) + "\nReturn to Nixorium."}
					renderedUser := assistantMessage(user, size[0]-4, dark, m.assistantNoColors)
					renderedReply := assistantMessage(reply, size[0]-4, dark, m.assistantNoColors)
					if lipgloss.Width(renderedUser) != size[0]-4 && profile > colorprofile.ASCII {
						t.Fatal("user block does not fill conversation")
					}
					if profile <= colorprofile.ASCII {
						if !strings.HasPrefix(ansi.Strip(renderedUser), "> ") {
							t.Fatal("missing monochrome user marker")
						}
					} else {
						if !strings.Contains(renderedUser, "48;") {
							t.Fatal("missing user background")
						}
						if !strings.HasPrefix(ansi.Strip(renderedUser), " ") {
							t.Fatal("missing inner padding")
						}
					}
					if !strings.HasPrefix(ansi.Strip(renderedReply), "● Prepared") || strings.Contains(renderedReply, "48;") {
						t.Fatal("assistant marker or background", renderedReply)
					}
					for _, line := range strings.Split(ansi.Strip(renderedReply), "\n")[1:] {
						if !strings.HasPrefix(line, "  ") {
							t.Fatal("missing continuation indent")
						}
					}
					for _, block := range []string{renderedUser, renderedReply} {
						if lipgloss.Width(block) > size[0]-4 {
							t.Fatal("message overflows")
						}
					}
					m.assistant.entries = []assistantEntry{{role: "user", text: "Prepare the course."}, {role: "activity", text: "● Reading modules/course.nix"}, {role: "activity", text: "✓ Evaluation passed"}, {role: "assistant", text: "Prepared the course."}}
					lines, _, _, _ := m.assistantChatLayout()
					for i := range lines {
						lines[i] = strings.TrimRight(ansi.Strip(lines[i]), " ")
					}
					body := strings.Join(lines, "\n")
					if strings.Contains(body, "› You") || strings.Contains(body, "Assistant\n") {
						t.Fatal("speaker labels remain", body)
					}
					if !strings.Contains(body, "✓ Evaluation passed\n\n● Prepared the course.") {
						t.Fatal("activity is not separated from the answer", body)
					}
					view := m.View().Content
					if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
						t.Fatal("screen overflows")
					}
					m.assistant.archivedEntries = []assistantEntry{{role: "user", text: "Previous request."}, {role: "assistant", text: "Previous answer."}}
					lines, _, _, _ = m.assistantChatLayout()
					body = ansi.Strip(strings.Join(lines, "\n"))
					if strings.Contains(body, "› You") || !strings.Contains(body, "● Previous answer.") {
						t.Fatal("archived speaker rendering", body)
					}
				})
			}
		}
	}
}
