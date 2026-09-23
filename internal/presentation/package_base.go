package presentation

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardPackageBaseMsg struct{ report domain.PackageBaseStatus }

func (model dashboardModel) updateTitle() string {
	if model.baseUpdate {
		return "Update system and packages"
	}
	return "Update Nixorium"
}

func (model dashboardModel) saveReviewedUpdate(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
	if plan.Kind == "package-base" {
		return model.actions.SavePackageBase(plan)
	}
	return model.actions.SaveUpdate(plan)
}

func (model dashboardModel) openPackageBase() (tea.Model, tea.Cmd) {
	model.baseUpdate, model.baseEditing, model.baseAllowUnverified = true, false, false
	model.screen = dashboardUpdate
	model.updatePlan, model.updateResult = domain.UpdatePlanReport{}, domain.UpdateApplyReport{}
	model.controllerPlan, model.controllerResult = domain.ControllerRebuildPlanReport{}, domain.ControllerRebuildExecutionReport{}
	model.message, model.baseTarget = "", ""
	model.baseStatus = domain.PackageBaseStatus{}
	if model.actions.LoadPackageBase == nil || model.actions.PlanPackageBase == nil || model.actions.SavePackageBase == nil {
		model.baseStatus.Issues = []domain.ValidationIssue{{Field: "capability", Message: "System updates are unavailable in this session"}}
		return model, nil
	}
	model.busy = "Reading the system and package pin"
	return model, func() tea.Msg { return dashboardPackageBaseMsg{report: model.actions.LoadPackageBase()} }
}

func (model dashboardModel) packageBaseView() string {
	lines := []string{tuiTitle(model.updateTitle(), model.isDark), "", "Channel: " + model.baseStatus.Channel,
		"Revision: " + shortRevision(model.baseStatus.Revision), "", "Updates can change kernel, desktop, services and applications.",
		"Nixorium and other input sources keep their existing revisions."}
	actions := []tuiAction{{key: "Enter", label: "Check and build"}, {key: "m", label: "Change channel"}, {key: "r", label: "Refresh"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	if model.baseEditing {
		lines = append(lines, "", "Target channel: "+model.baseTarget+"_", fmt.Sprintf("[%s] Accept unverified channel compatibility", map[bool]string{true: "x", false: " "}[model.baseAllowUnverified]), "A channel change requires this acknowledgement. Build failures still block.")
		actions = []tuiAction{{key: "Space", label: "Acknowledge"}, {key: "Enter", label: "Validate channel"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else {
		lines = append(lines, "", "Enter checks for a newer revision of the current channel.", "Review comes before saving and controller activation.")
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	if len(model.baseStatus.Issues) > 0 {
		lines = append(lines, "", operationLogIssues(model.baseStatus.Issues))
		actions = []tuiAction{{key: "r", label: "Refresh"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", model.updateTitle()}, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) updatePackageBaseKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		if model.baseEditing {
			model.baseEditing = false
			model.baseTarget = model.baseStatus.Channel
			model.baseAllowUnverified = false
		} else {
			model.screen = dashboardAdministration
		}
		return model, nil
	}
	if len(model.baseStatus.Issues) > 0 {
		if key.String() == "r" {
			return model.openPackageBase()
		}
		return model, nil
	}
	if model.baseEditing {
		switch key.String() {
		case "space":
			model.baseAllowUnverified = !model.baseAllowUnverified
		case "backspace":
			if len(model.baseTarget) > 0 {
				model.baseTarget = model.baseTarget[:len(model.baseTarget)-1]
			}
		case "enter":
			if model.baseTarget != model.baseStatus.Channel && !model.baseAllowUnverified {
				model.message = "Acknowledge unverified compatibility before validating a new channel."
				return model, nil
			}
			return model.startPackageBasePlan(model.baseTarget, model.baseAllowUnverified)
		default:
			for _, c := range key.Text {
				if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' {
					if len(model.baseTarget) < 32 {
						model.baseTarget += string(c)
					}
				}
			}
		}
		return model, nil
	}
	switch key.String() {
	case "r":
		return model.openPackageBase()
	case "m":
		model.baseEditing = true
		model.baseAllowUnverified = false
		model.message = ""
	case "enter":
		return model.startPackageBasePlan("current", false)
	}
	return model, nil
}

func (model dashboardModel) startPackageBasePlan(target string, allow bool) (tea.Model, tea.Cmd) {
	model.baseEditing = false
	model.updateTarget, model.message = target, ""
	model.busy, model.updatePlanning = "Validating system and package update", true
	model.updatePlanProgress = domain.UpdatePlanProgress{}
	model.updatePlanStarted = time.Now().UTC()
	events := make(chan tea.Msg)
	model.updatePlanEvents = events
	action := func(target string, acknowledge, _ bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
		return model.actions.PlanPackageBase(target, acknowledge, progress)
	}
	return model, startUpdatePlan(action, target, allow, false, events)
}
func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
