package presentation

import (
	"strings"
	"testing"
)

func TestTUIComponentsPreserveTextualMeaning(t *testing.T) {
	title := tuiTitle("Nixorium", true)
	failure := tuiError("Invalid value", true)
	success := tuiResult("Completed", true, true)
	warning := tuiResult("Needs attention", false, true)
	status := tuiStatus("ready", tuiStatusSuccess, true)
	section := tuiSection("Status", true)
	muted := tuiMuted("Secondary", true)
	help := tuiHelp(80, true,
		tuiHelpBinding([]string{"enter"}, "enter", "continue"),
		tuiHelpBinding([]string{"esc"}, "esc", "cancel"),
	)
	for _, expected := range []string{"Nixorium", "Invalid value", "Completed", "Needs attention", "ready", "Status", "Secondary", "enter", "continue", "esc", "cancel"} {
		if !strings.Contains(title+failure+success+warning+status+section+muted+help, expected) {
			t.Fatalf("component rendering omits %q", expected)
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
