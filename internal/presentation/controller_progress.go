package presentation

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// All controller entry points share progress tracking, including automatic
// follow-ups after saving a framework, package-base or software change.
func (model *dashboardModel) trackControllerApply(operation tea.Cmd) tea.Cmd {
	model.controller.applying = true
	model.controller.progress = domain.OperationProgress{}
	model.controller.started = time.Now().UTC()
	model.controller.progressID++
	model.controller.details = false
	model.controller.progressUnavailable = false
	model.progressDetails = false
	if model.actions.LoadControllerProgress == nil {
		return operation
	}
	return tea.Batch(operation, scheduleControllerProgressTick(model.controller.progressID))
}

func (model dashboardModel) controllerProgressView(path []string) string {
	lines := []string{tuiTitle("Controller configuration", model.isDark), "", model.busyView()}
	lines = append(lines, model.operationProgressView(model.controller.progress, "Current progress")...)
	if model.controller.progressUnavailable {
		lines = append(lines, tuiStatus("Managed progress could not be refreshed; the operation may still be running.", tuiStatusAttention, model.isDark))
	}
	if model.progressDetails {
		lines = append(lines, "", tuiMuted("These are managed phases, not the build log. Full output is in the controller service journal.", model.isDark))
		lines = append(lines, "  journalctl -u 'nixorium-apply-controller*' -f")
	}
	return model.renderShell(tuiShell{
		path: path, body: strings.Join(lines, "\n"),
		notices: []tuiNotice{{kind: tuiStatusAttention, title: "Controller update is running", detail: "Wait for verification. The managed job survives a lost terminal."}},
		actions: []tuiAction{{key: "l", label: "Progress details"}, {key: "F1", label: "Help"}},
	})
}

// controllerPlanCurrent reports a verified controller that already runs the
// reviewed revision: its review offers only a way back.
func controllerPlanCurrent(plan domain.ControllerRebuildPlanReport) bool {
	return !plan.HasErrors() && plan.State == "current" && plan.Current
}

// controllerChangeLines states in plain words what a controller application
// changes, or why that is unknown. It never infers live client state.
func controllerChangeLines(plan domain.ControllerRebuildPlanReport) []string {
	switch {
	case controllerPlanCurrent(plan):
		return []string{"Nothing: this controller already runs the saved configuration."}
	case !plan.ChangesKnown:
		return []string{"Unknown: no readable record of the last verified activation. The saved configuration is applied as a whole."}
	case len(plan.Changes) == 0:
		return []string{"No configuration files changed; the activation is repeated to verify this controller."}
	}
	return append([]string{}, plan.Changes...)
}
