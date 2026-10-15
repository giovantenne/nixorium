package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeGitUndoSource struct {
	revision    string
	snapshot    domain.GitReviewSnapshot
	facts       domain.GitUndoFacts
	factsErr    error
	files       map[string]map[string][]byte
	proposalErr error
	undoErr     error
	// advance moves HEAD when the undo fails, as a partial write would.
	advance string

	undone        int
	undoneFacts   domain.GitUndoFacts
	undoneTree    string
	undoneMessage string
}

func (source *fakeGitUndoSource) GitRevision(context.Context, string) (string, error) {
	return source.revision, nil
}

func (source *fakeGitUndoSource) GitReview(context.Context, string) (domain.GitReviewSnapshot, error) {
	return source.snapshot, nil
}

func (source *fakeGitUndoSource) GitUndoFacts(context.Context, string) (domain.GitUndoFacts, error) {
	return source.facts, source.factsErr
}

func (source *fakeGitUndoSource) GitFileAt(_ context.Context, _, revision, path string) ([]byte, bool, error) {
	content, exists := source.files[revision][path]
	return content, exists, nil
}

func (source *fakeGitUndoSource) GitUndoProposal(_ context.Context, _ string, facts domain.GitUndoFacts) (domain.GitUndoProposal, error) {
	if source.proposalErr != nil {
		return domain.GitUndoProposal{}, source.proposalErr
	}
	return domain.GitUndoProposal{TreeID: facts.ParentTree, Diff: domain.GitDiff{Scope: "undo-changes", Content: "-new\n+old\n"}}, nil
}

func (source *fakeGitUndoSource) UndoGitCommit(_ context.Context, _ string, facts domain.GitUndoFacts, tree, message string) (string, error) {
	source.undone++
	source.undoneFacts, source.undoneTree, source.undoneMessage = facts, tree, message
	if source.undoErr != nil {
		if source.advance != "" {
			source.revision = source.advance
		}
		return "", source.undoErr
	}
	source.revision = strings.Repeat("d", 40)
	return source.revision, nil
}

var (
	undoHead   = strings.Repeat("a", 40)
	undoParent = strings.Repeat("b", 40)
)

const undoSoftware = "{\n  \"schemaVersion\": 1,\n  \"packages\": []\n}\n"

func undoFixture() *fakeGitUndoSource {
	return &fakeGitUndoSource{
		revision: undoHead,
		facts: domain.GitUndoFacts{
			Revision: undoHead, Parents: []string{undoParent},
			Tree: strings.Repeat("1", 40), ParentTree: strings.Repeat("2", 40),
			Branch: "master", Subject: "chore: update laboratory software",
			Date:  time.Date(2026, 10, 15, 14, 3, 0, 0, time.UTC),
			Files: []domain.GitUndoFile{{Path: "lab-software.json", Change: "modified"}, {Path: "modules/site.nix", Change: "added"}},
		},
		files: map[string]map[string][]byte{undoParent: {"lab-software.json": []byte(undoSoftware)}},
	}
}

func TestGitUndoPlanReviewsTheLastSavedChange(t *testing.T) {
	source := undoFixture()
	manager := NewGitUndoManager(source)
	plan := manager.Plan(t.Context(), ".")
	if plan.HasErrors() || plan.State != "ready" || plan.Operation != "git-undo-plan" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Revision != undoHead || plan.Undone.Parent != undoParent || plan.Undone.Subject != "chore: update laboratory software" || len(plan.Undone.Files) != 2 {
		t.Fatalf("undone change = %+v", plan.Undone)
	}
	if plan.Title() != "Undo: chore: update laboratory software" || !strings.Contains(plan.CommitMessage, "This reverts commit "+undoHead+".") {
		t.Fatalf("message = %q", plan.CommitMessage)
	}
	if plan.TreeID != source.facts.ParentTree || plan.Confirmation != "UNDO" || !strings.HasPrefix(plan.ReviewToken, "sha256:") {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Impacts) != 2 || plan.Impacts[0].Kind != "controller-apply" || !strings.HasPrefix(plan.Impacts[0].Detail, "Apply to controller") ||
		plan.Impacts[1].Kind != "client-update" || !strings.HasPrefix(plan.Impacts[1].Detail, "Update computers") {
		t.Fatalf("impacts = %+v", plan.Impacts)
	}
	if again := manager.Plan(t.Context(), "."); again.ReviewToken != plan.ReviewToken {
		t.Fatal("the review token is not stable")
	}
	source.facts.Revision = strings.Repeat("c", 40)
	if other := manager.Plan(t.Context(), "."); other.ReviewToken == plan.ReviewToken {
		t.Fatal("the review token does not bind the reviewed change")
	}
}

