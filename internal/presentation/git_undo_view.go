package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// gitUndoSummary says what the undo saves. A small terminal moves it into
// the scrolling part so the subject, the date and the next steps stay in view.
func gitUndoSummary(plan domain.GitUndoPlanReport) string {
	return fmt.Sprintf("The undo saves one new change, \"%s\", that puts the files back as they were before. The undone change stays in the history; nothing is pushed and no computer changes.", plan.Title())
}

// gitUndoCompact is true on a terminal too short for the full review header.
func (model dashboardModel) gitUndoCompact() bool {
	return model.height > 0 && model.height < 30
}

// gitUndoHeaderLines is the fixed part of the undo review: what is undone
// and what follows. The files and the patch scroll below it.
func gitUndoHeaderLines(plan domain.GitUndoPlanReport, dark, compact bool) []string {
	lines := []string{fmt.Sprintf("Last change: %s (saved %s)", plan.Undone.Subject, gitUndoDate(plan.Undone.Date))}
	if !compact {
		lines = append(lines, gitUndoSummary(plan))
	}
	lines = append(lines, tuiSection("After saving it:", dark))
	for _, impact := range plan.Impacts {
		lines = append(lines, "  - "+impact.Detail)
	}
	return lines
}

// gitUndoScrollLines are the files of the undone change followed by the
// patch that puts them back.
func (model dashboardModel) gitUndoScrollLines(plan domain.GitUndoPlanReport) []string {
	lines := []string{}
	if model.gitUndoCompact() {
		// Wrap here so each line of the window is one line on screen.
		width := min(116, max(20, model.width-6))
		if model.width == 0 {
			width = 100
		}
		wrapped := lipgloss.NewStyle().Width(width).Render(gitUndoSummary(plan))
		lines = append(lines, strings.Split(strings.TrimRight(wrapped, "\n"), "\n")...)
		lines = append(lines, "")
	}
	lines = append(lines, "Files in that change:")
	for _, file := range plan.Undone.Files {
		lines = append(lines, fmt.Sprintf("  %-13s %s", file.Change, file.Path))
	}
	if plan.Diff.Content != "" {
		lines = append(lines, "", "What the undo changes:")
		lines = append(lines, strings.Split(strings.TrimSuffix(plan.Diff.Content, "\n"), "\n")...)
	}
	return lines
}

// gitUndoHeight is the room left for the scrolling part below the header:
// the largest window for which the whole review still fits the terminal,
// whatever the width wraps. A terminal too small for even three lines falls
// back to the shell's own scrolling.
func (model dashboardModel) gitUndoHeight() int {
	if model.height <= 0 {
		return 14
	}
	for height := model.gitReviewHeight(); height > 3; height-- {
		// The shell clips an overfull body and says so with its scroll hint.
		if view := model.gitUndoReviewWith(height); lipgloss.Height(view) <= model.height && !strings.Contains(view, "Shift ↑/↓ scroll") {
			return height
		}
	}
	return 3
}

func (model dashboardModel) maximumGitUndoScroll() int {
	maximum := len(model.gitUndoScrollLines(model.maintenance.gitUndoPlan)) - model.gitUndoHeight()
	if maximum < 0 {
		return 0
	}
	return maximum
}

// gitUndoRefusalNotices shows each refusal with its catalogued next step
// when it has one, and as plain text with the step in its words otherwise.
func gitUndoRefusalNotices(issues []domain.ValidationIssue) []tuiNotice {
	notices := make([]tuiNotice, 0, len(issues))
	for _, issue := range issues {
		notice := tuiNotice{kind: tuiStatusAttention, title: issue.Message, plain: true}
		if issue.Next != nil {
			notice.code, notice.plain = issue.Next.Code, false
		}
		notices = append(notices, notice)
	}
	return notices
}

func (model dashboardModel) gitUndoReviewView() string {
	return model.gitUndoReviewWith(model.gitUndoHeight())
}

