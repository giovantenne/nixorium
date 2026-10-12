package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

var assistantExamples = []string{
	"Install GIMP and Inkscape on all student PCs",
	"Block the Internet on pc01–pc06 every Tuesday 10–12",
	"pc03 still shows the old wallpaper: what is wrong?",
}
var assistantCommands = []string{"/review", "/save", "/new", "/resume", "/billing", "/report", "/share", "/help", "/name", "/code"}

type assistantOfferMsg struct {
	id    uint64
	offer domain.AssistantOffer
	err   error
}
type assistantConsentMsg struct {
	id  uint64
	err error
}

func (model dashboardModel) fallbackAssistantOffer() domain.AssistantOffer {
	return domain.AssistantOffer{TrialRequests: 10, SubscribeURL: model.actions.Assistant.SubscribeURL,
		TrialURL: strings.TrimSuffix(strings.TrimRight(model.actions.Assistant.Endpoint, "/"), "/v1") + "/trial"}
}
func (model dashboardModel) loadAssistantOffer() tea.Cmd {
	actions := model.actions.AssistantService
	if actions == nil || actions.Offer == nil {
		return nil
	}
	id := model.assistant.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		offer, err := actions.Offer(ctx)
		return assistantOfferMsg{id, offer, err}
	}
}
func (model dashboardModel) finishAssistantOffer(message assistantOfferMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardAssistant || message.id != model.assistant.id {
		return model, nil
	}
	model.assistant.offer = model.fallbackAssistantOffer()
	if message.err == nil {
		model.assistant.offer = message.offer
	}
	return model, nil
}
func (model dashboardModel) acceptAssistantConsent() (tea.Model, tea.Cmd) {
	actions := model.actions.AssistantService
	id, sharing := model.assistant.id, model.assistant.sharing
	return model, func() tea.Msg {
		var err error
		if actions == nil {
			err = &domain.AssistantError{Code: "service_unavailable"}
		} else if actions.WriteConsentSharing != nil {
			err = actions.WriteConsentSharing(sharing)
		} else if actions.WriteConsent != nil && !sharing {
			err = actions.WriteConsent()
		} else {
			err = &domain.AssistantError{Code: "service_unavailable"}
		}
		return assistantConsentMsg{id, err}
	}
}
func (model dashboardModel) finishAssistantConsent(message assistantConsentMsg) (tea.Model, tea.Cmd) {
	if message.id != model.assistant.id || model.screen != dashboardAssistant || model.assistant.state != "consent" {
		return model, nil
	}
	if message.err != nil {
		model.message = model.assistantNotice(message.err)
		return model, nil
	}
	model.message = ""
	model.assistant.state = "chat"
	if model.assistant.pendingSend {
		model.assistant.pendingSend = false
		if strings.TrimSpace(model.assistant.input.Value()) != "" {
			updated, command := model.sendAssistant(false)
			return updated, command
		}
	}
	return model, model.assistant.input.Focus()
}

// commandAvailable says whether a command applies to the chat right now and,
// when it does not, why. Unknown commands are reported as available so the
// caller keeps its own "Unknown command" text.
func (model assistantModel) commandAvailable(command string) (bool, string) {
	switch command {
	case "/review", "/save":
		if model.changedFiles <= 0 {
			return false, "No proposal to review yet. Ask Assistant for a change first."
		}
	case "/report":
		if !model.canReport {
			return false, "/report becomes available after Assistant answers."
		}
	case "/new":
		if len(model.entries) == 0 && len(model.messages) == 0 {
			return false, "This chat is already empty."
		}
	case "/billing":
		if model.token == "" || model.account.Plan == "" || model.account.Plan == "trial" {
			return false, "/billing becomes available after subscribing."
		}
	}
	return true, ""
}

func (model dashboardModel) assistantCommandAvailable(command string) (bool, string) {
	return model.assistant.commandAvailable(command)
}

func (model assistantModel) commandChoices() []string {
	draft := model.input.Value()
	if !strings.HasPrefix(draft, "/") || strings.ContainsAny(draft, " \n\t") {
		return nil
	}
	choices := []string{}
	for _, command := range assistantCommands {
		if ok, _ := model.commandAvailable(command); ok && strings.HasPrefix(command, draft) {
			choices = append(choices, command)
		}
	}
	return choices
}

