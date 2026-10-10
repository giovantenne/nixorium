package presentation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// AssistantActions keeps service calls and private filesystem state in the
// command/application layers. Presentation owns only the interaction state.
type AssistantActions struct {
	ChangedFiles        func(string) (int, error)
	PlanSave            func(context.Context, string) domain.AssistantSavePlan
	Save                func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult
	ReadToken           func() (string, error)
	WriteToken          func(string) error
	DeleteToken         func() error
	ReadConsent         func() (bool, error)
	WriteConsent        func() error
	Activate            func(context.Context, string) (domain.AssistantActivation, error)
	Account             func(context.Context, string) (domain.AssistantAccount, error)
	Cleanup             func() error
	PrepareConversation func(string) (domain.AssistantConversation, error)
	Run                 func(context.Context, domain.AssistantConversation, string, func(domain.AssistantEvent)) (domain.AssistantTurnResult, error)
}

type assistantModel struct {
	initialized                 bool
	changedFiles                int
	review                      assistantReviewModel
	state                       string
	input                       textarea.Model
	code                        textinput.Model
	messages                    []domain.AssistantMessage
	streamed, summary, activity string
	token                       string
	historyScroll               int
	id                          uint64
	cancel                      context.CancelFunc
	events                      <-chan tea.Msg
	conversation                domain.AssistantConversation
	account                     domain.AssistantAccount
}

type assistantAccessMsg struct {
	id      uint64
	token   string
	consent bool
	err     error
	account *domain.AssistantAccount
}
type assistantAccountMsg struct {
	id      uint64
	account *domain.AssistantAccount
	err     error
}
type assistantActivationMsg struct {
	id      uint64
	account domain.AssistantAccount
	err     error
}
type assistantStreamMsg struct {
	id           uint64
	event        domain.AssistantEvent
	done         bool
	err          error
	result       *domain.AssistantTurnResult
	changedFiles int
	conversation domain.AssistantConversation
}

type assistantCleanupMsg struct{ err error }

func newAssistantModel() assistantModel {
	input := textarea.New()
	input.Prompt = "› "
	input.Placeholder = "Describe what you want to prepare, for example: Python with Jupyter on pc01–pc10"
	input.ShowLineNumbers = false
	input.CharLimit = domain.AssistantTextLimit
	input.MaxHeight = 0
	input.SetHeight(3)
	input.SetWidth(68)
	input.Focus()
	code := textinput.New()
	code.Prompt = "› "
	code.Placeholder = "Activation code"
	code.CharLimit = 256
	code.EchoMode = textinput.EchoPassword
	code.SetWidth(60)
	return assistantModel{initialized: true, state: "chat", input: input, code: code}
}

func (model dashboardModel) openAssistant() (tea.Model, tea.Cmd) {
	if !model.actions.Assistant.Enabled || model.actions.ClassroomMode {
		return model, nil
	}
	if !model.assistant.initialized {
		model.assistant = newAssistantModel()
	}
	model.screen = dashboardAssistant
	model.message = ""
	model.assistant.id++
	commands := []tea.Cmd{model.assistant.input.Focus()}
	if command := model.refreshAssistantAccount(); command != nil {
		commands = append(commands, command)
	}
	if actions := model.actions.AssistantService; actions != nil && actions.Cleanup != nil {
		commands = append(commands, func() tea.Msg { return assistantCleanupMsg{err: actions.Cleanup()} })
	}
	return model, tea.Batch(commands...)
}

func (model dashboardModel) assistantAccess(ctx context.Context, id uint64, accept bool) tea.Msg {
	result := assistantAccessMsg{id: id}
	actions := model.actions.AssistantService
	if actions == nil || actions.ReadToken == nil {
		return result
	}
	result.token, result.err = actions.ReadToken()
	if result.err != nil || result.token == "" {
		return result
	}
	if actions.Account != nil {
		account, err := actions.Account(ctx, result.token)
		result.err = err
		if err != nil {
			return result
		}
		result.account = &account
		if account.Status != "active" {
			result.err = &domain.AssistantError{Code: "subscription_inactive", Status: 402}
			return result
		}
		if account.TurnsRemaining <= 0 {
			result.err = &domain.AssistantError{Code: "quota_exhausted", Status: 429}
			return result
		}
	}
	if accept {
		if actions.WriteConsent == nil {
			result.err = errors.New("Assistant consent storage unavailable")
			return result
		}
		result.err = actions.WriteConsent()
		if result.err != nil {
			return result
		}
	}
	if actions.ReadConsent != nil {
		result.consent, result.err = actions.ReadConsent()
	}
	if result.err != nil || !result.consent {
		return result
	}
	if actions.Run == nil {
		result.err = errors.New("Assistant service unavailable")
	}
	return result
}

