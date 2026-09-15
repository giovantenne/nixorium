package presentation

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"image/color"
)

func tuiAccent(dark bool) color.Color {
	return lipgloss.LightDark(dark)(lipgloss.Color("#76533F"), lipgloss.Color("#D8BA98"))
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
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#76533F"), lipgloss.Color("#D8BA98"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiError(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#B91C1C"), lipgloss.Color("#F7768E"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiSection(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#36332F"), lipgloss.Color("#E2DFD8"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiMuted(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#69665F"), lipgloss.Color("#A09D96"))
	return lipgloss.NewStyle().Foreground(color).Render(value)
}

func newTUISpinner(darkBackground bool) spinner.Model {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#76533F"), lipgloss.Color("#D8BA98"))
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(color)),
	)
}

func tuiStatus(value string, kind tuiStatusKind, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#475569"), lipgloss.Color("#A9B1D6"))
	symbol := "○ "
	switch kind {
	case tuiStatusSuccess:
		symbol = "✓ "
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#047857"), lipgloss.Color("#9ECE6A"))
	case tuiStatusAttention:
		symbol = "! "
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#B45309"), lipgloss.Color("#E0AF68"))
	case tuiStatusFailure:
		symbol = "× "
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#B91C1C"), lipgloss.Color("#F7768E"))
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(symbol + value)
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
