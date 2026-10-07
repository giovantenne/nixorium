package presentation

import tea "charm.land/bubbletea/v2"

func (model dashboardModel) openAssistant() (tea.Model, tea.Cmd) {
	if !model.actions.Assistant.Enabled || model.actions.ClassroomMode {
		return model, nil
	}
	model.screen = dashboardAssistant
	model.message = ""
	return model, nil
}

func (model dashboardModel) assistantView() string {
	return model.renderShell(tuiShell{
		path:    []string{"Assistant"},
		actions: []tuiAction{{key: "Esc", label: "Back"}, {key: "F1", label: "Help"}},
	})
}
