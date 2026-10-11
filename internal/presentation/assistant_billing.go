package presentation

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/giovantenne/nixorium/internal/domain"
	"time"
)

type assistantBillingMsg struct {
	id  uint64
	url string
	err error
}

func (model dashboardModel) openAssistantBilling() (tea.Model, tea.Cmd) {
	actions := model.actions.AssistantService
	if actions == nil || actions.BillingPortal == nil || actions.ReadToken == nil {
		return model, nil
	}
	model.assistant.id++
	id := model.assistant.id
	model.assistant.state = "billing-loading"
	model.assistant.historyScroll = 0
	model.message = ""
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	model.assistant.cancel = cancel
	return model, func() tea.Msg {
		token, err := actions.ReadToken()
		link := ""
		if err == nil {
			link, err = actions.BillingPortal(ctx, token)
		}
		return assistantBillingMsg{id: id, url: link, err: err}
	}
}

func (model dashboardModel) updateAssistantBillingKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" || key.String() == "ctrl+c" {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.id++
		model.assistant.state = "chat"
		model.message = ""
		return model, model.assistant.input.Focus()
	}
	return model, nil
}

func (model dashboardModel) finishAssistantBilling(message assistantBillingMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardAssistant || model.assistant.id != message.id || model.assistant.state != "billing-loading" {
		return model, nil
	}
	if model.assistant.cancel != nil {
		model.assistant.cancel()
		model.assistant.cancel = nil
	}
	model.assistant.state = "chat"
	var serviceError *domain.AssistantError
	if errors.As(message.err, &serviceError) && serviceError.Code == "no_subscription" {
		model.message = "Available after subscribing."
	} else if message.err != nil {
		model.message = model.assistantNotice(message.err)
	} else {
		// Keep this temporary capability out of transcripts and model messages.
		model.message = message.url + "\nThis link works for a few minutes.\nOfficial electronic invoices are delivered through SDI."
	}
	return model, model.assistant.input.Focus()
}
