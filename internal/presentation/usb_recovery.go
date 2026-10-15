package presentation

import (
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// An unfinished USB installation is reviewed in three blocks: what Nixorium
// found (the computer and its disk, in plain words), what you can do (each
// action named by what it does), and the confirmation. Cancelling safely or
// closing a record whose disk was never touched is confirmed with Enter;
// closing after the disk was written keeps the typed CLOSE (ADR 0026). Only
// the worker verifies identity and releases the reservation.

// remoteInstallComputerWords says how the computer answered the last check.
func remoteInstallComputerWords(response domain.RemoteInstallResponse) string {
	session := response.Session
	if session == nil {
		return ""
	}
	message := response.Message
	switch {
	case response.State == "verified" || session.BootVerified:
		return "The computer started from its disk and was checked."
	case response.State == "cancelled" || strings.HasPrefix(response.State, "closed"):
		return ""
	case response.State == "reboot-requested" || response.State == "reboot-dispatching" || session.RebootRequested:
		return "The computer was asked to restart from its disk."
	case strings.Contains(message, domain.RemoteStatusNoConnection) || strings.Contains(message, domain.RemoteStatusRefreshFailed):
		return "The computer does not answer."
	case strings.Contains(message, domain.RemoteStatusAnswered):
		return "The computer is still booted from the USB stick and answers."
	case session.Bootstrap != nil && response.State == "reconciliation-required":
		return "Nixorium is no longer connected to the computer started from the USB stick."
	case session.Bootstrap != nil:
		return "The computer was last seen booted from the USB stick."
	}
	return "No computer is connected yet."
}

// remoteInstallDiskWords says what may have happened to the chosen disk.
func remoteInstallDiskWords(response domain.RemoteInstallResponse) string {
	session := response.Session
	if session == nil {
		return ""
	}
	receipt := session.Receipt
	switch {
	case response.State == "cancelled" || domain.RemoteInstallNeverDispatched(*session):
		return "The disk was not touched."
	case remoteInstallRunning(response):
		return "The installer is writing the disk."
	case domain.RemoteInstallDiskCompleted(*session), receipt != nil && !session.DispatchUncertain && receipt.Installed:
		return "The system is installed on the disk."
	case receipt != nil && !session.DispatchUncertain && receipt.State == "failed" && !receipt.MutationStarted && !receipt.DiskMayBeModified && !receipt.Installed:
		return "The disk was not touched."
	}
	return "The disk may have been partly written."
}

// remoteInstallCloseNeedsWord: closing after the disk may have been written
// asks for CLOSE; a record that never reached the disk closes with Enter.
func remoteInstallCloseNeedsWord(response domain.RemoteInstallResponse) bool {
	return response.Session == nil || !domain.RemoteInstallNeverDispatched(*response.Session)
}

const remoteInstallCloseWord = "CLOSE"

// remoteInstallChoice is one way out of the current state.
type remoteInstallChoice struct {
	key, label, detail, command string
}

func remoteInstallHostOf(response domain.RemoteInstallResponse) string {
	if name := remoteInstallHostName(response); name != "" {
		return name
	}
	return "HOST"
}

// remoteInstallChoices lists what can be done now, in the order shown.
func remoteInstallChoices(response domain.RemoteInstallResponse) []remoteInstallChoice {
	session, state, id := response.Session, response.State, response.OperationID
	choices := []remoteInstallChoice{}
	add := func(key, label, detail, command string) {
		choices = append(choices, remoteInstallChoice{key: key, label: label, detail: detail, command: command})
	}
	if remoteInstallCanConnect(response) {
		add("a", "Resume the installation", "connect to the computer started from the USB stick; its identity is checked before any password is sent.", "nixorium install usb start --host "+remoteInstallHostOf(response))
	}
	neverDispatched := session != nil && domain.RemoteInstallNeverDispatched(*session)
	if state == "reconciliation-required" {
		if !neverDispatched {
			add("n", "Check how far it got", "read the computer's own record again; nothing is started again.", "nixorium install usb reconcile --id "+id)
		}
		if session != nil && session.Bootstrap != nil {
			add("a", "Resume the installation", "reconnect to the same computer; its identity is checked before any password is sent.", "nixorium install usb start --host "+remoteInstallHostOf(response))
		}
	}
	if state == "ready-to-reboot" {
		add("b", "Restart the computer", "remove the USB stick first; the controller then checks the installed system.", "nixorium install usb reboot --id "+id)
	}
	if state == "reboot-requested" || state == "verified" || (state == "reconciliation-required" && !neverDispatched) {
		label := "Verify installed system"
		if state == "reboot-requested" {
			label = "Verify now"
		}
		add("v", label, "check that the computer started from its disk as installed; it never installs again.", "nixorium install usb verify --id "+id)
	}
	if remoteInstallSafelyCancellable(response) {
		add("x", "Cancel safely", "remove the temporary access and free the computer; the disk was not touched.", "nixorium install usb cancel --id "+id)
	}
	switch {
	case state == "ready-to-reboot":
		add("c", "Close this record", "stop following it without restarting; the disk was written, so you type CLOSE.", "nixorium install usb close --id "+id)
	case neverDispatched && session.Bootstrap != nil && state != "closed-before-apply":
		add("c", "Close this record", "forget it when the computer started from the USB stick is gone; the disk was not touched.", "nixorium install usb close --id "+id)
	}
	return choices
}

// remoteInstallOffers reports whether a choice is available now.
func remoteInstallOffers(response domain.RemoteInstallResponse, key string) bool {
	for _, choice := range remoteInstallChoices(response) {
		if choice.key == key {
			return true
		}
	}
	return false
}

// remoteInstallFindingLines is the "What Nixorium found" and "What you can
// do" blocks of the result screen.
func (model dashboardModel) remoteInstallFindingLines(next string) []string {
	response := model.installation.remote.response
	switch response.State {
	case "verified", "cancelled", "closed-before-apply", "closed-installed":
		// A finished installation has nothing left to resolve.
		return []string{"Next: " + next}
	}
	lines := []string{}
	computer, disk := remoteInstallComputerWords(response), remoteInstallDiskWords(response)
	if computer != "" || disk != "" {
		lines = append(lines, tuiSection("What Nixorium found", model.isDark))
		for _, line := range []string{computer, disk} {
			if line != "" {
				lines = append(lines, "  "+line)
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, tuiSection("What you can do", model.isDark), "  "+next)
	for _, choice := range remoteInstallChoices(response) {
		lines = append(lines, fmt.Sprintf("  %s  %s: %s", choice.key, choice.label, choice.detail))
	}
	return lines
}

// remoteInstallSituation is a catalogued refusal in a worker answer, such as
// an interrupted network installation, whose screen Enter opens.
func remoteInstallSituation(response domain.RemoteInstallResponse) (domain.NextStep, bool) {
	if !remoteInstallResponseFailed(response) {
		return domain.NextStep{}, false
	}
	step, found := domain.NextStepFor(response.Message)
	if !found || step.Screen == "" || step.Code == "USB-RESERVED" {
		return domain.NextStep{}, false
	}
	return step, true
}

// RemoteInstallFindingsText is the plain summary of install usb status.
func RemoteInstallFindingsText(writer io.Writer, response domain.RemoteInstallResponse) {
	if response.Session == nil {
		return
	}
	computer, disk := remoteInstallComputerWords(response), remoteInstallDiskWords(response)
	if found := strings.TrimSpace(computer + " " + disk); found != "" {
		fmt.Fprintln(writer, "What Nixorium found: "+found)
	}
	choices := remoteInstallChoices(response)
	if len(choices) == 0 {
		return
	}
	fmt.Fprintln(writer, "What you can do:")
	for _, choice := range choices {
		fmt.Fprintf(writer, "  %s: %s\n", choice.label, choice.command)
	}
}
