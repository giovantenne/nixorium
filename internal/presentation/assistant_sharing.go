package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const assistantSharingText = "Save conversations on the service"
const assistantReportText = "This sends the whole conversation to Nixorium, including your messages, configuration and history contents, diagnostics and check results Assistant read. Known secret patterns are filtered, but filtering may leave personal or sensitive text. It is kept for 90 days."

type assistantReportMsg struct {
	id  uint64
	err error
}

func (model dashboardModel) assistantSharingCheckbox() string {
	if model.assistant.sharing {
		return "[x] " + assistantSharingText + ": On"
	}
	return "[ ] " + assistantSharingText + ": Off"
}
func (model dashboardModel) openAssistantSharing() (tea.Model, tea.Cmd) {
	actions := model.actions.AssistantService
	if actions == nil || actions.ReadSharing == nil || actions.WriteSharing == nil {
		model.message = "Conversation saving is unavailable. Update Nixorium and try again."
		return model, nil
	}
	share, err := actions.ReadSharing()
	if err != nil {
		model.message = assistantDisplayText(err.Error())
		return model, nil
	}
	model.assistant.sharing = share
	model.assistant.state = "sharing"
	model.message = ""
	return model, nil
}
func (model dashboardModel) openAssistantReport() (tea.Model, tea.Cmd) {
	if !model.assistant.canReport || model.assistant.conversation.ID == "" {
		return model, nil
	}
	model.assistant.reportRating = 0
	model.assistant.reportComment.Reset()
	model.assistant.state = "report"
	model.message = ""
	return model, model.assistant.reportComment.Focus()
}
func (model dashboardModel) updateAssistantSharingKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		if model.assistant.state == "report-sending" {
			if model.assistant.cancel != nil {
				model.assistant.cancel()
				model.assistant.cancel = nil
			}
			model.assistant.id++
		}
		model.assistant.state = "chat"
		model.message = ""
		return model, model.assistant.input.Focus()
	}
	if model.assistant.state == "report-sending" {
		return model, nil
	}
	if model.assistant.state == "sharing" {
		if key.String() == "enter" {
			share := !model.assistant.sharing
			if err := model.actions.AssistantService.WriteSharing(share); err != nil {
				model.message = assistantDisplayText(err.Error())
				return model, nil
			}
			model.assistant.id++
			model.assistant.sharing = share
			model.assistant.state = "chat"
			return model, model.assistant.input.Focus()
		}
		return model, nil
	}
	switch key.String() {
	case "up":
		model.assistant.reportRating = 0
		return model, nil
	case "down":
		model.assistant.reportRating = 1
		return model, nil
	case "enter":
		actions := model.actions.AssistantService
		if actions == nil || actions.Report == nil {
			model.message = "Reporting is unavailable. Update Nixorium and try again."
			return model, nil
		}
		id, conversation, comment := model.assistant.id, model.assistant.conversation, model.assistant.reportComment.Value()
		rating := "helpful"
		if model.assistant.reportRating == 1 {
			rating = "wrong"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		model.assistant.cancel = cancel
		model.assistant.state = "report-sending"
		return model, func() tea.Msg { return assistantReportMsg{id, actions.Report(ctx, conversation, rating, comment)} }
	}
	var command tea.Cmd
	model.assistant.reportComment, command = model.assistant.reportComment.Update(key)
	return model, command
}
func (model dashboardModel) finishAssistantReport(message assistantReportMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant || model.assistant.state != "report-sending" {
		return model, nil
	}
	if model.assistant.cancel != nil {
		model.assistant.cancel()
		model.assistant.cancel = nil
	}
	model.assistant.state = "chat"
	model.message = "Report sent. Thank you."
	if message.err != nil {
		notice := message.err.Error()
		if model.assistant.token != "" {
			notice = strings.ReplaceAll(notice, model.assistant.token, "[REDACTED]")
		}
		model.message = assistantDisplayText(notice)
	}
	return model, model.assistant.input.Focus()
}
func (model dashboardModel) assistantSharingView() string {
	title, body := "Assistant settings", "Saved on the Nixorium Assistant service for 90 days so support can review them. Applies to all future conversations of this lab.\n\n"+model.assistantSharingCheckbox()
	actions := []tuiAction{{key: "Enter", label: "Switch on/off"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	if model.assistant.state != "sharing" {
		title = "Report this answer"
		rating := "› Helpful\n  Wrong"
		if model.assistant.reportRating == 1 {
			rating = "  Helpful\n› Wrong"
		}
		comment := model.assistant.reportComment
		comment.SetWidth(max(20, min(90, model.width-12)))
		body = "Sends only this conversation, once.\n\n" + assistantReportText + "\n\n" + rating + "\n\nComment (optional, max 500 characters)\n" + comment.View()
		actions = []tuiAction{{key: "↑/↓", label: "Rating"}, {key: "Enter", label: "Send"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		if model.assistant.state == "report-sending" {
			body += "\n\nSending report…"
			actions = []tuiAction{{key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Assistant"}, body: tuiTitle(title, model.isDark) + "\n\n" + body, notices: notices, actions: actions})
}
