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
	if model.updates.packageBase {
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
	model.updates.packageBase, model.updates.baseEditing, model.updates.allowUnverified = true, false, false
	model.screen = dashboardUpdate
	model.updates.plan, model.updates.result = domain.UpdatePlanReport{}, domain.UpdateApplyReport{}
	model.controller.plan, model.controller.result = domain.ControllerRebuildPlanReport{}, domain.ControllerRebuildExecutionReport{}
	model.message, model.updates.baseTarget = "", ""
	model.updates.baseStatus = domain.PackageBaseStatus{}
	if model.actions.LoadPackageBase == nil || model.actions.PlanPackageBase == nil || model.actions.SavePackageBase == nil {
		model.updates.baseStatus.Issues = []domain.ValidationIssue{{Field: "capability", Message: "System updates are unavailable in this session"}}
		return model, nil
	}
	model.busy = "Reading the system and package pin"
	return model, func() tea.Msg { return dashboardPackageBaseMsg{report: model.actions.LoadPackageBase()} }
}

func (model dashboardModel) packageBaseView() string {
	lines := []string{tuiTitle(model.updateTitle(), model.isDark), "", "Channel: " + model.updates.baseStatus.Channel,
		"Revision: " + shortRevision(model.updates.baseStatus.Revision), "", "Updates can change kernel, desktop, services and applications.",
		"Nixorium and other input sources keep their existing revisions."}
	actions := []tuiAction{{key: "Enter", label: "Check and build"}, {key: "m", label: "Change channel"}, {key: "r", label: "Refresh"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	if model.updates.baseEditing {
		lines = append(lines, "", "Target channel: "+model.updates.baseTarget+"_", fmt.Sprintf("[%s] Accept unverified channel compatibility", map[bool]string{true: "x", false: " "}[model.updates.allowUnverified]), "A channel change requires this acknowledgement. Build failures still block.")
		actions = []tuiAction{{key: "Space", label: "Acknowledge"}, {key: "Enter", label: "Validate channel"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else {
		lines = append(lines, "", "Enter checks for a newer revision of the current channel.", "Review comes before saving and controller activation.")
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	if len(model.updates.baseStatus.Issues) > 0 {
		lines = append(lines, "", operationLogIssues(model.updates.baseStatus.Issues))
		actions = []tuiAction{{key: "r", label: "Refresh"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", model.updateTitle()}, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) updatePackageBaseKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		if model.updates.baseEditing {
			model.updates.baseEditing = false
			model.updates.baseTarget = model.updates.baseStatus.Channel
			model.updates.allowUnverified = false
		} else {
			model.screen = dashboardAdministration
		}
		return model, nil
	}
	if len(model.updates.baseStatus.Issues) > 0 {
		if key.String() == "r" {
			return model.openPackageBase()
		}
		return model, nil
	}
	if model.updates.baseEditing {
		switch key.String() {
		case "space":
			model.updates.allowUnverified = !model.updates.allowUnverified
		case "backspace":
			if len(model.updates.baseTarget) > 0 {
				model.updates.baseTarget = model.updates.baseTarget[:len(model.updates.baseTarget)-1]
			}
		case "enter":
			if model.updates.baseTarget != model.updates.baseStatus.Channel && !model.updates.allowUnverified {
				model.message = "Acknowledge unverified compatibility before validating a new channel."
				return model, nil
			}
			return model.startPackageBasePlan(model.updates.baseTarget, model.updates.allowUnverified)
		default:
			for _, c := range key.Text {
				if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' {
					if len(model.updates.baseTarget) < 32 {
						model.updates.baseTarget += string(c)
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
		model.updates.baseEditing = true
		model.updates.allowUnverified = false
		model.message = ""
	case "enter":
		return model.startPackageBasePlan("current", false)
	}
	return model, nil
}

func (model dashboardModel) startPackageBasePlan(target string, allow bool) (tea.Model, tea.Cmd) {
	model.updates.baseEditing = false
	model.updates.target, model.message = target, ""
	model.busy, model.updates.planning = "Validating system and package update", true
	model.updates.planProgress = domain.UpdatePlanProgress{}
	model.updates.planStarted = time.Now().UTC()
	events := make(chan tea.Msg)
	model.updates.planEvents = events
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
