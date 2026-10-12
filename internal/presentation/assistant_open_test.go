package presentation

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func openActivationStep(t *testing.T, display bool, choice int) (dashboardModel, *[]string, *error) {
	t.Helper()
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected HTTP") })
	opened := []string{}
	var failure error
	if display {
		m.actions.OpenURL = func(address string) error { opened = append(opened, address); return failure }
	}
	m.assistant.offer.TrialURL, m.assistant.offer.SubscribeURL = "https://assistant.example/trial", "https://example.org/assistant/"
	updated, _ := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	for i := 0; i < choice; i++ {
		updated, _ = m.Update(keyPress("down"))
		m = updated.(dashboardModel)
	}
	updated, _ = m.Update(keyPress("enter"))
	return updated.(dashboardModel), &opened, &failure
}

func TestActivationLinkStepShowsOnlyTheInstruction(t *testing.T) {
	m, opened, _ := openActivationStep(t, true, 0)
	view := ansi.Strip(m.View().Content)
	if m.assistant.state != "activate" || !strings.Contains(view, "Press Enter to open the free trial page in your browser.") || !strings.Contains(view, "https://assistant.example/trial") || strings.Contains(view, "Activation code") || len(*opened) != 0 {
		t.Fatalf("instruction step wrong:\n%s", view)
	}
	updated, _ := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	view = ansi.Strip(m.View().Content)
	if len(*opened) != 1 || (*opened)[0] != "https://assistant.example/trial" || !strings.Contains(view, "Activation code") || !strings.Contains(view, "Paste the code you receive by email.") || !strings.Contains(view, "https://assistant.example/trial") {
		t.Fatalf("Enter did not open the page and show the input (%v):\n%s", *opened, view)
	}
	updated, _ = m.Update(tea.PasteMsg{Content: "  code-1\n"})
	m = updated.(dashboardModel)
	if m.assistant.code.Value() != "code-1" {
		t.Fatalf("paste = %q", m.assistant.code.Value())
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	view = ansi.Strip(m.View().Content)
	if m.assistant.state != "activate" || strings.Contains(view, "Activation code") || m.assistant.code.Value() != "" || !strings.Contains(view, "Press Enter to open") {
		t.Fatalf("Esc did not return to the instruction:\n%s", view)
	}
	updated, _ = m.Update(keyPress("esc"))
	if updated.(dashboardModel).assistant.state != "welcome" {
		t.Fatal("second Esc did not return to the gate")
	}
}

func TestActivationSubscribeStepAndBrowserFailure(t *testing.T) {
	m, opened, failure := openActivationStep(t, true, 2)
	*failure = tea.ErrInterrupted
	if !strings.Contains(ansi.Strip(m.View().Content), "open the subscription page") {
		t.Fatal("subscription instruction missing")
	}
	updated, _ := m.Update(keyPress("enter"))
	view := ansi.Strip(updated.(dashboardModel).View().Content)
	if len(*opened) != 1 || !strings.Contains(view, "Could not open the browser; open the link above.") || !strings.Contains(view, "Activation code") {
		t.Fatalf("failure notice missing:\n%s", view)
	}
}

func TestActivationWithoutDisplayOnlyShowsTheInput(t *testing.T) {
	m, opened, _ := openActivationStep(t, false, 0)
	updated, _ := m.Update(keyPress("enter"))
	view := ansi.Strip(updated.(dashboardModel).View().Content)
	if len(*opened) != 0 || !strings.Contains(view, "Activation code") || !strings.Contains(view, "https://assistant.example/trial") {
		t.Fatalf("no-display step wrong:\n%s", view)
	}
}

func TestActivationCodeChoiceShowsTheInputImmediately(t *testing.T) {
	m, opened, _ := openActivationStep(t, true, 1)
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Activation code") || strings.Contains(view, "Press Enter to open") || len(*opened) != 0 {
		t.Fatalf("code choice wrong:\n%s", view)
	}
	updated, _ := m.Update(keyPress("esc"))
	if updated.(dashboardModel).assistant.state != "welcome" {
		t.Fatal("Esc did not return to the gate")
	}
}