func (model dashboardModel) sendAssistant(accept bool) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(model.assistant.input.Value()) == "" {
		return model, nil
	}
	model.assistant.id++
	id := model.assistant.id
	ctx, cancel := context.WithCancel(context.Background())
	model.assistant.cancel = cancel
	model.assistant.state = "running"
	model.assistant.activity = "Checking Assistant subscription"
	model.message = ""
	return model, func() tea.Msg { return model.assistantAccess(ctx, id, accept) }
}

func (model dashboardModel) finishAssistantAccess(message assistantAccessMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant {
		return model, nil
	}
	if message.account != nil {
		model.assistant.account = *message.account
	}
	if message.err != nil {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.state = "chat"
		model.message = model.assistantNotice(message.err)
		var serviceError *domain.AssistantError
		if errors.As(message.err, &serviceError) {
			switch serviceError.Code {
			case "consent_required":
				model.assistant.state = "consent"
				model.message = ""
			case "subscription_inactive":
				model.assistant.state = "cta-inactive"
				model.message = ""
			case "quota_exhausted":
				if model.assistant.account.Plan == "trial" && model.assistant.account.Status != "" {
					model.assistant.state = "cta-inactive"
					if model.assistant.account.UpgradeURL != "" {
						model.message = ""
					}
				}
			case "invalid_token":
				if actions := model.actions.AssistantService; actions != nil && actions.DeleteToken != nil {
					if err := actions.DeleteToken(); err != nil {
						model.message = "Could not remove the invalid Assistant token. Apply the controller configuration and try again."
						return model, nil
					}
				}
				model.assistant.token = ""
				model.assistant.account = domain.AssistantAccount{}
				model.assistant.state = "cta-none"
			}
		}
		return model, nil
	}
	if message.token == "" || !message.consent {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.state = "cta-none"
		if message.token != "" {
			model.assistant.state = "consent"
		}
		return model, nil
	}
	model.assistant.token = message.token
	return model.startAssistantRunner()
}

func (model dashboardModel) waitAssistantStream() tea.Cmd {
	events := model.assistant.events
	return func() tea.Msg { return <-events }
}

func (model dashboardModel) finishAssistantStream(message assistantStreamMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant {
		return model, nil
	}
	if message.done {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.state = "chat"
		if message.conversation.ID != "" {
			model.assistant.conversation = message.conversation
		}
		if message.result != nil {
			model.assistant.changedFiles = message.changedFiles
			model.assistant.messages = message.result.Messages
			model.assistant.summary = message.result.Summary
		}
		if message.err != nil {
			updated, _ := model.finishAssistantAccess(assistantAccessMsg{id: message.id, err: message.err})
			model = updated.(dashboardModel)
			return model, model.refreshAssistantAccount()
		}
		if model.assistant.changedFiles > 0 {
			model.message = "Proposal ready: " + countNoun(model.assistant.changedFiles, "file") + " changed. Press Ctrl+R to review."
		}
		return model, model.refreshAssistantAccount()
	}
	event := message.event
	event.Delta = strings.ReplaceAll(event.Delta, model.assistant.token, "[REDACTED]")
	event.Summary = strings.ReplaceAll(event.Summary, model.assistant.token, "[REDACTED]")
	event.Text = strings.ReplaceAll(event.Text, model.assistant.token, "[REDACTED]")
	switch event.Type {
	case "text":
		model.assistant.streamed += event.Delta
	case "activity":
		model.assistant.activity = event.Text
	case "final":
		model.assistant.summary = event.Summary
		model.assistant.messages = append(model.assistant.messages, domain.AssistantMessage{Role: "assistant", Content: model.assistant.streamed})
	case "error":
		model.message = model.assistantNotice(&domain.AssistantError{Code: event.Code})
	}
	return model, model.waitAssistantStream()
}

