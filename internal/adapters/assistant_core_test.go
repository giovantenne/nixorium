package adapters

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantCoreIdentity(t *testing.T) {
	valid := domain.AssistantCore{Owner: "giovantenne", Repo: "nixorium", Rev: strings.Repeat("a", 40)}
	for _, mode := range []string{"github", "path", "follows", "invalid-rev", "invalid-owner", "missing", "invalid-json"} {
		t.Run(mode, func(t *testing.T) {
			repo := workspaceRepository(t)
			locked := map[string]any{"type": "github", "owner": valid.Owner, "repo": valid.Repo, "rev": valid.Rev}
			var input any = "core-node"
			switch mode {
			case "path":
				locked["type"] = "path"
			case "follows":
				input = []string{"other", "nixorium"}
			case "invalid-rev":
				locked["rev"] = strings.Repeat("A", 40)
			case "invalid-owner":
				locked["owner"] = "bad/owner"
			}
			lock := map[string]any{"root": "deployment", "nodes": map[string]any{
				"deployment": map[string]any{"inputs": map[string]any{"nixorium": input}},
				"core-node":  map[string]any{"locked": locked},
			}}
			data, _ := json.Marshal(lock)
			if mode == "invalid-json" {
				data = []byte("{")
			}
			if mode != "missing" {
				writeGitReviewFile(t, repo, "flake.lock", string(data))
				workspaceTestGit(t, repo, "add", "flake.lock")
				workspaceTestGit(t, repo, "commit", "-qm", "synthetic core identity")
			} else {
				workspaceTestGit(t, repo, "rm", "flake.lock")
				workspaceTestGit(t, repo, "commit", "-qm", "synthetic missing lock")
			}
			base := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
			// Identity must come from the recorded revision, not current files.
			writeGitReviewFile(t, repo, "flake.lock", "{}")
			got, err := AssistantCoreIdentity(t.Context(), repo, base)
			if mode == "missing" || mode == "invalid-json" {
				if err == nil {
					t.Fatal("expected Git or JSON error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "github" {
				if !reflect.DeepEqual(got, &valid) {
					t.Fatal(got)
				}
				store := AssistantWorkspaceStore{}
				stored, err := store.CoreIdentity(domain.AssistantConversation{Repository: repo, Base: base})
				if err != nil || !reflect.DeepEqual(stored, got) {
					t.Fatal(stored, err)
				}
			} else if got != nil {
				t.Fatal("unsupported or invalid identity must be omitted", got)
			}
		})
	}
}
