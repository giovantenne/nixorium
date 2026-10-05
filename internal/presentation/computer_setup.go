package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

type computerSetupMsg struct {
	configured bool
	err        error
}

func (model dashboardModel) openComputersArea() (tea.Model, tea.Cmd) {
	model.screen = dashboardComputersArea
	model.message = ""
	model.computerSetup = ""
	if model.actions.ClassroomMode || model.actions.LoadClientSetup == nil {
		return model, nil
	}
	model.busy = "Checking saved client configuration"
	return model.startRead(func(ctx context.Context) tea.Msg {
		configured, err := model.actions.LoadClientSetup(ctx)
		return computerSetupMsg{configured: configured, err: err}
	})
}
func (model dashboardModel) finishComputerSetup(msg computerSetupMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardComputersArea {
		return model, nil
	}
	model.busy = ""
	model.computerSetup = "ready"
	if msg.err != nil {
		model.computerSetup = "unknown"
	} else if !msg.configured {
		model.computerSetup = "missing"
	}
	return model, nil
}
