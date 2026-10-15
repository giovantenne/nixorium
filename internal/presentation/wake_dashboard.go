package presentation

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Switch on in Power controls: a Wake-on-LAN packet for the selected
// computers that are off. The review says which computers receive it and
// which are skipped; the result counts packets sent and never says that a
// computer started.

type dashboardWakePlanMsg struct{ plan domain.WakePlan }
type dashboardWakeApplyMsg struct{ report domain.WakeReport }

// nextAction cycles Shut down, Restart and, when offered, Switch on.
func (model shutdownModel) nextAction() shutdownModel {
	switch {
	case model.wake:
		model.wake, model.action = false, domain.ClientPowerOff
	case model.action == domain.ClientPowerOff:
		model.action = domain.ClientRestart
	case model.wakeAvailable:
		model.wake = true
	default:
		model.action = domain.ClientPowerOff
	}
	return model
}

func (model shutdownModel) updateWakeReview(key tea.KeyPressMsg) (shutdownModel, shutdownIntent) {
	switch key.String() {
	case "esc":
		return model, shutdownIntent{kind: shutdownSelectionIntent, message: "Switch on cancelled; no packet was sent."}
	case "enter":
		if model.wakePlan.HasErrors() || model.wakePlan.Count(domain.WakeTargetSend) == 0 {
			return model, shutdownIntent{message: "No packet can be sent from this review."}
		}
		model.applying = true
		return model, shutdownIntent{kind: shutdownWakeApplyIntent}
	}
	return model, shutdownIntent{}
}

func hostIsOff(host domain.HostStatus) bool {
	return host.Reachability == domain.ReachabilityUnreachable
}

func (model dashboardModel) startWakePlan(requested string) (tea.Model, tea.Cmd) {
	if model.actions.PlanWake == nil {
		model.message = "Switching on is not available in this deployment."
		return model, nil
	}
	model.busy = "Checking which selected computers are already on"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardWakePlanMsg{plan: model.actions.PlanWake(ctx, requested)}
	})
}

func (model dashboardModel) startWakeApply() (tea.Model, tea.Cmd) {
	if model.actions.ApplyWake == nil {
		model.shutdown.applying = false
		model.message = "Switching on is not available in this deployment."
		return model, nil
	}
	model.busy = "Sending Wake-on-LAN packets"
	plan := model.shutdown.wakePlan
	return model, func() tea.Msg { return dashboardWakeApplyMsg{report: model.actions.ApplyWake(plan)} }
}

