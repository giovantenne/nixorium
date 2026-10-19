package presentation

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func undoReadyPlan() domain.GitUndoPlanReport {
	return domain.GitUndoPlanReport{
		State: "ready", Operation: "git-undo-plan", Repository: "/srv/lab", Revision: strings.Repeat("a", 40),
		Undone: domain.GitUndoCommit{
			Revision: strings.Repeat("a", 40), Subject: "chore: update laboratory software",
			Date:  time.Date(2026, 10, 15, 14, 3, 0, 0, time.UTC),
			Files: []domain.GitUndoFile{{Path: "lab-software.json", Change: "modified"}, {Path: "modules/site.nix", Change: "added"}},
		},
		CommitMessage: "Undo: chore: update laboratory software\n\nThis reverts commit " + strings.Repeat("a", 40) + ".\n",
		ReviewToken:   "sha256:token",
		Diff:          domain.GitDiff{Scope: "undo-changes", Content: "-  \"packages\": [vlc]\n+  \"packages\": []"},
		Impacts:       domain.GitUndoImpacts(true),
	}
}

func TestGitUndoPlanTextShowsWhatWillBeUndoneAndWhatFollows(t *testing.T) {
	var output bytes.Buffer
	GitUndoPlanText(&output, undoReadyPlan())
	text := output.String()
	for _, expected := range []string{
		"Undo the last saved change: READY",
		"Last change:  chore: update laboratory software",
		"Saved:        2026-10-15 14:03",
		"Revision:     " + strings.Repeat("a", 40),
		"modified      lab-software.json",
		"added         modules/site.nix",
		`"Undo: chore: update laboratory software"`,
		"The undone change stays in the history. No computer changes and nothing is pushed.",
		"What the undo changes:",
		"+  \"packages\": []\n",
		"After saving it:",
		"  - Apply to controller so this controller uses the configuration saved before that change.",
		"  - Update computers so the clients use it too; offline computers keep the changed configuration until updated.",
		"Review token: sha256:token",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("plan text lacks %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Blocked") {
		t.Fatalf("a ready plan reads as blocked:\n%s", text)
	}
}

func TestGitUndoPlanTextForARefusalShowsOnlyTheReason(t *testing.T) {
	plan := undoReadyPlan()
	plan.State = "blocked"
	plan.ReviewToken = ""
	plan.Issues = []domain.ValidationIssue{{Field: "revision", Message: "The last saved change is a merge of two histories."}}
	var output bytes.Buffer
	GitUndoPlanText(&output, plan)
	text := output.String()
	if !strings.Contains(text, "Undo the last saved change: BLOCKED") || !strings.Contains(text, "Blocked: The last saved change is a merge of two histories.") {
		t.Fatalf("text:\n%s", text)
	}
	for _, unexpected := range []string{"Review token", "After saving it", "What the undo changes", "Files in that change"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("a refusal offers %q:\n%s", unexpected, text)
		}
	}
}

func TestGitUndoResultTextNamesTheFollowingSteps(t *testing.T) {
	var output bytes.Buffer
	GitUndoResultText(&output, domain.GitUndoReport{
		State: "completed", Committed: true, PreviousRevision: strings.Repeat("a", 40), Revision: strings.Repeat("d", 40),
		Message: "The last saved change was undone and the undo was saved locally. No computer was changed and nothing was pushed.",
		Impacts: domain.GitUndoImpacts(true),
	})
	text := output.String()
	for _, expected := range []string{"was undone and the undo was saved locally", "Saved revision:  " + strings.Repeat("d", 40), "Next:", "  1. Apply to controller", "  2. Update computers"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("result text lacks %q:\n%s", expected, text)
		}
	}
	output.Reset()
	GitUndoResultText(&output, domain.GitUndoReport{State: "blocked", Message: "Nothing was undone; create a fresh review.", Issues: []domain.ValidationIssue{{Message: "review token does not match the current last saved change"}}})
	if strings.Contains(output.String(), "Next:") || !strings.Contains(output.String(), "Blocked: review token does not match") {
		t.Fatalf("blocked result:\n%s", output.String())
	}
}

func TestGitUndoAsksYesNo(t *testing.T) {
	for input, expected := range map[string]bool{"y\n": true, "yes\n": true, "  y  \n": true, "\n": false, "n\n": false, "UNDO\n": false} {
		var output bytes.Buffer
		approved, err := ConfirmGitUndo(strings.NewReader(input), &output, undoReadyPlan())
		if err != nil || approved != expected {
			t.Fatalf("input %q: approved = %t, err = %v", input, approved, err)
		}
		for _, line := range []string{"Last change: chore: update laboratory software (saved 2026-10-15 14:03)", "Files: lab-software.json, modules/site.nix", "Continue? [y/N] "} {
			if !strings.Contains(output.String(), line) {
				t.Fatalf("confirmation lacks %q:\n%s", line, output.String())
			}
		}
	}
	blocked := undoReadyPlan()
	blocked.State = "blocked"
	if approved, _ := ConfirmGitUndo(strings.NewReader("y\n"), &bytes.Buffer{}, blocked); approved {
		t.Fatal("a blocked review was confirmed")
	}
}
