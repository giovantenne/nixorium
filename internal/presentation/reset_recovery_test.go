package presentation

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

const resetRefusal = "evaluate deployment: unfinished deployment template reset: inspect .git/nixorium-template-reset.json and follow the deployment reset recovery guide"

// resetRecoveryFixture plans one case of an interrupted template reset.
type resetRecoveryFixture struct {
	kind    string
	plans   int
	applied []domain.TemplateResetRecoveryPlan
	issue   *domain.ValidationIssue
}

func (fixture *resetRecoveryFixture) actions() DashboardActions {
	return DashboardActions{
		PlanResetRecovery: func(context.Context) domain.TemplateResetRecoveryPlan {
			fixture.plans++
			plan := domain.TemplateResetRecoveryPlan{State: "ready", Case: fixture.kind, Confirmation: domain.ResetRecoveryConfirmation, ReviewToken: "sha256:x",
				Original: strings.Repeat("a", 40), Candidate: strings.Repeat("b", 40), Head: strings.Repeat("a", 40),
				BackupRef: "refs/nixorium/template-backups/20261015T150400.000000000Z-aaaaaaaaaaaa", Branch: "refs/heads/main"}
			if fixture.kind == domain.ResetRecoveryRestore {
				plan.Confirmation = domain.ResetRestoreConfirmation
				plan.Head = strings.Repeat("c", 40)
				plan.Changed = []string{"flake.nix", "lab-software.json", "modules/workstation.nix"}
			}
			if fixture.issue != nil {
				plan = domain.TemplateResetRecoveryPlan{State: "blocked", Message: fixture.issue.Message, Issues: []domain.ValidationIssue{*fixture.issue}}
			}
			return plan
		},
		ApplyResetRecovery: func(plan domain.TemplateResetRecoveryPlan) domain.TemplateResetRecoveryResult {
			fixture.applied = append(fixture.applied, plan)
			result := domain.TemplateResetRecoveryResult{State: "completed", Case: plan.Case, Revision: plan.Candidate,
				Archive: "/srv/lab/.git/nixorium-recovered/template-reset-20261015T150500.000000000Z.json"}
			if plan.Case == domain.ResetRecoveryRestore {
				result.Revision = strings.Repeat("d", 40)
				result.RecoveryRef = "refs/nixorium/template-backups/20261015T150500.000000000Z-recovery"
			}
			return result
		},
	}
}

