package presentation

import (
	"fmt"
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
	elapsed := max(time.Duration(0), time.Since(model.controller.started).Truncate(time.Second))
	lines := []string{tuiTitle("Controller configuration", model.isDark), "",
		fmt.Sprintf("%s  elapsed %s", model.busyView(), elapsed)}
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
		notices: []tuiNotice{{kind: tuiStatusAttention, title: "Controller update is running", detail: "Wait for the verified result before closing Nixorium."}},
		actions: []tuiAction{{key: "l", label: "Progress details"}, {key: "F1", label: "Help"}},
	})
}
