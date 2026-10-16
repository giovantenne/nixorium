package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func undoTestPlan() domain.GitUndoPlanReport {
	plan := undoReadyPlan()
	plan.Undone.Date = time.Date(2026, 10, 15, 14, 3, 0, 0, time.UTC)
	return plan
}

// openGitReviewFor opens the Git review screen the way Maintenance does.
func openGitReviewFor(t *testing.T, actions DashboardActions, width, height int) dashboardModel {
	t.Helper()
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: width, height: height}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	return updated.(dashboardModel)
}

func undoActions(t *testing.T, plan domain.GitUndoPlanReport, applied *int) DashboardActions {
	t.Helper()
	review := domain.GitReviewReport{Operation: "git-review", State: "clean"}
	return DashboardActions{
		LoadGitReview: func(context.Context) domain.GitReviewReport { return review },
		PlanController: func(context.Context) domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "ready", Controller: "pc99", Revision: strings.Repeat("d", 40), Confirmation: "REBUILD"}
		},
		ApplyController: func(controller domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{State: "completed", Applied: true, Verified: true, Revision: controller.Revision}
		},
		PlanGitUndo: func(context.Context) domain.GitUndoPlanReport { return plan },
		ApplyGitUndo: func(reviewed domain.GitUndoPlanReport) domain.GitUndoReport {
			*applied++
			if reviewed.ReviewToken != plan.ReviewToken {
				t.Fatalf("applied token = %q", reviewed.ReviewToken)
			}
			return domain.GitUndoReport{
				Operation: "git-undo", State: "completed", Committed: true, PreviousRevision: plan.Revision, Revision: strings.Repeat("d", 40),
				Message: "The last saved change was undone and the undo was saved locally. No computer was changed and nothing was pushed.",
				Impacts: plan.Impacts,
			}
		},
	}
}

func pressUndoKey(t *testing.T, model dashboardModel, key tea.KeyPressMsg) (dashboardModel, tea.Cmd) {
	t.Helper()
	updated, command := model.Update(key)
	return updated.(dashboardModel), command
}

func TestDashboardUndoesTheLastSavedChangeAfterReviewAndTypedWord(t *testing.T) {
	applied := 0
	plan := undoTestPlan()
	model := openGitReviewFor(t, undoActions(t, plan, &applied), 120, 40)
	if view := model.View().Content; !strings.Contains(view, "u") || !strings.Contains(view, "Undo the last saved change") {
		t.Fatalf("the undo key is not offered:\n%s", view)
	}

	model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	if command == nil || model.busy == "" {
		t.Fatalf("the review did not start: %+v", model)
	}
	updated, _ := model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if model.screen != dashboardGitUndoReview {
		t.Fatalf("screen = %d", model.screen)
	}
	for _, expected := range []string{
		"Undo the last saved change",
		"Last change: chore: update laboratory software",
		"(saved 2026-10-15 14:03)",
		`"Undo: chore: update laboratory software"`,
		"After saving it:",
		"Apply to controller so this controller uses the configuration saved before that change.",
		"Update computers so the clients use it too; offline computers keep the changed configuration until updated.",
		"modified      lab-software.json",
		"added         modules/site.nix",
		"What the undo changes:",
		"Type UNDO to continue:",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("the undo review lacks %q:\n%s", expected, view)
		}
	}

	// Enter without the word changes nothing.
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil || applied != 0 || !strings.Contains(model.View().Content, "Confirmation did not match; nothing was changed.") {
		t.Fatalf("an empty confirmation was accepted (applied %d):\n%s", applied, model.View().Content)
	}
	// A wrong word changes nothing either.
	model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Text: "undo"})
	if model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter}); command != nil || applied != 0 {
		t.Fatalf("a wrong word was accepted (applied %d)", applied)
	}

	model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Text: "UNDO"})
	if !strings.Contains(model.View().Content, "> UNDO_") {
		t.Fatalf("the typed word is not shown:\n%s", model.View().Content)
	}
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil || model.busy == "" {
		t.Fatalf("the undo did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view = model.View().Content
	if applied != 1 || model.screen != dashboardGitReview {
		t.Fatalf("applied = %d, screen = %d", applied, model.screen)
	}
	for _, expected := range []string{
		"The last saved change was undone",
		"Saved revision:  " + strings.Repeat("d", 40),
		"Next:",
		"1. Apply to controller so this controller uses the configuration saved before that change.",
		"2. Update computers so the clients use it too;",
		"Refresh review",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("the undo result lacks %q:\n%s", expected, view)
		}
	}
	if model.controllerObservation != controllerOverviewPending {
		t.Fatal("the Overview was not told that a new saved version waits for the controller")
	}
	if !strings.Contains(view, "Enter") || !strings.Contains(view, "Apply to this controller") || !strings.Contains(view, "Esc") || !strings.Contains(view, "Maintenance") {
		t.Fatalf("the result does not offer the controller review and the way back:\n%s", view)
	}
	// Esc returns to Maintenance without starting anything.
	if back, command := pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape}); command != nil || back.screen != dashboardAdministration {
		t.Fatalf("Esc did not return to Maintenance: screen = %d", back.screen)
	}
}

