package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func assistantHistoryFixture(t *testing.T) (*AssistantWorkspace, string, string) {
	t.Helper()
	repo := workspaceRepository(t)
	for path, content := range map[string]string{
		"course-old.nix":       "{ oldCourse = true; }\n",
		"lab-settings.json":    `{"name":"old classroom","adminPassword":"OpaqueOldSecret","nested":{"ApiToken":"OldApiSecret"}}`,
		"keys/private.nix":     "hidden-blob-marker",
		"lab-credentials.json": `{"adminPassword":"tracked-private-credentials-marker"}`,
		"AGENTS.md":            "hidden-instructions-marker",
		"retired.txt":          "newly-ignored-marker",
		"docs/.gitignore":      "*.private\n!public.private\n",
		"docs/secret.private":  "historical-ignored-marker",
		"docs/public.private":  "visible-exception\n",
		"secrets.nix":          "Bearer abcdef ghp_example $6$salt$oldhash\n",
		"gitlink-change":       "ordinary prior content\n",
	} {
		writeGitReviewFile(t, repo, path, content)
	}
	if err := os.Symlink("keys/private.nix", filepath.Join(repo, "symlink.nix")); err != nil {
		t.Fatal(err)
	}
	workspaceTestGit(t, repo, "add", "--all", "--force")
	workspaceTestGit(t, repo, "commit", "-qm", "course added ghp_subjecttoken")
	older := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	workspaceTestGit(t, repo, "mv", "course-old.nix", "course-new.nix")
	workspaceTestGit(t, repo, "mv", "keys/private.nix", "leaked-copy.nix")
	writeGitReviewFile(t, repo, "course-new.nix", "{ newCourse = true; }\n")
	writeGitReviewFile(t, repo, "lab-settings.json", `{"name":"new classroom","adminPassword":"OpaqueNewSecret","nested":{"ApiToken":"NewApiSecret"}}`)
	writeGitReviewFile(t, repo, ".gitignore", "retired.txt\n")
	writeGitReviewFile(t, repo, "AGENTS.md", "new-hidden-instructions-marker")
	writeGitReviewFile(t, repo, "docs/secret.private", "new-historical-ignored-marker")
	writeGitReviewFile(t, repo, "docs/public.private", "updated-visible-exception\n")
	if err := os.Remove(filepath.Join(repo, "symlink.nix")); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repo, "symlink.nix", "replacement symlink target marker\n")
	workspaceTestGit(t, repo, "add", "--all", "--force")
	workspaceTestGit(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+older+",gitlink-change")
	workspaceTestGit(t, repo, "commit", "-qm", "course update")
	base := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	return &AssistantWorkspace{Conversation: domain.AssistantConversation{Repository: repo, Base: base}}, repo, older
}

func assistantHistoryCall(t *testing.T, workspace *AssistantWorkspace, arguments any) (map[string]any, string) {
	t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	result := AssistantGitHistory(t.Context(), workspace, domain.AssistantToolCall{Name: "git_history", Arguments: encoded})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatal(err, result)
	}
	if len(result) > assistantHistoryOutputLimit {
		t.Fatalf("output exceeds bound: %d", len(result))
	}
	return decoded, result
}

func TestAssistantGitHistoryListsDatesSummariesAndVisibleChanges(t *testing.T) {
	workspace, _, _ := assistantHistoryFixture(t)
	result, output := assistantHistoryCall(t, workspace, map[string]any{"limit": 2})
	if result["ok"] != true || result["base"] != workspace.Conversation.Base || result["truncated"] != true {
		t.Fatal(result)
	}
	commits := result["commits"].([]any)
	if len(commits) != 2 {
		t.Fatal(commits)
	}
	for _, value := range commits {
		commit := value.(map[string]any)
		if len(commit["revision"].(string)) != 40 || commit["date"] == "" || commit["summary"] == "" {
			t.Fatal(commit)
		}
	}
	for _, private := range []string{"keys/", "AGENTS.md", "lab-credentials.json", "docs/secret.private", "retired.txt", "ghp_subjecttoken", "symlink.nix", "gitlink-change", "leaked-copy.nix"} {
		if strings.Contains(output, private) {
			t.Fatalf("disclosed %s: %s", private, output)
		}
	}
	if !strings.Contains(output, "course-new.nix") || !strings.Contains(output, "public.private") || !strings.Contains(output, "[REDACTED]") {
		t.Fatal(output)
	}
}

