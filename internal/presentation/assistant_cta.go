package presentation

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantDisclosure = "When you press Enter, your message and the files the assistant reads are sent to the Nixorium Assistant service. Do not enter passwords or student data."
const assistantNetworkNotice = "The Assistant service could not be reached. Check the controller's Internet connection and try again."
const assistantProviderBusyNotice = "The AI provider is busy right now. Your request was not completed; try again in a minute."
const assistantFailedNotice = "The Assistant could not complete this request. Try again in a minute."
const assistantConsentText = "Your messages, and the configuration files the assistant reads, are sent to\nthe Nixorium Assistant service and to its AI provider to prepare a proposal.\nThe service keeps their content only if you report an answer or turn on\nsharing below. Passwords and private keys in your deployment are never sent.\nLogs and student files are never read.\nNixorium keeps the text of your messages and Assistant's answers for 30 days to detect abuse, then deletes it.\nEach message you send uses one request."

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
		examples := []string{"  e.g. " + assistantExamples[0]}
		for _, example := range assistantExamples[1:] {
			examples = append(examples, "       "+example)
		}
		body = assistantWelcomeText + "\nEach message you send uses one request.\n\n" + tuiMuted(strings.Join(examples, "\n"), model.isDark) + "\n\n"
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
		if model.assistant.activationLink != "" {
			link := assistantDisplayText(model.assistant.activationLink)
			body = assistantLinkify(link) + "\nRegister with your email, then enter the code you receive.\n\n"
		}
		body += "Activation code\n" + code.View()
		hint = "Enter activate · Esc back"
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