func TestResetRecoveryReviewStatesInPlainWords(t *testing.T) {
	for _, test := range []struct {
		kind, found, action, enter string
	}{
		{domain.ResetRecoveryFinished, "The reset had already finished; only its marker is left.",
			"Press Enter to clear the marker. No file changes; other operations are unblocked.", "Enter Clear the marker"},
		{domain.ResetRecoveryCompleteBranch, "Only the branch move is missing: every file already matches the reset.",
			"Press Enter to move the branch to the reset commit and clear the marker. No file changes; other operations are unblocked.", "Enter Finish the reset"},
		{domain.ResetRecoveryNotStarted, "Nothing was replaced: every file still matches the configuration from before the reset.",
			"Press Enter to clear the marker and keep the configuration from before the reset. No file changes; other operations are unblocked.", "Enter Keep the previous configuration"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			fixture := &resetRecoveryFixture{kind: test.kind}
			model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 160, height: 40}
			model = runRead(model.openResetRecovery())
			view := plainView(model)
			for _, want := range []string{"Finish or undo the reset", test.found, test.action, test.enter, "r Check again", "Esc Back",
				"The configuration from before the reset is kept in refs/nixorium/template-backups/"} {
				if !contains(view, want) {
					t.Fatalf("review omits %q:\n%s", want, view)
				}
			}
			if contains(view, "RECOVERED") || contains(view, "Type ") || contains(view, "Review token") {
				t.Fatalf("review asks for a typed word:\n%s", view)
			}
			// Esc never changes anything.
			if left := cleanupPress(t, model, keyPress("esc")); len(fixture.applied) != 0 || left.screen != dashboardRecovery || !strings.Contains(left.message, "Nothing was changed") {
				t.Fatalf("Esc changed something: applied %d, screen %d", len(fixture.applied), left.screen)
			}
			model = runRead(model.Update(keyPress("r")))
			if fixture.plans != 2 {
				t.Fatalf("r did not check again (%d plans)", fixture.plans)
			}
			model = runRead(model.Update(keyPress("enter")))
			if len(fixture.applied) != 1 || fixture.applied[0].Case != test.kind {
				t.Fatalf("Enter did not confirm (%d applied)", len(fixture.applied))
			}
			view = plainView(model)
			headline := "The reset is finished."
			if test.kind == domain.ResetRecoveryNotStarted {
				headline = "The reset is undone; the configuration from before it is kept."
			}
			for _, want := range []string{headline, "Other operations are unblocked. Review the configuration, then apply it to the controller and the computers as usual.",
				"The marker is kept in /srv/lab/.git/nixorium-recovered/", "Enter Overview"} {
				if !contains(view, want) {
					t.Fatalf("result omits %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestResetRecoveryRestoreKeepsTheTypedWord(t *testing.T) {
	fixture := &resetRecoveryFixture{kind: domain.ResetRecoveryRestore}
	model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 80, height: 24}
	model = runRead(model.openResetRecovery())
	view := plainView(model)
	for _, want := range []string{"Files from before and after the reset are mixed.", "3 file(s) differ from before the reset:", "lab-software.json",
		"RESTORE saves the current files under a recovery reference and puts back the configuration from before the reset as a new commit; untracked and ignored files are not removed.",
		"Type RESTORE to continue:", "Enter Confirm", "Esc Cancel"} {
		if !contains(view, want) {
			t.Fatalf("restore review omits %q:\n%s", want, view)
		}
	}
	if !model.textEntry() {
		t.Fatal("typing RESTORE must not trigger global shortcuts")
	}
	// Enter without the word, or with another word, changes nothing.
	model = cleanupPress(t, model, keyPress("enter"))
	for _, letter := range "RESTOR" {
		model = cleanupPress(t, model, keyPress(string(letter)))
	}
	model = cleanupPress(t, model, keyPress("enter"))
	if len(fixture.applied) != 0 || !strings.Contains(model.message, "did not match") {
		t.Fatalf("a wrong word confirmed (%d applied, %q)", len(fixture.applied), model.message)
	}
	for _, letter := range "RESTORE" {
		model = cleanupPress(t, model, keyPress(string(letter)))
	}
	model = runRead(model.Update(keyPress("enter")))
	model.width, model.height = 160, 40
	view = plainView(model)
	if len(fixture.applied) != 1 || !contains(view, "The configuration from before the reset is restored.") || contains(view, "did not match") ||
		!contains(view, "The files found before restoring are kept in refs/nixorium/template-backups/20261015T150500.000000000Z-recovery.") {
		t.Fatalf("restore result (%d applied):\n%s", len(fixture.applied), view)
	}
}

func TestResetRecoveryReviewExplainsWhatBlocksIt(t *testing.T) {
	for _, test := range []struct {
		issue domain.ValidationIssue
		want  string
	}{
		{domain.ValidationIssue{Field: "git", Message: "Git is in the middle of another operation (index.lock); finish or abort it first"},
			"Nixorium cannot finish or undo the reset yet: Git is in the middle of another operation (index.lock); finish or abort it first. Then press r to check again."},
		{domain.ValidationIssue{Field: "record", Message: "the template reset marker is not valid JSON; keep .git/nixorium-template-reset.json and collect a support report"},
			"The record of the interrupted reset cannot be read. Keep it as it is and collect a support report from Maintenance → Diagnostics."},
	} {
		fixture := &resetRecoveryFixture{kind: domain.ResetRecoveryFinished, issue: &test.issue}
		model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 160, height: 40}
		model = runRead(model.openResetRecovery())
		view := plainView(model)
		if !contains(view, test.want) || contains(view, "Enter ") {
			t.Fatalf("blocked review:\n%s", view)
		}
		model = runRead(model.Update(keyPress("enter")))
		if len(fixture.applied) != 0 || !contains(plainView(model), test.want) {
			t.Fatal("Enter applied a blocked review")
		}
	}
}

func TestResetRecoveryNamesAnInterruptedAssistantSave(t *testing.T) {
	plan := domain.TemplateResetRecoveryPlan{State: "ready", Case: domain.ResetRecoveryNotStarted, BackupRef: "refs/nixorium/assistant-backups/x"}
	lines := strings.Join(templateResetRecoveryLines(plan, false), "\n")
	if !contains(lines, "configuration from before the Assistant save") || contains(lines, " reset") {
		t.Fatalf("Assistant save review:\n%s", lines)
	}
}

func TestUpdateResultOffersTheResetRecovery(t *testing.T) {
	fixture := &resetRecoveryFixture{kind: domain.ResetRecoveryCompleteBranch}
	actions := fixture.actions()
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 160, height: 40, screen: dashboardUpdate,
		updates: updateModel{result: domain.UpdateApplyReport{Operation: "update-apply", State: "failed", Issues: []domain.ValidationIssue{{Field: "repository", Message: resetRefusal}}}},
		message: resetRefusal}
	view := plainView(model)
	for _, want := range []string{
		"! Nixorium was not updated: a reset of the configuration template was interrupted.",
		"  The configuration may be half replaced. The previous version is kept as a backup.",
		"  Next: Finish or undo the reset (Enter)",
		"Enter Finish or undo the reset", "d Details",
	} {
		if !contains(view, want) {
			t.Fatalf("update result omits %q:\n%s", want, view)
		}
	}
	if contains(view, "nixorium-template-reset.json") {
		t.Fatalf("raw text shown by default:\n%s", view)
	}
	if view := plainView(cleanupPress(t, model, keyPress("d"))); !contains(view, "Details: "+resetRefusal) {
		t.Fatalf("d did not reveal the raw text:\n%s", view)
	}
	review := runRead(model.Update(keyPress("enter")))
	if review.screen != dashboardRecoveryReview || review.recoveryReview.kind != "reset" || !contains(plainView(review), "Only the branch move is missing") {
		t.Fatalf("Enter did not open the reset review (screen %d):\n%s", review.screen, plainView(review))
	}
	back := cleanupPress(t, review, keyPress("esc"))
	if back.screen != dashboardUpdate || back.message != resetRefusal || len(fixture.applied) != 0 {
		t.Fatalf("Esc did not return to the update result: screen %d", back.screen)
	}
}
