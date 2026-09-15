package presentation

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

func tuiTitle(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#1D4ED8"), lipgloss.Color("#7AA2F7"))
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(value)
}

func tuiError(value string, darkBackground bool) string {
	color := lipgloss.LightDark(darkBackground)(lipgloss.Color("#B91C1C"), lipgloss.Color("#F7768E"))
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
