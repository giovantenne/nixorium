package adapters

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantImportedPayloadAliasesStayLocalAcrossAncestry(t *testing.T) {
	repo := workspaceRepository(t)
	payload := "synthetic-import-payload-marker\n"
	imported := "home-files/imports/" + strings.Repeat("a", 64)
	writeGitReviewFile(t, repo, "home-files.json", `{"schemaVersion":1,"files":[],"directories":[]}`)
	writeGitReviewFile(t, repo, imported, payload)
	workspaceTestGit(t, repo, "add", "--all")
	workspaceTestGit(t, repo, "commit", "-qm", "import teaching material")
	importRevision := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	workspaceTestGit(t, repo, "mv", imported, "visible-alias.txt")
	writeGitReviewFile(t, repo, "second-alias.txt", payload)
	currentImport := "home-files/imports/" + strings.Repeat("b", 64)
	writeGitReviewFile(t, repo, currentImport, "second-synthetic-local-import\n")
	writeGitReviewFile(t, repo, "course.txt", "ordinary model-visible text\n")
	workspaceTestGit(t, repo, "add", "--all")
	workspaceTestGit(t, repo, "commit", "-qm", "move material")
	// Neither endpoint contains the import path. Full ancestry still withholds it.
	aliasRevision := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", "HEAD"))
	workspaceTestGit(t, repo, "commit", "--allow-empty", "-qm", "later baseline")
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
	defer workspace.Close()
	for _, path := range []string{imported, currentImport, "visible-alias.txt", "second-alias.txt"} {
		if _, err := workspace.Read(path); !errors.Is(err, fs.ErrPermission) {
			t.Fatal(path, err)
		}
		if err := workspace.Write(path, "replacement", false); !errors.Is(err, fs.ErrPermission) {
			t.Fatal(path, err)
		}
	}
	if err := workspace.Write("home-files", "collision", false); !errors.Is(err, fs.ErrPermission) {
		t.Fatal("hidden import parent could be replaced", err)
	}
	if err := workspace.Write("new-copy.txt", payload, false); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	// A legacy overlay must also be filtered at the model-facing read/list boundary.
	if err := os.WriteFile(filepath.Join(store.Directory, "conversations", conversation.ID, "overlay", "legacy-copy.txt"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	for _, call := range []domain.AssistantToolCall{
		{Name: "list_files", Arguments: []byte(`{}`)},
		{Name: "search", Arguments: []byte(`{"pattern":"synthetic-import|ordinary"}`)},
		{Name: "read_file", Arguments: []byte(`{"path":"legacy-copy.txt"}`)},
	} {
		output := AssistantFileTools{}.Execute(t.Context(), workspace, call)
		for _, private := range []string{payload[:len(payload)-1], "visible-alias.txt", "second-alias.txt", "legacy-copy.txt", imported, currentImport} {
			if strings.Contains(output, private) {
				t.Fatal(call.Name, output)
			}
		}
	}
	for _, args := range []map[string]any{{"limit": 5}, {"revision": importRevision}, {"revision": aliasRevision}, {"revision": aliasRevision, "path": "visible-alias.txt"}} {
		_, output := assistantHistoryCall(t, workspace, args)
		if strings.Contains(output, "synthetic-import-payload-marker") || strings.Contains(output, "visible-alias.txt") || strings.Contains(output, "second-alias.txt") {
			t.Fatal(args, output)
		}
	}
	// Native evaluation copies the whole tracked baseline, including opaque bytes.
	candidate := t.TempDir()
	workspaceTestGit(t, repo, "clone", "--shared", "--quiet", "--no-checkout", "--template=", repo, candidate)
	workspaceTestGit(t, candidate, "checkout", "--quiet", "--detach", conversation.Base)
	if err := workspace.Write("home-files.json", `{"schemaVersion":1,"files":[{"destination":"Desktop/README.txt","text":"authored inline","executable":false}],"directories":[]}`, false); err != nil {
		t.Fatal(err)
	}
	if err := copyAssistantOverlay(workspace, candidate); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join(candidate, "visible-alias.txt"))
	if err != nil || string(bytes) != payload {
		t.Fatal("evaluation lost imported source", err)
	}
	bytes, err = os.ReadFile(filepath.Join(candidate, currentImport))
	if err != nil || string(bytes) != "second-synthetic-local-import\n" {
		t.Fatal("evaluation lost original import path", err)
	}
	data, err := workspace.Read("home-files.json")
	if err != nil || !json.Valid(data) || !strings.Contains(string(data), "authored inline") {
		t.Fatal("inline text was hidden", err)
	}
}

func TestAssistantImportPrivacyCacheAndShallowHistoryFailClosed(t *testing.T) {
	workspace, store, repo := assistantWorkspaceFixture(t)
	// Count full ancestry reads without changing Git semantics. A private cache
	// has already been initialized by the first Open in the fixture.
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	marker := filepath.Join(directory, "history-scans")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nfor arg do\n  if [ \"$arg\" = log ]; then printf 'scan\\n' >> " + quote(marker) + "; fi\ndone\nexec " + quote(realGit) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(directory, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	for i := 0; i < 2; i++ {
		opened, err := store.Open(workspace.Conversation.ID)
		if err != nil {
			t.Fatal(err)
		}
		opened.Close()
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("cached privacy rescanned ancestry", err)
	}
	// Even a previously cached success cannot grant visibility after the local
	// repository acquires an incomplete shallow boundary.
	shallow := filepath.Join(repo, ".git", "shallow")
	if err := os.WriteFile(shallow, []byte(workspace.Conversation.Base+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if opened, err := store.Open(workspace.Conversation.ID); err == nil {
		opened.Close()
		t.Fatal("accepted cached privacy in shallow repository")
	}
	if _, err := assistantImportedBlobs(t.Context(), repo, workspace.Conversation.Base); err == nil {
		t.Fatal("accepted shallow history without cache")
	}
	if err := os.Remove(shallow); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{`{"base":"wrong","objects":[]}`, `{"base":"` + workspace.Conversation.Base + `","objects":["invalid"]}`} {
		if err := assistantWriteState(workspace.root, "import-privacy-v1.json", []byte(content)); err != nil {
			t.Fatal(err)
		}
		if opened, err := store.Open(workspace.Conversation.ID); err == nil {
			opened.Close()
			t.Fatal("accepted invalid privacy cache")
		}
	}
}
