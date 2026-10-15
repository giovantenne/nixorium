package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Plain words for the state of each computer of an interrupted update.
var deploymentRecoveryStateWords = map[string]string{
	domain.RecoveryTargetReviewed:    "finished, running the new version",
	domain.RecoveryTargetOther:       "finished, but running another version",
	domain.RecoveryTargetActivating:  "still updating",
	domain.RecoveryTargetUnreachable: "not answering (switched off, or not reachable at this address)",
}

const (
	deploymentRecoveryUSBState     = "not answering: its USB installation was not finished, so it was never reachable"
	deploymentRecoveryUSBNext      = "Close or resume that installation from Installation → USB over SSH after this review."
	deploymentRecoveryAddressHint  = `A computer that does not answer at its laboratory address may be switched off, still on an old version, or connected with a second network card; see "Computer not reachable" in the troubleshooting guide.`
	deploymentRecoveryPXEHint      = "A network installation is still open on this controller. A computer installed through it may not have finished; finish or stop the session from Installation → Network boot (PXE) after this review, then install it again."
	deploymentRecoveryUnreadable   = "The record of the interrupted update cannot be read. Keep it as it is and collect a support report from Maintenance → Diagnostics."
	deploymentRecoveryResultText   = "The interrupted update is marked as reviewed. Other operations are unblocked."
	deploymentRecoveryResultNextUI = "Use Computers → Update computers to bring every computer to the current version."
)

func deploymentRecoveryIssue(plan domain.DeploymentRecoveryPlan, field string) bool {
	for _, issue := range plan.Issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}

func deploymentRecoveryIntro(plan domain.DeploymentRecoveryPlan) string {
	return fmt.Sprintf("The update of %s to %d computer(s) stopped before Nixorium could see how it ended.",
		plan.Pending.StartedAt.Local().Format("2006-01-02 15:04"), len(plan.Pending.Targets))
}

// deploymentRecoveryRows lists each checked computer in plain words and
// reports whether the computer of an unfinished USB installation is among
// those that do not answer.
func deploymentRecoveryRows(plan domain.DeploymentRecoveryPlan, usbHost string) ([]string, bool) {
	nameWidth, addressWidth := 0, 0
	for _, target := range plan.Targets {
		nameWidth, addressWidth = max(nameWidth, len(target.Name)), max(addressWidth, len(target.IP))
	}
	rows, usbListed := []string{}, false
	for _, target := range plan.Targets {
		state, known := deploymentRecoveryStateWords[target.State]
		if !known {
			state = target.Detail
		}
		if target.State == domain.RecoveryTargetUnreachable && usbHost != "" && target.Name == usbHost {
			state, usbListed = deploymentRecoveryUSBState, true
		}
		rows = append(rows, fmt.Sprintf("  %-*s  %-*s  %s", nameWidth, target.Name, addressWidth, target.IP, state))
	}
	return rows, usbListed
}

func deploymentRecoveryActivating(plan domain.DeploymentRecoveryPlan) int {
	count := 0
	for _, target := range plan.Targets {
		if target.State == domain.RecoveryTargetActivating {
			count++
		}
	}
	return count
}

// deploymentRecoveryLines is the body of the interrupted update review.
func deploymentRecoveryLines(plan domain.DeploymentRecoveryPlan, usbHost string, dark bool) []string {
	switch {
	case plan.State == "unchanged":
		return []string{"No interrupted client update is recorded. Nothing needs reviewing."}
	case deploymentRecoveryIssue(plan, "record"):
		return []string{deploymentRecoveryUnreadable}
	case deploymentRecoveryIssue(plan, "process"):
		return []string{deploymentRecoveryIntro(plan), "", fmt.Sprintf("Nixorium is still open in another window or terminal (started %s). Close it, then press r.", plan.Pending.StartedAt.Local().Format("15:04"))}
	}
	lines := []string{deploymentRecoveryIntro(plan) + " Each computer was checked:", ""}
	rows, usbListed := deploymentRecoveryRows(plan, usbHost)
	lines = append(lines, rows...)
	if plan.Unreachable > 0 {
		lines = append(lines, "", tuiMuted(deploymentRecoveryAddressHint, dark))
		if plan.PXEMode != "" {
			lines = append(lines, tuiMuted(deploymentRecoveryPXEHint, dark))
		}
	}
	lines = append(lines, "")
	usbNext := ""
	if usbListed {
		usbNext = " " + deploymentRecoveryUSBNext
	}
	ready := plan.State == "ready" && !plan.HasErrors()
	switch activating := deploymentRecoveryActivating(plan); {
	case activating > 0:
		lines = append(lines, fmt.Sprintf("%d computer(s) are still updating. Wait a minute, then press r to check again.", activating))
	case plan.Unreachable > 0 && !plan.AcknowledgeUnreachable:
		lines = append(lines, fmt.Sprintf("%d computer(s) do not answer. Switch them on and press r, or press c to continue without them; you can update them later from Computers → Update computers.", plan.Unreachable)+usbNext)
	case ready:
		lines = append(lines, "Press Enter to mark this update as reviewed. Nothing changes on the computers; other operations are unblocked. Then run a new update so every computer is current."+usbNext)
	case plan.Message != "":
		lines = append(lines, plan.Message)
	}
	return lines
}

