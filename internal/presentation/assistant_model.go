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
	Version             string
	ChangedFiles        func(string) (int, error)
	PlanSave            func(context.Context, string) domain.AssistantSavePlan
	Save                func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult
	ReadToken           func() (string, error)
	WriteToken          func(string) error
	DeleteToken         func() error
	ReadConsent         func() (bool, error)
	WriteConsent        func() error
	Activate            func(context.Context, string) (domain.AssistantActivation, error)
	OpenTurn            func(context.Context, string) (domain.AssistantTurn, error)
	Step                func(context.Context, string, string, domain.AssistantStepRequest, func(domain.AssistantEvent)) error
	Cancel              func(context.Context, string, string) error
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
	token, turnID               string
	historyScroll               int
	id                          uint64
	cancel                      context.CancelFunc
	events                      <-chan tea.Msg
	conversation                domain.AssistantConversation
}

type assistantAccessMsg struct {
	id      uint64
	token   string
	consent bool
	turn    domain.AssistantTurn
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
	commands := []tea.Cmd{model.assistant.input.Focus()}
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
	if actions.Run != nil {
		return result
	}
	if actions.OpenTurn == nil {
		result.err = errors.New("Assistant service unavailable")
		return result
	}
	result.turn, result.err = actions.OpenTurn(ctx, result.token)
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
	if message.err != nil {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.state = "chat"
		model.message = assistantServiceNotice(message.err, model.actions.Assistant.SubscribeURL)
		var serviceError *domain.AssistantError
		if errors.As(message.err, &serviceError) {
			switch serviceError.Code {
			case "consent_required":
				model.assistant.state = "consent"
				model.message = ""
			case "subscription_inactive":
				model.assistant.state = "cta-inactive"
			case "invalid_token":
				if actions := model.actions.AssistantService; actions != nil && actions.DeleteToken != nil {
					if err := actions.DeleteToken(); err != nil {
						model.message = "Could not remove the invalid Assistant token. Apply the controller configuration and try again."
						return model, nil
					}
				}
				model.assistant.token = ""
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
	model.assistant.token, model.assistant.turnID = message.token, message.turn.TurnID
	if model.actions.AssistantService.Run != nil {
		return model.startAssistantRunner()
	}
	model.assistant.activity = "Preparing a proposal"
	model.assistant.streamed, model.assistant.summary = "", ""
	prompt := strings.ReplaceAll(model.assistant.input.Value(), message.token, "[REDACTED]")
	model.assistant.messages = append(model.assistant.messages, domain.AssistantMessage{Role: "user", Content: prompt})
	request := domain.AssistantStepRequest{Messages: append([]domain.AssistantMessage(nil), model.assistant.messages...)}
	actions := model.actions.AssistantService
	if len(model.assistant.messages) == 1 {
		request.Context = &domain.AssistantContext{Nixorium: actions.Version, Files: []string{}}
	}
	// Streaming has its own cancellation context; OpenTurn is already finished.
	if model.assistant.cancel != nil {
		model.assistant.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	model.assistant.cancel = cancel
	events := make(chan tea.Msg, 32)
	model.assistant.events = events
	id, token, turnID := model.assistant.id, message.token, message.turn.TurnID
	return model, func() tea.Msg {
		go func() {
			defer close(events)
			var err error
			if actions.Step == nil {
				err = errors.New("Assistant streaming unavailable")
			} else {
				err = actions.Step(ctx, token, turnID, request, func(event domain.AssistantEvent) {
					select {
					case events <- assistantStreamMsg{id: id, event: event}:
					case <-ctx.Done():
					}
				})
			}
			select {
			case events <- assistantStreamMsg{id: id, done: true, err: err}:
			case <-ctx.Done():
			}
		}()
		return <-events
	}
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
			if model.actions.AssistantService.Run != nil {
				return model.finishAssistantAccess(assistantAccessMsg{id: message.id, err: message.err})
			}
			model.message = assistantServiceNotice(message.err, model.actions.Assistant.SubscribeURL)
			return model, model.cancelAssistantRemote()
		}
		return model, nil
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
		model.message = assistantServiceNotice(&domain.AssistantError{Code: event.Code}, model.actions.Assistant.SubscribeURL)
	case "tool_calls":
		// Tool execution is added by C5, after the candidate workspace exists.
		model.assistant.state = "chat"
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.message = "This Assistant client cannot execute tools yet. Return to the chat after updating Nixorium."
		return model, model.cancelAssistantRemote()
	}
	return model, model.waitAssistantStream()
}

func (model dashboardModel) cancelAssistantRemote() tea.Cmd {
	actions := model.actions.AssistantService
	token, turnID := model.assistant.token, model.assistant.turnID
	if actions == nil || actions.Cancel == nil || token == "" || turnID == "" {
		return nil
	}
	if actions.Run != nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = actions.Cancel(ctx, token, turnID)
		return nil
	}
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
			return model, model.cancelAssistantRemote()
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
		model.assistant = newAssistantModel()
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
	model.message = fmt.Sprintf("Assistant is active for %s.", assistantDisplayText(message.account.Name))
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
	actions := []tuiAction{{key: "Ctrl+S", label: "Send"}, {key: "Enter", label: "New line"}, {key: "PgUp/PgDn", label: "History"}, {key: "Ctrl+N", label: "New chat"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	if model.assistant.changedFiles > 0 {
		actions = append([]tuiAction{{key: "Ctrl+R", label: "Review"}}, actions...)
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
