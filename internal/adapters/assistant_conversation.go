package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (store AssistantWorkspaceStore) ReadToken() (string, error) {
	return (AssistantStateStore{Directory: store.Directory}).ReadToken()
}
func (store AssistantWorkspaceStore) ReadTranscript(conversation domain.AssistantConversation) ([]domain.AssistantMessage, error) {
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		return nil, err
	}
	defer workspace.Close()
	data, err := assistantReadStateLimit(workspace.root, "transcript.json", 4*1024*1024)
	if err != nil {
		return nil, err
	}
	var messages []domain.AssistantMessage
	if assistantDecodeStrict(data, &messages) != nil {
		return nil, errors.New("invalid Assistant transcript")
	}
	return messages, nil
}
func (store AssistantWorkspaceStore) WriteTranscript(conversation domain.AssistantConversation, messages []domain.AssistantMessage) error {
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		return err
	}
	defer workspace.Close()
	token, err := store.ReadToken()
	if err != nil {
		return err
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	if len(data) > 4*1024*1024 {
		return &domain.AssistantError{Status: 413, Code: "request_too_large"}
	}
	if token != "" {
		data = []byte(strings.ReplaceAll(string(data), token, "[REDACTED]"))
	}
	return assistantWriteState(workspace.root, "transcript.json", data)
}
func (store AssistantWorkspaceStore) VisibleFiles(conversation domain.AssistantConversation) ([]string, error) {
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		return nil, err
	}
	defer workspace.Close()
	entries, err := workspace.Entries()
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.Path)
	}
	return files, nil
}
func (store AssistantWorkspaceStore) PrepareConversation(repository, id string) (domain.AssistantConversation, error) {
	if id == "" {
		return store.CreateConversation(repository)
	}
	workspace, err := store.Open(id)
	if err != nil {
		return domain.AssistantConversation{}, err
	}
	defer workspace.Close()
	return workspace.Conversation, nil
}

// Cleanup removes only expired, correctly identified private conversations.
func (store AssistantWorkspaceStore) Cleanup() error {
	parent, err := store.conversations(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	entries, err := parent.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !assistantTurnPattern.MatchString(entry.Name()) {
			continue
		}
		root, err := assistantDirectory(parent, entry.Name(), false)
		if err != nil {
			return err
		}
		data, err := assistantReadState(root, "meta.json")
		root.Close()
		if err != nil {
			return err
		}
		var metadata domain.AssistantConversation
		if assistantDecodeStrict(data, &metadata) != nil || metadata.ID != entry.Name() {
			return errors.New("invalid Assistant conversation metadata")
		}
		created, err := time.Parse(time.RFC3339, metadata.CreatedAt)
		if err != nil {
			return err
		}
		if created.Before(time.Now().Add(-30 * 24 * time.Hour)) {
			if err = os.RemoveAll(filepath.Join(store.Directory, "conversations", entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

type AssistantWorkspaceTools struct{ Store AssistantWorkspaceStore }

func (tools AssistantWorkspaceTools) Execute(ctx context.Context, conversation domain.AssistantConversation, call domain.AssistantToolCall) string {
	if !domain.ValidateAssistantArguments(call.Name, call.Arguments) {
		return assistantToolFailure(nil)
	}
	// C6 is deliberately separate: these two tools cannot evaluate or search yet.
	if call.Name == "evaluate" || call.Name == "search_packages" {
		return assistantToolResult(map[string]any{"ok": false, "error": "invalid_arguments", "message": "This tool is not available in this client version."})
	}
	workspace, err := tools.Store.Open(conversation.ID)
	if err != nil {
		return assistantToolFailure(err)
	}
	defer workspace.Close()
	token, err := tools.Store.ReadToken()
	if err != nil {
		return assistantToolFailure(err)
	}
	if token != "" {
		call.Arguments = []byte(strings.ReplaceAll(string(call.Arguments), token, "[REDACTED]"))
	}
	result := AssistantFileTools{}.Execute(ctx, workspace, call)
	if token != "" {
		result = strings.ReplaceAll(result, token, "[REDACTED]")
	}
	return result
}

func (store AssistantWorkspaceStore) ReadConsent() (bool, error) {
	return (AssistantStateStore{Directory: store.Directory}).ReadConsent()
}
