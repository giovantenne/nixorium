package presentation

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// remoteInstallWatchInterval paces automatic status observation while the
// independent installer runs and read-only verification after its reboot.
const remoteInstallWatchInterval = 5 * time.Second

type remoteInstallTickMsg struct {
	operationID string
	watch       uint64
}

// remoteInstallRunning reports a dispatched disk job that is still working.
func remoteInstallRunning(response domain.RemoteInstallResponse) bool {
	switch response.State {
	case "accepted", "running", "dispatching":
		return !remoteInstallDispatchUncertain(response)
	}
	return false
}

// remoteInstallAwaitingVerification reports a safely rebooted installation
// whose installed identity can be checked repeatedly without replaying a
// mutation. The reboot authorization is already persisted by the worker.
func remoteInstallAwaitingVerification(response domain.RemoteInstallResponse) bool {
	return response.State == "reboot-requested" && response.Session != nil &&
		response.Session.RebootRequested && response.Session.Preparation != nil
}

func remoteInstallDispatchUncertain(response domain.RemoteInstallResponse) bool {
	return (response.Session != nil && response.Session.DispatchUncertain) ||
		(response.Execution != nil && response.Execution.DispatchUncertain)
}

// withRemoteInstallWatch schedules the next automatic status refresh while a
// job runs, and starts the elapsed-time clock on the first observation.
func (model dashboardModel) withRemoteInstallWatch(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	updated, ok := next.(dashboardModel)
	if !ok {
		return next, cmd
	}
	remote := &updated.installation.remote
	if remote.stage != remoteInstallResult ||
		(!remoteInstallRunning(remote.response) && !remoteInstallAwaitingVerification(remote.response)) ||
		remote.operationID == "" {
		remote.watchStarted = time.Time{}
		return updated, cmd
	}
	if remote.watchStarted.IsZero() {
		remote.watchStarted = time.Now()
	}
	remote.watch++
	tick := tea.Tick(remoteInstallWatchInterval, func(time.Time) tea.Msg {
		return remoteInstallTickMsg{operationID: remote.operationID, watch: remote.watch}
	})
	return updated, tea.Batch(cmd, tick)
}

func (model dashboardModel) handleRemoteInstallTick(message remoteInstallTickMsg) (tea.Model, tea.Cmd) {
	remote := model.installation.remote
	if model.screen != dashboardUSBInstall || model.busy != "" || remote.stage != remoteInstallResult ||
		remote.operationID != message.operationID || remote.watch != message.watch {
		return model, nil
	}
	if remoteInstallRunning(remote.response) {
		// Only the read-only status request; never apply, reboot or reconcile.
		return model.remoteInstallCommand("status", domain.RemoteInstallRequest{Operation: domain.RemoteInstallStatusOperation, OperationID: remote.operationID})
	}
	if remoteInstallAwaitingVerification(remote.response) {
		// Verification is read-only and bound to the preserved host key, static
		// address, hostname, closure and deployment revision.
		return model.remoteInstallCommand("verify", domain.RemoteInstallRequest{Operation: domain.RemoteInstallVerifyOperation, OperationID: remote.operationID})
	}
	return model, nil
}

func remoteInstallPhaseWords(phase domain.RemoteInstallPhase) string {
	switch phase {
	case domain.RemoteInstallPhasePreflight, domain.RemoteInstallPhaseRevalidate:
		return "checking the reviewed computer and disk again"
	case domain.RemoteInstallPhasePrepare, domain.RemoteInstallPhaseTransfer:
		return "copying the system to the computer"
	case domain.RemoteInstallPhaseProbe:
		return "reading the disks"
	case domain.RemoteInstallPhasePartition:
		return "erasing and partitioning the disk"
	case domain.RemoteInstallPhaseInstall:
		return "installing the system on the disk"
	case domain.RemoteInstallPhaseVerify:
		return "checking the installed system"
	case domain.RemoteInstallPhaseReadyToReboot:
		return "finished, waiting to restart"
	case domain.RemoteInstallPhaseReboot:
		return "restarting"
	case domain.RemoteInstallPhasePostBootVerify:
		return "checking the computer after restart"
	case "":
		return "starting"
	}
	return string(phase)
}

func remoteInstallCurrentPhase(response domain.RemoteInstallResponse) domain.RemoteInstallPhase {
	if response.Session != nil && response.Session.Receipt != nil && response.Session.Receipt.Phase != "" {
		return response.Session.Receipt.Phase
	}
	if response.Execution != nil && response.Execution.Phase != "" {
		return response.Execution.Phase
	}
	if response.Session != nil && len(response.Session.Events) > 0 {
		return response.Session.Events[len(response.Session.Events)-1].Phase
	}
	return ""
}

func remoteInstallDiskMayBeChanged(response domain.RemoteInstallResponse) bool {
	if response.Execution != nil && response.Execution.DiskMayBeModified {
		return true
	}
	return response.Session != nil && response.Session.Receipt != nil && response.Session.Receipt.DiskMayBeModified
}

// remoteInstallOutcome states what happened in one sentence and the next step.
func (model dashboardModel) remoteInstallOutcome() (string, string) {
	remote := model.installation.remote
	response := remote.response
	host := remote.host
	if host == "" && response.Session != nil {
		host = response.Session.Plan.Host.Name
	}
	if host == "" {
		host = "the computer"
	}
	switch {
	case remoteInstallDispatchUncertain(response) || response.State == "reconciliation-required":
		return "It is not certain whether the installation of " + host + " started or finished.",
			"Do not start another installation. Press n to read the computer's own record again."
	case remoteInstallRunning(response):
		elapsed := ""
		if !remote.watchStarted.IsZero() {
			elapsed = fmt.Sprintf(" (%s so far)", time.Since(remote.watchStarted).Truncate(time.Second).String())
		}
		return "Installing " + host + ": " + remoteInstallPhaseWords(remoteInstallCurrentPhase(response)) + elapsed + ".",
			"This view refreshes by itself every few seconds. You can leave it; the job continues on the computer."
	case response.State == "ready-to-reboot":
		return "The system is installed on " + host + " and is waiting to restart.",
			"Remove the USB stick (or make the disk boot first), then press b to restart it."
	case response.State == "reboot-requested" || response.State == "reboot-dispatching":
		return "Restart of " + host + " was requested.",
			"The controller checks the installed identity automatically every few seconds; press v to retry now."
	case response.State == "verified":
		return host + " is installed and verified.", "It now appears among the configured computers."
	case response.State == "cancelled" || response.State == "closed-before-apply":
		return "The installation was cancelled; no disk was changed.", "Start again from Installation when the computer is ready."
	case response.State == "closed-installed":
		return "The session was closed without restarting; the system is installed.", "Restart the computer locally when ready."
	case remoteInstallCanConnect(response):
		return "The installation files for " + host + " are ready.", "Press a to connect to the computer started from the USB stick."
	case remoteInstallResponseFailed(response):
		disk := "No disk was changed."
		if remoteInstallDiskMayBeChanged(response) {
			disk = "The disk may have been changed."
		}
		return "The installation of " + host + " did not complete. " + disk,
			"Read the reason below and the details (d), fix the cause, then refresh or cancel safely."
	}
	return "Current state: " + response.State + ".", "Press r to refresh the state."
}

// humanBytes prints a disk size in decimal units, as disks are labelled.
func humanBytes(size uint64) string {
	units := []string{"B", "kB", "MB", "GB", "TB", "PB"}
	value := float64(size)
	unit := 0
	for value >= 1000 && unit < len(units)-1 {
		value /= 1000
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