// openAssistantGate keeps the unsent request and offers the ways to activate.
func (model dashboardModel) openAssistantGate() dashboardModel {
	model.assistant.state = "welcome"
	model.assistant.onboarding = true
	model.assistant.pendingSend = true
	model.assistant.welcomeCursor = 0
	model.message = ""
	return model
}
func (model dashboardModel) newAssistantChat() (tea.Model, tea.Cmd) {
	old := model.assistant
	model.assistant = newAssistantModel()
	model.assistant.id = old.id + 1
	model.assistant.account = old.account
	model.assistant.accessError = old.accessError
	model.assistant.token = old.token
	model.assistant.offer = old.offer
	model.assistant.sharing = old.sharing
	model.message = ""
	return model, model.assistant.input.Focus()
}
func (model dashboardModel) assistantCommand(command string) (tea.Model, tea.Cmd) {
	if ok, why := model.assistantCommandAvailable(command); !ok {
		model.assistant.input.Reset()
		model.message = why
		return model, nil
	}
	switch command {
	case "/review", "/save":
		model.assistant.input.Reset()
		return model.openAssistantReview()
	case "/new":
		return model.newAssistantChat()
	case "/resume":
		model.assistant.input.Reset()
		return model.openAssistantResume()
	case "/billing":
		model.assistant.input.Reset()
		return model.openAssistantBilling()
	case "/report":
		model.assistant.input.Reset()
		return model.openAssistantReport()
	case "/share":
		model.assistant.input.Reset()
		return model.openAssistantSharing()
	case "/help":
		model.assistant.input.Reset()
		model.helpOpen = true
		model.pageScroll = 0
		return model, nil
	case "/name":
		model.assistant.input.Reset()
		return model.openAssistantName()
	case "/code":
		model.assistant.input.Reset()
		model.assistant.state = "activate"
		model.assistant.onboarding = false
		model.assistant.pendingSend = false
		model.assistant.activationLink = ""
		model.assistant.activationNotice = ""
		model.assistant.codeStep = true
		model.assistant.code.Reset()
		model.message = ""
		return model, model.assistant.code.Focus()
	}
	model.message = "Unknown command. Type / to see the commands."
	return model, nil
}
func (model dashboardModel) updateAssistantKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.assistant.state == "billing-loading" {
		return model.updateAssistantBillingKey(key)
	}
	if model.assistant.state == "resume" || model.assistant.state == "history" {
		return model.updateAssistantResumeKey(key)
	}
	if model.assistant.state == "chat" || model.assistant.state == "running" {
		switch key.String() {
		case "pgup":
			return model.scrollAssistantHistory(4), nil
		case "pgdown":
			return model.scrollAssistantHistory(-4), nil
		case "end":
			model.assistant.historyScroll = 0
			return model, nil
		case "up", "down":
			if model.assistant.input.Value() == "" {
				delta := 1
				if key.String() == "down" {
					delta = -1
				}
				return model.scrollAssistantHistory(delta), nil
			}
		}
	}
	switch model.assistant.state {
	case "running":
		switch key.String() {
		case "esc", "ctrl+c":
			if model.assistant.cancel != nil {
				model.assistant.cancel()
				model.assistant.cancel = nil
			}
			model.assistant.id++
			model.assistant.state = "chat"
			model.assistant.pendingSend = false
			model.message = "Stopped. Your deployment is unchanged."
		}
		return model, nil
	case "welcome":
		switch key.String() {
		case "esc":
			model.assistant.state = "chat"
			model.assistant.onboarding = false
			model.assistant.pendingSend = false
			model.message = ""
			return model, model.assistant.input.Focus()
		case "up":
			model.assistant.welcomeCursor = (model.assistant.welcomeCursor + 2) % 3
		case "down":
			model.assistant.welcomeCursor = (model.assistant.welcomeCursor + 1) % 3
		case "enter":
			model.assistant.state = "activate"
			model.message = ""
			model.assistant.code.Reset()
			model.assistant.activationLink = ""
			model.assistant.activationKind = ""
			model.assistant.activationNotice = ""
			if model.assistant.welcomeCursor == 0 {
				model.assistant.activationLink = model.assistant.offer.TrialURL
				model.assistant.activationKind = "free trial"
			}
			if model.assistant.welcomeCursor == 2 {
				model.assistant.activationLink = model.assistant.offer.SubscribeURL
				model.assistant.activationKind = "subscription"
			}
			model.assistant.codeStep = model.assistant.activationLink == ""
			if !model.assistant.codeStep {
				return model, nil
			}
			return model, model.assistant.code.Focus()
		}
		return model, nil
	case "name", "name-saving":
		return model.updateAssistantNameKey(key)
	case "sharing", "report", "report-sending":
		return model.updateAssistantSharingKey(key)
	case "consent":
		switch key.String() {
		case "esc":
			model.assistant.state = "chat"
			model.assistant.pendingSend = false
			model.message = ""
			return model, model.assistant.input.Focus()
		case "space":
			model.assistant.sharing = !model.assistant.sharing
		case "enter":
			return model.acceptAssistantConsent()
		}
		return model, nil
	case "activate":
		if !model.assistant.codeStep {
			switch key.String() {
			case "enter":
				model.assistant.activationNotice = ""
				if open := model.actions.OpenURL; open != nil {
					if err := open(model.assistant.activationLink); err != nil {
						model.assistant.activationNotice = "Could not open the browser; open the link above."
					}
				}
				model.assistant.codeStep = true
				model.message = ""
				return model, model.assistant.code.Focus()
			case "esc":
				model.message = ""
				model.assistant.state = "chat"
				if model.assistant.onboarding {
					model.assistant.state = "welcome"
				} else {
					model.assistant.pendingSend = false
				}
				return model, model.assistant.input.Focus()
			}
			return model, nil
		}
		switch key.String() {
		case "esc":
			model.assistant.code.Reset()
			if model.assistant.activationLink != "" {
				model.assistant.codeStep = false
				model.assistant.activationNotice = ""
				model.message = ""
				return model, nil
			}
			model.message = ""
			model.assistant.state = "chat"
			if model.assistant.onboarding {
				model.assistant.state = "welcome"
			} else {
				model.assistant.pendingSend = false
			}
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
			model.assistant.started = time.Now()
			model.assistant.activity = "Activating Assistant"
			return model, func() tea.Msg {
				activation, err := actions.Activate(ctx, code)
				if err == nil {
					err = ctx.Err()
				}
				if err == nil {
					err = actions.WriteToken(activation.Token)
				}
				accepted := false
				if err == nil && actions.ReadConsent != nil {
					accepted, err = actions.ReadConsent()
				}
				sharing := false
				if err == nil && accepted && actions.ReadSharing != nil {
					sharing, err = actions.ReadSharing()
				}
				return assistantActivationMsg{id: id, token: activation.Token, account: activation.Account, consent: accepted, sharing: sharing, err: err}
			}
		}
		var command tea.Cmd
		model.assistant.code, command = model.assistant.code.Update(key)
		return model, command
	}
	choices := model.assistant.commandChoices()
	if len(choices) > 0 {
		switch key.String() {
		case "up":
			model.assistant.commandCursor = (model.assistant.commandCursor + len(choices) - 1) % len(choices)
			return model, nil
		case "down":
			model.assistant.commandCursor = (model.assistant.commandCursor + 1) % len(choices)
			return model, nil
		case "enter":
			// Like modern agents: Enter runs the highlighted command, Tab only completes it.
			return model.assistantCommand(choices[model.assistant.commandCursor%len(choices)])
		case "tab":
			model.assistant.input.SetValue(choices[model.assistant.commandCursor%len(choices)])
			model.assistant.input.CursorEnd()
			return model, nil
		}
	}

	switch key.String() {
	case "esc":
		model.screen = dashboardHome
		return model, nil
	case "ctrl+r":
		if ok, why := model.assistantCommandAvailable("/review"); !ok {
			model.message = why
			return model, nil
		}
		return model.openAssistantReview()
	case "ctrl+n":
		return model.newAssistantChat()
	case "ctrl+s", "ctrl+p", "ctrl+f":
		return model, nil
	case "enter":
		if model.assistant.changedFiles > 0 {
			switch strings.ToLower(model.assistant.input.Value()) {
			case "save", "salva", "/save":
				model.assistant.input.Reset()
				return model.openAssistantReview()
			}
		}
		if strings.HasPrefix(model.assistant.input.Value(), "/") {
			return model.assistantCommand(strings.TrimSpace(model.assistant.input.Value()))
		}
		if model.assistant.token == "" {
			if actions := model.actions.AssistantService; actions != nil && actions.ReadToken != nil {
				model.assistant.token, _ = actions.ReadToken()
			}
		}
		if model.assistant.token == "" {
			if strings.TrimSpace(model.assistant.input.Value()) == "" {
				return model, nil
			}
			return model.openAssistantGate(), nil
		}
		return model.sendAssistant(false)
	case "alt+enter", "ctrl+j":
		var command tea.Cmd
		model.assistant.input, command = model.assistant.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		return model, command
	}
	var command tea.Cmd
	model.assistant.commandCursor = 0
	model.assistant.input, command = model.assistant.input.Update(key)
	return model, command
}