func TestUndoResultContinuesToTheOrdinaryControllerReview(t *testing.T) {
	applied := 0
	model := openGitReviewFor(t, undoActions(t, undoTestPlan(), &applied), 120, 40)
	model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	updated, _ := model.Update(command())
	model = updated.(dashboardModel)
	model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Text: "UNDO"})
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)

	// Enter reviews the saved revision for this controller; the undo result
	// stays behind it.
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil || model.busy == "" || !model.controller.fromSave || model.controller.saveOrigin != dashboardGitReview {
		t.Fatalf("the controller review did not start: busy %q, fromSave %t", model.busy, model.controller.fromSave)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if model.screen != dashboardControllerReview || !strings.Contains(view, "Apply the saved configuration to this controller?") || !strings.Contains(view, "Press Enter to build, activate, and verify this controller.") {
		t.Fatalf("the ordinary Apply to controller review did not open (screen %d):\n%s", model.screen, view)
	}
	if applied != 1 {
		t.Fatalf("the undo ran %d times", applied)
	}
	// Esc leaves the review untouched and returns to the undo result, whose
	// numbered next steps are still there; Esc again returns to Maintenance.
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if command != nil || model.screen != dashboardGitReview || model.controller.fromSave || model.maintenance.gitUndoResult.Operation != "git-undo" {
		t.Fatalf("Esc did not return to the undo result: screen %d", model.screen)
	}
	if view := model.View().Content; !strings.Contains(view, "1. Apply to controller") || !strings.Contains(view, "2. Update computers") {
		t.Fatalf("the next steps were lost:\n%s", view)
	}
	model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.screen != dashboardAdministration {
		t.Fatalf("Esc did not return to Maintenance: screen = %d", model.screen)
	}
}

func TestUndoResultEnterReturnsWhenThereIsNothingToChainTo(t *testing.T) {
	applied := 0
	actions := undoActions(t, undoTestPlan(), &applied)
	actions.PlanController, actions.ApplyController = nil, nil
	model := openGitReviewFor(t, actions, 120, 40)
	model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	updated, _ := model.Update(command())
	model = updated.(dashboardModel)
	model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Text: "UNDO"})
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if view := model.View().Content; strings.Contains(view, "Apply to this controller") {
		t.Fatalf("a controller review is offered without its callbacks:\n%s", view)
	}
	if model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter}); command != nil || model.screen != dashboardAdministration {
		t.Fatalf("Enter did not return to Maintenance: screen = %d", model.screen)
	}
}

func TestDashboardUndoCancelsWithEscAndChangesNothing(t *testing.T) {
	applied := 0
	model := openGitReviewFor(t, undoActions(t, undoTestPlan(), &applied), 120, 40)
	model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	updated, _ := model.Update(command())
	model = updated.(dashboardModel)
	model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Text: "UNDO"})
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if command != nil || applied != 0 || model.screen != dashboardGitReview || model.confirmation != "" {
		t.Fatalf("cancel: applied %d, screen %d, confirmation %q", applied, model.screen, model.confirmation)
	}
	if !strings.Contains(model.View().Content, "Undo cancelled; no repository state changed.") {
		t.Fatalf("view:\n%s", model.View().Content)
	}
}

