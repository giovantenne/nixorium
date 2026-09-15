package presentation

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

type tuiStatusKind int

const (
	tuiStatusNeutral tuiStatusKind = iota
	tuiStatusSuccess
	tuiStatusAttention
	tuiStatusFailure
)

func tuiTitle(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#1D4ED8"), lipgloss.Color("#7AA2F7"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiError(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#B91C1C"), lipgloss.Color("#F7768E"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiSection(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#334155"), lipgloss.Color("#C0CAF5"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiMuted(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#64748B"), lipgloss.Color("#7F849C"))
	return lipgloss.NewStyle().Foreground(color).Render(value)
}

func newTUISpinner(darkBackground bool) spinner.Model {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#1D4ED8"), lipgloss.Color("#7AA2F7"))
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(color)),
	)
}

func tuiStatus(value string, kind tuiStatusKind, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#475569"), lipgloss.Color("#A9B1D6"))
	switch kind {
	case tuiStatusSuccess:
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#047857"), lipgloss.Color("#9ECE6A"))
	case tuiStatusAttention:
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#B45309"), lipgloss.Color("#E0AF68"))
	case tuiStatusFailure:
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#B91C1C"), lipgloss.Color("#F7768E"))
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(strings.ToUpper(value))
}

func tuiResult(value string, success, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#B45309"), lipgloss.Color("#E0AF68"))
	if success {
		color = lipgloss.LightDark(darkBackground)(lipgloss.Color("#047857"), lipgloss.Color("#9ECE6A"))
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiHelp(width int, darkBackground bool, bindings ...key.Binding) string {
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