func TestAssistantGitHistoryFindsLateChangesAndRedactsAssignments(t *testing.T) {
	workspace, repo, _ := assistantHistoryFixture(t)
	common := strings.Repeat("unchanged course setting\n", 1100)
	writeGitReviewFile(t, repo, "long-course.nix", common+"old late change\npassword=OpaqueOldPassword\napiToken = \"OpaqueOldToken\";\n")
	workspaceTestGit(t, repo, "add", "long-course.nix")
	workspaceTestGit(t, repo, "commit", "-qm", "course version")
	older := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	writeGitReviewFile(t, repo, "long-course.nix", common+"new late change\npassword=OpaqueNewPassword\napiToken = \"OpaqueNewToken\";\n")
	workspaceTestGit(t, repo, "add", "long-course.nix")
	workspaceTestGit(t, repo, "commit", "-qm", "password=OpaqueSubjectPassword teacher@example.invalid")
	workspace.Conversation.Base = strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	result, output := assistantHistoryCall(t, workspace, map[string]any{"revision": older, "path": "long-course.nix"})
	if result["ok"] != true || !strings.Contains(output, "old late change") || !strings.Contains(output, "new late change") || !strings.Contains(output, "@@ -1098") {
		t.Fatal("late change missing", output)
	}
	for _, secret := range []string{"OpaqueOldPassword", "OpaqueNewPassword", "OpaqueOldToken", "OpaqueNewToken"} {
		if strings.Contains(output, secret) {
			t.Fatal("secret disclosed", secret, output)
		}
	}
	result, output = assistantHistoryCall(t, workspace, map[string]any{"limit": 1, "path": "long-course.nix"})
	if result["ok"] != true || strings.Contains(output, "OpaqueSubjectPassword") || strings.Contains(output, "teacher@example.invalid") || !strings.Contains(output, "[WITHHELD]") {
		t.Fatal(output)
	}
}

func TestAssistantGitHistoryWithholdsBaselineIgnoredAliases(t *testing.T) {
	workspace, repo, _ := assistantHistoryFixture(t)
	writeGitReviewFile(t, repo, "once-visible.txt", "base-hidden-alias-marker\n")
	workspaceTestGit(t, repo, "add", "once-visible.txt")
	workspaceTestGit(t, repo, "commit", "-qm", "visible course note")
	older := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	workspaceTestGit(t, repo, "mv", "once-visible.txt", "now-private.txt")
	writeGitReviewFile(t, repo, ".gitignore", "retired.txt\nnow-private.txt\n")
	workspaceTestGit(t, repo, "add", ".gitignore")
	workspaceTestGit(t, repo, "commit", "-qm", "course note hidden")
	workspace.Conversation.Base = strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	for _, args := range []map[string]any{{"limit": 2}, {"limit": 2, "path": "once-visible.txt"}, {"revision": older}, {"revision": older, "path": "once-visible.txt"}} {
		result, output := assistantHistoryCall(t, workspace, args)
		if result["ok"] != true || strings.Contains(output, "once-visible.txt") || strings.Contains(output, `"path":"now-private.txt"`) || strings.Contains(output, "base-hidden-alias-marker") {
			t.Fatal(args, output)
		}
	}
}

