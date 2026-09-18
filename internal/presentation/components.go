package presentation

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"image/color"
)

type tuiTheme struct {
	accent    color.Color
	text      color.Color
	muted     color.Color
	success   color.Color
	attention color.Color
	failure   color.Color
}

func newTUITheme(dark bool) tuiTheme {
	return tuiTheme{
		accent:    lipgloss.LightDark(dark)(lipgloss.Color("#1D4ED8"), lipgloss.Color("#7AA2F7")),
		text:      lipgloss.LightDark(dark)(lipgloss.Color("#292524"), lipgloss.Color("#E7E5E4")),
		muted:     lipgloss.LightDark(dark)(lipgloss.Color("#6B7280"), lipgloss.Color("#A8A29E")),
		success:   lipgloss.LightDark(dark)(lipgloss.Color("#047857"), lipgloss.Color("#86EFAC")),
		attention: lipgloss.LightDark(dark)(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")),
		failure:   lipgloss.LightDark(dark)(lipgloss.Color("#B91C1C"), lipgloss.Color("#FDA4AF")),
	}
}

func tuiAccent(dark bool) color.Color {
	return newTUITheme(dark).accent
}

func tuiProgress(width int, dark bool) progress.Model {
	return progress.New(progress.WithColors(tuiAccent(dark)), progress.WithWidth(width))
}

func tuiListDelegate(dark bool) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.Styles = list.NewDefaultItemStyles(dark)
	delegate.SetSpacing(0)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Bold(true).Foreground(tuiAccent(dark)).PaddingLeft(2)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().PaddingLeft(2)
	return delegate
}

type tuiStatusKind int

const (
	tuiStatusNeutral tuiStatusKind = iota
	tuiStatusSuccess
	tuiStatusAttention
	tuiStatusFailure
)

func tuiTitle(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).accent).Render(value)
}

func tuiError(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).failure).Render(value)
}

func tuiSection(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).text).Render(value)
}

func tuiMuted(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Foreground(newTUITheme(darkBackground).muted).Render(value)
}

func newTUISpinner(darkBackground bool) spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(newTUITheme(darkBackground).accent)),
	)
}

func tuiStatus(value string, kind tuiStatusKind, darkBackground bool) string {
	theme := newTUITheme(darkBackground)
	color := theme.muted
	symbol := "○ "
	switch kind {
	case tuiStatusSuccess:
		symbol = "✓ "
		color = theme.success
	case tuiStatusAttention:
		symbol = "! "
		color = theme.attention
	case tuiStatusFailure:
		symbol = "× "
		color = theme.failure
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(symbol + value)
}

type tuiNotice struct {
	kind   tuiStatusKind
	title  string
	detail string
}

type tuiAction struct {
	key   string
	label string
}

type tuiShell struct {
	path    []string
	body    string
	notices []tuiNotice
	actions []tuiAction
}

func renderTUIShell(shell tuiShell, width int, darkBackground bool) string {
	header := tuiTitle("Nixorium", darkBackground)
	if len(shell.path) > 0 {
		header += tuiMuted("  /  "+strings.Join(shell.path, "  /  "), darkBackground)
	}
	lines := []string{
		header,
		"",
		strings.TrimSpace(shell.body),
	}
	if len(shell.notices) > 0 {
		lines = append(lines, "", tuiMuted("NOTICE", darkBackground))
		for _, notice := range shell.notices {
			lines = append(lines, tuiStatus(notice.title, notice.kind, darkBackground))
			if notice.detail != "" {
				lines = append(lines, "  "+notice.detail)
			}
		}
	}
	if len(shell.actions) > 0 {
		lines = append(lines, "", tuiActionBar(width, darkBackground, shell.actions...))
	}
	return strings.Join(lines, "\n") + "\n"
}

func tuiActionBar(width int, darkBackground bool, actions ...tuiAction) string {
	items := make([]string, 0, len(actions))
	for _, action := range actions {
		items = append(items, tuiSection(action.key, darkBackground)+" "+action.label)
	}
	separator := tuiMuted("  ·  ", darkBackground)
	bar := strings.Join(items, separator)
	if width > 0 {
		return lipgloss.NewStyle().Width(max(20, width-6)).Render(bar)
	}
	return bar
}

func tuiResult(value string, success, darkBackground bool) string {
	kind := tuiStatusAttention
	if success {
		kind = tuiStatusSuccess
	}
	return tuiStatus(value, kind, darkBackground)
}

func tuiHelp(width int, darkBackground bool, bindings ...key.Binding) string {
	if len(bindings) > 4 {
		bindings = bindings[:4]
	}
	hasHelp := false
	for _, binding := range bindings {
		if binding.Help().Desc == "help" {
			hasHelp = true
		}
	}
	if !hasHelp {
		bindings = append(bindings, tuiHelpBinding([]string{"f1"}, "F1", "help"))
	}
	model := help.New()
	model.Styles = help.DefaultStyles(darkBackground)
	if width > 0 {
		model.SetWidth(width)
	}
	return model.ShortHelpView(bindings)
}

func tuiHelpBinding(keys []string, label, description string) key.Binding {
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(label, description),
	)
}
