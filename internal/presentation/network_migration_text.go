package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// networkMigrationTitles name the three steps of the change of laboratory
// network in reviews, results and the TUI.
var networkMigrationTitles = map[string]string{
	domain.NetworkMigrationStart:  "Start the change of laboratory network",
	domain.NetworkMigrationFinish: "Finish the change of laboratory network",
	domain.NetworkMigrationUndo:   "Undo the change of laboratory network",
}

// networkMigrationComputerState is the plain description of one computer.
func networkMigrationComputerState(computer domain.NetworkMigrationComputer) string {
	label := map[string]string{
		domain.MigrationComputerReachable:    "answers",
		domain.MigrationComputerOff:          "off or not answering",
		domain.MigrationComputerAcknowledged: "off; continues without it",
		domain.MigrationComputerMigrated:     "moved",
		domain.MigrationComputerOutdated:     "on the new network, older configuration",
		domain.MigrationComputerPrevious:     "still on the previous network",
		domain.MigrationComputerWaiting:      "off; queued",
		domain.MigrationComputerUnknown:      "not answering",
	}[computer.State]
	if label == "" {
		label = computer.State
	}
	return label
}

func networkMigrationComputerLine(computer domain.NetworkMigrationComputer) string {
	line := fmt.Sprintf("%-6s %-15s → %-15s %s", computer.Name, computer.From, computer.To, networkMigrationComputerState(computer))
	if computer.Detail != "" {
		line += ": " + computer.Detail
	}
	return line
}

// NetworkMigrationPlanText prints a review for the terminal.
func NetworkMigrationPlanText(writer io.Writer, plan domain.NetworkMigrationPlan) {
	state := strings.ToUpper(plan.State)
	fmt.Fprintf(writer, "%s: %s\n", networkMigrationTitles[plan.Action], state)
	if plan.From != "" {
		fmt.Fprintf(writer, "Laboratory network:  %s → %s\n", plan.From, plan.To)
	}
	if plan.ControllerFrom != "" {
		fmt.Fprintf(writer, "Controller address:  %s → %s\n", plan.ControllerFrom, plan.ControllerTo)
	}
	if len(plan.Computers) > 0 {
		fmt.Fprintln(writer, "Computers:")
		for _, computer := range plan.Computers {
			fmt.Fprintln(writer, "  "+networkMigrationComputerLine(computer))
		}
	}
	if len(plan.Steps) > 0 {
		fmt.Fprintln(writer, "What happens:")
		for index, step := range plan.Steps {
			fmt.Fprintf(writer, "  %d. %s\n", index+1, step)
		}
	}
	if plan.Message != "" {
		fmt.Fprintln(writer, plan.Message)
	}
	for _, issue := range plan.Issues {
		fmt.Fprintf(writer, "Blocked: %s\n", issue.Message)
		if issue.Next != nil {
			fmt.Fprintln(writer, "  "+issue.Next.Line())
		}
	}
	if plan.ReviewToken != "" {
		fmt.Fprintf(writer, "Review token: %s\n", plan.ReviewToken)
	}
}

// NetworkMigrationResultText prints the outcome of a saved step.
func NetworkMigrationResultText(writer io.Writer, result domain.NetworkMigrationResult) {
	fmt.Fprintln(writer, result.Message)
	if result.Revision != "" {
		fmt.Fprintf(writer, "Saved revision: %s\n", result.Revision)
	}
	if len(result.Rekeyed) > 0 {
		fmt.Fprintf(writer, "Trusted keys recorded for the new addresses of: %s\n", strings.Join(result.Rekeyed, ", "))
	}
	for _, issue := range result.Issues {
		fmt.Fprintf(writer, "Problem: %s\n", issue.Message)
		if issue.Next != nil {
			fmt.Fprintln(writer, "  "+issue.Next.Line())
		}
	}
	if len(result.Next) > 0 {
		fmt.Fprintln(writer, "Next:")
		for index, step := range result.Next {
			fmt.Fprintf(writer, "  %d. %s\n", index+1, step)
		}
	}
}

// NetworkMigrationStatusText prints the change in progress.
func NetworkMigrationStatusText(writer io.Writer, status domain.NetworkMigrationStatus) {
	fmt.Fprintln(writer, status.Message)
	if !status.InProgress() {
		for _, issue := range status.Issues {
			fmt.Fprintf(writer, "Problem: %s\n", issue.Message)
		}
		return
	}
	controller := "answers on both networks"
	if !status.ControllerReady {
		controller = "not applied yet: it does not carry both addresses"
	}
	fmt.Fprintf(writer, "Started:     %s\n", status.StartedAt)
	fmt.Fprintf(writer, "Controller:  %s → %s, %s\n", status.ControllerFrom, status.ControllerTo, controller)
	fmt.Fprintf(writer, "Computers:   %d moved, %d waiting (queued), %d still on the previous network, %d need an update, %d not answering\n", status.Migrated, status.Waiting, status.Previous, status.Outdated, status.Unknown)
	for _, computer := range status.Computers {
		fmt.Fprintln(writer, "  "+networkMigrationComputerLine(computer))
	}
	for _, issue := range status.Issues {
		fmt.Fprintf(writer, "Problem: %s\n", issue.Message)
	}
	if status.Next != "" {
		fmt.Fprintf(writer, "Next: %s\n", status.Next)
	}
	if status.CanFinish {
		fmt.Fprintln(writer, "Finish: nixorium network migrate finish")
	}
	fmt.Fprintln(writer, "Undo is possible until the change is finished: nixorium network migrate undo")
}

// ConfirmNetworkMigration shows the review and asks for Enter. Saving changes
// no computer, so no typed word is required.
func ConfirmNetworkMigration(input io.Reader, output io.Writer, plan domain.NetworkMigrationPlan) (bool, error) {
	if plan.HasErrors() {
		return false, nil
	}
	NetworkMigrationPlanText(output, plan)
	fmt.Fprint(output, "Press Enter to save this step, or type n to cancel: ")
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "", nil
}