func TestAssistantGitHistoryComparesSanitizedImmutableRevisions(t *testing.T) {
	workspace, repo, older := assistantHistoryFixture(t)
	writeGitReviewFile(t, repo, "course-new.nix", "dirty-worktree-marker")
	writeGitReviewFile(t, repo, "untracked.txt", "untracked-worktree-marker")
	workspaceTestGit(t, repo, "notes", "add", "-m", "hidden-notes-marker")
	before := assistantRepositorySnapshot(t, repo)
	indexBefore, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	result, output := assistantHistoryCall(t, workspace, map[string]any{"revision": older})
	if result["ok"] != true || result["revision"] != older || !strings.Contains(output, "newCourse") || !strings.Contains(output, "oldCourse") || !strings.Contains(output, "new classroom") || !strings.Contains(output, "old classroom") {
		t.Fatal(output)
	}
	for _, private := range []string{"OpaqueOldSecret", "OpaqueNewSecret", "OldApiSecret", "NewApiSecret", "hidden-blob-marker", "instructions-marker", "ignored-marker", "historical-ignored-marker", "dirty-worktree-marker", "untracked-worktree-marker", "hidden-notes-marker", "ghp_", "$6$", "Bearer abcdef", "replacement symlink target marker"} {
		if strings.Contains(output, private) {
			t.Fatalf("disclosed %s: %s", private, output)
		}
	}
	indexAfter, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	headAfter := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	if err != nil || string(indexBefore) != string(indexAfter) || !reflect.DeepEqual(before, assistantRepositorySnapshot(t, repo)) || headAfter != workspace.Conversation.Base {
		t.Fatal("history changed the deployment or index", err)
	}
	_, listed := assistantHistoryCall(t, workspace, map[string]any{"limit": 2})
	if strings.Contains(listed, "hidden-notes-marker") {
		t.Fatal("history read Git notes", listed)
	}
	selected, narrowed := assistantHistoryCall(t, workspace, map[string]any{"revision": older, "path": "lab-settings.json"})
	if selected["ok"] != true || len(selected["changes"].([]any)) != 1 || strings.Contains(narrowed, "course-new.nix") {
		t.Fatal(narrowed)
	}
	_, narrowed = assistantHistoryCall(t, workspace, map[string]any{"revision": older, "path": "leaked-copy.nix"})
	if strings.Contains(narrowed, "hidden-blob-marker") || strings.Contains(narrowed, "leaked-copy.nix") {
		t.Fatal(narrowed)
	}
}

func TestAssistantGitHistoryRejectsRefsObjectsAndUnrelatedRevisions(t *testing.T) {
	workspace, repo, older := assistantHistoryFixture(t)
	for _, arguments := range []map[string]any{
		{"revision": "HEAD"}, {"revision": older + "~1"}, {"revision": older + ":flake.nix"},
		{"revision": "--all"}, {"revision": strings.ToUpper(older)}, {"limit": 21}, {"path": "../outside"},
		{"path": "AGENTS.md"}, {"path": "lab-credentials.json"}, {"path": "keys/private.nix"}, {"path": ".git/config"}, {"path": "ghp_patfile.nix"},
	} {
		result, _ := assistantHistoryCall(t, workspace, arguments)
		if result["ok"] != false {
			t.Fatal("accepted", arguments, result)
		}
	}
	blob := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", older+":flake.nix"))
	result, _ := assistantHistoryCall(t, workspace, map[string]any{"revision": blob})
	if result["ok"] != false {
		t.Fatal("accepted blob", result)
	}
	workspaceTestGit(t, repo, "checkout", "--orphan", "unrelated")
	workspaceTestGit(t, repo, "commit", "-qm", "unrelated commit")
	unrelated := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	result, _ = assistantHistoryCall(t, workspace, map[string]any{"revision": unrelated})
	if result["ok"] != false {
		t.Fatal("accepted unrelated commit", result)
	}
	// Even a later descendant is outside this conversation's pinned ancestry.
	workspaceTestGit(t, repo, "checkout", "master")
	workspaceTestGit(t, repo, "commit", "--allow-empty", "-qm", "future commit")
	future := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	result, _ = assistantHistoryCall(t, workspace, map[string]any{"revision": future})
	if result["ok"] != false {
		t.Fatal("accepted later commit", result)
	}
	copy := *workspace
	copy.Conversation.Repository = filepath.Join(repo, ".git")
	result, _ = assistantHistoryCall(t, &copy, map[string]any{})
	if result["ok"] != false {
		t.Fatal("accepted Git internals", result)
	}
	bare := filepath.Join(t.TempDir(), "bare-repository")
	workspaceTestGit(t, repo, "clone", "--bare", "--quiet", repo, bare)
	copy.Conversation.Repository = bare
	result, _ = assistantHistoryCall(t, &copy, map[string]any{})
	if result["ok"] != false {
		t.Fatal("accepted bare repository", result)
	}
}

