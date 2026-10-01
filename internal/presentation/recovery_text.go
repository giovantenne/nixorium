package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func DeploymentRecoveryPlanText(w io.Writer, plan domain.DeploymentRecoveryPlan) {
	fmt.Fprintf(w, "Interrupted client update: %s\n%s\n", plan.State, plan.Message)
	if plan.Pending.Revision != "" {
		fmt.Fprintf(w, "Recorded update: revision %s, started %s\n", plan.Pending.Revision, plan.Pending.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	for _, target := range plan.Targets {
		fmt.Fprintf(w, "  %-8s %-15s %s\n", target.Name, target.IP, target.Detail)
	}
	for _, issue := range plan.Issues {
		fmt.Fprintf(w, "  %s: %s\n", issue.Field, issue.Message)
	}
	if plan.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", plan.ReviewToken)
	}
}

func DeploymentRecoveryResultText(w io.Writer, result domain.DeploymentRecoveryResult) {
	fmt.Fprintln(w, result.Message)
	if result.Archive != "" {
		fmt.Fprintf(w, "Archived record: %s\n", result.Archive)
	}
}

func TemplateResetRecoveryPlanText(w io.Writer, plan domain.TemplateResetRecoveryPlan) {
	fmt.Fprintf(w, "Interrupted template reset: %s\n%s\n", plan.State, plan.Message)
	if plan.Original != "" {
		fmt.Fprintf(w, "Before the reset: %s\nReset commit:     %s\nCurrent HEAD:     %s\nBackup:           %s\n", plan.Original, plan.Candidate, plan.Head, plan.BackupRef)
	}
	if len(plan.Changed) > 0 {
		fmt.Fprintf(w, "Files that differ from before the reset (%d):\n", len(plan.Changed))
		for _, name := range plan.Changed[:min(len(plan.Changed), 30)] {
			fmt.Fprintf(w, "  %s\n", name)
		}
		if len(plan.Changed) > 30 {
			fmt.Fprintf(w, "  … and %d more\n", len(plan.Changed)-30)
		}
	}
	for _, issue := range plan.Issues {
		fmt.Fprintf(w, "  %s: %s\n", issue.Field, issue.Message)
	}
	if plan.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", plan.ReviewToken)
	}
}

func TemplateResetRecoveryResultText(w io.Writer, result domain.TemplateResetRecoveryResult) {
	fmt.Fprintln(w, result.Message)
	if result.Revision != "" {
		fmt.Fprintf(w, "Revision: %s\n", result.Revision)
	}
}

// ConfirmWord asks for an exact, case-sensitive confirmation word.
func ConfirmWord(input io.Reader, output io.Writer, word string) (bool, error) {
	fmt.Fprintf(output, "Type %s to continue: ", word)
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.TrimSpace(line) == word, nil
}
