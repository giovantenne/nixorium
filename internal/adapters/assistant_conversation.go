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
	"golang.org/x/sys/unix"
)

func (store AssistantWorkspaceStore) ReadToken() (string, error) {
	return store.state().ReadToken()
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

// assistantTurnErrorsFile is a private per-conversation diagnostic record. It is
// separate from the transcript, which is sent to the service on every step.
const assistantTurnErrorsFile = "turn-errors-v1.json"
const assistantTurnErrorsKept = 20

type assistantTurnErrorRecord struct {
	At     string `json:"at"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// RecordTurnError appends why a turn failed to the conversation's private
// record, keeping the latest entries. It is best effort and local only.
func (store AssistantWorkspaceStore) RecordTurnError(conversation domain.AssistantConversation, at time.Time, code, detail string) error {
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		return err
	}
	defer workspace.Close()
	var records []assistantTurnErrorRecord
	if data, err := assistantReadStateLimit(workspace.root, assistantTurnErrorsFile, 64*1024); err == nil {
		if assistantDecodeStrict(data, &records) != nil {
			records = nil
		}
	}
	records = append(records, assistantTurnErrorRecord{At: at.UTC().Format(time.RFC3339), Code: code, Detail: assistantErrorDetail(detail)})
	if len(records) > assistantTurnErrorsKept {
		records = records[len(records)-assistantTurnErrorsKept:]
	}
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return assistantWriteState(workspace.root, assistantTurnErrorsFile, append(data, '\n'))
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
	return store.cleanupNpmArtifacts()
}

type AssistantWorkspaceTools struct {
	Store       AssistantWorkspaceStore
	LabStatus   func(context.Context, string) string
	Diagnostics func(context.Context, string, string, string) string
	// SystemFiles reads one allowlisted system file of a computer (ADR 0032).
	SystemFiles func(context.Context, string, string, string) string
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
	case "host_diagnostics", "git_history", "prefetch", "read_system_file":
		accepted, consentErr := tools.Store.ReadConsent()
		if consentErr != nil || !accepted {
			result = `{"ok":false,"error":"consent_required"}`
		} else if call.Name == "read_system_file" {
			var args struct{ Host, Path string }
			// Malformed arguments leave an empty host and path, which the tool rejects.
			_ = json.Unmarshal(call.Arguments, &args)
			if tools.SystemFiles == nil {
				result = `{"ok":false,"error":"system_files_unavailable"}`
			} else {
				result = tools.SystemFiles(ctx, workspace.Conversation.Repository, args.Host, args.Path)
			}
		} else if call.Name == "git_history" {
			result = AssistantGitHistory(ctx, workspace, call)
		} else if call.Name == "prefetch" {
			result = prefetchAssistantFile(ctx, tools.Store, call)
		} else if tools.Diagnostics == nil {
			result = `{"ok":false,"error":"diagnostics_unavailable"}`
		} else {
			var args struct{ Host, Area string }
			// Malformed arguments leave an empty host and area, which the tool rejects.
			_ = json.Unmarshal(call.Arguments, &args)
			result = tools.Diagnostics(ctx, workspace.Conversation.Repository, args.Host, args.Area)
		}
	case "prepare_npm":
		result = prepareAssistantNpm(ctx, tools.Store, workspace, call)
	case "lab_status":
		if tools.LabStatus == nil {
			result = `{"ok":false,"error":"status_unavailable"}`
		} else {
			result = tools.LabStatus(ctx, workspace.Conversation.Repository)
		}
	case "search_packages":
		var args struct{ Query string }
		// Malformed arguments leave the query empty.
		_ = json.Unmarshal(call.Arguments, &args)
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
	return store.state().ReadConsent()
}

func (store AssistantWorkspaceStore) ReadSharing() (bool, error) {
	return store.state().ReadSharing()
}

// ErrAssistantProposalConflict reports that files of an unsaved proposal
// changed in the deployment after that chat started.
var ErrAssistantProposalConflict = errors.New("assistant proposal conflicts with the current configuration")

// CarryOverProposal copies the unsaved proposal of an outdated chat into a new
// chat on the current configuration. It copies only when none of the proposal's
// files changed between the two bases, so nothing is merged or overwritten.
func (store AssistantWorkspaceStore) CarryOverProposal(from, to string) (int, error) {
	root, old, err := store.historyRoot(from)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	if old.SavedAt != "" {
		return 0, nil
	}
	modes := map[string]bool{}
	data, err := assistantReadStateLimit(root, "modes.json", 128*1024)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		return 0, err
	}
	if err == nil && (json.Unmarshal(data, &modes) != nil || len(modes) > 200) {
		return 0, errors.New("invalid Assistant modes")
	}
	var deleted []string
	data, err = assistantReadStateLimit(root, "deleted.json", 128*1024)
	if err != nil || json.Unmarshal(data, &deleted) != nil || len(deleted) > 200 {
		return 0, errors.New("invalid Assistant deleted files")
	}
	paths := append([]string{}, deleted...)
	for path := range modes {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return 0, nil
	}
	for _, path := range paths {
		if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) || domain.AssistantReadOnlyPath(path) {
			return 0, errors.New("invalid Assistant proposal path")
		}
	}
	target, err := store.Open(to)
	if err != nil {
		return 0, err
	}
	defer target.Close()
	if target.Conversation.Repository != old.Repository {
		return 0, errors.New("invalid Assistant conversation repository")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	changed, err := resetGit(ctx, old.Repository, nil, 1024*1024, append([]string{"diff", "--name-only", old.Base, target.Conversation.Base, "--"}, paths...)...)
	if err != nil || strings.TrimSpace(changed) != "" {
		return 0, ErrAssistantProposalConflict
	}
	overlay, err := assistantDirectory(root, "overlay", false)
	if err != nil {
		return 0, err
	}
	defer overlay.Close()
	removed := map[string]bool{}
	for _, path := range deleted {
		removed[path] = true
	}
	for path, executable := range modes {
		if removed[path] {
			continue
		}
		parent, name, err := assistantParent(overlay, path, false)
		if err != nil {
			return 0, err
		}
		content, err := assistantReadStateLimit(parent, name, 512*1024)
		parent.Close()
		if err != nil {
			return 0, err
		}
		if err = target.Write(path, string(content), executable); err != nil {
			return 0, err
		}
	}
	for _, path := range deleted {
		if _, exists := target.base[path]; exists {
			if err = target.Delete(path); err != nil {
				return 0, err
			}
		}
	}
	return store.ChangedFiles(to)
}
