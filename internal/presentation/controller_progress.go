package presentation

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// controllerProgressMissesShown is how many consecutive unreadable polls
// (about 350 ms apart) pass before the progress is reported unreadable.
const controllerProgressMissesShown = 20

// All controller entry points share progress tracking, including automatic
// follow-ups after saving a framework, package-base or software change.
func (model *dashboardModel) trackControllerApply(operation tea.Cmd) tea.Cmd {
	model.controller.applying = true
	model.controller.progress = domain.OperationProgress{}
	model.controller.started = time.Now().UTC()
	model.controller.progressID++
	model.controller.details = false
	model.controller.progressUnavailable = false
	model.controller.progressFailures = 0
	// What led here (for example "update saved") is not the progress.
	model.message = ""
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
		// Without readable progress the journal is the only view of the job:
		// it is shown at once, not behind the details key.
		lines = append(lines, tuiStatus("Progress cannot be read right now; the work continues on the controller.", tuiStatusAttention, model.isDark))
		lines = append(lines, tuiMuted("  Full log: journalctl -u 'nixorium-apply-controller*' -f", model.isDark))
	}
	if model.progressDetails && model.controllerDetailsAvailable() && !model.controller.progressUnavailable {
		lines = append(lines, "", tuiMuted("These are managed phases, not the build log. Full output is in the controller service journal.", model.isDark))
		lines = append(lines, "  journalctl -u 'nixorium-apply-controller*' -f")
	}
	actions := []tuiAction{{key: "F1", label: "Help"}}
	if model.controllerDetailsAvailable() {
		actions = append([]tuiAction{{key: "l", label: "Progress details"}}, actions...)
	}
	return model.renderShell(tuiShell{
		path: path, body: strings.Join(lines, "\n"),
		notices: []tuiNotice{{kind: tuiStatusAttention, title: "Controller update is running", detail: "Wait for verification. The managed job survives a lost terminal."}},
		actions: actions,
	})
}

// The `l` progress toggle exists only while expanding shows something the
// compact view does not. Each predicate mirrors what its expanded view renders.

// operationDetailsAvailable reports whether the expanded managed-progress view
// adds anything: the compact one already shows the phase, the step counter and
// the latest activity, so only earlier activities are new.
func operationDetailsAvailable(operation domain.OperationProgress) bool {
	return len(operation.Recent) > 1
}

// controllerDetailsAvailable reports earlier managed activities. When the
// progress cannot be refreshed the compact view already names the journal.
func (model dashboardModel) controllerDetailsAvailable() bool {
	return operationDetailsAvailable(model.controller.progress)
}

// deploymentDetailsAvailable reports captured command output or authored
// activities, neither of which the compact deployment view shows. Its step and
// computer counters are already in the compact view.
func (model dashboardModel) deploymentDetailsAvailable() bool {
	progress := model.deployment.progress
	return progress.Phase != "" && (len(progress.Output) > 0 || len(model.deployment.recent) > 0)
}

// updatePlanDetailsAvailable reports a check detail line from the candidate
// validation; the compact view shows only the phase and counter.
func (model dashboardModel) updatePlanDetailsAvailable() bool {
	return model.updates.planProgress.Detail != ""
}

// operationDetailsActive reports a running controller, PXE or deployment
// operation whose expanded view would add something.
func (model dashboardModel) operationDetailsActive() bool {
	return (model.deployment.applying && model.deploymentDetailsAvailable()) ||
		(model.controller.applying && model.controllerDetailsAvailable()) ||
		(model.installation.pxePreparing && operationDetailsAvailable(model.installation.pxeProgress))
}

// progressDetailsAvailable is the single condition for the `l` hint and key.
func (model dashboardModel) progressDetailsAvailable() bool {
	return model.operationDetailsActive() || (model.updates.planning && model.updatePlanDetailsAvailable())
}

// progressOperationRunning reports that `l` belongs to a running operation,
// whether or not it has details yet; the key is then inert instead of falling
// through to unrelated handling.
func (model dashboardModel) progressOperationRunning() bool {
	return model.deployment.applying || model.controller.applying || model.installation.pxePreparing || model.updates.planning
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
