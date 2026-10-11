package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type AssistantTurnClient interface {
	OpenTurn(context.Context, string) (domain.AssistantTurn, error)
	Step(context.Context, string, string, domain.AssistantStepRequest, func(domain.AssistantEvent)) error
	Cancel(context.Context, string, string) error
}
type AssistantConversationStore interface {
	ReadToken() (string, error)
	ReadConsent() (bool, error)
	ReadTranscript(domain.AssistantConversation) ([]domain.AssistantMessage, error)
	WriteTranscript(domain.AssistantConversation, []domain.AssistantMessage) error
	VisibleFiles(domain.AssistantConversation) ([]string, error)
	CoreIdentity(domain.AssistantConversation) (*domain.AssistantCore, error)
}
type AssistantToolExecutor interface {
	Execute(context.Context, domain.AssistantConversation, domain.AssistantToolCall) string
}
type AssistantTurnRunner struct {
	Sharing *AssistantConversationReporter
	Client  AssistantTurnClient
	Tools   AssistantToolExecutor
	Store   AssistantConversationStore
	Version string
	// Qualification only; production leaves it nil.
	CoreOverride *domain.AssistantCore
}

func assistantSafeText(text, token string) string {
	return domain.RedactForModel(strings.ReplaceAll(text, token, "[REDACTED]"))
}
func assistantActivity(call domain.AssistantToolCall) string {
	var args struct {
		Path    string
		Pattern string
		Query   string
	}
	_ = json.Unmarshal(call.Arguments, &args)
	switch call.Name {
	case "list_files":
		return "Listing configuration files"
	case "read_file":
		return "Reading " + args.Path
	case "write_file":
		return "Writing " + args.Path
	case "delete_file":
		return "Removing " + args.Path
	case "search":
		return "Searching " + args.Pattern
	case "search_packages":
		return "Searching packages for " + args.Query
	case "evaluate":
		return "Evaluating the configuration"
	case "build":
		return "Building new parts of the proposal"
	case "lab_status":
		return "Checking the laboratory state"
	}
	return "Checking Assistant tool request"
}
func (runner AssistantTurnRunner) Run(ctx context.Context, conversation domain.AssistantConversation, prompt string, emit func(domain.AssistantEvent)) (result domain.AssistantTurnResult, runErr error) {
	if strings.TrimSpace(prompt) == "" {
		return result, nil
	}
	if runner.Store == nil || runner.Client == nil || runner.Tools == nil {
		return result, errors.New("Assistant is not configured.")
	}
	if runner.Sharing != nil {
		defer func() {
			if result.Outcome == "final" || result.Outcome == "error" {
				runner.Sharing.ShareCompleted(conversation)
			}
		}()
	}
	token, err := runner.Store.ReadToken()
	if err != nil {
		return result, err
	}
	if token == "" {
		return result, &domain.AssistantError{Status: 401, Code: "invalid_token"}
	}
	accepted, err := runner.Store.ReadConsent()
	if err != nil {
		return result, err
	}
	if !accepted {
		return result, &domain.AssistantError{Code: "consent_required"}
	}
	messages, err := runner.Store.ReadTranscript(conversation)
	if err != nil {
		return result, err
	}
	for i := range messages {
		messages[i].Content = assistantSafeText(messages[i].Content, token)
		for j := range messages[i].ToolCalls {
			messages[i].ToolCalls[j].Arguments = []byte(strings.ReplaceAll(string(messages[i].ToolCalls[j].Arguments), token, "[REDACTED]"))
		}
	}
	messages = append(messages, domain.AssistantMessage{Role: "user", Content: assistantSafeText(prompt, token)})
	result.Messages = messages
	emit(domain.AssistantEvent{Type: "activity", Text: "Checking Assistant subscription"})
	turn, err := runner.Client.OpenTurn(ctx, token)
	if err != nil {
		return result, err
	}
	// Local UI acknowledgement; emitted only after the service admits the turn.
	emit(domain.AssistantEvent{Type: "turn_started"})
	emitTranscriptSize := func() {
		if data, err := json.Marshal(messages); err == nil {
			emit(domain.AssistantEvent{Type: "transcript_size", TranscriptBytes: len(data), MaxRequestBytes: turn.Limits.MaxRequestBytes})
		}
	}
	finished := false
	defer func() {
		if !finished {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = runner.Client.Cancel(cancelCtx, token, turn.TurnID)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(turn.Limits.Seconds)*time.Second)
	defer cancel()
	files, err := runner.Store.VisibleFiles(conversation)
	if err != nil {
		return result, err
	}
	core := runner.CoreOverride
	if core == nil {
		core, err = runner.Store.CoreIdentity(conversation)
		if err != nil {
			core = nil
		}
	}
	contextData := &domain.AssistantContext{Nixorium: runner.Version, Core: core, Files: files}
	textBytes := 0
	for step := 0; step < turn.Limits.Steps; step++ {
		emitTranscriptSize()
		if err = ctx.Err(); err != nil {
			return result, err
		}
		var terminal domain.AssistantEvent
		var service []domain.AssistantMessage
		invalidService := false
		stepText := ""
		pendingText := ""
		flushText := func(final bool) {
			pendingText = strings.ReplaceAll(pendingText, token, "[REDACTED]")
			hold := 0
			if !final {
				for length := 1; length < len(token) && length <= len(pendingText); length++ {
					if strings.HasSuffix(pendingText, token[:length]) {
						hold = length
					}
				}
			}
			part := assistantSafeText(pendingText[:len(pendingText)-hold], token)
			pendingText = pendingText[len(pendingText)-hold:]
			if part != "" {
				stepText += part
				emit(domain.AssistantEvent{Type: "text", Delta: part})
			}
		}
		overflow := false
		err = runner.Client.Step(ctx, token, turn.TurnID, domain.AssistantStepRequest{Context: contextData, Messages: messages}, func(event domain.AssistantEvent) {
			// Preserve token prefixes across SSE chunk boundaries until safe to emit.
			event.Text = assistantSafeText(event.Text, token)
			event.Summary = assistantSafeText(event.Summary, token)
			textBytes += len(event.Delta) + len(event.Summary)
			if textBytes > domain.AssistantTextLimit {
				overflow = true
				cancel()
				return
			}
			switch event.Type {
			case "text":
				pendingText += event.Delta
				flushText(false)
			case "activity":
				emit(event)
			case "service_messages":
				if !domain.ValidAssistantServiceMessages(event.Messages) {
					invalidService = true
					cancel()
					return
				}
				service = event.Messages
			case "tool_calls", "final", "error":
				terminal = event
			}
		})
		flushText(true)
		if overflow || invalidService {
			err = &domain.AssistantError{Code: "protocol_error"}
		}
		if err != nil {
			if stepText != "" {
				messages = append(messages, domain.AssistantMessage{Role: "assistant", Content: stepText})
			}
			result.Messages = messages
			if saveErr := runner.Store.WriteTranscript(conversation, messages); saveErr != nil {
				return result, saveErr
			}
			return result, err
		}
		switch terminal.Type {
		case "final":
			messages = append(messages, service...)
			messages = append(messages, domain.AssistantMessage{Role: "assistant", Content: stepText})
			result.Summary = terminal.Summary
			result.Messages = messages
			if err = runner.Store.WriteTranscript(conversation, messages); err != nil {
				return result, err
			}
			result.Outcome = "final"
			finished = true
			emitTranscriptSize()
			emit(terminal)
			return result, nil
		case "error":
			if stepText != "" {
				messages = append(messages, domain.AssistantMessage{Role: "assistant", Content: stepText})
			}
			result.Messages = messages
			if err = runner.Store.WriteTranscript(conversation, messages); err != nil {
				return result, err
			}
			result.Outcome = "error"
			return result, &domain.AssistantError{Code: terminal.Code}
		case "tool_calls":
			if len(terminal.Calls) == 0 {
				return result, &domain.AssistantError{Code: "protocol_error"}
			}
			for _, call := range terminal.Calls {
				known := false
				for _, tool := range domain.AssistantToolsV1() {
					if tool.Name == call.Name {
						known = true
					}
				}
				if !known {
					return result, &domain.AssistantError{Code: "protocol_error"}
				}
			}
			for i := range terminal.Calls {
				terminal.Calls[i].Arguments = []byte(strings.ReplaceAll(string(terminal.Calls[i].Arguments), token, "[REDACTED]"))
			}
			messages = append(messages, service...)
			messages = append(messages, domain.AssistantMessage{Role: "assistant", Content: stepText, ToolCalls: terminal.Calls})
			for _, call := range terminal.Calls {
				emit(domain.AssistantEvent{Type: "activity", Text: assistantSafeText(assistantActivity(call), token)})
				output := runner.Tools.Execute(ctx, conversation, call)
				if call.Name == "evaluate" || call.Name == "build" {
					emit(domain.AssistantEvent{Type: "tool_result", ToolName: call.Name, ToolResult: assistantSafeText(output, token)})
				}
				messages = append(messages, domain.AssistantMessage{Role: "tool", ToolCallID: call.ID, Content: assistantSafeText(output, token)})
			}
			result.Messages = messages
			if err = runner.Store.WriteTranscript(conversation, messages); err != nil {
				return result, err
			}
		default:
			return result, &domain.AssistantError{Code: "protocol_error"}
		}
	}
	return result, &domain.AssistantError{Status: 429, Code: "step_limit", Message: fmt.Sprintf("Assistant reached its limit of %d steps.", turn.Limits.Steps)}
}