func TestDashboardUndoRefusalNamesTheNextStepAndCannotBeConfirmed(t *testing.T) {
	applied := 0
	dirty := undoTestPlan()
	dirty.State, dirty.ReviewToken, dirty.TreeID, dirty.Confirmation = "blocked", "", "", ""
	dirty.Impacts, dirty.Diff = nil, domain.GitDiff{}
	dirty.Issues = []domain.ValidationIssue{domain.BlockedIssue("git", "GIT-DIRTY", "deployment worktree has 2 changed path(s); save or inspect them in Maintenance → Review Git changes (`nixorium git review`)")}
	model := openGitReviewFor(t, undoActions(t, dirty, &applied), 120, 40)
	model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	updated, _ := model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	for _, expected := range []string{"The last saved change was not undone; nothing was changed.", "The configuration has unsaved changes", "Next: Review the changes and save or discard them", "Review again"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("the refusal lacks %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "Type UNDO") {
		t.Fatalf("a refusal can be confirmed:\n%s", view)
	}
	for _, key := range []tea.KeyPressMsg{{Text: "U"}, {Text: "N"}, {Text: "D"}, {Text: "O"}, {Code: tea.KeyEnter}} {
		model, command = pressUndoKey(t, model, key)
		if applied != 0 || command != nil && key.Code != tea.KeyEnter {
			t.Fatalf("a refusal started the undo (applied %d)", applied)
		}
	}
	if model.screen != dashboardGitReview {
		t.Fatalf("Enter did not leave the refusal: screen = %d", model.screen)
	}

	// A refusal without a catalogued step says what to do in its own words.
	merge := undoTestPlan()
	merge.State, merge.ReviewToken = "blocked", ""
	merge.Issues = []domain.ValidationIssue{{Field: "revision", Message: "The last saved change is a merge of two histories. Only an ordinary saved change can be undone here; review the merge with Git by hand."}}
	model = openGitReviewFor(t, undoActions(t, merge, &applied), 120, 40)
	model, command = pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	updated, _ = model.Update(command())
	if view := updated.(dashboardModel).View().Content; !strings.Contains(view, "merge with Git by hand") {
		t.Fatalf("the refusal text is missing:\n%s", view)
	}
}

func TestDashboardUndoIsOfferedOnlyWithItsCallbacks(t *testing.T) {
	review := domain.GitReviewReport{Operation: "git-review", State: "clean"}
	actions := DashboardActions{LoadGitReview: func(context.Context) domain.GitReviewReport { return review }}
	model := openGitReviewFor(t, actions, 120, 40)
	if strings.Contains(model.View().Content, "Undo the last saved change") {
		t.Fatalf("the undo is offered without callbacks:\n%s", model.View().Content)
	}
	model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
	if command != nil || model.screen != dashboardGitReview || model.busy != "" {
		t.Fatalf("u did something without callbacks: %+v", model)
	}
}

func TestDashboardUndoReviewFitsSmallTerminalsAndScrolls(t *testing.T) {
	plan := undoTestPlan()
	var files []domain.GitUndoFile
	for index := 0; index < 30; index++ {
		files = append(files, domain.GitUndoFile{Path: "modules/site-" + strings.Repeat("x", index) + ".nix", Change: "added"})
	}
	plan.Undone.Files = files
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		applied := 0
		model := openGitReviewFor(t, undoActions(t, plan, &applied), size[0], size[1])
		model, command := pressUndoKey(t, model, tea.KeyPressMsg{Text: "u"})
		updated, _ := model.Update(command())
		model = updated.(dashboardModel)
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("the undo review does not fit %v (%dx%d):\n%s", size, lipgloss.Width(view), lipgloss.Height(view), view)
		}
		if !strings.Contains(view, "Type UNDO to continue:") || !strings.Contains(view, "Update computers so the clients use it too") && size[0] >= 120 {
			t.Fatalf("the confirmation or the next steps are hidden at %v:\n%s", size, view)
		}
		before := model.maintenance.gitScroll
		for range 40 {
			model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
		}
		if model.maintenance.gitScroll <= before || model.maintenance.gitScroll > model.maximumGitUndoScroll() {
			t.Fatalf("scroll = %d (maximum %d)", model.maintenance.gitScroll, model.maximumGitUndoScroll())
		}
		end := model.View().Content
		if !strings.Contains(end, "+  \"packages\": []") || lipgloss.Height(end) > size[1] {
			t.Fatalf("the end of the patch is unreachable at %v:\n%s", size, end)
		}
		for range 60 {
			model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
		}
		start := model.View().Content
		if !strings.Contains(start, "Last change: chore: update laboratory software") || !strings.Contains(start, "Update computers so the clients use it too") || !strings.Contains(start, "Type UNDO to continue:") {
			t.Fatalf("the review lost its header at %v:\n%s", size, start)
		}
		// The window opens on the summary of a short terminal, or on the
		// files of a tall one; the files are one scroll away.
		first := "Files in that change:"
		if size[1] < 30 {
			first = "The undo saves one new change"
		}
		if !strings.Contains(start, first) {
			t.Fatalf("the start of the scrolling part is unreachable at %v:\n%s", size, start)
		}
		for range 3 {
			model, _ = pressUndoKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
		}
		if middle := model.View().Content; !strings.Contains(middle, "added") || !strings.Contains(middle, "modules/site-") {
			t.Fatalf("the files are unreachable at %v:\n%s", size, middle)
		}
	}
}

func TestGitReviewHelpMentionsTheUndo(t *testing.T) {
	applied := 0
	model := openGitReviewFor(t, undoActions(t, undoTestPlan(), &applied), 160, 50)
	model.helpOpen = true
	view := model.View().Content
	for _, expected := range []string{"u undo the last saved change", "UNDO saves a new change", "Enter on the result opens Apply to controller; then update the computers"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("help lacks %q:\n%s", expected, view)
		}
	}
}
