package presentation

import (
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) controllerView() string {
	path := []string{"Maintenance", "Controller"}
	lines := []string{tuiTitle("Controller configuration", model.isDark)}
	notices := []tuiNotice{}
	if model.controller.applying {
		return model.controllerProgressView(path)
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.screen == dashboardControllerReview {
		plan := model.controller.plan
		details := []string{"", tuiMuted("Technical details", model.isDark), tuiMuted("Revision  "+plan.Revision, model.isDark)}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		if controllerPlanCurrent(plan) {
			body := strings.Join(append([]string{
				tuiResult("This controller is up to date", true, model.isDark),
				"",
				plan.Controller + " already runs the saved configuration and its activation",
				"was verified. There is nothing to apply.",
			}, details...), "\n")
			return model.renderShell(tuiShell{path: append(path, "Review"), body: body, notices: notices, actions: []tuiAction{{key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}})
		}
		lines := []string{tuiTitle("Apply the saved configuration to this controller?", model.isDark), "", "What changes since the last verified activation:"}
		for _, line := range controllerChangeLines(plan) {
			lines = append(lines, "  • "+line)
		}
		lines = append(lines, "", "Affects   "+plan.Controller+" (this controller only; client computers are not changed)", "", "Press Enter to build, activate, and verify this controller.")
		body := strings.Join(append(lines, details...), "\n")
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Services and networking may restart", detail: "This connection may be interrupted. Nixorium builds, activates and verifies the reviewed configuration; a reboot is not normally required."})
		return model.renderShell(tuiShell{path: append(path, "Review"), body: body, notices: notices, actions: []tuiAction{{key: "Enter", label: "Apply to controller"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}})
	}
	if model.controller.result.Operation != "" {
		resultTitle := "Controller action needs attention"
		if !model.controller.result.HasErrors() && model.controller.result.Applied && model.controller.result.Verified {
			resultTitle = "Applied to this controller and verified"
		}
		lines = append(lines,
			tuiResult(resultTitle, !model.controller.result.HasErrors(), model.isDark),
			"",
			fmt.Sprintf("Last result: %s at phase %s", model.controller.result.State, model.controller.result.Phase),
			fmt.Sprintf("Applied: %t   Verified: %t", model.controller.result.Applied, model.controller.result.Verified),
		)
		controllerReload := !model.controller.result.HasErrors() && model.controller.result.Applied && model.controller.result.Verified && model.reload.state != interfaceReloadNone
		if controllerReload {
			lines = append(lines, "", model.reload.line())
		}
		if model.controller.details && model.controller.progress.Operation != "" {
			lines = append(lines, "")
			lines = append(lines, model.operationProgressView(model.controller.progress, "Last controller apply")...)
		}
		if model.message != "" {
			notices = append(notices, failureNotice("The controller was not updated", model.message))
		}
		notices = append(notices, model.heldRecordNotices()...)
		detailsLabel := "Show details"
		if model.controller.details {
			detailsLabel = "Hide details"
		}
		returnLabel := "Maintenance"
		if model.setupMode {
			returnLabel = "Setup"
		}
		actions := []tuiAction{{key: "Enter", label: returnLabel}, {key: "d", label: detailsLabel}, {key: "l", label: "Logs"}, {key: "n", label: "New review"}, {key: "F1", label: "Help"}}
		if controllerReload && model.reload.counting() {
			actions = model.reloadActions()
		}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: actions})
	}
	lines = append(lines,
		"",
		"Review the current committed controller configuration before rebuilding.",
	)
	if model.message != "" {
		notices = append(notices, failureNotice("The controller cannot be updated now", model.message))
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "n", label: "Create review"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}})
}

func controllerPlanIssues(report domain.ControllerRebuildPlanReport) string {
	parts := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		parts = append(parts, issue.Field+": "+issue.Message)
	}
	return strings.Join(parts, "; ")
}
