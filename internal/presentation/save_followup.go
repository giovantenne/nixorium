package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// These rows describe this workflow's evidence, not live fleet state. In
// particular, an unchanged declaration is not proof of system activation.
func saveStatusLines(state string, recovery, controllerAffected, clientsAffected, verified bool) []string {
	configuration := "! Configuration — Save not confirmed"
	controller := "○ This controller — Unknown; not checked here"
	clients := "○ Client computers — Unknown until checked; none updated here"
	if state == "saved" && !recovery {
		configuration = "✓ Configuration — Saved locally"
		controller = "! This controller — Needs applying"
		if !controllerAffected {
			controller = "○ This controller — Not affected by this change"
		} else if verified {
			controller = "✓ This controller — Up to date at the saved revision"
		}
		if !clientsAffected {
			clients = "○ Client computers — Not affected by this change"
		} else {
			clients = "○ Client computers — Not updated here; unknown until checked"
		}
	} else if recovery {
		configuration = "! Configuration — Save needs recovery"
	} else if state == "unchanged" {
		configuration = "○ Configuration — Unchanged; already saved"
	}
	return []string{configuration, controller, clients}
}

func controllerVerifiedForSave(revision string, result domain.ControllerRebuildExecutionReport) bool {
	return revision != "" && result.Revision == revision && result.Operation != "" && !result.HasErrors() && result.Applied && result.Verified
}

func workspaceRecordNeedsInspection(result domain.WorkspaceApplyReport) bool {
	return result.RecoveryRequired || result.State == "conflict" || (result.State == "saved" && !result.Recorded)
}

func (model dashboardModel) saveFollowupState() (saved bool, revision string) {
	switch model.screen {
	case dashboardSettings:
		r := model.settings.result
		return !r.HasErrors() && !r.RecoveryRequired && (r.State == "saved" || r.State == "unchanged"), r.Revision
	case dashboardTemplateReset:
		r := model.templateReset.result
		return r.State == "saved" && !r.RecoveryRequired, r.Revision
	case dashboardWorkspace:
		r := model.workspace.result
		return !r.HasErrors() && !r.RecoveryRequired && r.State == "saved" && r.Recorded, r.Revision
	}
	return false, ""
}

func (model dashboardModel) saveFollowupActions(back string) []tuiAction {
	actions := []tuiAction{}
	if saved, revision := model.saveFollowupState(); saved {
		if controllerVerifiedForSave(revision, model.controller.result) {
			if model.actions.LoadInventory != nil && model.actions.PlanDeployment != nil {
				actions = append(actions, tuiAction{key: "Enter", label: "Update computers"})
			}
		} else if model.actions.PlanController != nil && model.actions.ApplyController != nil {
			actions = append(actions, tuiAction{key: "Enter", label: "Apply to this controller"})
		}
	}
	return append(actions, tuiAction{key: "Esc", label: back}, tuiAction{key: "F1", label: "Help"})
}

func (model dashboardModel) continueSavedConfiguration() (tea.Model, tea.Cmd) {
	saved, revision := model.saveFollowupState()
	if !saved {
		return model, nil
	}
	if controllerVerifiedForSave(revision, model.controller.result) {
		return model.openSavedDeployment()
	}
	if model.actions.PlanController == nil || model.actions.ApplyController == nil {
		model.message = "Controller application is unavailable in this session; the configuration remains saved."
		return model, nil
	}
	if reason := model.managedJobConflict(); reason != "" {
		model.message = reason
		return model, nil
	}
	model.controller.fromSave, model.controller.saveOrigin = true, model.screen
	model.installation.flow = false
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
	model.confirmation, model.message = "", ""
	model.busy = "Reviewing the saved configuration for this controller"
	// Keep the result visible and return there on cancellation. This read never
	// chains an apply: the ordinary controller review still requires Enter.
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardControllerPlanMsg{report: model.actions.PlanController(ctx)}
	})
}

func (model dashboardModel) openSavedDeployment() (tea.Model, tea.Cmd) {
	if model.actions.LoadInventory == nil || model.actions.PlanDeployment == nil {
		model.message = "Computer deployment is unavailable in this session."
		return model, nil
	}
	return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) {
		if len(ready.report.Meta.Clients.Hosts) == 0 {
			ready.message = "No client computers are configured."
			return ready, nil
		}
		ready.screen = dashboardDeploy
		ready.deployment = deploymentModel{chosen: map[string]bool{}, context: "Saved configuration: select computers, review again and confirm. Student-home and keyboard changes may require the next computer start."}
		ready.confirmation, ready.message, ready.pageScroll = "", "", 0
		return ready, nil
	})
}
