package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func writeHistoryMetadata(t *testing.T, store AssistantWorkspaceStore, c domain.AssistantConversation) {
	t.Helper()
	parent, err := store.conversations(false)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	root, err := assistantDirectory(parent, c.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = assistantWriteState(root, "meta.json", data); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantHistoryStatesOrderingLegacyAndStrictState(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	messages := []domain.AssistantMessage{{Role: "user", Content: strings.Repeat("è", 61) + "\nSecond line"}, {Role: "assistant", Content: "Legacy answer"}, {Role: "assistant", Content: "Tool text", ToolCalls: []domain.AssistantToolCall{{ID: "synthetic", Name: "evaluate", Arguments: json.RawMessage(`{}`)}}}, {Role: "assistant"}}
	if err := store.WriteTranscript(w.Conversation, messages); err != nil {
		t.Fatal(err)
	}
	legacy := w.Conversation
	legacy.LastActivityAt = ""
	legacy.Title = ""
	legacy.LastAnswer = nil
	writeHistoryMetadata(t, store, legacy)
	h, err := store.ConversationHistory(t.Context(), repo, legacy.ID)
	if err != nil || h.State != "" || h.Conversation.LastActivityAt != legacy.CreatedAt || h.Conversation.Title != strings.Repeat("è", 59)+"…" || *h.Conversation.LastAnswer != "Legacy answer" {
		t.Fatal(h, err)
	}
	if err := w.Write("new.nix", "{}", false); err != nil {
		t.Fatal(err)
	}
	h, err = store.ConversationHistory(t.Context(), repo, legacy.ID)
	if err != nil || h.State != "proposal not saved" || h.ChangedFiles != 1 {
		t.Fatal(h, err)
	}
	legacy.Base = strings.Repeat("b", 40)
	writeHistoryMetadata(t, store, legacy)
	h, err = store.ConversationHistory(t.Context(), repo, legacy.ID)
	if err != nil || h.State != "outdated" || len(h.Messages) != 4 {
		t.Fatal(h, err)
	}
	if _, err = store.Open(legacy.ID); err == nil {
		t.Fatal("history relaxed writable HEAD guard")
	}
	legacy.SavedAt = time.Now().UTC().Format(time.RFC3339)
	writeHistoryMetadata(t, store, legacy)
	h, err = store.ConversationHistory(t.Context(), repo, legacy.ID)
	if err != nil || h.State != "saved" {
		t.Fatal(h, err)
	}
	fresh, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	fresh.LastActivityAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	writeHistoryMetadata(t, store, fresh)
	items, err := store.ListConversations(t.Context(), repo, "")
	if err != nil || len(items) != 2 || items[0].Conversation.ID != fresh.ID || len(items[0].Messages) != 0 {
		t.Fatal(items, err)
	}
	items, err = store.ListConversations(t.Context(), repo, fresh.ID)
	if err != nil || len(items) != 1 || items[0].Conversation.ID != legacy.ID {
		t.Fatal(items, err)
	}
	if _, err = store.ConversationHistory(t.Context(), t.TempDir(), fresh.ID); err == nil {
		t.Fatal("cross-repository history")
	}
	path := filepath.Join(store.Directory, "conversations", fresh.ID, "meta.json")
	data, _ := os.ReadFile(path)
	if err = os.WriteFile(path, append(data[:len(data)-1], []byte(`,"unknown":true}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ConversationHistory(t.Context(), repo, fresh.ID); err == nil {
		t.Fatal("unknown metadata accepted")
	}
	answer := strings.Repeat("é", 2001)
	fresh.LastAnswer = &answer
	writeHistoryMetadata(t, store, fresh)
	if _, err = store.ConversationHistory(t.Context(), repo, fresh.ID); err == nil {
		t.Fatal("unbounded answer accepted")
	}
}

func TestAssistantHistoryActivityAnswerAndSavedState(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	old := w.Conversation
	old.LastActivityAt = time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	writeHistoryMetadata(t, store, old)
	answer := strings.Repeat("é", 2001)
	messages := []domain.AssistantMessage{{Role: "user", Content: "Original title\nDetails"}, {Role: "assistant", Content: answer}}
	if err := store.WriteTranscript(old, messages); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTurn(old, messages, &answer); err != nil {
		t.Fatal(err)
	}
	h, err := store.ConversationHistory(t.Context(), repo, old.ID)
	if err != nil || h.Conversation.LastActivityAt == old.LastActivityAt || h.Conversation.Title != "Original title" || *h.Conversation.LastAnswer != strings.Repeat("é", 1999)+"…" {
		t.Fatal(h, err)
	}
	partial := append(messages, domain.AssistantMessage{Role: "user", Content: "Later question"}, domain.AssistantMessage{Role: "assistant", Content: "Failed partial"})
	if err := store.WriteTranscript(old, partial); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTurn(old, partial, nil); err != nil {
		t.Fatal(err)
	}
	h, err = store.ConversationHistory(t.Context(), repo, old.ID)
	if err != nil || strings.Contains(*h.Conversation.LastAnswer, "Failed partial") || h.Conversation.Title != "Original title" {
		t.Fatal(h, err)
	}
	if err := store.markConversationSaved(old.ID); err != nil {
		t.Fatal(err)
	}
	h, err = store.ConversationHistory(t.Context(), repo, old.ID)
	if err != nil || h.State != "saved" || h.Conversation.SavedAt == "" {
		t.Fatal(h, err)
	}
}

func TestAssistantHistoryRetentionActivityNinetyDaysThirtyAndCurrent(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	current := w.Conversation
	current.CreatedAt = time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	current.LastActivityAt = current.CreatedAt
	writeHistoryMetadata(t, store, current)
	ids := []string{}
	for i := 0; i < 34; i++ {
		c, err := store.CreateConversation(repo)
		if err != nil {
			t.Fatal(err)
		}
		c.CreatedAt = current.CreatedAt
		c.LastActivityAt = time.Now().Add(-time.Duration(i) * time.Hour).UTC().Format(time.RFC3339)
		writeHistoryMetadata(t, store, c)
		ids = append(ids, c.ID)
	}
	expired, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	expired.LastActivityAt = time.Now().Add(-91 * 24 * time.Hour).UTC().Format(time.RFC3339)
	writeHistoryMetadata(t, store, expired)
	if err = store.Cleanup(current.ID); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(store.Directory, "conversations"))
	if err != nil || len(entries) != 30 {
		t.Fatal("retention count", len(entries), err)
	}
	for i, id := range ids {
		_, err := os.Stat(filepath.Join(store.Directory, "conversations", id))
		if i < 29 && err != nil || i >= 29 && !os.IsNotExist(err) {
			t.Fatal("activity ordering", i, err)
		}
	}
	if _, err = os.Stat(filepath.Join(store.Directory, "conversations", current.ID)); err != nil {
		t.Fatal("removed current", err)
	}
	if _, err = os.Stat(filepath.Join(store.Directory, "conversations", expired.ID)); !os.IsNotExist(err) {
		t.Fatal("expired kept", err)
	}
}

func TestAssistantLegacyErrorDoesNotRecordLastAnswer(t *testing.T) {
	w, store, _ := assistantWorkspaceFixture(t)
	c := w.Conversation
	c.LastAnswer = nil
	writeHistoryMetadata(t, store, c)
	messages := []domain.AssistantMessage{{Role: "user", Content: "Legacy request"}, {Role: "assistant", Content: "Failed partial"}}
	if err := store.WriteTranscript(c, messages); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTurn(c, messages, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.Directory, "conversations", c.ID, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted domain.AssistantConversation
	if err := assistantDecodeStrict(data, &persisted); err != nil || persisted.LastAnswer != nil {
		t.Fatal(persisted, err)
	}
}