func TestAssistantGitHistoryPathFilterIsLiteralAndSkipsUnrelatedCommits(t *testing.T) {
	workspace, repo, _ := assistantHistoryFixture(t)
	name := ":(glob)*.nix"
	writeGitReviewFile(t, repo, name, "literal file first\n")
	workspaceTestGit(t, repo, "add", "--all")
	workspaceTestGit(t, repo, "commit", "-qm", "literal file change")
	relevant := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	writeGitReviewFile(t, repo, "ordinary.nix", "unrelated change\n")
	workspaceTestGit(t, repo, "add", "ordinary.nix")
	workspaceTestGit(t, repo, "commit", "-qm", "unrelated later change")
	workspace.Conversation.Base = strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	result, output := assistantHistoryCall(t, workspace, map[string]any{"limit": 1, "path": name})
	if result["ok"] != true || result["commits"].([]any)[0].(map[string]any)["revision"] != relevant || strings.Contains(output, "ordinary.nix") || strings.Contains(output, "unrelated later change") {
		t.Fatal(output)
	}
}

func TestAssistantGitHistoryBoundsContentAndSkipsHelpers(t *testing.T) {
	workspace, repo, _ := assistantHistoryFixture(t)
	older := workspace.Conversation.Base
	for i := 0; i < 25; i++ {
		path := "file-" + string(rune('a'+i)) + ".txt"
		writeGitReviewFile(t, repo, path, strings.Repeat("é", 5000))
	}
	writeGitReviewFile(t, repo, "oversize.txt", strings.Repeat("a", assistantHistoryBlobLimit+1))
	writeGitReviewFile(t, repo, "binary.dat", "\x00private-binary-marker")
	workspaceTestGit(t, repo, "add", "--all")
	workspaceTestGit(t, repo, "commit", "-qm", "bounded comparison")
	workspace.Conversation.Base = strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	marker := filepath.Join(t.TempDir(), "helper-was-run")
	helper := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	workspaceTestGit(t, repo, "config", "diff.external", helper)
	workspaceTestGit(t, repo, "config", "core.fsmonitor", helper)
	workspaceTestGit(t, repo, "config", "core.pager", helper)
	workspaceTestGit(t, repo, "config", "log.showSignature", "true")
	workspaceTestGit(t, repo, "config", "gpg.program", helper)
	t.Setenv("GIT_EXTERNAL_DIFF", helper)
	result, output := assistantHistoryCall(t, workspace, map[string]any{"revision": older})
	if result["ok"] != true || result["truncated"] != true || len(result["changes"].([]any)) > 20 || !strings.Contains(output, "contentTruncated") || strings.Contains(output, "private-binary-marker") {
		t.Fatal(result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("external helper ran", err)
	}
	result, output = assistantHistoryCall(t, workspace, map[string]any{"revision": older, "path": "oversize.txt"})
	if result["ok"] != true || !strings.Contains(output, "contentUnavailable") || strings.Contains(output, strings.Repeat("a", 100)) {
		t.Fatal(result)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	output = AssistantGitHistory(ctx, workspace, domain.AssistantToolCall{Name: "git_history", Arguments: []byte(`{}`)})
	if !strings.Contains(output, `"error":"timeout"`) {
		t.Fatal(output)
	}
}
