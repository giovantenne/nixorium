package presentation

import (
	"errors"
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantNetworkNotice = "The Assistant service could not be reached. Check the controller's Internet connection and try again."
const assistantProviderBusyNotice = "The AI provider is busy right now. Your request was not completed; try again in a minute."
const assistantFailedNotice = "The Assistant could not complete this request. Try again in a minute."
const assistantConsentText = "Your messages, configuration files and saved configuration history read by\nAssistant are sent to the Nixorium Assistant service and its AI provider.\nAssistant may also read system state and limited recent logs from declared\ncomputers. Known secret patterns and common private identifiers are filtered;\nfiltering is best effort and may leave personal or sensitive text.\nStudent home files are not read. The service does not keep configuration files.\nIt keeps messages and answers for 30 days to detect abuse, then deletes them.\nWhole conversations, including files and diagnostics read, are kept for\n90 days only if you report an answer or turn on sharing below.\nEach message you send uses one request."

func assistantServiceNotice(err error, subscribeURL string) string {
	if err != nil {
		switch err.Error() {
		case "Commit or discard the deployment changes before using Assistant.", "The deployment changed since this chat started. Press Ctrl+N to start a new chat.":
			return err.Error()
		}
	}
	var serviceError *domain.AssistantError
	if errors.As(err, &serviceError) {
		switch serviceError.Code {
		case "turn_active":
			return "Another Assistant request is still running for this laboratory. Wait a few minutes and try again."
		case "quota_exhausted":
			return "All Assistant requests included in your plan for this period are used. Contact hello@nixorium.org"
		case "client_upgrade_required":
			return "Update Nixorium to keep using Assistant."
		case "request_too_large":
			return "This chat became too long. Press Ctrl+N to start a new chat."
		case "step_limit":
			return "This Assistant request reached its limit. Start a new request."
		case "protocol_error":
			return "The Assistant service returned an invalid response. Try again after updating Nixorium."
		case "model_unavailable":
			return assistantProviderBusyNotice
		default:
			// The service answered with a code: the controller was online.
			return assistantFailedNotice
		}
	}
	return assistantNetworkNotice
}

const assistantActivationInvalidNotice = "This controller's Assistant activation is no longer valid. Activate it again to send your request."

func (model dashboardModel) assistantNotice(err error) string {
	var serviceError *domain.AssistantError
	if errors.As(err, &serviceError) {
		account := model.assistant.account
		switch serviceError.Code {
		case "subscription_inactive", "quota_exhausted":
			if account.Plan == "trial" {
				return "Your free trial has ended. Subscribe to continue:\n" + account.UpgradeURL + "\nThis controller stays activated. Type /code to enter a new activation code."
			}
			if serviceError.Code == "subscription_inactive" {
				return "Your subscription is not active. Renew at:\n" + model.actions.Assistant.SubscribeURL + "\nOr type /code to enter a new activation code."
			}
			return "All requests for this year are used. They renew on " + assistantDisplayText(account.PeriodEnd) + ". Questions: hello@nixorium.org"
		}
	}
	return assistantServiceNotice(err, model.actions.Assistant.SubscribeURL)
}

func (model dashboardModel) assistantGateView() string {
	width := max(20, model.width-4)
	if model.width == 0 {
		width = 100
	}
	offer := model.assistant.offer
	if offer.TrialRequests == 0 {
		offer = model.fallbackAssistantOffer()
	}
	title, body, hint := "Welcome to Assistant", "", "↑↓ choose · Enter confirm · Esc back"
	switch model.assistant.state {
	case "welcome":
		body = "Activate Assistant to send your request.\n\n"
		subscribe := "Subscribe"
		if offer.PriceText != "" {
			subscribe += " · " + offer.PriceText
		}
		for i, choice := range []string{fmt.Sprintf("Start a free trial · %d requests, no payment", offer.TrialRequests), "I have an activation code", subscribe} {
			body += tuiSelection(choice, i == model.assistant.welcomeCursor, model.isDark) + "\n"
		}
	case "consent":
		title = "Before you send"
		body = assistantConsentText + "\n\n" + model.assistantSharingCheckbox()
		hint = "Space toggle sharing · Enter continue · Esc back"
	case "activate":
		title = "Activate Assistant"
		code := model.assistant.code
		code.SetWidth(width - 2)
		link := assistantDisplayText(model.assistant.activationLink)
		if !model.assistant.codeStep {
			if model.actions.OpenURL != nil {
				body = "Press Enter to open the " + model.assistant.activationKind + " page in your browser.\n\n"
			} else {
				body = "Open the " + model.assistant.activationKind + " page in your browser, then press Enter to enter the code.\n\n"
			}
			body += assistantLinkify(link)
			hint = "Enter continue · Esc back"
		} else {
			if link != "" {
				body = assistantLinkify(link) + "\n\n"
			}
			body += "Activation code\n" + code.View() + "\n" + tuiMuted("Paste the code you receive by email.", model.isDark)
			if model.assistant.activationNotice != "" {
				body += "\n" + tuiMuted(model.assistant.activationNotice, model.isDark)
			}
			hint = "Enter activate · Esc back"
		}
	}
	if model.message != "" {
		body += "\n" + tuiError(assistantDisplayText(model.message), model.isDark)
	}
	body = lipgloss.NewStyle().Width(width).Render(assistantLinkify(body))
	header := model.assistantHeader(width) + "\n\n" + tuiTitle(title, model.isDark) + "\n\n"
	capacity := max(1, model.height-2-lipgloss.Height(header)-2)
	body = lipgloss.NewStyle().MaxHeight(capacity).Render(body)
	return lipgloss.NewStyle().Padding(1, 2).Render(header + body + "\n" + tuiMuted(hint, model.isDark))
}
