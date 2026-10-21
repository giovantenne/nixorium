package presentation

import (
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
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

func TestTUIChromeSeparatesFocusShortcutsAndText(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := newTUITheme(dark)
		if theme.controls == theme.accent || theme.text == theme.accent || theme.controls == theme.muted {
			t.Fatal("focus, shortcuts and neutral text must have distinct roles")
		}
		bar := tuiActionBar(80, dark, tuiAction{key: "Enter", label: "Open"})
		if !strings.Contains(bar, tuiShortcut("Enter", dark)) || !strings.Contains(bar, tuiMuted("Open", dark)) {
			t.Fatalf("action bar does not distinguish keys from descriptions: %q", bar)
		}
		if theme.heading == theme.accent || tuiTitle("Heading", dark) == tuiSection("Heading", dark) {
			t.Fatal("page headings must stand apart from focus and neutral section labels")
		}
	}
}

func TestTUIShellScrollsOnlyStructuredBodyRegions(t *testing.T) {
	body := []string{"NOTICE is ordinary body content", "This wording may change without changing layout"}
	for index := 0; index < 20; index++ {
		body = append(body, "body row")
	}
	model := dashboardModel{width: 80, height: 16, pageScroll: 4}
	view := model.renderShell(tuiShell{
		path:      []string{"Translated screen"},
		body:      strings.Join(body, "\n"),
		fixedBody: "Localized confirmation prompt\n> _",
		notices:   []tuiNotice{{kind: tuiStatusAttention, title: "Localized warning"}},
		actions:   []tuiAction{{key: "Enter", label: "Continue"}, {key: "Esc", label: "Cancel"}},
	})
	for _, expected := range []string{"Translated screen", "Localized confirmation prompt", "Localized warning", "Enter", "Continue", "Esc", "Cancel", "scroll"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("structured shell hid %q:\n%s", expected, view)
		}
	}
	if lipgloss.Width(view) > model.width || lipgloss.Height(view) > model.height {
		t.Fatalf("structured shell overflowed: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}

func TestTUIShellUsesBoundedFallbackWhenFixedRegionsExceedHeight(t *testing.T) {
	model := dashboardModel{width: 80, height: 8}
	view := model.renderShell(tuiShell{
		path:      []string{"Small terminal"},
		body:      "body row one\nbody row two\nbody row three",
		fixedBody: "confirmation one\nconfirmation two\nconfirmation three",
		notices:   []tuiNotice{{kind: tuiStatusAttention, title: "warning", detail: "long fixed detail"}},
		actions:   []tuiAction{{key: "Enter", label: "Continue"}, {key: "Esc", label: "Cancel"}},
	})
	if view == "" || lipgloss.Width(view) > model.width || lipgloss.Height(view) > model.height {
		t.Fatalf("small-terminal fallback is not bounded: %dx%d\n%s", lipgloss.Width(view), lipgloss.Height(view), view)
	}
}

func TestShellShowsTheNextStepAboveTheFooter(t *testing.T) {
	shell := tuiShell{path: []string{"Computers"}, body: "Two computers selected.", actions: []tuiAction{{key: "Esc", label: "Selection"}, {key: "Enter", label: "Update computers"}, {key: "F1", label: "Help"}}}
	lines := strings.Split(demoANSI.ReplaceAllString(renderTUIShell(shell, 100, false), ""), "\n")
	top, label, bottom, footer := -1, -1, -1, -1
	for index, line := range lines {
		if strings.HasPrefix(line, "╭") {
			top = index
		}
		if strings.Contains(line, "│ ⏎ Enter  Update computers │") {
			label = index
		}
		if strings.HasPrefix(line, "╰") {
			bottom = index
		}
		if strings.Contains(line, "Esc Selection") {
			footer = index
		}
	}
	if top < 0 || label != top+1 || bottom != top+2 || footer != bottom+1 || strings.Contains(lines[footer], "Enter") {
		t.Fatalf("next step button not above the footer:\n%s", strings.Join(lines, "\n"))
	}
	shell.compactPrimary = true
	compact := demoANSI.ReplaceAllString(renderTUIShell(shell, 100, false), "")
	if strings.Contains(compact, "╭") || !strings.Contains(compact, "Esc Selection  ·  ⏎ Enter Update computers") {
		t.Fatalf("short terminal moved the next step:\n%s", compact)
	}
	navigation := tuiShell{actions: []tuiAction{{key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}}}
	if view := demoANSI.ReplaceAllString(renderTUIShell(navigation, 100, false), ""); strings.Contains(view, "▶") {
		t.Fatalf("navigation shown as next step:\n%s", view)
	}
}

func TestFinishedNetworkInstallationChecksComputersOnEnter(t *testing.T) {
	m := newTestModel(t, withActions(DashboardActions{LoadHosts: func(context.Context) (domain.HostsReport, error) { return domain.HostsReport{}, nil }}))
	m.screen = dashboardPXE
	m.report.PXEPreparation.Ready = true
	m.installation.pxeResult = domain.PXELifecycleReport{Operation: "pxe-stop", State: "completed"}
	if !m.pxeJustFinished() {
		t.Fatal("fixture does not represent a finished network installation")
	}
	if action := m.pxePrimaryAction(); action.key != "h" {
		t.Fatalf("primary after finishing = %+v", action)
	}
	if view := m.View().Content; !strings.Contains(view, "Press Enter to open the computer inventory") {
		t.Fatalf("next step text:\n%s", view)
	}
}
