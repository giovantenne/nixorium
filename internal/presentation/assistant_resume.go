package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantReadOnlyText = "This conversation is based on an older configuration. You can read it, or continue in a new chat."

type assistantResumeMsg struct {
	id           uint64
	items        []domain.AssistantConversationHistory
	history      *domain.AssistantConversationHistory
	conversation domain.AssistantConversation
	prefill      string
	err          error
}

func (model dashboardModel) openAssistantResume() (tea.Model, tea.Cmd) {
	actions := model.actions.AssistantService
	if actions == nil || actions.ListConversations == nil {
		return model, nil
	}
	model.assistant.state = "resume"
	model.assistant.resumeCursor = 0
	model.assistant.resumeLoading = true
	model.assistant.id++
	id, current := model.assistant.id, model.assistant.conversation.ID
	model.message = ""
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	model.assistant.cancel = cancel
	return model, func() tea.Msg {
		var err error
		if actions.Cleanup != nil {
			err = actions.Cleanup(current)
		}
		var items []domain.AssistantConversationHistory
		if err == nil {
			items, err = actions.ListConversations(ctx, current)
		}
		return assistantResumeMsg{id: id, items: items, err: err}
	}
}

func (model dashboardModel) finishAssistantResume(msg assistantResumeMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardAssistant || msg.id != model.assistant.id || !model.assistant.resumeLoading {
		return model, nil
	}
	model.assistant.resumeLoading = false
	if model.assistant.cancel != nil {
		model.assistant.cancel()
		model.assistant.cancel = nil
	}
	if msg.err != nil {
		model.message = model.assistantNotice(msg.err)
		return model, nil
	}
	if msg.conversation.ID != "" {
		updated, command := model.newAssistantChat()
		model = updated.(dashboardModel)
		model.assistant.conversation = msg.conversation
		model.assistant.input.SetValue(msg.prefill)
		model.assistant.input.CursorEnd()
		return model, command
	}
	if msg.history == nil {
		model.assistant.resumeItems = msg.items
		return model, nil
	}
	h := *msg.history
	if h.State == "saved" || h.State == "outdated" {
		model.assistant.readOnlyHistory = h
		model.assistant.readOnlyCursor = 0
		model.assistant.historyScroll = 0
		model.assistant.state = "history"
		return model, nil
	}
	updated, command := model.newAssistantChat()
	model = updated.(dashboardModel)
	model.assistant.conversation = h.Conversation
	model.assistant.messages = h.Messages
	model.assistant.changedFiles = h.ChangedFiles
	if h.Conversation.LastAnswer != nil {
		model.assistant.summary = *h.Conversation.LastAnswer
	}
	model.assistant.canReport = model.assistant.summary != ""
	return model, command
}

func assistantResumePrefill(c domain.AssistantConversation) string {
	// Titles are user text: retain literal quotation marks as required by the command.
	text := "Continuing a previous chat: \"" + c.Title + "\"."
	if c.LastAnswer != nil && *c.LastAnswer != "" {
		text += " Last answer: " + domain.AssistantHistoryText(*c.LastAnswer, 600)
	}
	return text + "\n\n"
}

func (model dashboardModel) updateAssistantResumeKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.id++
		model.assistant.resumeLoading = false
		if model.assistant.state == "history" {
			model.assistant.state = "resume"
		} else {
			model.assistant.state = "chat"
		}
		model.assistant.historyScroll = 0
		model.message = ""
		return model, nil
	}
	if model.assistant.resumeLoading {
		return model, nil
	}
	if model.assistant.state == "history" {
		switch key.String() {
		case "pgup":
			return model.scrollAssistantHistory(4), nil
		case "pgdown":
			return model.scrollAssistantHistory(-4), nil
		case "up", "down", "tab":
			model.assistant.readOnlyCursor = 1 - model.assistant.readOnlyCursor
		case "enter":
			if model.assistant.readOnlyCursor == 1 {
				model.assistant.state = "resume"
				model.assistant.historyScroll = 0
				return model, nil
			}
			actions := model.actions.AssistantService
			if actions == nil || actions.PrepareConversation == nil {
				return model, nil
			}
			prefill := assistantResumePrefill(model.assistant.readOnlyHistory.Conversation)
			model.assistant.id++
			id := model.assistant.id
			model.assistant.resumeLoading = true
			return model, func() tea.Msg {
				conversation, err := actions.PrepareConversation("")
				return assistantResumeMsg{id: id, conversation: conversation, prefill: prefill, err: err}
			}
		}
		return model, nil
	}
	count := len(model.assistant.resumeItems)
	if count == 0 {
		return model, nil
	}
	switch key.String() {
	case "up", "k":
		model.assistant.resumeCursor = (model.assistant.resumeCursor + count - 1) % count
	case "down", "j":
		model.assistant.resumeCursor = (model.assistant.resumeCursor + 1) % count
	case "enter":
		actions := model.actions.AssistantService
		if actions == nil || actions.ConversationHistory == nil {
			return model, nil
		}
		conversationID := model.assistant.resumeItems[model.assistant.resumeCursor].Conversation.ID
		model.assistant.id++
		id := model.assistant.id
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		model.assistant.cancel = cancel
		model.assistant.resumeLoading = true
		return model, func() tea.Msg {
			h, err := actions.ConversationHistory(ctx, conversationID)
			return assistantResumeMsg{id: id, history: &h, err: err}
		}
	}
	return model, nil
}

func assistantHistoryDate(stamp string, now time.Time) string {
	date, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return ""
	}
	date = date.In(now.Location())
	if date.Format("2006-01-02") == now.Format("2006-01-02") {
		return "Today " + date.Format("15:04")
	}
	if date.Format("2006-01-02") == now.AddDate(0, 0, -1).Format("2006-01-02") {
		return "Yesterday"
	}
	return date.Format("2006-01-02")
}

func (model dashboardModel) assistantResumeView() string {
	width := max(30, min(100, model.width-4))
	lines := []string{}
	if model.assistant.resumeLoading {
		lines = append(lines, model.activitySpinner.View()+" Opening conversation…")
	} else {
		capacity := max(1, model.height-8)
		start := max(0, model.assistant.resumeCursor-capacity+1)
		for i := start; i < min(len(model.assistant.resumeItems), start+capacity); i++ {
			item := model.assistant.resumeItems[i]
			state := tuiMuted(item.State, model.isDark)
			titleWidth := max(1, width-19-lipgloss.Width(state))
			title := ansi.Truncate(assistantDisplayText(item.Conversation.Title), titleWidth, "…")
			row := fmt.Sprintf("%-15s %s", assistantHistoryDate(item.Conversation.LastActivityAt, time.Now()), title)
			row += strings.Repeat(" ", max(1, width-lipgloss.Width(row)-lipgloss.Width(state)-2)) + state
			lines = append(lines, tuiSelection(row, i == model.assistant.resumeCursor, model.isDark))
		}
		if len(model.assistant.resumeItems) == 0 {
			lines = append(lines, tuiMuted("No previous conversations.", model.isDark))
		}
	}
	if model.message != "" {
		lines = append(lines, tuiMuted(model.message, model.isDark))
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(tuiSection("Assistant / Resume", model.isDark) + "\n\n" + strings.Join(lines, "\n") + "\n\n" + tuiMuted("Enter open · Esc back", model.isDark))
}
