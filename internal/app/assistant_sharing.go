package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/giovantenne/nixorium/internal/domain"
	"time"
)

type AssistantSharingClient interface {
	ShareConversation(context.Context, string, string, domain.AssistantStepRequest) error
	ReportConversation(context.Context, string, string, string, string, domain.AssistantStepRequest) error
}
type AssistantSharingStore interface {
	AssistantConversationStore
	ReadSharing() (bool, error)
}
type AssistantConversationReporter struct {
	Store   AssistantSharingStore
	Client  AssistantSharingClient
	Version string
}

// Build a private snapshot from the saved transcript, never from a TUI draft.
func (reporter AssistantConversationReporter) snapshot(conversation domain.AssistantConversation) (string, domain.AssistantStepRequest, error) {
	var payload domain.AssistantStepRequest
	if reporter.Store == nil || reporter.Client == nil {
		return "", payload, errors.New("Assistant reporting is unavailable.")
	}
	token, err := reporter.Store.ReadToken()
	if err != nil {
		return "", payload, err
	}
	if token == "" {
		return "", payload, &domain.AssistantError{Code: "invalid_token", Status: 401}
	}
	messages, err := reporter.Store.ReadTranscript(conversation)
	if err != nil {
		return "", payload, err
	}
	// Copy before redacting so an in-memory store cannot be mutated by an upload.
	data, err := json.Marshal(messages)
	if err != nil {
		return "", payload, err
	}
	if err = json.Unmarshal(data, &messages); err != nil {
		return "", payload, err
	}
	for i := range messages {
		messages[i].Content = assistantSafeText(messages[i].Content, token)
		for j := range messages[i].ToolCalls {
			var args any
			decoder := json.NewDecoder(bytes.NewReader(messages[i].ToolCalls[j].Arguments))
			decoder.UseNumber()
			if decoder.Decode(&args) != nil {
				return "", payload, errors.New("Invalid Assistant transcript.")
			}
			args = redactSharingArguments(args, token)
			messages[i].ToolCalls[j].Arguments, err = json.Marshal(args)
			if err != nil {
				return "", payload, err
			}
		}
	}
	files, err := reporter.Store.VisibleFiles(conversation)
	if err != nil {
		return "", payload, err
	}
	core, _ := reporter.Store.CoreIdentity(conversation)
	payload.Context = &domain.AssistantContext{Nixorium: reporter.Version, Core: core, Files: files, Capabilities: []string{"prepare_npm_v1"}}
	payload.Messages = messages
	return token, payload, nil
}
func redactSharingArguments(value any, token string) any {
	switch value := value.(type) {
	case string:
		return assistantSafeText(value, token)
	case []any:
		for i := range value {
			value[i] = redactSharingArguments(value[i], token)
		}
		return value
	case map[string]any:
		for key := range value {
			value[key] = redactSharingArguments(value[key], token)
		}
		return value
	default:
		return value
	}
}
func (reporter AssistantConversationReporter) Report(ctx context.Context, conversation domain.AssistantConversation, rating, comment string) error {
	token, payload, err := reporter.snapshot(conversation)
	if err != nil {
		return err
	}
	return reporter.Client.ReportConversation(ctx, token, conversation.ID, rating, assistantSafeText(comment, token), payload)
}

// Submission failures are silent. Snapshotting finishes before the next turn can
// change the saved transcript; the network request never blocks the turn result.
func (reporter AssistantConversationReporter) ShareCompleted(conversation domain.AssistantConversation) {
	if reporter.Store == nil {
		return
	}
	share, err := reporter.Store.ReadSharing()
	if err != nil || !share {
		return
	}
	token, payload, err := reporter.snapshot(conversation)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		share, err := reporter.Store.ReadSharing()
		if err == nil && share {
			_ = reporter.Client.ShareConversation(ctx, token, conversation.ID, payload)
		}
	}()
}
