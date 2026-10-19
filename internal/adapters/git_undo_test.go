package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// undoRepository has an initial version and one ordinary saved change on top:
// it changes lab-software.json and adds modules/site.nix.
func undoRepository(t *testing.T) string {
	t.Helper()
	repository := newGitReviewRepository(t)
	software, err := domain.MarshalLabSoftware(domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}}})
	if err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "lab-software.json", string(software))
	writeGitReviewFile(t, repository, "modules/site.nix", "{ ... }: { }\n")
	workspaceTestGit(t, repository, "add", ".")
	workspaceTestGit(t, repository, "commit", "-qm", "chore: update laboratory software")
	return repository
}

func undoRefusal(t *testing.T, repository, expected string) domain.GitUndoPlanReport {
	t.Helper()
	plan := app.NewGitUndoManager(Local{}).Plan(t.Context(), repository)
	if !plan.HasErrors() || plan.ReviewToken != "" {
		t.Fatalf("the undo was not refused: %+v", plan)
	}
	var messages []string
	for _, issue := range plan.Issues {
		messages = append(messages, issue.Message)
	}
	if !strings.Contains(strings.Join(messages, "\n"), expected) {
		t.Fatalf("refusal = %q, want it to mention %q", messages, expected)
	}
	return plan
}

func TestUndoRevertsTheLastSavedChange(t *testing.T) {
	repository := undoRepository(t)
	saved := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	initial := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD~1"))
	manager := app.NewGitUndoManager(Local{})

	plan := manager.Plan(t.Context(), repository)
	if plan.HasErrors() || plan.Revision != saved || plan.Undone.Parent != initial || plan.Confirmation != "" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Undone.Subject != "chore: update laboratory software" || time.Since(plan.Undone.Date) > time.Minute {
		t.Fatalf("undone change = %+v", plan.Undone)
	}
	if len(plan.Undone.Files) != 2 || plan.Undone.Files[0] != (domain.GitUndoFile{Path: "lab-software.json", Change: "modified"}) || plan.Undone.Files[1] != (domain.GitUndoFile{Path: "modules/site.nix", Change: "added"}) {
		t.Fatalf("files = %+v", plan.Undone.Files)
	}
	if plan.Title() != "Undo: chore: update laboratory software" || !strings.Contains(plan.CommitMessage, "This reverts commit "+saved+".") {
		t.Fatalf("message = %q", plan.CommitMessage)
	}
	if !strings.Contains(plan.Diff.Content, "diff --git a/lab-software.json") || !strings.Contains(plan.Diff.Content, "-      \"package\": \"vlc\"") || !strings.Contains(plan.Diff.Content, "deleted file mode") {
		t.Fatalf("the review does not show what changes: %q", plan.Diff.Content)
	}
	if len(plan.Impacts) != 2 || plan.Impacts[0].Kind != "controller-apply" || plan.Impacts[1].Kind != "client-update" {
		t.Fatalf("impacts = %+v", plan.Impacts)
	}
	if status := workspaceTestGit(t, repository, "status", "--porcelain"); status != "" {
		t.Fatalf("the review changed the worktree: %q", status)
	}

	if refused := manager.Apply(t.Context(), repository, "sha256:wrong"); refused.State != "blocked" || refused.Committed {
		t.Fatalf("a wrong token undid the change: %+v", refused)
	}
	result := manager.Apply(t.Context(), repository, plan.ReviewToken)
	if result.State != "completed" || !result.Committed || result.PreviousRevision != saved || len(result.Impacts) != 2 {
		t.Fatalf("result = %+v", result)
	}
	head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	if head != result.Revision || strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD~1")) != saved {
		t.Fatalf("history = %q", workspaceTestGit(t, repository, "log", "--oneline"))
	}
	if workspaceTestGit(t, repository, "rev-parse", "HEAD^{tree}") != workspaceTestGit(t, repository, "rev-parse", initial+"^{tree}") {
		t.Fatal("the undo did not return to the earlier saved version")
	}
	message := workspaceTestGit(t, repository, "log", "-1", "--format=%B")
	if !strings.HasPrefix(message, "Undo: chore: update laboratory software\n\nThis reverts commit "+saved+".") {
		t.Fatalf("commit message = %q", message)
	}
	if identity := strings.TrimSpace(workspaceTestGit(t, repository, "show", "-s", "--format=%an <%ae>|%cn <%ce>", "HEAD")); identity != "Nixorium <nixorium@localhost>|Nixorium <nixorium@localhost>" {
		t.Fatalf("identity = %q", identity)
	}
	if _, err := os.Stat(filepath.Join(repository, "modules", "site.nix")); !os.IsNotExist(err) {
		t.Fatalf("the added file is still there: %v", err)
	}
	if content, _ := os.ReadFile(filepath.Join(repository, "lab-software.json")); string(content) != "{\n  \"schemaVersion\": 1,\n  \"packages\": []\n}\n" {
		t.Fatalf("lab-software.json = %q", content)
	}
	if status := workspaceTestGit(t, repository, "status", "--porcelain"); status != "" {
		t.Fatalf("worktree is not clean after the undo: %q", status)
	}
	// The undone change stays in history and can be restored from it.
	if kept := workspaceTestGit(t, repository, "show", saved+":modules/site.nix"); !strings.Contains(kept, "{ ... }") {
		t.Fatalf("the undone change is not kept in history: %q", kept)
	}

	// The undo is an ordinary saved change that records itself, and undoing
	// an undo would only bring the change back.
	undoRefusal(t, repository, "already undoes an earlier one")
}

