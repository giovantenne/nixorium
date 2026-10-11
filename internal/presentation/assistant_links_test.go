package presentation

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAssistantLinkifyPreservesTextAndExistingLinks(t *testing.T) {
	urls := []string{"https://example.invalid/trial?code=a%2Bb&lab=1#activate", "http://example.invalid/wiki/Test_(lab)", "https://example.invalid/é"}
	text := "Visit " + urls[0] + ". See (" + urls[1] + "), then " + urls[2] + "."
	linked := assistantLinkify(text)
	if ansi.Strip(linked) != text || ansi.StringWidth(linked) != ansi.StringWidth(text) {
		t.Fatal("linkification changed the visible text")
	}
	for _, url := range urls {
		if !strings.Contains(linked, "\x1b]8;;"+url+"\x1b\\"+url+"\x1b]8;;\x1b\\") {
			t.Fatalf("URL not linked intact: %q", url)
		}
	}
	if assistantLinkify(linked) != linked {
		t.Fatal("existing links nested or changed")
	}
	styled := "\x1b[1m" + urls[0] + "\x1b[m"
	if got := assistantLinkify(styled); !strings.HasPrefix(got, "\x1b[1m\x1b]8;;") || !strings.HasSuffix(got, "\x1b[m") || ansi.Strip(got) != urls[0] {
		t.Fatal("terminal styles changed", got)
	}
	belLink := "\x1b]8;;" + urls[0] + "\x07" + urls[0] + "\x1b]8;;\x07"
	if assistantLinkify(belLink) != belLink {
		t.Fatal("existing BEL-terminated link changed")
	}
}

func TestAssistantLongLinksUseVisibleWidth(t *testing.T) {
	url := "https://example.invalid/" + strings.Repeat("long-path/", 32) + "?code=a%2Bb&lab=1#activate"
	text := "Read " + url + " for details."
	for _, width := range []int{20, 76, 116} {
		plain := lipgloss.NewStyle().Width(width).Render(text)
		linked := lipgloss.NewStyle().Width(width).Render(assistantLinkify(text))
		if ansi.Strip(linked) != plain || lipgloss.Width(linked) != lipgloss.Width(plain) || lipgloss.Height(linked) != lipgloss.Height(plain) {
			t.Fatalf("URL escapes changed wrapping at width %d", width)
		}
		if demoANSI.ReplaceAllString(linked, "") != plain {
			t.Fatal("demo export lost visible URL text")
		}
		if !strings.Contains(linked, "\x1b]8;;"+url+"\x1b\\") || assistantLinkify(linked) != linked {
			t.Fatal("wrapping split the link destination or relinked it")
		}
		for _, line := range strings.Split(linked, "\n") {
			if ansi.StringWidth(line) != width {
				t.Fatal("URL changed line width", ansi.StringWidth(line), width)
			}
		}
	}
}

func TestAssistantLinksInResponsesNoticesAndScreens(t *testing.T) {
	url := "https://example.invalid/trial?code=a%2Bb&lab=1"
	opening := "\x1b]8;;" + url + "\x1b\\"
	for _, dark := range []bool{false, true} {
		m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("rendering sent HTTP") })
		m.width, m.height, m.isDark = 120, 40, dark
		for _, text := range []string{"Read " + url, "Read **" + url + "**", "```\n" + url + "\n```"} {
			if !strings.Contains(assistantMarkdown(text, dark), opening) {
				t.Fatal("response URL not linked")
			}
		}
		for _, kind := range []tuiStatusKind{tuiStatusAttention, tuiStatusSuccess, tuiStatusFailure} {
			box := m.assistantBox("Visit "+url+".", 76, kind)
			if !strings.Contains(box, opening) || lipgloss.Width(box) > 76 || !strings.Contains(ansi.Strip(box), url+".") {
				t.Fatal("notice URL not linked intact", box)
			}
		}
		m.assistant.entries = []assistantEntry{{role: "assistant", text: "Read " + url}}
		m.assistant.archivedEntries = []assistantEntry{{role: "assistant", text: "Previous " + url}}
		if strings.Count(m.View().Content, opening) != 2 {
			t.Fatal("current or archived answer not linked")
		}
		for _, state := range []string{"welcome", "consent", "activate", "sharing", "report", "chat", "running"} {
			m.assistant.state = state
			m.message = "Visit " + url + "."
			if !strings.Contains(m.View().Content, opening) || m.View().MouseMode != tea.MouseModeNone {
				t.Fatal("screen URL missing or mouse captured", state)
			}
		}
		m.message = ""
		m.assistant.state, m.assistant.activationLink = "activate", url
		if strings.Count(m.View().Content, opening) != 1 {
			t.Fatal("activation link missing or nested")
		}
		m.screen = dashboardAssistantReview
		m.assistant.review.plan.State = "blocked"
		m.assistant.review.plan.Message = "Visit " + url
		if !strings.Contains(m.View().Content, opening) || m.View().MouseMode != tea.MouseModeNone {
			t.Fatal("review link missing or mouse captured")
		}
	}
}
