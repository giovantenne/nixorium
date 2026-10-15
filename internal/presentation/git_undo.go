package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func gitUndoDate(date time.Time) string {
	if date.IsZero() {
		return "unknown"
	}
	return date.Format("2006-01-02 15:04")
}

// GitUndoPlanText prints the review of undoing the last saved change.
func GitUndoPlanText(writer io.Writer, report domain.GitUndoPlanReport) {
	fmt.Fprintf(writer, "Undo the last saved change: %s\n", strings.ToUpper(report.State))
	if report.Repository != "" {
		fmt.Fprintf(writer, "Repository:   %s\n", report.Repository)
	}
	if report.Undone.Revision != "" {
		fmt.Fprintf(writer, "Last change:  %s\n", report.Undone.Subject)
		fmt.Fprintf(writer, "Saved:        %s\n", gitUndoDate(report.Undone.Date))
		fmt.Fprintf(writer, "Revision:     %s\n", report.Undone.Revision)
	}
	if !report.HasErrors() {
		fmt.Fprintln(writer, "Files in that change:")
		for _, file := range report.Undone.Files {
			fmt.Fprintf(writer, "  %-13s %s\n", file.Change, file.Path)
		}
		fmt.Fprintf(writer, "The undo saves one new change, \"%s\", that puts these files back as they were before.\n", report.Title())
		fmt.Fprintln(writer, "The undone change stays in the history. No computer changes and nothing is pushed.")
		if report.Diff.Content != "" {
			fmt.Fprintln(writer, "\nWhat the undo changes:")
			fmt.Fprint(writer, report.Diff.Content)
			if !strings.HasSuffix(report.Diff.Content, "\n") {
				fmt.Fprintln(writer)
			}
		}
		fmt.Fprintln(writer, "\nAfter saving it:")
		for _, impact := range report.Impacts {
			fmt.Fprintf(writer, "  - %s\n", impact.Detail)
		}
		fmt.Fprintf(writer, "Review token: %s\n", report.ReviewToken)
		fmt.Fprintf(writer, "Confirmation: %s\n", report.Confirmation)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "Blocked: %s\n", issue.Message)
	}
}

// GitUndoResultText prints the outcome of undoing the last saved change.
func GitUndoResultText(writer io.Writer, report domain.GitUndoReport) {
	fmt.Fprintln(writer, report.Message)
	if report.Committed {
		if report.PreviousRevision != "" {
			fmt.Fprintf(writer, "Undone revision: %s\n", report.PreviousRevision)
		}
		if report.Revision != "" {
			fmt.Fprintf(writer, "Saved revision:  %s\n", report.Revision)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "Blocked: %s\n", issue.Message)
	}
	if len(report.Impacts) > 0 && !report.HasErrors() {
		fmt.Fprintln(writer, "Next:")
		for index, impact := range report.Impacts {
			fmt.Fprintf(writer, "  %d. %s\n", index+1, impact.Detail)
		}
	}
}

// ConfirmGitUndo asks for the typed word after a summary of the reviewed undo.
func ConfirmGitUndo(input io.Reader, output io.Writer, plan domain.GitUndoPlanReport) (bool, error) {
	if plan.HasErrors() {
		return false, nil
	}
	fmt.Fprintln(output, "Undo the last saved change: review")
	fmt.Fprintf(output, "Last change: %s (saved %s)\n", plan.Undone.Subject, gitUndoDate(plan.Undone.Date))
	paths := make([]string, 0, len(plan.Undone.Files))
	for _, file := range plan.Undone.Files {
		paths = append(paths, file.Path)
	}
	fmt.Fprintf(output, "Files: %s\n", strings.Join(paths, ", "))
	fmt.Fprintf(output, "Action: save one new change, \"%s\", that puts these files back as they were before\n", plan.Title())
	fmt.Fprintln(output, "Safety: the undone change stays in the history; no computer changes and nothing is pushed")
	fmt.Fprintf(output, "Type %s to continue: ", plan.Confirmation)
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == plan.Confirmation, nil
}
