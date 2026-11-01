package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (model dashboardModel) updateDeploymentStop(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.deployment.stopReview = false
		model.deployment.confirmation = ""
	case "enter":
		if model.deployment.cancel != nil {
			model.deployment.stopReview = false
			model.deployment.stopRequested = true
			model.deployment.cancel()
		}
		model.deployment.confirmation = ""
	}
	return model, nil
}

func (model dashboardModel) deploymentStopView() string {
	return model.renderShell(tuiShell{
		path: []string{"Computers", "Update computers", "Stop waiting"},
		body: strings.Join([]string{
			tuiTitle("Stop supervising this deployment?", model.isDark), "",
			"This stops the local copy and SSH processes, not a remote rollback.",
			"A client activation may continue after the connection closes.",
			"If apply started, new operations remain blocked until reviewed recovery.",
			"Nixorium will still attempt bounded, authenticated client observations.",
		}, "\n"),
		fixedBody: "Enter stops waiting here; a computer already applying the update may keep going.",
		actions:   []tuiAction{{key: "Enter", label: "Confirm"}, {key: "Esc", label: "Keep waiting"}, {key: "F1", label: "Help"}},
	})
}
