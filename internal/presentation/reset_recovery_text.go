package presentation

import (
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The review of an interrupted template reset (or Assistant save) says what
// Nixorium found in the repository, what confirming does, and how to
// confirm: Enter for the three cases that change no file, RESTORE for the
// mixed checkout (ADR 0026).

const (
	resetRecoveryUnreadable   = "The record of the interrupted %s cannot be read. Keep it as it is and collect a support report from Maintenance → Diagnostics."
	resetRecoveryUnblocked    = "No file changes; other operations are unblocked."
	resetRecoveryResultNextUI = "Review the configuration, then apply it to the controller and the computers as usual."
	resetRecoveryShownChanged = 8
)

// resetRecoveryNoun names what was interrupted: the template reset or a
// native Assistant save, which leaves the same marker.
func resetRecoveryNoun(plan domain.TemplateResetRecoveryPlan) string {
	if plan.AssistantSave() {
		return "Assistant save"
	}
	return "reset"
}

func resetRecoveryIssue(plan domain.TemplateResetRecoveryPlan, field string) (domain.ValidationIssue, bool) {
	for _, issue := range plan.Issues {
		if issue.Field == field {
			return issue, true
		}
	}
	return domain.ValidationIssue{}, false
}

// templateResetRecoveryFound states in plain words what the repository
// shows: the first line of the review.
func templateResetRecoveryFound(plan domain.TemplateResetRecoveryPlan) string {
	noun := resetRecoveryNoun(plan)
	switch plan.Case {
	case domain.ResetRecoveryFinished:
		return "The " + noun + " had already finished; only its marker is left."
	case domain.ResetRecoveryCompleteBranch:
		return "Only the branch move is missing: every file already matches the " + noun + "."
	case domain.ResetRecoveryNotStarted:
		return "Nothing was replaced: every file still matches the configuration from before the " + noun + "."
	case domain.ResetRecoveryRestore:
		return "Files from before and after the " + noun + " are mixed."
	}
	return ""
}

// templateResetRecoveryAction says what confirming does.
func templateResetRecoveryAction(plan domain.TemplateResetRecoveryPlan) string {
	noun := resetRecoveryNoun(plan)
	switch plan.Case {
	case domain.ResetRecoveryFinished:
		return "Press Enter to clear the marker. " + resetRecoveryUnblocked
	case domain.ResetRecoveryCompleteBranch:
		return "Press Enter to move the branch to the " + noun + " commit and clear the marker. " + resetRecoveryUnblocked
	case domain.ResetRecoveryNotStarted:
		return "Press Enter to clear the marker and keep the configuration from before the " + noun + ". " + resetRecoveryUnblocked
	case domain.ResetRecoveryRestore:
		commit := ""
		if plan.Head != plan.Original {
			commit = " as a new commit"
		}
		return "RESTORE saves the current files under a recovery reference and puts back the configuration from before the " + noun + commit + "; untracked and ignored files are not removed."
	}
	return ""
}

// templateResetRecoveryEnterLabel names what Enter does in the action bar.
func templateResetRecoveryEnterLabel(plan domain.TemplateResetRecoveryPlan) string {
	switch plan.Case {
	case domain.ResetRecoveryFinished:
		return "Clear the marker"
	case domain.ResetRecoveryCompleteBranch:
		return "Finish the " + resetRecoveryNoun(plan)
	case domain.ResetRecoveryNotStarted:
		return "Keep the previous configuration"
	}
	return "Confirm"
}

// TemplateResetRecoveryQuestion is the yes/no question of the command line
// for the cases that change no file.
func TemplateResetRecoveryQuestion(plan domain.TemplateResetRecoveryPlan) string {
	noun := resetRecoveryNoun(plan)
	switch plan.Case {
	case domain.ResetRecoveryFinished:
		return "Clear the marker of the finished " + noun + "?"
	case domain.ResetRecoveryCompleteBranch:
		return "Move the branch to the " + noun + " commit and clear the marker?"
	}
	return "Keep the configuration from before the " + noun + " and clear the marker?"
}

// templateResetRecoveryBlocked explains a review that cannot be confirmed.
func templateResetRecoveryBlocked(plan domain.TemplateResetRecoveryPlan) string {
	noun := resetRecoveryNoun(plan)
	if plan.State == "unchanged" {
		return "No interrupted " + noun + " is recorded. Nothing needs to be done."
	}
	if _, found := resetRecoveryIssue(plan, "record"); found {
		return fmt.Sprintf(resetRecoveryUnreadable, noun)
	}
	reason := plan.Message
	for _, field := range []string{"git", "repository"} {
		if issue, found := resetRecoveryIssue(plan, field); found {
			reason = issue.Message
			break
		}
	}
	reason = strings.TrimRight(strings.TrimSpace(reason), ".")
	if reason == "" {
		return "Nixorium cannot finish or undo the " + noun + " yet. Press r to check again."
	}
	return "Nixorium cannot finish or undo the " + noun + " yet: " + reason + ". Then press r to check again."
}

// templateResetRecoveryLines is the body of the TUI review.
func templateResetRecoveryLines(plan domain.TemplateResetRecoveryPlan, dark bool) []string {
	if plan.State != "ready" || plan.HasErrors() {
		return []string{templateResetRecoveryBlocked(plan)}
	}
	noun := resetRecoveryNoun(plan)
	lines := []string{templateResetRecoveryFound(plan)}
	if plan.Case == domain.ResetRecoveryRestore && len(plan.Changed) > 0 {
		lines = append(lines, "", fmt.Sprintf("%d file(s) differ from before the %s:", len(plan.Changed), noun))
		for _, name := range plan.Changed[:min(len(plan.Changed), resetRecoveryShownChanged)] {
			lines = append(lines, "  "+name)
		}
		if len(plan.Changed) > resetRecoveryShownChanged {
			lines = append(lines, tuiMuted(fmt.Sprintf("  … and %d more", len(plan.Changed)-resetRecoveryShownChanged), dark))
		}
	}
	if plan.BackupRef != "" {
		lines = append(lines, "", tuiMuted("The configuration from before the "+noun+" is kept in "+plan.BackupRef+".", dark))
	}
	if plan.TypedConfirmation() != "" {
		// The sentence on RESTORE stays next to the prompt (fixed body).
		return lines
	}
	return append(lines, "", templateResetRecoveryAction(plan))
}

func templateResetRecoveryResultHeadline(result domain.TemplateResetRecoveryResult, noun string) string {
	switch result.Case {
	case domain.ResetRecoveryNotStarted:
		return "The " + noun + " is undone; the configuration from before it is kept."
	case domain.ResetRecoveryRestore:
		return "The configuration from before the " + noun + " is restored."
	}
	return "The " + noun + " is finished."
}

func templateResetRecoveryResultLines(result domain.TemplateResetRecoveryResult, noun string, dark bool) []string {
	if result.State != "completed" {
		return []string{tuiResult("The interrupted "+noun+" was not resolved", false, dark), "", result.Message, "",
			"The marker is kept, so other operations stay blocked. Press Enter, then open the review again."}
	}
	lines := []string{tuiResult(templateResetRecoveryResultHeadline(result, noun), true, dark), "Other operations are unblocked. " + resetRecoveryResultNextUI}
	if result.RecoveryRef != "" {
		lines = append(lines, "", tuiMuted("The files found before restoring are kept in "+result.RecoveryRef+".", dark))
	}
	if result.Revision != "" {
		lines = append(lines, tuiMuted("Revision: "+result.Revision, dark))
	}
	if result.Archive != "" {
		lines = append(lines, tuiMuted("The marker is kept in "+result.Archive, dark))
	}
	if _, warning, found := strings.Cut(result.Message, "operation outcome was not recorded: "); found {
		lines = append(lines, tuiMuted("The operation history was not updated: "+warning, dark))
	}
	return lines
}

// TemplateResetRecoveryPlanText is the command line review.
func TemplateResetRecoveryPlanText(w io.Writer, plan domain.TemplateResetRecoveryPlan) {
	noun := resetRecoveryNoun(plan)
	if plan.State != "ready" || plan.HasErrors() {
		fmt.Fprintln(w, templateResetRecoveryBlocked(plan))
		for _, issue := range plan.Issues {
			fmt.Fprintf(w, "  %s: %s\n", issue.Field, issue.Message)
		}
		return
	}
	fmt.Fprintln(w, templateResetRecoveryFound(plan))
	if len(plan.Changed) > 0 {
		fmt.Fprintf(w, "Files that differ from before the %s (%d):\n", noun, len(plan.Changed))
		for _, name := range plan.Changed[:min(len(plan.Changed), 30)] {
			fmt.Fprintf(w, "  %s\n", name)
		}
		if len(plan.Changed) > 30 {
			fmt.Fprintf(w, "  … and %d more\n", len(plan.Changed)-30)
		}
	}
	fmt.Fprintf(w, "Before the %s: %s\n%s commit: %s\nCurrent HEAD: %s\nBackup: %s\n", noun, plan.Original, upperFirstWord(noun), plan.Candidate, plan.Head, plan.BackupRef)
	action := templateResetRecoveryAction(plan)
	if plan.TypedConfirmation() == "" {
		action = strings.TrimPrefix(action, "Press Enter to ")
		action = "Confirming will " + action
	}
	fmt.Fprintln(w, action)
	if plan.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", plan.ReviewToken)
	}
}

// TemplateResetRecoveryResultText reports the outcome of the reviewed plan.
func TemplateResetRecoveryResultText(w io.Writer, plan domain.TemplateResetRecoveryPlan, result domain.TemplateResetRecoveryResult) {
	if result.State != "completed" {
		fmt.Fprintln(w, result.Message)
		fmt.Fprintln(w, "The marker is kept, so other operations stay blocked. Create a fresh review with nixorium template-reset recover plan.")
		return
	}
	fmt.Fprintln(w, templateResetRecoveryResultHeadline(result, resetRecoveryNoun(plan))+" Other operations are unblocked. "+resetRecoveryResultNextUI)
	if result.RecoveryRef != "" {
		fmt.Fprintf(w, "Files found before restoring: %s\n", result.RecoveryRef)
	}
	if result.Revision != "" {
		fmt.Fprintf(w, "Revision: %s\n", result.Revision)
	}
	if result.Archive != "" {
		fmt.Fprintf(w, "Archived marker: %s\n", result.Archive)
	}
	if _, warning, found := strings.Cut(result.Message, "operation outcome was not recorded: "); found {
		fmt.Fprintln(w, "Warning: operation outcome was not recorded: "+warning)
	}
}

func upperFirstWord(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
