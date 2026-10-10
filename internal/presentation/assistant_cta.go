package presentation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantDisclosure = "When you press Ctrl+S, your message and the files the assistant reads are sent to the Nixorium Assistant service. Do not enter passwords or student data."
const assistantNetworkNotice = "The Assistant service could not be reached. Check the controller's Internet connection and try again."
const assistantConsentText = "Your messages, and the configuration files the assistant reads, are sent to\nthe Nixorium Assistant service and to its AI provider to prepare a proposal.\nThe service does not keep their content. Passwords and private keys in your\ndeployment are never sent. Logs and student files are never read."

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
		}
	}
	return assistantNetworkNotice
}

func (model dashboardModel) assistantNotice(err error) string {
	var serviceError *domain.AssistantError
	if errors.As(err, &serviceError) && serviceError.Code == "quota_exhausted" && model.assistant.account.Plan == "trial" && model.assistant.account.UpgradeURL != "" {
		return "All Assistant requests included in your plan for this period are used. Subscribe at " + model.assistant.account.UpgradeURL
	}
	return assistantServiceNotice(err, model.actions.Assistant.SubscribeURL)
}

func (model dashboardModel) assistantGateView() string {
	title, body := "Assistant needs an active subscription", "Your request was not sent. Subscribe at "+model.actions.Assistant.SubscribeURL+", then press a and\nenter the activation code shown after payment."
	actions := []tuiAction{{key: "a", label: "Enter activation code"}, {key: "Esc", label: "Back to the message"}, {key: "F1", label: "Help"}}
	switch model.assistant.state {
	case "cta-inactive":
		title = "Your Assistant subscription is not active"
		body = "Your request was not sent. Renew at " + model.actions.Assistant.SubscribeURL + ", or press a to enter a\nnew activation code."
		if model.assistant.account.Plan == "trial" && model.assistant.account.UpgradeURL != "" {
			title = "Your free trial has ended"
			body = "Your request was not sent. Subscribe at " + model.assistant.account.UpgradeURL + ".\nYour controller stays activated."
			actions = []tuiAction{{key: "Esc", label: "Back to the message"}, {key: "F1", label: "Help"}}
		}
	case "consent":
		title, body = "Before you send", assistantConsentText
		actions = []tuiAction{{key: "Enter", label: "Accept and send"}, {key: "Esc", label: "Back"}}
	case "activate":
		title = "Activate Assistant"
		code := model.assistant.code
		code.SetWidth(max(20, min(80, model.width-12)))
		body = "Activation code\n" + code.View()
		actions = []tuiAction{{key: "Enter", label: "Activate"}, {key: "Esc", label: "Cancel"}}
	}
	if model.assistant.account.Plan == "trial" && model.assistant.account.Status != "" {
		body = fmt.Sprintf("Free trial: %d of 5 requests left\n\n", model.assistant.account.TurnsRemaining) + body
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: strings.TrimSpace(model.message)})
	}
	return model.renderShell(tuiShell{path: []string{"Assistant"}, body: tuiTitle(title, model.isDark) + "\n\n" + body, notices: notices, actions: actions})
}