func (model dashboardModel) updateAssistantKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.assistant.state {
	case "running":
		if key.String() == "esc" || key.String() == "ctrl+c" {
			if model.assistant.cancel != nil {
				model.assistant.cancel()
				model.assistant.cancel = nil
			}
			model.assistant.id++
			model.assistant.state = "chat"
			model.message = "Stopped. Your deployment is unchanged."
			return model, nil
		}
		return model, nil
	case "cta-none", "cta-inactive":
		switch key.String() {
		case "esc":
			model.assistant.state = "chat"
			model.message = ""
			return model, model.assistant.input.Focus()
		case "a":
			model.assistant.state = "activate"
			model.message = ""
			model.assistant.code.Reset()
			return model, model.assistant.code.Focus()
		}
		return model, nil
	case "consent":
		switch key.String() {
		case "esc":
			model.assistant.state = "chat"
			model.message = ""
			return model, model.assistant.input.Focus()
		case "enter":
			return model.sendAssistant(true)
		}
		return model, nil
	case "activate":
		switch key.String() {
		case "esc":
			model.assistant.state = "chat"
			model.assistant.code.Reset()
			model.message = ""
			return model, model.assistant.input.Focus()
		case "enter":
			if strings.TrimSpace(model.assistant.code.Value()) == "" {
				return model, nil
			}
			actions := model.actions.AssistantService
			if actions == nil || actions.Activate == nil || actions.WriteToken == nil {
				model.message = assistantNetworkNotice
				return model, nil
			}
			code := model.assistant.code.Value()
			model.assistant.code.Reset()
			model.assistant.id++
			id := model.assistant.id
			ctx, cancel := context.WithCancel(context.Background())
			model.assistant.cancel = cancel
			model.assistant.state = "running"
			model.assistant.activity = "Activating Assistant"
			return model, func() tea.Msg {
				activation, err := actions.Activate(ctx, code)
				if err == nil {
					err = ctx.Err()
				}
				if err == nil {
					err = actions.WriteToken(activation.Token)
				}
				return assistantActivationMsg{id: id, account: activation.Account, err: err}
			}
		}
		var command tea.Cmd
		model.assistant.code, command = model.assistant.code.Update(key)
		return model, command
	}
	switch key.String() {
	case "esc":
		model.screen = dashboardHome
		return model, nil
	case "ctrl+r":
		return model.openAssistantReview()
	case "ctrl+s":
		return model.sendAssistant(false)
	case "ctrl+n":
		id := model.assistant.id + 1
		account := model.assistant.account
		model.assistant = newAssistantModel()
		model.assistant.account = account
		model.assistant.id = id
		model.message = ""
		return model, model.assistant.input.Focus()
	case "pgup":
		model.assistant.historyScroll += 4
		return model, nil
	case "pgdown":
		model.assistant.historyScroll = max(0, model.assistant.historyScroll-4)
		return model, nil
	}
	var command tea.Cmd
	model.assistant.input, command = model.assistant.input.Update(key)
	return model, command
}

func (model dashboardModel) finishAssistantActivation(message assistantActivationMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant {
		return model, nil
	}
	if model.assistant.cancel != nil {
		model.assistant.cancel()
		model.assistant.cancel = nil
	}
	if message.err != nil {
		model.assistant.state = "activate"
		model.message = assistantServiceNotice(message.err, model.actions.Assistant.SubscribeURL)
		var serviceError *domain.AssistantError
		if errors.As(message.err, &serviceError) && serviceError.Code == "invalid_code" {
			model.message = "This code is not valid or was already used."
		}
		return model, model.assistant.code.Focus()
	}
	model.assistant.state = "consent"
	model.assistant.account = message.account
	model.message = fmt.Sprintf("Assistant is active for %s.", assistantDisplayText(message.account.Name))
	return model, model.refreshAssistantAccount()
}

func (model dashboardModel) refreshAssistantAccount() tea.Cmd {
	actions := model.actions.AssistantService
	if actions == nil || actions.Account == nil || actions.ReadToken == nil {
		return nil
	}
	id := model.assistant.id
	return func() tea.Msg {
		result := assistantAccountMsg{id: id}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		token, err := actions.ReadToken()
		result.err = err
		if err != nil || token == "" {
			return result
		}
		account, err := actions.Account(ctx, token)
		result.err = err
		if err == nil {
			result.account = &account
		}
		return result
	}
}

func (model dashboardModel) finishAssistantAccount(message assistantAccountMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant || model.assistant.state == "running" {
		return model, nil
	}
	if message.account != nil {
		model.assistant.account = *message.account
	}
	if message.err != nil && model.message == "" {
		model.message = model.assistantNotice(message.err)
	}
	return model, nil
}

func assistantDisplayText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
}