func (model dashboardModel) gitUndoReviewWith(height int) string {
	path := []string{"Maintenance", "Changes", "Undo review"}
	lines := []string{tuiTitle("Undo the last saved change", model.isDark), ""}
	compact := model.gitUndoCompact()
	if compact {
		lines = nil
	}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	plan := model.maintenance.gitUndoPlan
	if plan.HasErrors() {
		if plan.Undone.Revision != "" {
			lines = append(lines, "Last change: "+plan.Undone.Subject, "Saved: "+gitUndoDate(plan.Undone.Date), "")
		}
		lines = append(lines, tuiResult("The last saved change was not undone; nothing was changed.", false, model.isDark))
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			notices: gitUndoRefusalNotices(plan.Issues),
			actions: []tuiAction{{key: "r", label: "Review again"}, {key: "Esc", label: "Changes"}, {key: "F1", label: "Help"}},
		})
	}
	lines = append(lines, gitUndoHeaderLines(plan, model.isDark, compact)...)
	content := model.gitUndoScrollLines(plan)
	scroll := model.maintenance.gitScroll
	if maximum := max(0, len(content)-height); scroll > maximum {
		scroll = maximum
	}
	end := scroll + height
	if end > len(content) {
		end = len(content)
	}
	lines = append(lines, "", fmt.Sprintf("Lines %d-%d of %d", displayedLineStart(scroll, len(content)), end, len(content)))
	lines = append(lines, content[scroll:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message, plain: true})
	}
	return model.renderShell(tuiShell{
		path:      path,
		body:      strings.Join(lines, "\n"),
		fixedBody: tuiSection("Enter saves a new change that puts these files back.", model.isDark),
		notices:   notices,
		actions:   []tuiAction{{key: "↑/↓/Pg", label: "Scroll"}, {key: "Enter", label: "Undo"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}},
	})
}

func (model dashboardModel) gitUndoResultView(path []string, backLabel string) string {
	result := model.maintenance.gitUndoResult
	success := !result.HasErrors() && result.Committed
	title := "The last saved change could not be undone"
	if success {
		title = "The last saved change was undone"
	}
	if result.State == "partial" {
		title = "The undo was saved but needs attention"
	}
	lines := []string{tuiTitle("Repository changes", model.isDark), "", tuiResult(title, success, model.isDark), ""}
	if result.Committed {
		lines = append(lines, "Undone revision: "+result.PreviousRevision, "Saved revision:  "+result.Revision)
	}
	if success && len(result.Impacts) > 0 {
		lines = append(lines, "", tuiSection("Next:", model.isDark))
		for index, impact := range result.Impacts {
			lines = append(lines, fmt.Sprintf("  %d. %s", index+1, impact.Detail))
		}
	}
	returnLabel := backLabel
	if model.setupMode {
		returnLabel = "Setup"
	}
	actions := []tuiAction{{key: "Enter", label: returnLabel}, {key: "r", label: "Refresh review"}, {key: "F1", label: "Help"}}
	if success {
		if followup := model.saveFollowupActions(returnLabel); hasAction(followup, "Enter") {
			// Enter continues to the controller; Esc is the way back.
			actions = insertBeforeHelp(followup, tuiAction{key: "r", label: "Refresh review"})
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		kind := tuiStatusAttention
		if success {
			kind = tuiStatusSuccess
		}
		notices = append(notices, tuiNotice{kind: kind, title: model.message, plain: true})
	}
	if !success {
		notices = append(notices, gitUndoRefusalNotices(result.Issues)...)
	}
	return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

// startGitUndoReview asks for the review of undoing the last saved change.
func (model dashboardModel) startGitUndoReview() (tea.Model, tea.Cmd) {
	model.maintenance.gitUndoResult = domain.GitUndoReport{}
	model.busy = "Reviewing the last saved change"
	model.message = ""
	plan := model.actions.PlanGitUndo
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardGitUndoPlanMsg{report: plan(ctx)}
	})
}

func (model dashboardModel) updateGitUndoReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	plan := model.maintenance.gitUndoPlan
	if plan.HasErrors() {
		switch key.String() {
		case "esc", "left", "enter":
			model.screen = dashboardGitReview
			model.message = ""
		case "r":
			return model.startGitUndoReview()
		}
		return model, nil
	}
	maximum := model.maximumGitUndoScroll()
	switch key.String() {
	case "esc":
		model.screen = dashboardGitReview
		model.confirmation = ""
		model.message = "Undo cancelled; no repository state changed."
	case "up":
		if model.maintenance.gitScroll > 0 {
			model.maintenance.gitScroll--
		}
	case "down":
		if model.maintenance.gitScroll < maximum {
			model.maintenance.gitScroll++
		}
	case "pgup":
		model.maintenance.gitScroll -= model.gitUndoHeight()
		if model.maintenance.gitScroll < 0 {
			model.maintenance.gitScroll = 0
		}
	case "pgdown":
		model.maintenance.gitScroll += model.gitUndoHeight()
		if model.maintenance.gitScroll > maximum {
			model.maintenance.gitScroll = maximum
		}
	case "enter":
		model.busy = "Revalidating and saving the undo of the last saved change"
		model.confirmation = ""
		model.message = ""
		apply := model.actions.ApplyGitUndo
		return model, func() tea.Msg {
			result := apply(plan)
			ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
			defer cancel()
			return dashboardGitUndoResultMsg{report: result, review: model.actions.LoadGitReview(ctx)}
		}
	}
	return model, nil
}
