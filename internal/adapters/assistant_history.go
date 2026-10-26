package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Allow JSON escaping for the bounded Unicode title and 2,000-character answer.
const assistantMetadataLimit = 32 * 1024

func assistantHistoryMessages(root *os.File) ([]domain.AssistantMessage, error) {
	data, err := assistantReadStateLimit(root, "transcript.json", 4*1024*1024)
	if err != nil {
		return nil, err
	}
	var messages []domain.AssistantMessage
	if assistantDecodeStrict(data, &messages) != nil {
		return nil, errors.New("invalid Assistant transcript")
	}
	return messages, nil
}

func assistantHistoryMetadata(root *os.File, id string, data []byte) (domain.AssistantConversation, error) {
	var c domain.AssistantConversation
	invalid := errors.New("invalid Assistant conversation metadata")
	if assistantDecodeStrict(data, &c) != nil || c.ID != id || !filepath.IsAbs(c.Repository) {
		return c, invalid
	}
	if len(c.Base) != 40 && len(c.Base) != 64 {
		return c, invalid
	}
	if _, err := hex.DecodeString(c.Base); err != nil {
		return c, invalid
	}
	if c.LastActivityAt == "" {
		c.LastActivityAt = c.CreatedAt
	}
	for _, stamp := range []string{c.CreatedAt, c.LastActivityAt, c.SavedAt} {
		if stamp != "" {
			if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				return c, invalid
			}
		}
	}
	if c.CreatedAt == "" || !utf8.ValidString(c.Title) || utf8.RuneCountInString(c.Title) > 60 || strings.ContainsAny(c.Title, "\r\n") {
		return c, invalid
	}
	if c.LastAnswer != nil && (!utf8.ValidString(*c.LastAnswer) || utf8.RuneCountInString(*c.LastAnswer) > 2000) {
		return c, invalid
	}
	if c.Title == "" || c.LastAnswer == nil {
		messages, err := assistantHistoryMessages(root)
		if err != nil {
			return c, err
		}
		if c.Title == "" {
			c.Title = domain.AssistantConversationTitle(messages)
		}
		if c.LastAnswer == nil {
			answer := domain.AssistantPreviousAnswer(messages)
			c.LastAnswer = &answer
		}
	}
	return c, nil
}

// historyRoot reads private state without relaxing the writable workspace's HEAD guard.
func (store AssistantWorkspaceStore) historyRoot(id string) (*os.File, domain.AssistantConversation, error) {
	var c domain.AssistantConversation
	if !assistantTurnPattern.MatchString(id) {
		return nil, c, errors.New("invalid Assistant conversation")
	}
	parent, err := store.conversations(false)
	if err != nil {
		return nil, c, err
	}
	defer parent.Close()
	root, err := assistantDirectory(parent, id, false)
	if err != nil {
		return nil, c, err
	}
	data, err := assistantReadStateLimit(root, "meta.json", assistantMetadataLimit)
	if err == nil {
		c, err = assistantHistoryMetadata(root, id, data)
	}
	if err != nil {
		root.Close()
		return nil, c, err
	}
	return root, c, nil
}

func (store AssistantWorkspaceStore) CompleteTurn(c domain.AssistantConversation, messages []domain.AssistantMessage, answer *string) error {
	root, metadata, err := store.historyRoot(c.ID)
	if err != nil {
		return err
	}
	defer root.Close()
	metadata.LastActivityAt = time.Now().UTC().Format(time.RFC3339Nano)
	metadata.Title = domain.AssistantConversationTitle(messages)
	metadata.Unfinished = answer == nil
	if answer != nil {
		text := domain.AssistantHistoryText(*answer, 2000)
		metadata.LastAnswer = &text
	} else {
		// Preserve the stored field exactly: legacy fallback may see error text
		// in the transcript, but an unsuccessful turn must not record it.
		data, err := assistantReadStateLimit(root, "meta.json", assistantMetadataLimit)
		if err != nil {
			return err
		}
		var persisted domain.AssistantConversation
		if err := assistantDecodeStrict(data, &persisted); err != nil {
			return err
		}
		metadata.LastAnswer = persisted.LastAnswer
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return assistantWriteState(root, "meta.json", data)
}

func (store AssistantWorkspaceStore) markConversationSaved(id string) error {
	root, c, err := store.historyRoot(id)
	if err != nil {
		return err
	}
	defer root.Close()
	c.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return assistantWriteState(root, "meta.json", data)
}

func (store AssistantWorkspaceStore) ConversationHistory(ctx context.Context, repository, id string) (domain.AssistantConversationHistory, error) {
	var result domain.AssistantConversationHistory
	root, c, err := store.historyRoot(id)
	if err != nil {
		return result, err
	}
	defer root.Close()
	repository, err = filepath.Abs(repository)
	if err != nil || repository != c.Repository {
		return result, errors.New("invalid Assistant conversation repository")
	}
	result.Conversation = c
	result.Messages, err = assistantHistoryMessages(root)
	if err != nil {
		return result, err
	}
	head, err := resetGit(ctx, repository, nil, 128, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return result, err
	}
	switch {
	case c.SavedAt != "":
		result.State = "saved"
	case strings.TrimSpace(head) != c.Base:
		result.State = "outdated"
	default:
		result.ChangedFiles, err = store.ChangedFiles(id)
		if err != nil {
			return result, err
		}
		if result.ChangedFiles > 0 {
			result.State = "proposal not saved"
		}
	}
	return result, nil
}

func (store AssistantWorkspaceStore) ListConversations(ctx context.Context, repository, current string) ([]domain.AssistantConversationHistory, error) {
	parent, err := store.conversations(false)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.AssistantConversationHistory{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	entries, err := parent.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	items := []domain.AssistantConversationHistory{}
	for _, entry := range entries {
		if !entry.IsDir() || !assistantTurnPattern.MatchString(entry.Name()) || entry.Name() == current {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item, err := store.ConversationHistory(ctx, repository, entry.Name())
		if err != nil {
			return nil, err
		}
		// Only metadata and status cross into the list view.
		item.Messages = nil
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339, items[i].Conversation.LastActivityAt)
		b, _ := time.Parse(time.RFC3339, items[j].Conversation.LastActivityAt)
		if a.Equal(b) {
			return items[i].Conversation.ID < items[j].Conversation.ID
		}
		return a.After(b)
	})
	return items, nil
}