func TestUndoNeverRunsRepositoryHooks(t *testing.T) {
	repository := undoRepository(t)
	hooks := filepath.Join(repository, ".hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(repository, "hook-ran")
	for _, name := range []string{"post-commit", "post-checkout", "post-merge", "pre-commit", "commit-msg"} {
		if err := os.WriteFile(filepath.Join(hooks, name), []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	workspaceTestGit(t, repository, "config", "core.hooksPath", hooks)
	// The hook directory is untracked: ignore it so the tree is clean.
	if err := os.WriteFile(filepath.Join(repository, ".git", "info", "exclude"), []byte(".hooks\nhook-ran\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := app.NewGitUndoManager(Local{})
	plan := manager.Plan(t.Context(), repository)
	if plan.HasErrors() {
		t.Fatalf("plan = %+v", plan)
	}
	if result := manager.Apply(t.Context(), repository, plan.ReviewToken); result.State != "completed" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("a repository hook ran or the marker could not be inspected: %v", err)
	}
}

func TestUndoUsesTheInternalIdentityWithoutUserConfiguration(t *testing.T) {
	repository := undoRepository(t)
	workspaceTestGit(t, repository, "config", "--unset", "user.name")
	workspaceTestGit(t, repository, "config", "--unset", "user.email")
	manager := app.NewGitUndoManager(Local{})
	plan := manager.Plan(t.Context(), repository)
	if result := manager.Apply(t.Context(), repository, plan.ReviewToken); result.State != "completed" {
		t.Fatalf("result = %+v", result)
	}
}

func TestUndoRefusesUnsavedChangesWithTheExistingNextStep(t *testing.T) {
	repository := undoRepository(t)
	writeGitReviewFile(t, repository, "notes.txt", "unsaved\n")
	plan := undoRefusal(t, repository, "changed path(s)")
	if plan.Issues[0].Next == nil || plan.Issues[0].Next.Code != "GIT-DIRTY" {
		t.Fatalf("issue = %+v", plan.Issues[0])
	}
	if result := app.NewGitUndoManager(Local{}).Apply(t.Context(), repository, "sha256:any"); result.Committed || result.State != "blocked" {
		t.Fatalf("result = %+v", result)
	}
	if content, _ := os.ReadFile(filepath.Join(repository, "notes.txt")); string(content) != "unsaved\n" {
		t.Fatal("an unsaved file was touched")
	}
}

func TestUndoRefusesWhatIsNotAnOrdinarySavedChange(t *testing.T) {
	t.Run("the first saved version", func(t *testing.T) {
		undoRefusal(t, newGitReviewRepository(t), "first saved version")
	})
	t.Run("a merge", func(t *testing.T) {
		repository := undoRepository(t)
		main := strings.TrimSpace(workspaceTestGit(t, repository, "branch", "--show-current"))
		workspaceTestGit(t, repository, "checkout", "-q", "-b", "side", "HEAD~1")
		writeGitReviewFile(t, repository, "side.nix", "{ ... }: { }\n")
		workspaceTestGit(t, repository, "add", "side.nix")
		workspaceTestGit(t, repository, "commit", "-qm", "side change")
		workspaceTestGit(t, repository, "checkout", "-q", main)
		workspaceTestGit(t, repository, "merge", "--no-ff", "-q", "-m", "merge side", "side")
		undoRefusal(t, repository, "merge of two histories")
	})
	t.Run("an undo made by Git itself", func(t *testing.T) {
		repository := undoRepository(t)
		workspaceTestGit(t, repository, "revert", "--no-edit", "HEAD")
		undoRefusal(t, repository, "already undoes an earlier one")
	})
	t.Run("an update of Nixorium or the package base", func(t *testing.T) {
		plan := undoRefusal(t, workspaceRepository(t), "UPDATES.md")
		if !strings.Contains(plan.Issues[0].Message, "Recovery and long-term autonomy") {
			t.Fatalf("issue = %+v", plan.Issues[0])
		}
	})
	t.Run("public keys", func(t *testing.T) {
		repository := newGitReviewRepository(t)
		writeGitReviewFile(t, repository, "keys/admin-ssh.pub", "ssh-ed25519 AAAA test\n")
		workspaceTestGit(t, repository, "add", "keys/admin-ssh.pub")
		workspaceTestGit(t, repository, "commit", "-qm", "chore: update laboratory public keys")
		undoRefusal(t, repository, "Keys are missing or inconsistent")
	})
	t.Run("a detached HEAD", func(t *testing.T) {
		repository := undoRepository(t)
		workspaceTestGit(t, repository, "checkout", "-q", "--detach")
		undoRefusal(t, repository, "not on a branch")
	})
	t.Run("an unfinished Git operation", func(t *testing.T) {
		repository := undoRepository(t)
		head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
		if err := os.WriteFile(filepath.Join(repository, ".git", "MERGE_HEAD"), []byte(head+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		undoRefusal(t, repository, "A Git merge is unfinished")
	})
	t.Run("a symbolic link", func(t *testing.T) {
		repository := undoRepository(t)
		if err := os.Symlink("module.nix", filepath.Join(repository, "linked.nix")); err != nil {
			t.Fatal(err)
		}
		workspaceTestGit(t, repository, "add", "linked.nix")
		workspaceTestGit(t, repository, "commit", "-qm", "link")
		undoRefusal(t, repository, "symbolic link")
	})
	t.Run("a content-transforming attribute", func(t *testing.T) {
		repository := undoRepository(t)
		writeGitReviewFile(t, repository, ".gitattributes", "filtered filter=danger\n")
		workspaceTestGit(t, repository, "add", ".gitattributes")
		workspaceTestGit(t, repository, "commit", "-qm", "attributes")
		writeGitReviewFile(t, repository, "filtered", "content\n")
		workspaceTestGit(t, repository, "add", "filtered")
		workspaceTestGit(t, repository, "commit", "-qm", "filtered file")
		undoRefusal(t, repository, "content-transforming attribute")
	})
	t.Run("a change to the Git attribute rules", func(t *testing.T) {
		repository := undoRepository(t)
		writeGitReviewFile(t, repository, ".gitattributes", "*.nix text\n")
		workspaceTestGit(t, repository, "add", ".gitattributes")
		workspaceTestGit(t, repository, "commit", "-qm", "attributes")
		undoRefusal(t, repository, "Git attribute rules")
	})
}

func TestUndoRefusesWhenTheFilesCannotGoBackSafely(t *testing.T) {
	t.Run("an ignored file is in the way", func(t *testing.T) {
		repository := undoRepository(t)
		writeGitReviewFile(t, repository, "old.nix", "{ ... }: { }\n")
		workspaceTestGit(t, repository, "add", "old.nix")
		workspaceTestGit(t, repository, "commit", "-qm", "add old")
		workspaceTestGit(t, repository, "rm", "-q", "old.nix")
		workspaceTestGit(t, repository, "commit", "-qm", "remove old")
		writeGitReviewFile(t, repository, "old.nix", "precious ignored content\n")
		if err := os.WriteFile(filepath.Join(repository, ".git", "info", "exclude"), []byte("old.nix\n"), 0600); err != nil {
			t.Fatal(err)
		}
		plan := undoRefusal(t, repository, "is not saved in Git")
		manager := app.NewGitUndoManager(Local{})
		if result := manager.Apply(t.Context(), repository, "sha256:any"); result.Committed {
			t.Fatalf("result = %+v", result)
		}
		if content, _ := os.ReadFile(filepath.Join(repository, "old.nix")); string(content) != "precious ignored content\n" {
			t.Fatalf("the ignored file was overwritten: %q (plan %+v)", content, plan)
		}
	})
	t.Run("a file is marked assume-unchanged", func(t *testing.T) {
		repository := undoRepository(t)
		workspaceTestGit(t, repository, "update-index", "--assume-unchanged", "lab-software.json")
		undoRefusal(t, repository, "assume-unchanged")
	})
	t.Run("the earlier version holds a secret", func(t *testing.T) {
		repository := newGitReviewRepository(t)
		writeGitReviewFile(t, repository, "unsafe.nix", "{ ... }: { password = \"plaintext\"; }\n")
		workspaceTestGit(t, repository, "add", "unsafe.nix")
		workspaceTestGit(t, repository, "commit", "-qm", "unsafe by hand")
		writeGitReviewFile(t, repository, "unsafe.nix", "{ ... }: { }\n")
		workspaceTestGit(t, repository, "add", "unsafe.nix")
		workspaceTestGit(t, repository, "commit", "-qm", "remove the secret")
		plan := undoRefusal(t, repository, "must not be saved again")
		for _, issue := range plan.Issues {
			if strings.Contains(issue.Message, "plaintext\"") && strings.Contains(issue.Message, "password =") {
				t.Fatalf("the secret leaked into the refusal: %q", issue.Message)
			}
		}
	})
}

func TestUndoRefusesAReviewThatIsNoLongerCurrent(t *testing.T) {
	repository := undoRepository(t)
	manager := app.NewGitUndoManager(Local{})
	plan := manager.Plan(t.Context(), repository)
	writeGitReviewFile(t, repository, "later.nix", "{ ... }: { }\n")
	workspaceTestGit(t, repository, "add", "later.nix")
	workspaceTestGit(t, repository, "commit", "-qm", "a later saved change")
	later := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	if result := manager.Apply(t.Context(), repository, plan.ReviewToken); result.State != "blocked" || result.Committed {
		t.Fatalf("a stale token undid a different change: %+v", result)
	}
	if head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD")); head != later {
		t.Fatal("HEAD moved on a stale token")
	}
}

func TestUndoLeavesHeadAloneWhenTheFilesCannotMove(t *testing.T) {
	repository := undoRepository(t)
	saved := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	manager := app.NewGitUndoManager(Local{})
	plan := manager.Plan(t.Context(), repository)
	// Another Git process holds the index: the files cannot move.
	lock := filepath.Join(repository, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	result := manager.Apply(t.Context(), repository, plan.ReviewToken)
	if result.State != "failed" || result.Committed || !result.RetrySafe || !strings.Contains(result.Message, "could not be moved back") {
		t.Fatalf("result = %+v", result)
	}
	if head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD")); head != saved {
		t.Fatalf("HEAD moved to %s although the files did not", head)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if retry := manager.Apply(t.Context(), repository, plan.ReviewToken); retry.State != "completed" {
		t.Fatalf("retry = %+v", retry)
	}
}

func TestUndoKeepsTheLaboratoryNetworkInItsGuidedChange(t *testing.T) {
	template, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	base, issues := domain.DecodeLabSettings(template)
	if len(issues) > 0 {
		t.Fatalf("template issues = %+v", issues)
	}
	base.Lab.PCCount = 3
	base.Lab.MasterDHCPIP = "192.168.0.198"
	save := func(repository string, settings domain.LabSettingsFile, message string) {
		t.Helper()
		if issues := settings.Validate(); len(issues) > 0 {
			t.Fatalf("fixture issues = %+v", issues)
		}
		data, err := domain.MarshalLabSettings(settings)
		if err != nil {
			t.Fatal(err)
		}
		writeGitReviewFile(t, repository, "lab-settings.json", string(data))
		workspaceTestGit(t, repository, "add", "lab-settings.json")
		workspaceTestGit(t, repository, "commit", "-qm", message)
	}

	t.Run("an ordinary settings change goes back", func(t *testing.T) {
		repository := newGitReviewRepository(t)
		save(repository, base, "chore: update laboratory settings")
		changed := base
		changed.Lab.PCCount = 4
		save(repository, changed, "chore: update laboratory settings")
		manager := app.NewGitUndoManager(Local{})
		plan := manager.Plan(t.Context(), repository)
		if plan.HasErrors() || len(plan.Impacts) != 2 {
			t.Fatalf("plan = %+v", plan)
		}
		if result := manager.Apply(t.Context(), repository, plan.ReviewToken); result.State != "completed" {
			t.Fatalf("result = %+v", result)
		}
		restored, _ := os.ReadFile(filepath.Join(repository, "lab-settings.json"))
		if decoded, _ := domain.DecodeLabSettings(restored); decoded.Lab.PCCount != 3 {
			t.Fatalf("restored settings = %+v", decoded.Lab)
		}
	})
	t.Run("the start of a change of laboratory network", func(t *testing.T) {
		repository := newGitReviewRepository(t)
		save(repository, base, "chore: update laboratory settings")
		moving := base
		moving.Lab.NetworkBase = "10.1.0.0"
		moving.Lab.NetworkTransition = &domain.NetworkTransition{PreviousBase: base.Lab.NetworkBase, PreviousPrefixLength: base.Lab.NetworkPrefix, StartedAt: "2026-10-15T10:00:00Z"}
		save(repository, moving, "chore: update laboratory settings")
		undoRefusal(t, repository, "Change network addressing")
	})
	t.Run("an address change while computers are configured", func(t *testing.T) {
		repository := newGitReviewRepository(t)
		save(repository, base, "chore: update laboratory settings")
		moved := base
		moved.Lab.NetworkBase = "10.1.0.0"
		save(repository, moved, "hand edit")
		undoRefusal(t, repository, "laboratory network cannot be changed")
	})
	t.Run("a controller-only deployment has no computers to update", func(t *testing.T) {
		repository := newGitReviewRepository(t)
		controller := base
		controller.Lab.DeploymentMode = "controller"
		controller.Lab.PCCount = 0
		save(repository, controller, "chore: update laboratory settings")
		other := controller
		other.Lab.TimeZone = "Europe/Rome"
		save(repository, other, "chore: update laboratory settings")
		plan := app.NewGitUndoManager(Local{}).Plan(t.Context(), repository)
		if plan.HasErrors() || len(plan.Impacts) != 1 || plan.Impacts[0].Kind != "controller-apply" {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("the earlier settings are not valid", func(t *testing.T) {
		// The initial version of the fixture repository holds an invalid
		// lab-settings.json: going back to it is refused, not guessed.
		repository := newGitReviewRepository(t)
		save(repository, base, "chore: update laboratory settings")
		undoRefusal(t, repository, "is not valid")
	})
}

func TestGitUndoFactsReadTheCommitObject(t *testing.T) {
	object := "tree 1111111111111111111111111111111111111111\n" +
		"parent 2222222222222222222222222222222222222222\n" +
		"author A <a@x> 1700000000 +0000\n" +
		"committer C <c@x> 1700000000 -0530\n" +
		"gpgsig -----BEGIN PGP SIGNATURE-----\n \n abc\n -----END PGP SIGNATURE-----\n" +
		"\nchore: update\x1b[31m laboratory settings\n\nbody\n\nThis reverts commit " + strings.Repeat("a", 40) + ".\n"
	facts := parseGitUndoCommit(object)
	if facts.Tree != strings.Repeat("1", 40) || len(facts.Parents) != 1 || facts.Parents[0] != strings.Repeat("2", 40) {
		t.Fatalf("facts = %+v", facts)
	}
	if facts.Subject != "chore: update�[31m laboratory settings" {
		t.Fatalf("subject = %q", facts.Subject)
	}
	if facts.Reverts != strings.Repeat("a", 40) {
		t.Fatalf("reverts = %q", facts.Reverts)
	}
	_, offset := facts.Date.Zone()
	if facts.Date.Unix() != 1700000000 || offset != -(5*3600+30*60) {
		t.Fatalf("date = %v (offset %d)", facts.Date, offset)
	}
}

func TestGitFileAtReadsOneSavedFile(t *testing.T) {
	repository := undoRepository(t)
	head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	content, exists, err := (Local{}).GitFileAt(t.Context(), repository, head, "modules/site.nix")
	if err != nil || !exists || string(content) != "{ ... }: { }\n" {
		t.Fatalf("content = %q, exists = %t, error = %v", content, exists, err)
	}
	parent := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD~1"))
	if _, exists, err := (Local{}).GitFileAt(t.Context(), repository, parent, "modules/site.nix"); err != nil || exists {
		t.Fatalf("exists = %t, error = %v", exists, err)
	}
	if _, _, err := (Local{}).GitFileAt(t.Context(), repository, "HEAD", "modules/site.nix"); err == nil {
		t.Fatal("a symbolic revision was accepted")
	}
}