func TestGitUndoMessageNamesTheSubjectAndStaysBounded(t *testing.T) {
	facts := undoFixture().facts
	facts.Subject = strings.Repeat("é", 300)
	title, _, _ := strings.Cut(gitUndoMessage(facts), "\n")
	if want := "Undo: " + strings.Repeat("é", 120) + "…"; title != want {
		t.Fatalf("title = %q", title)
	}
	facts.Subject = ""
	if title, _, _ := strings.Cut(gitUndoMessage(facts), "\n"); title != "Undo: saved change aaaaaaa" {
		t.Fatalf("title = %q", title)
	}
}

func TestGitUndoRefusesWhatIsNotAnOrdinarySavedChange(t *testing.T) {
	cases := []struct {
		name   string
		change func(*domain.GitUndoFacts)
		want   string
	}{
		{"unfinished operation", func(f *domain.GitUndoFacts) { f.InProgress = "rebase" }, "A Git rebase is unfinished"},
		{"detached HEAD", func(f *domain.GitUndoFacts) { f.Branch = "" }, "not on a branch"},
		{"first saved version", func(f *domain.GitUndoFacts) { f.Parents = nil }, "first saved version"},
		{"merge", func(f *domain.GitUndoFacts) { f.Parents = append(f.Parents, strings.Repeat("c", 40)) }, "merge of two histories"},
		{"an undo", func(f *domain.GitUndoFacts) { f.Reverts = strings.Repeat("e", 40) }, "already undoes an earlier one"},
		{"too many files", func(f *domain.GitUndoFacts) { f.Truncated = true }, "more than 1000 files"},
		{"no files", func(f *domain.GitUndoFacts) { f.Files = nil }, "no file changes"},
		{"Nixorium update", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "flake.lock", Change: "modified"}, {Path: "flake.nix", Change: "modified"}}
		}, "UPDATES.md"},
		{"package base update", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "flake.lock", Change: "modified"}}
		}, "Recovery and long-term autonomy"},
		{"public keys", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "keys/cache-public-key", Change: "added"}}
		}, "Keys are missing or inconsistent"},
		{"recovery file", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "nixorium-recovery.age", Change: "modified"}}
		}, "encrypted recovery file"},
		{"private file", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "secret-key", Change: "added"}}
		}, "private key or credentials file"},
		{"attribute rules", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "modules/.gitattributes", Change: "added"}}
		}, "Git attribute rules"},
		{"symbolic link", func(f *domain.GitUndoFacts) {
			f.Files = []domain.GitUndoFile{{Path: "link", Change: "added", Special: true}}
		}, "symbolic link"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source := undoFixture()
			test.change(&source.facts)
			manager := NewGitUndoManager(source)
			plan := manager.Plan(t.Context(), ".")
			if !plan.HasErrors() || plan.State != "blocked" || plan.ReviewToken != "" || plan.TreeID != "" {
				t.Fatalf("the undo was not refused: %+v", plan)
			}
			var messages []string
			for _, issue := range plan.Issues {
				messages = append(messages, issue.Message)
			}
			if !strings.Contains(strings.Join(messages, "\n"), test.want) {
				t.Fatalf("refusal = %q, want %q", messages, test.want)
			}
			if result := manager.Apply(t.Context(), ".", "sha256:any"); result.State != "blocked" || result.Committed || source.undone != 0 {
				t.Fatalf("a refused undo was saved: %+v", result)
			}
		})
	}
}

func TestGitUndoRefusesUnsavedChangesWithTheExistingNextStep(t *testing.T) {
	source := undoFixture()
	source.snapshot = domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "notes.txt", Untracked: true}, {Path: "module.nix", Unstaged: "modified"}}}
	plan := NewGitUndoManager(source).Plan(t.Context(), ".")
	if !plan.HasErrors() || len(plan.Issues) != 1 || plan.Issues[0].Next == nil || plan.Issues[0].Next.Code != "GIT-DIRTY" || !strings.Contains(plan.Issues[0].Message, "2 changed path(s)") {
		t.Fatalf("plan = %+v", plan)
	}
	private := undoFixture()
	private.snapshot = domain.GitReviewSnapshot{Changes: []domain.GitChange{{Path: "secret-key", Untracked: true, Private: true}}}
	if plan := NewGitUndoManager(private).Plan(t.Context(), "."); !plan.HasErrors() || plan.State != "blocked" {
		t.Fatalf("private paths were not refused: %+v", plan)
	}
}

