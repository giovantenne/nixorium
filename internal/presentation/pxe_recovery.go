package presentation

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// An interrupted network installation leaves the controller on its
// temporary network setup. The PXE screen then becomes the recovery review:
// what Nixorium found (the session and its two services in plain words),
// what you can do (recover the network, or stop the network installation),
// and Enter to confirm. Recovery changes only the controller's own network,
// so Enter is proportionate (ADR 0026); no typed word is asked.

type dashboardPXELifecycleMsg struct {
	report    domain.PXELifecycleReport
	status    domain.StatusReport
	statusErr error
}

func pxeRecoveryMode(mode string) bool {
	return mode == "degraded" || mode == "recovery-required"
}

// pxeRecoveryFound says how the session was left.
func pxeRecoveryFound(state domain.PXELifecycleState) string {
	switch {
	case state.Mode == "recovery-required":
		return "The network installation was interrupted: the temporary network setup is still in place, but nothing serves installations."
	case state.Listener.Active && !state.Network.Active:
		return "The network installation was interrupted: the network boot service runs without its temporary network setup."
	}
	return "The network installation was interrupted: one of its services failed."
}

func pxeListenerWords(service domain.ServiceState) string {
	switch {
	case service.Active:
		return "running"
	case service.State == "failed":
		return "failed"
	}
	return "stopped"
}

func pxeNetworkWords(service domain.ServiceState) string {
	switch {
	case service.Active:
		return "still in place"
	case service.State == "failed":
		return "failed"
	}
	return "not in place"
}

const (
	pxeRecoverAction = "Recover the network: restore the normal address, turn network installation off."
	pxeStopAction    = "Stop the network installation: stop both services, check the normal address."
	pxeRecoverEnter  = "Press Enter to recover the network. It changes only the controller; nothing changes on the student computers."
)

// pxeRecoveryLines is the body of the recovery review.
func (model dashboardModel) pxeRecoveryLines() []string {
	state := model.report.PXE
	lines := []string{
		tuiSection("What Nixorium found", model.isDark),
		"  " + pxeRecoveryFound(state),
	}
	lines = append(lines, tuiFields(model.isDark,
		[2]string{"Network boot service", pxeListenerWords(state.Listener)},
		[2]string{"Temporary network setup", pxeNetworkWords(state.Network)},
	)...)
	lines = append(lines, "",
		tuiSection("What you can do", model.isDark),
		"  Enter  "+pxeRecoverAction,
		"  x      "+pxeStopAction,
		"",
		pxeRecoverEnter,
	)
	return lines
}

// pxeLifecycleOutcome states the result of stopping or recovering the
// network installation in plain words, and the next step when it failed.
func pxeLifecycleOutcome(report domain.PXELifecycleReport, cli bool) (string, string) {
	recover := report.Operation == "pxe-recover"
	if !report.HasErrors() {
		if recover {
			return "The controller network is back to normal; network installation is off.", "Nothing changed on the student computers."
		}
		return "Network installation is off and the controller network is back to normal.", "Computers can no longer start from the network."
	}
	headline := "Network installation was not stopped."
	if recover {
		headline = "The controller network was not recovered."
	}
	message := strings.ToLower(report.Message)
	switch {
	case strings.Contains(message, "normal controller address") && strings.Contains(message, "missing"):
		next := "The normal controller address did not come back. Apply the saved configuration to the controller (Maintenance → Apply to controller), then open Maintenance → Diagnostics."
		if cli {
			next = "The normal controller address did not come back. Run nixorium controller plan and apply it, then nixorium doctor."
		}
		return headline, next
	case strings.Contains(message, "still active"):
		next := "A network installation service is still running. Try again; if it stays, open Maintenance → Diagnostics and collect a support report."
		if cli {
			next = "A network installation service is still running. Try again; if it stays, run nixorium doctor and collect a support report."
		}
		return headline, next
	}
	if step, found := domain.NextStepFor(report.Message); found {
		return headline, step.Why + " Next: " + step.Action + "."
	}
	if cli {
		return headline, "Try again, or run nixorium doctor."
	}
	return headline, "Try again, or open Maintenance → Diagnostics."
}

// pxeLifecycleNotices reports the last stop or recovery on the PXE screen;
// the technical text stays behind d.
func (model dashboardModel) pxeLifecycleNotices() []tuiNotice {
	report := model.installation.pxeResult
	if report.Operation == "" {
		return nil
	}
	headline, next := pxeLifecycleOutcome(report, false)
	notice := tuiNotice{kind: tuiStatusSuccess, title: headline, detail: next, plain: true}
	if report.HasErrors() {
		notice.kind = tuiStatusFailure
		if model.noticeDetails && report.Message != "" {
			notice.detail += "\n  " + tuiMuted("Details: "+report.Message, model.isDark)
		}
	}
	return []tuiNotice{notice}
}

// runPXELifecycle stops or recovers network installation and keeps the
// typed report for the result.
func (model dashboardModel) runPXELifecycle(action func() domain.PXELifecycleReport) tea.Cmd {
	refresh := model.actions.Refresh
	return func() tea.Msg {
		report := action()
		ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer cancel()
		status, err := refresh(ctx)
		return dashboardPXELifecycleMsg{report: report, status: status, statusErr: err}
	}
}

func (model dashboardModel) handlePXELifecycle(message dashboardPXELifecycleMsg) (tea.Model, tea.Cmd) {
	model.busy = ""
	model.noticeDetails = false
	model.installation.pxeResult = message.report
	model.message = ""
	if message.statusErr != nil {
		model.message = "The network installation state could not be read again; press r to refresh."
	} else {
		model.report = message.status
	}
	model.screen = dashboardPXE
	return model, nil
}

// PXELifecycleOutcomeText is the plain first line of pxe stop and pxe
// recover on the command line.
func PXELifecycleOutcomeText(writer io.Writer, report domain.PXELifecycleReport) {
	if report.Operation != "pxe-stop" && report.Operation != "pxe-recover" {
		return
	}
	headline, next := pxeLifecycleOutcome(report, true)
	fmt.Fprintln(writer, headline+" "+next)
}
