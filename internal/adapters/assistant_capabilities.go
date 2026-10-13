package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type assistantHomeFilesCapability struct {
	Base      string `json:"base"`
	Supported bool   `json:"supported"`
}

// Discovery is advisory and runs once per immutable conversation baseline,
// never during TUI startup. It reads only the pinned input's pure API marker;
// no host output, candidate source or imported payload is evaluated here.
func (store AssistantWorkspaceStore) HomeFilesSupported(ctx context.Context, conversation domain.AssistantConversation) bool {
	root, metadata, err := store.historyRoot(conversation.ID)
	if err != nil {
		return false
	}
	defer root.Close()
	if metadata.Base != conversation.Base || metadata.Repository != conversation.Repository {
		return false
	}
	return assistantHomeFilesSupported(ctx, &AssistantWorkspace{Conversation: metadata, root: root}, runBoundedNix)
}

func assistantHomeFilesSupported(ctx context.Context, workspace *AssistantWorkspace, run assistantNixRunner) bool {
	var cached assistantHomeFilesCapability
	data, cacheErr := assistantReadStateLimit(workspace.root, "home-files-capability-v1.json", 1024)
	if cacheErr == nil {
		if assistantDecodeStrict(data, &cached) == nil && cached.Base == workspace.Conversation.Base {
			return cached.Supported
		}
		return false
	}
	if !errors.Is(cacheErr, fs.ErrNotExist) {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ref := (&url.URL{Scheme: "git+file", Path: workspace.Conversation.Repository, RawQuery: "rev=" + workspace.Conversation.Base}).String()
	quoted, _ := json.Marshal(ref)
	expression := "let f = builtins.getFlake " + strings.ReplaceAll(string(quoted), "${", `\${`) + "; in (f.inputs.nixorium.lib.homeFilesVersion or 0) == 1"
	output, err := run(ctx, 1024, "eval", "--offline", "--json", "--no-write-lock-file", "--no-update-lock-file",
		"--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false", "--expr", expression)
	supported := false
	if err == nil {
		_ = json.Unmarshal([]byte(output), &supported)
	}
	cached = assistantHomeFilesCapability{Base: workspace.Conversation.Base, Supported: supported}
	encoded, _ := json.Marshal(cached)
	if assistantWriteState(workspace.root, "home-files-capability-v1.json", encoded) != nil {
		return false
	}
	return supported
}