func TestGitUndoRefusesWhenTheFactsCannotBeRead(t *testing.T) {
	source := undoFixture()
	source.factsErr = errors.New("repository is damaged")
	if plan := NewGitUndoManager(source).Plan(t.Context(), "."); !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "repository is damaged") {
		t.Fatalf("plan = %+v", plan)
	}
	source.factsErr = domain.Blocked("RESET-PENDING", errors.New(domain.MessageResetPending+": recover first"))
	plan := NewGitUndoManager(source).Plan(t.Context(), ".")
	if !plan.HasErrors() || plan.Issues[0].Next == nil || plan.Issues[0].Next.Code != "RESET-PENDING" {
		t.Fatalf("plan = %+v", plan)
	}
	source = undoFixture()
	source.proposalErr = errors.New("\"old.nix\" exists in this folder but is not saved in Git")
	plan = NewGitUndoManager(source).Plan(t.Context(), ".")
	if !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "Use Git by hand") || !strings.Contains(plan.Issues[0].Message, "old.nix") {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestGitUndoChecksTheEarlierVersionOfManagedFiles(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		source := undoFixture()
		source.files[undoParent]["lab-software.json"] = []byte(`{"schemaVersion":1,"packages":[{"package":"vlc"}]}`)
		plan := NewGitUndoManager(source).Plan(t.Context(), ".")
		if !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "lab-software.json as it was before that change is not valid") {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("removed by the undo", func(t *testing.T) {
		source := undoFixture()
		delete(source.files[undoParent], "lab-software.json")
		plan := NewGitUndoManager(source).Plan(t.Context(), ".")
		if !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "would remove lab-software.json") {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("other files are not read", func(t *testing.T) {
		source := undoFixture()
		source.facts.Files = []domain.GitUndoFile{{Path: "modules/site.nix", Change: "modified"}}
		source.files = nil
		if plan := NewGitUndoManager(source).Plan(t.Context(), "."); plan.HasErrors() {
			t.Fatalf("plan = %+v", plan)
		}
	})
}

func undoSettingsSource(t *testing.T, current, after domain.LabSettingsFile) *fakeGitUndoSource {
	t.Helper()
	source := undoFixture()
	source.facts.Files = []domain.GitUndoFile{{Path: "lab-settings.json", Change: "modified"}}
	encode := func(settings domain.LabSettingsFile) []byte {
		data, err := domain.MarshalLabSettings(settings)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	source.files = map[string]map[string][]byte{
		undoHead:   {"lab-settings.json": encode(current)},
		undoParent: {"lab-settings.json": encode(after)},
	}
	return source
}

func TestGitUndoKeepsTheLaboratoryNetworkInItsGuidedChange(t *testing.T) {
	_, base := networkSettingsFixture(t)
	t.Run("an ordinary settings change goes back", func(t *testing.T) {
		current := base
		current.Lab.PCCount = 4
		plan := NewGitUndoManager(undoSettingsSource(t, current, base)).Plan(t.Context(), ".")
		if plan.HasErrors() || len(plan.Impacts) != 2 {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("the start of a change of laboratory network", func(t *testing.T) {
		moving := base
		moving.Lab.NetworkBase = "10.1.0.0"
		moving.Lab.NetworkTransition = &domain.NetworkTransition{PreviousBase: base.Lab.NetworkBase, PreviousPrefixLength: base.Lab.NetworkPrefix, StartedAt: "2026-10-15T10:00:00Z"}
		plan := NewGitUndoManager(undoSettingsSource(t, moving, base)).Plan(t.Context(), ".")
		if !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "Maintenance → Change network addressing") || !strings.Contains(plan.Issues[0].Message, "nixorium network migrate status") {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("the end of a change of laboratory network", func(t *testing.T) {
		moving := base
		moving.Lab.NetworkBase = "10.1.0.0"
		moving.Lab.NetworkTransition = &domain.NetworkTransition{PreviousBase: base.Lab.NetworkBase, PreviousPrefixLength: base.Lab.NetworkPrefix, StartedAt: "2026-10-15T10:00:00Z"}
		finished := moving
		finished.Lab.NetworkTransition = nil
		plan := NewGitUndoManager(undoSettingsSource(t, finished, moving)).Plan(t.Context(), ".")
		if !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "Change network addressing") {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("an address change while computers are configured", func(t *testing.T) {
		moved := base
		moved.Lab.NetworkBase = "10.1.0.0"
		plan := NewGitUndoManager(undoSettingsSource(t, moved, base)).Plan(t.Context(), ".")
		if !plan.HasErrors() || plan.Issues[0].Next == nil || plan.Issues[0].Next.Code != "NETWORK-LOCKED" {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("settings that cannot be read are repaired by going back", func(t *testing.T) {
		source := undoSettingsSource(t, base, base)
		source.files[undoHead]["lab-settings.json"] = []byte(`{"schemaVersion":1`)
		plan := NewGitUndoManager(source).Plan(t.Context(), ".")
		if plan.HasErrors() || len(plan.Impacts) != 2 {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("a controller-only deployment has no computers to update", func(t *testing.T) {
		controller := base
		controller.Lab.DeploymentMode = "controller"
		controller.Lab.PCCount = 0
		changed := controller
		changed.Lab.TimeZone = "Europe/Rome"
		plan := NewGitUndoManager(undoSettingsSource(t, changed, controller)).Plan(t.Context(), ".")
		if plan.HasErrors() || len(plan.Impacts) != 1 || plan.Impacts[0].Kind != "controller-apply" {
			t.Fatalf("plan = %+v", plan)
		}
		// Leaving controller-only mode brings computers back.
		plan = NewGitUndoManager(undoSettingsSource(t, controller, base)).Plan(t.Context(), ".")
		if plan.HasErrors() || len(plan.Impacts) != 2 {
			t.Fatalf("plan = %+v", plan)
		}
	})
}

func TestGitUndoApplyRequiresTheReviewToken(t *testing.T) {
	source := undoFixture()
	manager := NewGitUndoManager(source)
	plan := manager.Plan(t.Context(), ".")
	for _, token := range []string{"", "sha256:other"} {
		result := manager.Apply(t.Context(), ".", token)
		if result.State != "blocked" || result.Committed || source.undone != 0 || !result.RetrySafe {
			t.Fatalf("token %q: result = %+v", token, result)
		}
		if len(result.Issues) != 1 || !strings.Contains(result.Issues[0].Message, "token does not match") {
			t.Fatalf("issues = %+v", result.Issues)
		}
	}
	result := manager.Apply(t.Context(), ".", plan.ReviewToken)
	if result.State != "completed" || !result.Committed || result.RetrySafe || result.Operation != "git-undo" || result.HasErrors() {
		t.Fatalf("result = %+v", result)
	}
	if source.undone != 1 || source.undoneTree != source.facts.ParentTree || source.undoneMessage != plan.CommitMessage || source.undoneFacts.Revision != undoHead {
		t.Fatalf("undo call = %d %q %q %+v", source.undone, source.undoneTree, source.undoneMessage, source.undoneFacts)
	}
	if result.PreviousRevision != undoHead || result.Revision != strings.Repeat("d", 40) || len(result.Impacts) != 2 || result.Title() != plan.Title() {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Message, "No computer was changed") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestGitUndoApplyReportsFailureAndPartialWrites(t *testing.T) {
	source := undoFixture()
	manager := NewGitUndoManager(source)
	plan := manager.Plan(t.Context(), ".")
	source.undoErr = errors.New("HEAD changed after review")
	result := manager.Apply(t.Context(), ".", plan.ReviewToken)
	if result.State != "failed" || result.Committed || !result.RetrySafe || !result.HasErrors() {
		t.Fatalf("result = %+v", result)
	}
	source.undoErr = errors.New("the undo was saved but the files could not be moved back")
	source.advance = strings.Repeat("d", 40)
	result = manager.Apply(t.Context(), ".", plan.ReviewToken)
	if result.State != "partial" || !result.Committed || result.RetrySafe || result.Revision != source.advance || !strings.Contains(result.Message, "nixorium git review") {
		t.Fatalf("result = %+v", result)
	}
}

func TestGitUndoOutcomeIsRecordedLikeOtherReviewedCommits(t *testing.T) {
	record, ok := operationRecordFor(domain.GitUndoReport{Operation: "git-undo", State: "completed", Committed: true})
	if !ok || record.Operation != "git-undo" || record.State != "completed" || !strings.Contains(record.Summary, "committed=true") {
		t.Fatalf("record = %+v, ok = %t", record, ok)
	}
}
