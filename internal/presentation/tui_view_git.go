package presentation

import (
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) gitReviewView() string {
	if model.screen == dashboardGitCommitSelect {
		return model.gitCommitSelectView()
	}
	if model.screen == dashboardGitCommitReview {
		return model.gitCommitReviewView()
	}
	if model.screen == dashboardGitUndoReview {
		return model.gitUndoReviewView()
	}
	path := []string{"Maintenance", "Changes"}
	backLabel := "Maintenance"
	if model.maintenance.gitFromHomeFiles {
		path = []string{"Settings", "Student workspace", "Initial files", "Changes"}
		backLabel = "Initial files"
	}
	if model.maintenance.gitFromWorkspace {
		path = []string{"Settings", "Student workspace", "Changes"}
		backLabel = "Workspace"
	}
	lines := []string{tuiTitle("Repository changes", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.maintenance.gitUndoResult.Operation != "" {
		return model.gitUndoResultView(path, backLabel)
	}
	if model.maintenance.gitCommitResult.Operation != "" {
		success := !model.maintenance.gitCommitResult.HasErrors() && model.maintenance.gitCommitResult.Committed
		title := "Git commit needs attention"
		if success {
			title = "Git changes committed locally"
		}
		if model.maintenance.gitCommitResult.Operation == "git-discard" {
			success = !model.maintenance.gitCommitResult.HasErrors()
			title = "Discard needs attention"
			if success {
				title = "Changes discarded; the files match the last saved version"
			}
		}
		returnLabel := backLabel
		if model.setupMode {
			returnLabel = "Setup"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Committed: %t", model.maintenance.gitCommitResult.State, model.maintenance.gitCommitResult.Committed),
			"HEAD: "+model.maintenance.gitCommitResult.Revision,
		)
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Enter", label: returnLabel}, {key: "r", label: "Refresh review"}, {key: "F1", label: "Help"}}})
	}
	content := gitReviewContentLines(model.maintenance.gitReview)
	height := model.gitReviewHeight()
	maximum := maximumGitReviewScroll(model.maintenance.gitReview, height)
	if model.maintenance.gitScroll > maximum {
		model.maintenance.gitScroll = maximum
	}
	end := model.maintenance.gitScroll + height
	if end > len(content) {
		end = len(content)
	}
	lines = append(lines,
		fmt.Sprintf("State: %s   paths: %d   staged/unstaged/untracked: %d/%d/%d", tuiStatus(model.maintenance.gitReview.State, gitReviewStatusKind(model.maintenance.gitReview), model.isDark), len(model.maintenance.gitReview.Changes), model.maintenance.gitReview.Summary.Staged, model.maintenance.gitReview.Summary.Unstaged, model.maintenance.gitReview.Summary.Untracked),
		fmt.Sprintf("Managed/unexpected/private: %d/%d/%d", model.maintenance.gitReview.Summary.Managed, model.maintenance.gitReview.Summary.Unexpected, model.maintenance.gitReview.Summary.Private),
		fmt.Sprintf("Showing lines %d-%d of %d", displayedLineStart(model.maintenance.gitScroll, len(content)), end, len(content)),
		"",
	)
	lines = append(lines, content[model.maintenance.gitScroll:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{{key: "↑/↓/Pg", label: "Scroll"}}
	if len(model.maintenance.gitReview.Changes) > 0 && !model.maintenance.gitReview.HasErrors() {
		actions = append(actions, tuiAction{key: "c", label: "Select commit paths"})
		if model.actions.PlanGitDiscard != nil {
			actions = append(actions, tuiAction{key: "x", label: "Discard changes"})
		}
	}
	if model.actions.PlanGitUndo != nil && model.actions.ApplyGitUndo != nil {
		actions = append(actions, tuiAction{key: "u", label: "Undo the last saved change"})
	}
	actions = append(actions, tuiAction{key: "r", label: "Refresh"}, tuiAction{key: "Esc", label: backLabel}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) gitCommitSelectView() string {
	path := []string{"Maintenance", "Changes", "Select paths"}
	title := "Select paths for the local commit"
	if model.maintenance.gitDiscard {
		title = "Select files to restore to the last saved version"
	}
	lines := []string{tuiTitle(title, model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	start, end := listWindow(len(model.maintenance.gitReview.Changes), model.maintenance.gitCommitCursor, model.rowCapacity())
	for index := start; index < end; index++ {
		change := model.maintenance.gitReview.Changes[index]
		chosen := "[ ]"
		if model.maintenance.gitCommitChosen[change.Path] {
			chosen = "[x]"
		}
		if change.Private {
			chosen = "[!]"
		}
		row := fmt.Sprintf("%s %-10s %-10s %-9s %s", chosen, gitChangeOwnership(change), gitChangeIndex(change), gitChangeWorktree(change), change.Path)
		lines = append(lines, tuiSelection(row, index == model.maintenance.gitCommitCursor, model.isDark))
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Space", label: "Toggle"}, {key: "a", label: "All safe"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Changes"}, {key: "F1", label: "Help"}}})
}

func (model dashboardModel) gitCommitReviewView() string {
	path := []string{"Maintenance", "Changes", "Commit review"}
	lines := []string{tuiTitle("Review local commit", model.isDark), ""}
	if model.maintenance.gitDiscard {
		path = []string{"Maintenance", "Changes", "Discard review"}
		lines = []string{tuiTitle("Review the changes that will be discarded", model.isDark), tuiMuted("They are kept under a private Git reference, shown in the result.", model.isDark), ""}
	}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	diffLines := strings.Split(strings.TrimSuffix(model.maintenance.gitCommitPlan.Diff.Content, "\n"), "\n")
	height := model.gitReviewHeight()
	maximum := maximumGitCommitPlanScroll(model.maintenance.gitCommitPlan, height)
	if model.maintenance.gitScroll > maximum {
		model.maintenance.gitScroll = maximum
	}
	end := model.maintenance.gitScroll + height
	if end > len(diffLines) {
		end = len(diffLines)
	}
	lines = append(lines,
		"Paths: "+strings.Join(model.maintenance.gitCommitPlan.Paths, ", "),
		"Message: "+model.maintenance.gitCommitPlan.CommitMessage,
		"No hooks, signing actions, remote operations, or push will run.",
		fmt.Sprintf("Diff lines %d-%d of %d", displayedLineStart(model.maintenance.gitScroll, len(diffLines)), end, len(diffLines)),
		"",
	)
	lines = append(lines, diffLines[model.maintenance.gitScroll:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	fixedBody := tuiSection("Enter saves these changes as one local commit.", model.isDark)
	if model.maintenance.gitDiscard {
		fixedBody = tuiSection("Type "+model.maintenance.gitCommitPlan.Confirmation+" to continue:", model.isDark) + "\n> " + model.confirmation + "_"
	}
	return model.renderShell(tuiShell{
		path:      path,
		body:      strings.Join(lines, "\n"),
		fixedBody: fixedBody,
		notices:   notices,
		actions:   gitReviewActions(model.maintenance.gitDiscard),
	})
}

func selectedGitCommitPaths(changes []domain.GitChange, chosen map[string]bool) string {
	paths := []string{}
	for _, change := range changes {
		if chosen[change.Path] && !change.Private {
			paths = append(paths, change.Path)
		}
	}
	return strings.Join(paths, ",")
}

func toggleAllGitCommitPaths(changes []domain.GitChange, chosen map[string]bool) map[string]bool {
	all := true
	for _, change := range changes {
		if !change.Private && !chosen[change.Path] {
			all = false
		}
	}
	result := map[string]bool{}
	if !all {
		for _, change := range changes {
			if !change.Private {
				result[change.Path] = true
			}
		}
	}
	return result
}

func maximumGitCommitPlanScroll(report domain.GitCommitPlanReport, height int) int {
	maximum := len(strings.Split(strings.TrimSuffix(report.Diff.Content, "\n"), "\n")) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func gitReviewContentLines(report domain.GitReviewReport) []string {
	lines := []string{}
	if len(report.Changes) == 0 {
		lines = append(lines, "The deployment worktree is clean.")
	}
	for _, change := range report.Changes {
		lines = append(lines, fmt.Sprintf("%-10s %-10s %-9s %s", gitChangeOwnership(change), gitChangeIndex(change), gitChangeWorktree(change), change.Path))
		if change.OriginalPath != "" {
			lines = append(lines, "  from "+change.OriginalPath)
		}
	}
	for _, diff := range report.Diffs {
		lines = append(lines, "", gitScopeTitle(diff.Scope)+" diff"+truncatedGitDiffLabel(diff.Truncated)+":")
		lines = append(lines, strings.Split(strings.TrimSuffix(diff.Content, "\n"), "\n")...)
	}
	if report.Summary.Untracked > 0 {
		lines = append(lines, "", "Untracked file contents are not opened automatically.")
	}
	for _, issue := range report.Issues {
		lines = append(lines, "", "BLOCKED: "+issue.Field+": "+issue.Message)
	}
	return lines
}

func (model dashboardModel) gitReviewHeight() int {
	if model.height <= 0 {
		return 14
	}
	height := model.height - ctaRows(model.height) - 13
	if height < 4 {
		return 4
	}
	return height
}

func maximumGitReviewScroll(report domain.GitReviewReport, height int) int {
	maximum := len(gitReviewContentLines(report)) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func gitReviewStatusKind(report domain.GitReviewReport) tuiStatusKind {
	if report.HasErrors() {
		return tuiStatusFailure
	}
	if len(report.Changes) > 0 {
		return tuiStatusAttention
	}
	return tuiStatusSuccess
}

func gitReviewActions(discard bool) []tuiAction {
	primary := "Create commit"
	if discard {
		primary = "Discard"
	}
	return []tuiAction{{key: "↑/↓/Pg", label: "Scroll diff"}, {key: "Enter", label: primary}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
}