func (model shutdownModel) wakeView(shell tuiShell, screen dashboardScreen, context shutdownViewContext) tuiShell {
	if screen == dashboardShutdownResult {
		shell.body = strings.Join(model.wakeResultView(context), "\n")
		shell.actions = []tuiAction{{key: "n", label: "New review"}, {key: "h", label: "Check computers"}, {key: "Enter", label: "Computers"}, {key: "F1", label: "Help"}}
		return shell
	}
	body, fixedBody := model.wakeReviewView(context)
	shell.body = strings.Join(body, "\n")
	shell.fixedBody = strings.Join(fixedBody, "\n")
	if fixedBody != nil {
		shell.actions = []tuiAction{{key: "Enter", label: "Send packets"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else {
		shell.actions = []tuiAction{{key: "Esc", label: "Selection"}, {key: "F1", label: "Help"}}
	}
	if context.message != "" && context.message != model.wakePlan.Message {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: context.message})
	}
	return shell
}

func (model shutdownModel) wakeReviewView(context shutdownViewContext) ([]string, []string) {
	plan := model.wakePlan
	send := plan.Count(domain.WakeTargetSend)
	title := fmt.Sprintf("Switch on %s?", countNoun(send, "computer"))
	if send == 0 {
		title = "No computer to switch on"
	}
	lines := []string{tuiSection(title, context.dark), ""}
	lines = append(lines, tuiFields(context.dark,
		[2]string{"Selected", fmt.Sprint(len(plan.Targets))},
		[2]string{"Receive the packet", fmt.Sprint(send)},
		[2]string{"Already on", fmt.Sprint(plan.Count(domain.WakeTargetOn))},
		[2]string{"Address not known", fmt.Sprint(plan.Count(domain.WakeTargetUnknown))},
		[2]string{"Controller", "excluded"},
	)...)
	lines = append(lines, "")
	start, end := listWindow(len(plan.Targets), 0, max(3, context.height-22))
	for _, target := range plan.Targets[start:end] {
		lines = append(lines, wakeTargetStatus(target, context.dark))
	}
	if end < len(plan.Targets) {
		lines = append(lines, tuiMuted(fmt.Sprintf("Showing %d of %d reviewed computers", end, len(plan.Targets)), context.dark))
	}
	if plan.Count(domain.WakeTargetUnknown) > 0 {
		lines = append(lines, "", tuiMuted(domain.WakeUnknownDetail, context.dark))
	}
	lines = append(lines, "", "A packet is not proof that a computer starts: Wake-on-LAN must be allowed in its firmware.")
	if plan.HasErrors() || send == 0 {
		return append(lines, "", tuiStatus("No packet can be sent from this review", tuiStatusAttention, context.dark), plan.Message), nil
	}
	return lines, []string{tuiSection(fmt.Sprintf("Press Enter to send the packet to %s.", countNoun(send, "computer")), context.dark)}
}

func wakeTargetStatus(target domain.WakeTarget, dark bool) string {
	switch target.State {
	case domain.WakeTargetSend:
		return tuiStatus(target.Name+" · Off · receives the packet", tuiStatusSuccess, dark)
	case domain.WakeTargetOn:
		return "○ " + target.Name + " · Already on · skipped"
	case domain.WakeTargetUnknown:
		return tuiStatus(target.Name+" · Address not known yet · skipped", tuiStatusAttention, dark)
	default:
		return tuiStatus(target.Name+" · "+strings.TrimSuffix(target.Detail, "."), tuiStatusAttention, dark)
	}
}

func (model shutdownModel) wakeResultView(context shutdownViewContext) []string {
	report := model.wakeResult
	title := "Wake-on-LAN packets sent"
	switch {
	case report.State == "blocked":
		title = "No packet was sent"
	case report.State != "completed":
		title = "Packets sent; some computers need attention"
	}
	lines := []string{tuiResult(title, report.State == "completed", context.dark), "", fmt.Sprintf("Sent  %d    Already on  %d    Address unknown  %d    Not sent  %d", report.Sent, report.AlreadyOn, report.Unknown, report.Failed), ""}
	for _, target := range report.Targets {
		marker := "!"
		switch target.State {
		case domain.WakeOutcomeSent:
			marker = "✓"
		case domain.WakeOutcomeOn:
			marker = "○"
		}
		lines = append(lines, fmt.Sprintf("%s %-10s %s", marker, target.Name, wakeOutcomeWords(target.State)), tuiMuted("  "+target.Detail, context.dark))
	}
	return append(lines, "", report.Message, "", "A sent packet does not prove that a computer started; press h in a few minutes to see which computers answer.")
}

func wakeOutcomeWords(state string) string {
	switch state {
	case domain.WakeOutcomeSent:
		return "sent"
	case domain.WakeOutcomeOn:
		return "already on"
	case domain.WakeOutcomeUnknown:
		return "address unknown"
	case domain.WakeOutcomeInvalid:
		return "invalid target"
	default:
		return "not sent"
	}
}

// WakePlanText explains a switch-on request that was refused: one line for
// each requested name that is not a client computer.
func WakePlanText(w io.Writer, p domain.WakePlan) {
	for _, target := range p.Targets {
		if target.State == domain.WakeTargetInvalid {
			detail := target.Detail
			if detail != "" {
				detail = strings.ToLower(detail[:1]) + detail[1:]
			}
			fmt.Fprintf(w, "%s  invalid target: %s\n", target.Name, detail)
		}
	}
	fmt.Fprintln(w, p.Message)
}

// WakeReportText prints one line for each computer, then what was sent.
func WakeReportText(w io.Writer, r domain.WakeReport) {
	for _, target := range r.Targets {
		line := target.Name + "  " + wakeOutcomeWords(target.State)
		switch target.State {
		case domain.WakeOutcomeUnknown:
			line += ": " + strings.TrimPrefix(target.Detail, "Address not known yet: ")
		case domain.WakeOutcomeFailed:
			line += ": " + strings.TrimPrefix(target.Detail, "The packet could not be sent: ")
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, r.Message)
}
