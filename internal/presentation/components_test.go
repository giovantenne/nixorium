package presentation

import (
	"strings"
	"testing"
)

func TestTUIComponentsPreserveTextualMeaning(t *testing.T) {
	title := tuiTitle("Nixorium", true)
	failure := tuiError("Invalid value", true)
	help := tuiHelp(80, true,
		tuiHelpBinding([]string{"enter"}, "enter", "continue"),
		tuiHelpBinding([]string{"esc"}, "esc", "cancel"),
	)
	for _, expected := range []string{"Nixorium", "Invalid value", "enter", "continue", "esc", "cancel"} {
		if !strings.Contains(title+failure+help, expected) {
			t.Fatalf("component rendering omits %q: %q", expected, title+failure+help)
		}
	}
}

func TestTUIHelpRespectsTerminalWidth(t *testing.T) {
	full := tuiHelp(80, false,
		tuiHelpBinding([]string{"enter"}, "enter", "continue"),
		tuiHelpBinding([]string{"shift+tab"}, "shift+tab", "back"),
		tuiHelpBinding([]string{"esc"}, "esc", "cancel"),
	)
	compact := tuiHelp(20, false,
		tuiHelpBinding([]string{"enter"}, "enter", "continue"),
		tuiHelpBinding([]string{"shift+tab"}, "shift+tab", "back"),
		tuiHelpBinding([]string{"esc"}, "esc", "cancel"),
	)
	if len(compact) >= len(full) || !strings.Contains(compact, "enter") {
		t.Fatalf("help did not adapt to width: full=%q compact=%q", full, compact)
	}
}
