package presentation

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantLabNameLimit = 60
const assistantNameHint = "Tip: type /name to give this laboratory a name."

type assistantNameMsg struct {
	id      uint64
	account domain.AssistantAccount
	err     error
}

func (model dashboardModel) openAssistantName() (tea.Model, tea.Cmd) {
	if model.assistant.token == "" {
		model.message = "Activate Assistant first. Type /code to enter an activation code."
		return model, nil
	}
	model.assistant.labName.SetValue(assistantDisplayText(model.assistant.account.LabName))
	model.assistant.labName.CursorEnd()
	model.assistant.state = "name"
	model.message = ""
	return model, model.assistant.labName.Focus()
}

func (model dashboardModel) updateAssistantNameKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+c":
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.id++
		model.assistant.state = "chat"
		model.message = ""
		return model, model.assistant.input.Focus()
	}
	if model.assistant.state == "name-saving" {
		return model, nil
	}
	if key.String() == "enter" {
		actions := model.actions.AssistantService
		if actions == nil || actions.SetLabName == nil {
			model.message = "Naming the laboratory is unavailable. Update Nixorium and try again."
			return model, nil
		}
		name, token, id := strings.TrimSpace(model.assistant.labName.Value()), model.assistant.token, model.assistant.id
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		model.assistant.cancel = cancel
		model.assistant.state = "name-saving"
		model.message = ""
		return model, func() tea.Msg {
			account, err := actions.SetLabName(ctx, token, name)
			return assistantNameMsg{id: id, account: account, err: err}
		}
	}
	var command tea.Cmd
	model.assistant.labName, command = model.assistant.labName.Update(key)
	return model, command
}

func (model dashboardModel) finishAssistantName(message assistantNameMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant || model.assistant.state != "name-saving" {
		return model, nil
	}
	if model.assistant.cancel != nil {
		model.assistant.cancel()
		model.assistant.cancel = nil
	}
	if message.err != nil {
		// Stay in the editor so the name can be corrected.
		model.assistant.state = "name"
		var serviceError *domain.AssistantError
		if errors.As(message.err, &serviceError) && (serviceError.Code == "invalid_request" || serviceError.Code == "rate_limited") && serviceError.Message != "" {
			model.message = assistantDisplayText(serviceError.Message)
		} else {
			model.message = model.assistantNotice(message.err)
		}
		return model, model.assistant.labName.Focus()
	}
	model.assistant.account.LabName = message.account.LabName
	model.assistant.state = "chat"
	model.message = "Laboratory name saved."
	return model, model.assistant.input.Focus()
}

func (model dashboardModel) assistantNameView() string {
	width := max(20, model.width-4)
	if model.width == 0 {
		width = 100
	}
	input := model.assistant.labName
	input.SetWidth(max(10, min(70, width-2)))
	body := "Shown on invoices, emails and in support, to tell this laboratory apart from the others of your school.\n\nLaboratory name (up to 60 characters)\n" + input.View()
	hint := "Enter save · Esc cancel · empty name removes it"
	if model.assistant.state == "name-saving" {
		body += "\n\nSaving…"
		hint = "Esc cancel"
	}
	if model.message != "" {
		body += "\n\n" + tuiError(assistantDisplayText(model.message), model.isDark)
	}
	body = lipgloss.NewStyle().Width(width).Render(body)
	return lipgloss.NewStyle().Padding(1, 2).Render(model.assistantHeader(width) + "\n\n" + tuiTitle("Laboratory name", model.isDark) + "\n\n" + body + "\n" + tuiMuted(hint, model.isDark))
}
