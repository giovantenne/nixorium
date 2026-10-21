package presentation

import (
	tea "charm.land/bubbletea/v2"
)

// handleAssistantMessage handles the assistant message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleAssistantMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case assistantBillingMsg:
		return model.finishAssistantBilling(message)
	case assistantResumeMsg:
		return model.finishAssistantResume(message)
	case tea.MouseWheelMsg:
		if model.screen == dashboardAssistant && !model.helpOpen && (model.assistant.state == "chat" || model.assistant.state == "running" || model.assistant.state == "history") {
			switch message.Button {
			case tea.MouseWheelUp:
				return model.scrollAssistantHistory(4), nil
			case tea.MouseWheelDown:
				return model.scrollAssistantHistory(-4), nil
			}
		}
		return model, nil
	case assistantOfferMsg:
		return model.finishAssistantOffer(message)
	case assistantConsentMsg:
		return model.finishAssistantConsent(message)
	case assistantSavePlanMsg:
		model.busy = ""
		model.assistant.review.plan = message.plan
		return model, nil
	case assistantSaveResultMsg:
		return model.finishAssistantSave(message)
	case assistantNameMsg:
		return model.finishAssistantName(message)
	case assistantReportMsg:
		return model.finishAssistantReport(message)
	case assistantCleanupMsg:
		if message.err != nil && model.screen == dashboardAssistant {
			model.message = "Could not clean expired Assistant chats. Apply the controller configuration and try again."
		}
		return model, nil
	case assistantPreparedMsg:
		return model.finishAssistantPrepared(message)
	case assistantAccessMsg:
		return model.finishAssistantAccess(message)
	case assistantAccountMsg:
		return model.finishAssistantAccount(message)
	case assistantAccountTickMsg:
		return model.finishAssistantAccountTick(message)
	case assistantActivationMsg:
		return model.finishAssistantActivation(message)
	case assistantStreamMsg:
		return model.finishAssistantStream(message)
	default:
		return model.handleUpdateMessage(message)
	}
}
