package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleWorkspaceMessage handles the workspace message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleWorkspaceMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case templateResetCatalogMsg:
		return model.finishTemplateResetCatalog(message)
	case templateResetPlanMsg:
		return model.finishTemplateResetPlan(message)
	case templateResetProgressMsg:
		if model.screen == dashboardTemplateReset && message.id == model.templateReset.requestID && model.busy != "" {
			if strings.TrimSpace(message.detail) != "" {
				model.busy = message.detail
			}
			return model, waitForActivityEvent(model.read.ctx, model.read.id, model.templateReset.events)
		}
		return model, nil
	case templateResetResultMsg:
		return model.finishTemplateResetResult(message)
	case telemetryMsg:
		return model.finishTelemetry(message)
	case dashboardSupportPreviewMsg:
		return model.finishSupportPreview(message)
	case dashboardSupportExportMsg:
		return model.finishSupportExport(message)
	case dashboardWorkspacePlanMsg:
		return model.finishWorkspacePlan(message)
	case homeFilesReadMsg:
		return model.finishHomeFilesRead(message)
	case homeFilesSaveMsg:
		return model.finishHomeFilesSave(message)
	case dashboardWorkspaceSaveMsg:
		return model.finishWorkspaceSave(message)
	case dashboardWorkspaceSearchMsg:
		return model.finishWorkspaceSearch(message)
	case dashboardWorkspaceMarketplaceMsg:
		return model.finishWorkspaceMarketplace(message)
	case dashboardClassroomViewMsg:
		model.busy = ""
		if message.err != nil {
			model.message = message.err.Error()
		} else {
			model.message = message.text
		}
		return model, nil
	default:
		return model.handleDeploymentMessage(message)
	}
}