func (model dashboardModel) assistantView() string {
	if model.assistant.state == "cta-none" || model.assistant.state == "cta-inactive" || model.assistant.state == "consent" || model.assistant.state == "activate" {
		return model.assistantGateView()
	}
	input := model.assistant.input
	input.SetWidth(max(20, min(106, model.width-10)))
	input.SetHeight(3)
	input.SetStyles(textarea.DefaultStyles(model.isDark))
	history := []string{}
	for _, message := range model.assistant.messages {
		if message.Role == "tool" {
			continue
		}
		history = append(history, message.Role+": "+assistantDisplayText(message.Content))
	}
	if model.assistant.streamed != "" && model.assistant.state == "running" {
		history = append(history, assistantDisplayText(model.assistant.streamed))
	}
	if model.assistant.summary != "" {
		history = append(history, assistantDisplayText(model.assistant.summary))
	}
	wrappedHistory := lipgloss.NewStyle().Width(max(20, min(106, model.width-10))).Render(strings.Join(history, "\n"))
	lines := strings.Split(wrappedHistory, "\n")
	end := max(0, len(lines)-model.assistant.historyScroll)
	start := max(0, end-max(1, model.height-19))
	body := strings.Join(lines[start:end], "\n")
	fixedBody := assistantDisclosure + "\n\n" + input.View()
	if model.assistant.account.Plan == "trial" && model.assistant.account.Status != "" {
		fixedBody = fmt.Sprintf("Free trial: %d of 5 requests left\n\n", model.assistant.account.TurnsRemaining) + fixedBody
	}
	actions := []tuiAction{{key: "Ctrl+S", label: "Send"}, {key: "Enter", label: "New line"}, {key: "PgUp/PgDn", label: "History"}, {key: "Ctrl+N", label: "New chat"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	if model.assistant.changedFiles > 0 {
		actions = append([]tuiAction{{key: "Ctrl+R", label: "Review proposal"}}, actions...)
	}
	if model.assistant.state == "running" {
		fixedBody = model.activitySpinner.View() + " " + assistantDisplayText(model.assistant.activity) + "\n\n" + fixedBody
		actions = []tuiAction{{key: "Esc", label: "Stop"}, {key: "F1", label: "Help"}}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Assistant"}, body: body, fixedBody: fixedBody, notices: notices, actions: actions})
}

type assistantPreparedMsg struct {
	id           uint64
	conversation domain.AssistantConversation
	ctx          context.Context
	err          error
}

func (model dashboardModel) startAssistantRunner() (tea.Model, tea.Cmd) {
	if model.assistant.cancel != nil {
		model.assistant.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	model.assistant.cancel = cancel
	id := model.assistant.id
	actions := model.actions.AssistantService
	conversationID := model.assistant.conversation.ID
	model.assistant.streamed, model.assistant.summary = "", ""
	model.assistant.activity = "Preparing a private configuration workspace"
	return model, func() tea.Msg {
		if actions.PrepareConversation == nil {
			return assistantPreparedMsg{id: id, ctx: ctx, err: errors.New("Assistant workspace unavailable")}
		}
		conversation, err := actions.PrepareConversation(conversationID)
		if err == nil {
			err = ctx.Err()
		}
		return assistantPreparedMsg{id: id, conversation: conversation, ctx: ctx, err: err}
	}
}
func (model dashboardModel) finishAssistantPrepared(message assistantPreparedMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant {
		return model, nil
	}
	if message.err != nil {
		return model.finishAssistantAccess(assistantAccessMsg{id: message.id, err: message.err})
	}
	model.assistant.conversation = message.conversation
	events := make(chan tea.Msg, 32)
	model.assistant.events = events
	actions := model.actions.AssistantService
	prompt := model.assistant.input.Value()
	ctx := message.ctx
	id := message.id
	return model, func() tea.Msg {
		go func() {
			defer close(events)
			result, err := actions.Run(ctx, message.conversation, prompt, func(event domain.AssistantEvent) {
				select {
				case events <- assistantStreamMsg{id: id, event: event}:
				case <-ctx.Done():
				}
			})
			changedFiles := 0
			if actions.ChangedFiles != nil {
				changedFiles, _ = actions.ChangedFiles(message.conversation.ID)
			}
			select {
			case events <- assistantStreamMsg{id: id, done: true, result: &result, changedFiles: changedFiles, conversation: message.conversation, err: err}:
			case <-ctx.Done():
			}
		}()
		return <-events
	}
}
