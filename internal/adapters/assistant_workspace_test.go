package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func assistantRepositorySnapshot(t *testing.T, repo string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(repo, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result[strings.TrimPrefix(path, repo+"/")] = fmt.Sprintf("%x %o", sha256.Sum256(data), info.Mode())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func assistantWorkspaceFixture(t *testing.T) (*AssistantWorkspace, AssistantWorkspaceStore, string) {
	t.Helper()
	repo := workspaceRepository(t)
	for path, content := range map[string]string{
		"lab-settings.json": `{"lab":{"name":"Synthetic","PASSWORD":"private-marker","nested":{"ApiToken":"private-marker","count":1}},"array":[{"credential":"private-marker"}]}`,
		"flake.nix":         "{ outputs = _: {}; }\n", "keys/public": "hidden-marker", "modules/.env.local": "hidden-marker", "AGENTS.md": "Read-only instructions", ".gitignore": "ignored.txt\n", "binary.bin": "\x00\x01", "secrets-in-text.nix": "sk-example ghp_example Bearer abcdef_123 $6$salt$hash\n", "large.txt": strings.Repeat("a", 70000),
	} {
		writeGitReviewFile(t, repo, path, content)
	}
	workspaceTestGit(t, repo, "add", "--all")
	workspaceTestGit(t, repo, "commit", "-qm", "synthetic assistant base")
	writeGitReviewFile(t, repo, "ignored.txt", "private-untracked-marker")
	before := assistantRepositorySnapshot(t, repo)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, assistantRepositorySnapshot(t, repo)) {
			t.Error("Assistant changed deployment bytes or modes")
		}
	})
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	store := AssistantWorkspaceStore{Directory: state}
	conversation, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workspace.Close() })
	return workspace, store, repo
}
func assistantFileCall(t *testing.T, workspace *AssistantWorkspace, name string, args any) map[string]any {
	t.Helper()
	data, _ := json.Marshal(args)
	result := AssistantFileTools{}.Execute(context.Background(), workspace, domain.AssistantToolCall{ID: "c1", Name: name, Arguments: data})
	var decoded map[string]any
	if json.Unmarshal([]byte(result), &decoded) != nil {
		t.Fatal(result)
	}
	return decoded
}
func TestAssistantCandidateUsesGitBaseAndOverlayOnly(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	original, _ := w.Read("flake.nix")
	// Even a later worktree modification must not become model input.
	live := filepath.Join(repo, "flake.nix")
	os.WriteFile(live, []byte("uncommitted-private-marker"), 0644)
	read, err := w.Read("flake.nix")
	os.WriteFile(live, original, 0644)
	if err != nil || string(read) != string(original) {
		t.Fatal("read used working tree")
	}
	result := assistantFileCall(t, w, "write_file", map[string]any{"path": "modules/course.nix", "content": "{ programs.test.enable = true; }\n", "executable": true})
	if result["ok"] != true {
		t.Fatal(result)
	}
	reopened, err := store.Open(w.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.modes["modules/course.nix"] {
		t.Fatal("executable metadata lost")
	}
	content, err := reopened.Read("modules/course.nix")
	if err != nil || !strings.Contains(string(content), "programs.test") {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.Directory, "conversations", w.Conversation.ID, "overlay/modules/course.nix"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe overlay mode")
	}
	if assistantFileCall(t, w, "delete_file", map[string]any{"path": "module.nix"})["ok"] != true {
		t.Fatal("base delete failed")
	}
	if assistantFileCall(t, w, "read_file", map[string]any{"path": "module.nix"})["error"] != "not_found" {
		t.Fatal("deleted base visible")
	}
	if assistantFileCall(t, w, "write_file", map[string]any{"path": "module.nix", "content": "replacement"})["ok"] != true {
		t.Fatal("restore failed")
	}
}
func TestAssistantToolsFilterAndRedact(t *testing.T) {
	w, _, _ := assistantWorkspaceFixture(t)
	listed := assistantFileCall(t, w, "list_files", map[string]any{})
	data, _ := json.Marshal(listed)
	for _, hidden := range []string{"keys/public", ".env.local", "ignored.txt"} {
		if strings.Contains(string(data), hidden) {
			t.Fatal("hidden file listed")
		}
	}
	settings := assistantFileCall(t, w, "read_file", map[string]any{"path": "lab-settings.json"})
	s, _ := json.Marshal(settings)
	if strings.Contains(string(s), "private-marker") || !strings.Contains(string(s), "[WITHHELD]") {
		t.Fatal("settings secret disclosed")
	}
	secrets := assistantFileCall(t, w, "read_file", map[string]any{"path": "secrets-in-text.nix"})
	s, _ = json.Marshal(secrets)
	for _, secret := range []string{"sk-example", "ghp_example", "Bearer abcdef", "$6$"} {
		if strings.Contains(string(s), secret) {
			t.Fatal("pattern secret disclosed")
		}
	}
	if assistantFileCall(t, w, "read_file", map[string]any{"path": "binary.bin"})["error"] != "not_text" {
		t.Fatal("binary file accepted")
	}
	read := assistantFileCall(t, w, "read_file", map[string]any{"path": "large.txt"})
	if len(read["content"].(string)) != 65536 || read["truncated"] != true || read["totalBytes"] != float64(70000) {
		t.Fatal(read)
	}
	matches := assistantFileCall(t, w, "search", map[string]any{"pattern": "WITHHELD", "path": "lab-settings.json"})
	if matches["ok"] != true || len(matches["matches"].([]any)) == 0 {
		t.Fatal("search bypassed redaction")
	}
	if assistantFileCall(t, w, "search", map[string]any{"pattern": "["})["error"] != "invalid_arguments" {
		t.Fatal("invalid regex accepted")
	}
}
func TestAssistantToolsRejectTraversalProtectedFilesAndSymlinks(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	for _, path := range []string{"../outside", "/tmp/outside", "a\\b", "a//b", "a/../b", "keys/public", ".git/config", "ignored.txt"} {
		if assistantFileCall(t, w, "read_file", map[string]any{"path": path})["ok"] == true {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"lab-settings.json", "flake.lock", "AGENTS.md", "CLAUDE.md", ".gitignore", ".gitattributes", ".gitmodules", "keys/public", ".env.private"} {
		for _, name := range []string{"write_file", "delete_file"} {
			args := map[string]any{"path": path}
			if name == "write_file" {
				args["content"] = "changed"
			}
			if assistantFileCall(t, w, name, args)["error"] != "protected_path" {
				t.Fatalf("%s %s", name, path)
			}
		}
	}
	overlay := filepath.Join(store.Directory, "conversations", w.Conversation.ID, "overlay")
	for _, directory := range []bool{false, true} {
		path := filepath.Join(overlay, "escape")
		target := filepath.Join(repo, "flake.nix")
		readPath := "escape"
		if directory {
			target = repo
			readPath = "escape/flake.nix"
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"read_file", "write_file", "delete_file"} {
			args := map[string]any{"path": readPath}
			if name == "write_file" {
				args["content"] = "changed"
			}
			if assistantFileCall(t, w, name, args)["ok"] == true {
				t.Fatal("overlay link followed")
			}
		}
		os.Remove(path)
	}
}
func TestAssistantWorkspaceLimitsAndSearchBounds(t *testing.T) {
	w, _, _ := assistantWorkspaceFixture(t)
	if assistantFileCall(t, w, "write_file", map[string]any{"path": "large.nix", "content": strings.Repeat("x", 512*1024+1)})["error"] != "limit_exceeded" {
		t.Fatal("file limit ignored")
	}
	if assistantFileCall(t, w, "write_file", map[string]any{"path": "binary", "content": "\x00"})["error"] != "not_text" {
		t.Fatal("binary write accepted")
	}
	text := strings.Repeat(strings.Repeat("x", 400)+"\n", 300)
	if err := w.Write("matches.txt", text, false); err != nil {
		t.Fatal(err)
	}
	matched := assistantFileCall(t, w, "search", map[string]any{"pattern": "x", "path": "matches.txt"})["matches"].([]any)
	if len(matched) != 200 || len([]rune(matched[0].(map[string]any)["text"].(string))) != 300 {
		t.Fatal("search limits ignored")
	}
	for i := 0; i < 7; i++ {
		if err := w.Write(fmt.Sprintf("big-%d", i), strings.Repeat("x", 512*1024), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Write("overflow", strings.Repeat("x", 512*1024), false); err == nil {
		t.Fatal("conversation byte limit ignored")
	}
}
func TestAssistantConversationFileCountLimit(t *testing.T) {
	w, _, _ := assistantWorkspaceFixture(t)
	for i := 0; i < 200; i++ {
		if err := w.Write(fmt.Sprintf("new/%03d.nix", i), "{}", false); err != nil {
			t.Fatal(i, err)
		}
	}
	if err := w.Write("extra.nix", "{}", false); err == nil {
		t.Fatal("file count limit ignored")
	}
}
func TestAssistantRejectsDirtyRepositoryAndChangedHead(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	writeGitReviewFile(t, repo, "untracked.txt", "temporary")
	if _, err := store.CreateConversation(repo); err == nil || err.Error() != "Commit or discard the deployment changes before using Assistant." {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(repo, "untracked.txt"))
	workspaceTestGit(t, repo, "commit", "--allow-empty", "-qm", "new synthetic HEAD")
	if _, err := store.Open(w.Conversation.ID); err == nil || err.Error() != "The deployment changed since this chat started. Press Ctrl+N to start a new chat." {
		t.Fatal(err)
	}
	workspaceTestGit(t, repo, "reset", "--soft", w.Conversation.Base)
}
