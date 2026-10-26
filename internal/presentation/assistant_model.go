package presentation

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// AssistantActions keeps service calls and private filesystem state in the
// command/application layers. Presentation owns only the interaction state.
type AssistantActions struct {
	BillingPortal       func(context.Context, string) (string, error)
	ListConversations   func(context.Context, string) ([]domain.AssistantConversationHistory, error)
	ConversationHistory func(context.Context, string) (domain.AssistantConversationHistory, error)
	ChangedFiles        func(string) (int, error)
	PlanSave            func(context.Context, string) domain.AssistantSavePlan
	Save                func(context.Context, domain.AssistantSavePlan) domain.TemplateResetResult
	ReadToken           func() (string, error)
	WriteToken          func(string) error
	DeleteToken         func() error
	ReadConsent         func() (bool, error)
	WriteConsent        func() error
	WriteConsentSharing func(bool) error
	ReadSharing         func() (bool, error)
	WriteSharing        func(bool) error
	Report              func(context.Context, domain.AssistantConversation, string, string) error
	Activate            func(context.Context, string) (domain.AssistantActivation, error)
	Account             func(context.Context, string) (domain.AssistantAccount, error)
	SetLabName          func(context.Context, string, string) (domain.AssistantAccount, error)
	Offer               func(context.Context) (domain.AssistantOffer, error)
	Cleanup             func(string) error
	PrepareConversation func(string) (domain.AssistantConversation, error)
	CarryOverProposal   func(from, to string) (int, error)
	Run                 func(context.Context, domain.AssistantConversation, string, func(domain.AssistantEvent)) (domain.AssistantTurnResult, error)
}

type assistantModel struct {
	resumeItems                  []domain.AssistantConversationHistory
	resumeCursor                 int
	resumeLoading                bool
	readOnlyHistory              domain.AssistantConversationHistory
	readOnlyCursor               int
	sharing                      bool
	saved                        bool
	subscriptionCheckShown       bool
	longChatShown                bool
	maxRequestBytes              int
	archivedEntries              []assistantEntry
	canReport                    bool
	reportRating                 int
	reportComment                textinput.Model
	labName                      textinput.Model
	initialized                  bool
	changedFiles                 int
	unfinished, turnAdmitted     bool
	review                       assistantReviewModel
	state                        string
	input                        textarea.Model
	code                         textinput.Model
	messages                     []domain.AssistantMessage
	streamed, summary, activity  string
	token                        string
	historyScroll                int
	id                           uint64
	accountTickID                uint64
	cancel                       context.CancelFunc
	events                       <-chan tea.Msg
	conversation                 domain.AssistantConversation
	account                      domain.AssistantAccount
	accessError                  string
	noticeDetail                 assistantNoticeDetailLine
	offer                        domain.AssistantOffer
	entries                      []assistantEntry
	started                      time.Time
	welcomeCursor, commandCursor int
	activationLink               string
	onboarding                   bool
	codeStep                     bool   // the activation code input is shown (no link, or the page was opened)
	activationKind               string // "free trial" or "subscription" for the instruction step
	activationNotice             string // muted note below the activation step
	pendingSend                  bool   // an unsent request waits for activation and consent
}

type assistantAccessMsg struct {
	id      uint64
	token   string
	consent bool
	sharing bool
	err     error
	account *domain.AssistantAccount
}
type assistantAccountMsg struct {
	id      uint64
	token   string
	consent *bool
	account *domain.AssistantAccount
	sharing *bool
	err     error
}
type assistantAccountTickMsg struct{ id uint64 }

const assistantAccountTickInterval = 15 * time.Second

type assistantActivationMsg struct {
	sharing bool
	id      uint64
	token   string
	consent bool
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
	input.Prompt = "> "
	input.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "> "
		}
		return "  "
	})
	input.Placeholder = "Describe what your lab should do"
	input.ShowLineNumbers = false
	input.CharLimit = domain.AssistantTextLimit
	input.DynamicHeight = true
	input.MinHeight = 1
	input.MaxHeight = 6
	// Keep the content bound independent of the six-row visible viewport.
	input.MaxContentHeight = domain.AssistantTextLimit
	input.SetHeight(1)
	input.SetWidth(68)
	input.Focus()
	code := textinput.New()
	code.Prompt = "› "
	code.Placeholder = "Activation code"
	code.CharLimit = 256
	code.EchoMode = textinput.EchoPassword
	code.SetWidth(60)
	comment := textinput.New()
	comment.Prompt = "› "
	comment.Placeholder = "Optional comment"
	comment.CharLimit = 500
	labName := textinput.New()
	labName.Prompt = "› "
	labName.Placeholder = "For example Lab 2 – Informatica"
	labName.CharLimit = assistantLabNameLimit
	return assistantModel{initialized: true, state: "chat", sharing: true, input: input, code: code, reportComment: comment, labName: labName}
}