func deploymentRecoveryResultLines(result domain.DeploymentRecoveryResult, dark bool) []string {
	if result.State != "completed" {
		lines := []string{tuiResult("The interrupted update was not marked as reviewed", false, dark), "", result.Message, "", "Nothing was changed. Press Enter, then open the review again."}
		return lines
	}
	lines := []string{tuiResult("The interrupted update is marked as reviewed.", true, dark), "Other operations are unblocked. " + deploymentRecoveryResultNextUI}
	if result.Archive != "" {
		lines = append(lines, "", tuiMuted("The record is kept in "+result.Archive, dark))
	}
	if _, warning, found := strings.Cut(result.Message, "operation outcome was not recorded: "); found {
		lines = append(lines, tuiMuted("The operation history was not updated: "+warning, dark))
	}
	return lines
}

func DeploymentRecoveryPlanText(w io.Writer, plan domain.DeploymentRecoveryPlan) {
	fmt.Fprintf(w, "Interrupted client update: %s\n%s\n", plan.State, plan.Message)
	if plan.Pending.Revision != "" {
		fmt.Fprintf(w, "Recorded update: revision %s, started %s\n", plan.Pending.Revision, plan.Pending.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	rows, _ := deploymentRecoveryRows(plan, "")
	for _, row := range rows {
		fmt.Fprintln(w, row)
	}
	if plan.Unreachable > 0 {
		fmt.Fprintln(w, deploymentRecoveryAddressHint)
		if plan.PXEMode != "" {
			fmt.Fprintln(w, deploymentRecoveryPXEHint)
		}
		if !plan.AcknowledgeUnreachable {
			fmt.Fprintln(w, "To continue without the computers that do not answer, add --acknowledge-unreachable.")
		}
	}
	for _, issue := range plan.Issues {
		fmt.Fprintf(w, "  %s: %s\n", issue.Field, issue.Message)
	}
	if plan.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", plan.ReviewToken)
		fmt.Fprintln(w, "Marking it as reviewed changes nothing on the computers; it archives the record and unblocks other operations.")
	}
}

func DeploymentRecoveryResultText(w io.Writer, result domain.DeploymentRecoveryResult) {
	if result.State != "completed" {
		fmt.Fprintln(w, result.Message)
		return
	}
	fmt.Fprintln(w, deploymentRecoveryResultText+" Run nixorium deploy plan --on @lab to bring every computer to the current version.")
	if _, warning, found := strings.Cut(result.Message, "operation outcome was not recorded: "); found {
		fmt.Fprintln(w, "Warning: operation outcome was not recorded: "+warning)
	}
	if result.Archive != "" {
		fmt.Fprintf(w, "Archived record: %s\n", result.Archive)
	}
}

// ConfirmDeploymentRecovery asks before marking an interrupted update as
// reviewed. It changes nothing on any computer, so a yes/no answer is
// proportionate (ADR 0026).
func ConfirmDeploymentRecovery(input io.Reader, output io.Writer) (bool, error) {
	return ConfirmYesNo(input, output, "Mark this update as reviewed?")
}

// ConfirmYesNo asks a question for a recovery that changes nothing that
// cannot be taken back; only "y" or "yes" approves it.
func ConfirmYesNo(input io.Reader, output io.Writer, question string) (bool, error) {
	fmt.Fprint(output, question+" [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
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
