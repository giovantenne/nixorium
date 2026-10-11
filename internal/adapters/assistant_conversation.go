package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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

// Cleanup protects the current chat while expiring inactive and excess history.
func (store AssistantWorkspaceStore) Cleanup(current ...string) error {
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
	items := []domain.AssistantConversation{}
	for _, entry := range entries {
		if !entry.IsDir() || !assistantTurnPattern.MatchString(entry.Name()) {
			continue
		}
		root, err := assistantDirectory(parent, entry.Name(), false)
		if err != nil {
			return err
		}
		data, err := assistantReadStateLimit(root, "meta.json", assistantMetadataLimit)
		var metadata domain.AssistantConversation
		if err == nil {
			metadata, err = assistantHistoryMetadata(root, entry.Name(), data)
		}
		root.Close()
		if err != nil {
			return err
		}
		items = append(items, metadata)
	}
	sort.Slice(items, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339, items[i].LastActivityAt)
		b, _ := time.Parse(time.RFC3339, items[j].LastActivityAt)
		if a.Equal(b) {
			return items[i].ID < items[j].ID
		}
		return a.After(b)
	})
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	protected := ""
	if len(current) > 0 {
		protected = current[0]
	}
	kept := 0
	for _, item := range items {
		if item.ID == protected {
			kept = 1
			break
		}
	}
	for _, item := range items {
		if item.ID == protected {
			continue
		}
		active, _ := time.Parse(time.RFC3339, item.LastActivityAt)
		if kept >= 30 || active.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(store.Directory, "conversations", item.ID)); err != nil {
				return err
			}
		} else {
			kept++
		}
	}
	return nil
}

type AssistantWorkspaceTools struct {
	Store     AssistantWorkspaceStore
	LabStatus func(context.Context, string) string
}

func (tools AssistantWorkspaceTools) Execute(ctx context.Context, conversation domain.AssistantConversation, call domain.AssistantToolCall) string {
	if !domain.ValidateAssistantArguments(call.Name, call.Arguments) {
		return assistantToolFailure(nil)
	}
	workspace, err := tools.Store.Open(conversation.ID)
	if err != nil {
		if call.Name == "lab_status" {
			return `{"ok":false,"error":"status_unavailable"}`
		}
		return assistantToolFailure(err)
	}
	defer workspace.Close()
	token, err := tools.Store.ReadToken()
	if err != nil {
		if call.Name == "lab_status" {
			return `{"ok":false,"error":"status_unavailable"}`
		}
		return assistantToolFailure(err)
	}
	if token != "" {
		call.Arguments = []byte(strings.ReplaceAll(string(call.Arguments), token, "[REDACTED]"))
	}
	result := ""
	switch call.Name {
	case "lab_status":
		if tools.LabStatus == nil {
			result = `{"ok":false,"error":"status_unavailable"}`
		} else {
			result = tools.LabStatus(ctx, workspace.Conversation.Repository)
		}
	case "search_packages":
		var args struct{ Query string }
		json.Unmarshal(call.Arguments, &args)
		searchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		items, err := (Local{}).SearchSoftwarePackages(searchCtx, workspace.Conversation.Repository, args.Query, 50)
		if err != nil {
			result = assistantToolResult(map[string]any{"ok": false, "error": "evaluation_failed"})
		} else {
			packages := []map[string]string{}
			for _, item := range items {
				packages = append(packages, map[string]string{"attr": item.ID, "version": item.Version, "description": item.Summary})
			}
			result = assistantToolResult(map[string]any{"ok": true, "packages": packages})
		}
	case "evaluate":
		evaluation, err := EvaluateAssistantCandidate(ctx, workspace)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			result = assistantToolResult(map[string]any{"ok": false, "error": "timeout"})
		} else if err != nil || !evaluation.OK {
			result = assistantToolResult(map[string]any{"ok": false, "error": "evaluation_failed", "message": evaluation.Error})
		} else {
			result = assistantToolResult(map[string]any{"ok": true, "changedHosts": evaluation.ChangedHosts})
		}
	case "build":
		result = BuildAssistantCandidate(ctx, workspace, runBoundedCommand)
	default:
		result = AssistantFileTools{}.Execute(ctx, workspace, call)
	}
	if token != "" {
		result = strings.ReplaceAll(result, token, "[REDACTED]")
	}
	return result
}

func (store AssistantWorkspaceStore) ReadConsent() (bool, error) {
	return (AssistantStateStore{Directory: store.Directory}).ReadConsent()
}

func (store AssistantWorkspaceStore) ReadSharing() (bool, error) {
	return AssistantStateStore(store).ReadSharing()
}