func (model dashboardModel) openAssistant() (tea.Model, tea.Cmd) {
	if !model.actions.Assistant.Enabled || model.actions.ClassroomMode {
		return model, nil
	}
	if !model.assistant.initialized {
		model.assistant = newAssistantModel()
		model.assistant.offer = model.fallbackAssistantOffer()
	}
	model.screen = dashboardAssistant
	model.message = ""
	model.assistant.id++
	commands := []tea.Cmd{model.assistant.input.Focus()}
	if command := model.loadAssistantOffer(); command != nil {
		commands = append(commands, command)
	}
	if command := model.refreshAssistantAccount(); command != nil {
		commands = append(commands, command)
	}
	if actions := model.actions.AssistantService; actions != nil && actions.Cleanup != nil {
		current := model.assistant.conversation.ID
		commands = append(commands, func() tea.Msg { return assistantCleanupMsg{err: actions.Cleanup(current)} })
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
		if actions.WriteConsent == nil && actions.WriteConsentSharing == nil {
			result.err = errors.New("Assistant consent storage unavailable")
			return result
		}
		if actions.WriteConsentSharing != nil {
			result.err = actions.WriteConsentSharing(model.assistant.sharing)
		} else if !model.assistant.sharing {
			result.err = actions.WriteConsent()
		} else {
			result.err = errors.New("Conversation sharing storage unavailable")
		}
		if result.err != nil {
			return result
		}
	}
	if actions.ReadConsent != nil {
		result.consent, result.err = actions.ReadConsent()
	}
	if result.err == nil && actions.ReadSharing != nil {
		result.sharing, result.err = actions.ReadSharing()
	}
	if result.err == nil && !result.consent {
		// Saving is pre-selected On on the consent screen; the administrator can switch it off.
		result.sharing = true
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
	model.assistant.historyScroll = 0
	model.assistant.started = time.Now()
	model.assistant.activity = "Checking Assistant subscription"
	model.message = ""
	return model, func() tea.Msg { return model.assistantAccess(ctx, id, accept) }
}

func (model dashboardModel) finishAssistantAccess(message assistantAccessMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant {
		return model, nil
	}
	model.assistant.sharing = message.sharing
	if message.account != nil {
		model.assistant.account = *message.account
		model.assistant.accessError = ""
	}
	if message.err != nil {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.state = "chat"
		model.message = model.assistantNotice(message.err)
		model.assistant.noticeDetail = assistantNoticeDetail(model.message, message.err)
		var serviceError *domain.AssistantError
		if errors.As(message.err, &serviceError) {
			model.assistant.noteAccessError(serviceError.Code)
			switch serviceError.Code {
			case "consent_required":
				model.assistant.state = "consent"
				model.message = ""
			case "invalid_token":
				model.message = assistantActivationInvalidNotice
				if actions := model.actions.AssistantService; actions != nil && actions.DeleteToken != nil {
					if err := actions.DeleteToken(); err != nil {
						model.message = "Could not remove the invalid Assistant token. Apply the controller configuration and try again."
						return model, nil
					}
				}
				model.assistant.token = ""
				model.assistant.account = domain.AssistantAccount{}
				model.assistant.state = "chat"
				model.assistant.onboarding = false
				model.assistant.pendingSend = false
			}
		}
		return model, nil
	}
	if message.token == "" || !message.consent {
		if model.assistant.cancel != nil {
			model.assistant.cancel()
			model.assistant.cancel = nil
		}
		model.assistant.state = "welcome"
		model.assistant.onboarding = true
		model.assistant.pendingSend = message.token == "" && strings.TrimSpace(model.assistant.input.Value()) != ""
		if message.token != "" {
			model.assistant.onboarding = false
			model.assistant.token = message.token
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
		// Only an admitted turn can change the proposal; one refused before it
		// started leaves the previous state as it was.
		if model.assistant.turnAdmitted {
			model.assistant.unfinished = message.result == nil || message.result.Outcome != "final"
		}
		if message.result != nil {
			if model.assistant.summary == "" && message.result.Summary != "" {
				model.assistant.appendSummary(message.result.Summary)
			}
			model.assistant.changedFiles = message.changedFiles
			model.assistant.messages = message.result.Messages
			model.assistant.summary = message.result.Summary
			if data, err := json.Marshal(message.result.Messages); err == nil {
				model.assistant.noteTranscriptSize(len(data))
			}
		}
		if message.err != nil {
			model.assistant.canReport = false
			updated, _ := model.finishAssistantAccess(assistantAccessMsg{id: message.id, err: message.err})
			model = updated.(dashboardModel)
			return model, model.refreshAssistantAccount()
		}
		return model, model.refreshAssistantAccount()
	}
	event := message.event
	if model.assistant.token != "" {
		event.Delta = strings.ReplaceAll(event.Delta, model.assistant.token, "[REDACTED]")
		event.Summary = strings.ReplaceAll(event.Summary, model.assistant.token, "[REDACTED]")
		event.Text = strings.ReplaceAll(event.Text, model.assistant.token, "[REDACTED]")
		event.ToolResult = strings.ReplaceAll(event.ToolResult, model.assistant.token, "[REDACTED]")
	}
	switch event.Type {
	case "turn_started":
		model.assistant.turnAdmitted = true
		model.assistant.input.Reset()
	case "transcript_size":
		model.assistant.maxRequestBytes = event.MaxRequestBytes
		model.assistant.noteTranscriptSize(event.TranscriptBytes)
	case "text":
		model.assistant.streamed += event.Delta
		model.assistant.appendText(event.Delta)
	case "activity":
		model.assistant.activity = event.Text
		if event.Text == "Checking Assistant subscription" {
			if !model.assistant.subscriptionCheckShown {
				model.assistant.appendActivity(assistantActivityLine(event.Text))
				model.assistant.subscriptionCheckShown = true
			}
		} else if event.Text != "Preparing a proposal" {
			model.assistant.appendActivity(assistantActivityLine(event.Text))
		}
	case "tool_result":
		model.assistant.entries = append(model.assistant.entries, assistantEntry{role: "activity", text: assistantToolResultLine(event.ToolName, event.ToolResult, model.report.Meta.Controller.Name)})
	case "final":
		model.assistant.canReport = true
		model.assistant.summary = event.Summary
		model.assistant.appendFinal(event.Text, event.Summary)
		model.assistant.messages = append(model.assistant.messages, domain.AssistantMessage{Role: "assistant", Content: model.assistant.streamed})
	case "error":
		model.assistant.noteAccessError(event.Code)
		model.message = model.assistantNotice(&domain.AssistantError{Code: event.Code})
	}
	return model, model.waitAssistantStream()
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
	if message.consent {
		model.assistant.state = "chat"
	}
	model.assistant.sharing = message.sharing
	model.assistant.account = message.account
	model.assistant.accessError = ""
	model.assistant.token = message.token
	model.assistant.onboarding = false
	model.assistant.activationLink = ""
	model.message = ""
	if model.assistant.state == "chat" && model.assistant.pendingSend {
		model.assistant.pendingSend = false
		if strings.TrimSpace(model.assistant.input.Value()) != "" {
			updated, command := model.sendAssistant(false)
			return updated, command
		}
	}
	return model, model.assistant.input.Focus()
}

func (model dashboardModel) refreshAssistantAccount() tea.Cmd {
	actions := model.actions.AssistantService
	if actions == nil || actions.ReadToken == nil {
		return nil
	}
	id := model.assistant.id
	return func() tea.Msg {
		result := assistantAccountMsg{id: id}
		if actions.ReadSharing != nil {
			share, err := actions.ReadSharing()
			if err != nil {
				result.err = err
				return result
			}
			result.sharing = &share
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		token, err := actions.ReadToken()
		result.token = token
		result.err = err
		if err != nil || token == "" {
			return result
		}
		if actions.ReadConsent != nil {
			consent, err := actions.ReadConsent()
			result.err = err
			result.consent = &consent
			if err != nil {
				return result
			}
		}
		if actions.Account == nil {
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
	var serviceError *domain.AssistantError
	if errors.As(message.err, &serviceError) && serviceError.Code == "invalid_token" {
		return model.finishAssistantAccess(assistantAccessMsg{id: message.id, err: message.err})
	}
	if message.token == "" && message.account == nil && message.err == nil && (model.assistant.state == "chat" || model.assistant.state == "welcome") {
		model.assistant.token = ""
		model.assistant.account = domain.AssistantAccount{}
	}
	if message.token != "" {
		model.assistant.token = message.token
	}
	if message.account != nil {
		next := *message.account
		if next.Status == "active" && next.TurnsRemaining > 0 {
			previous := model.assistant.account
			held := model.message == model.assistantNotice(&domain.AssistantError{Code: "quota_exhausted"}) ||
				model.message == model.assistantNotice(&domain.AssistantError{Code: "subscription_inactive"})
			if previous.Plan == "trial" && next.Plan != "trial" {
				model.message = "Subscription active · " + strconv.Itoa(next.TurnsRemaining) + " requests per year."
			} else if held {
				model.message = ""
			}
		}
		if model.assistant.accessError != "" && message.account.Status == "active" && message.account.TurnsRemaining > 0 {
			model.message = ""
		}
		model.assistant.account = *message.account
		model.assistant.accessError = ""
	}
	if errors.As(message.err, &serviceError) {
		model.assistant.noteAccessError(serviceError.Code)
	}
	if model.assistant.state == "welcome" && message.token != "" {
		model.assistant.onboarding = false
		model.assistant.state = "chat"
		if message.consent != nil && !*message.consent {
			model.assistant.state = "consent"
		}
	}
	if message.sharing != nil && model.assistant.state == "chat" {
		model.assistant.sharing = *message.sharing
	}
	if message.account != nil && model.assistant.state == "chat" {
		if message.account.Status != "active" {
			model.message = model.assistantNotice(&domain.AssistantError{Code: "subscription_inactive"})
		} else if message.account.TurnsRemaining <= 0 {
			model.message = model.assistantNotice(&domain.AssistantError{Code: "quota_exhausted"})
		}
	}
	if message.err != nil && model.message == "" {
		model.message = model.assistantNotice(message.err)
		model.assistant.noticeDetail = assistantNoticeDetail(model.message, message.err)
	}
	cmd := model.scheduleAssistantAccountTick()
	return model, cmd
}

func (model *dashboardModel) scheduleAssistantAccountTick() tea.Cmd {
	account := model.assistant.account
	if model.screen != dashboardAssistant || model.assistant.state != "chat" || model.assistant.token == "" {
		return nil
	}
	if account.Status == "active" && account.TurnsRemaining > 0 {
		return nil
	}
	model.assistant.accountTickID++
	id := model.assistant.accountTickID
	return tea.Tick(assistantAccountTickInterval, func(time.Time) tea.Msg { return assistantAccountTickMsg{id: id} })
}

func (model dashboardModel) finishAssistantAccountTick(message assistantAccountTickMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.accountTickID || model.screen != dashboardAssistant || model.assistant.state == "running" {
		return model, nil
	}
	return model, model.refreshAssistantAccount()
}

func assistantDisplayText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
}

type assistantPreparedMsg struct {
	id           uint64
	conversation domain.AssistantConversation
	ctx          context.Context
	err          error
}

func (model dashboardModel) startAssistantRunner() (tea.Model, tea.Cmd) {
	if model.assistant.saved {
		previous := model.assistant.entries
		if len(previous) == 0 {
			for _, message := range model.assistant.messages {
				if message.Role == "user" || message.Role == "assistant" && message.Content != "" {
					previous = append(previous, assistantEntry{role: message.Role, text: message.Content})
				}
			}
		}
		if len(model.assistant.archivedEntries) > 0 {
			model.assistant.archivedEntries = append(model.assistant.archivedEntries, assistantEntry{role: "separator", text: "New chat"})
		}
		model.assistant.archivedEntries = append(model.assistant.archivedEntries, previous...)
		model.assistant.entries = nil
		model.assistant.messages = nil
		model.assistant.conversation = domain.AssistantConversation{}
		model.assistant.review = assistantReviewModel{}
		model.assistant.saved, model.assistant.longChatShown = false, false
		model.assistant.subscriptionCheckShown = false
		model.assistant.maxRequestBytes = 0
	}
	if model.assistant.cancel != nil {
		model.assistant.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	model.assistant.cancel = cancel
	id := model.assistant.id
	actions := model.actions.AssistantService
	conversationID := model.assistant.conversation.ID
	model.assistant.canReport = false
	model.assistant.turnAdmitted = false
	model.assistant.streamed, model.assistant.summary = "", ""
	model.assistant.entries = append(model.assistant.entries, assistantEntry{role: "user", text: model.assistant.input.Value()})
	model.assistant.historyScroll = 0
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
