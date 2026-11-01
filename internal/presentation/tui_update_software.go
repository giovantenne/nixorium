package presentation

import (
	tea "charm.land/bubbletea/v2"
)

// handleSoftwareMessage handles the software message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleSoftwareMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardSoftwareCatalogMsg:
		model.busy = ""
		software, result := model.software.loadCatalog(message.report)
		model.software = software
		model.message = result.message
		model.screen = dashboardSoftware
		return model, nil
	case dashboardSoftwareSearchStartMsg:
		if model.screen != dashboardSoftware {
			return model, nil
		}
		software, result, command := model.software.startSearch(message, model.actions.SearchSoftware)
		if !result.accepted {
			return model, nil
		}
		model.software = software
		return model, command
	case dashboardSoftwareSearchMsg:
		if model.screen != dashboardSoftware {
			return model, nil
		}
		software, result := model.software.finishSearch(message)
		if !result.accepted {
			return model, nil
		}
		model.software = software
		model.message = result.message
		return model, nil
	case dashboardSoftwarePlanMsg:
		model.busy = ""
		software, result := model.software.finishPlan(message.report)
		model.software = software
		model.message = result.message
		if model.software.reviewing() {
			model.confirmation = ""
		}
		return model, nil
	case dashboardSoftwareApplyMsg:
		if !message.report.HasErrors() && message.report.State == "saved" && message.report.AffectedController != "" {
			model.noteControllerSave(message.report.Revision)
		}
		model.busy = ""
		software, result := model.software.finishApply(message.report)
		model.software = software
		model.message = result.message
		if result.startController {
			return model.startSoftwareControllerApply()
		}
		return model, nil
	case dashboardSoftwarePresetCatalogMsg:
		model.busy = ""
		software, result := model.software.loadProfiles(message.report)
		model.software = software
		model.message = result.message
		return model, nil
	case dashboardSoftwarePresetPlanMsg:
		model.busy = ""
		software, result := model.software.finishProfilePlan(message.report)
		model.software = software
		model.message = result.message
		return model, nil
	case dashboardSoftwarePresetApplyMsg:
		if !message.report.HasErrors() && message.report.State == "saved" && message.report.AffectedController != "" {
			model.noteControllerSave(message.report.Revision)
		}
		model.busy = ""
		software, result := model.software.finishProfileApply(message.report)
		model.software = software
		model.message = result.message
		if result.startController {
			return model.startSoftwareControllerApply()
		}
		return model, nil
	case dashboardSoftwareControllerMsg:
		model.busy = ""
		model.controller.applying = false
		model.controller.plan = message.plan
		model.controller.result = message.report
		model.observeControllerPlan(message.plan)
		model.observeControllerResult(message.report)
		software, result := model.software.finishController(message.plan, message.report)
		model.software = software
		model.message = result.message
		return model, nil
	default:
		return model.handleOperationsMessage(message)
	}
}
