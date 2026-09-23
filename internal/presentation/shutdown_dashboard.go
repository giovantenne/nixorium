package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateShutdown(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardShutdown:
		hosts := model.report.Meta.Clients.Hosts
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "up", "k":
			model.shutdownCursor = max(0, model.shutdownCursor-1)
		case "down", "j":
			model.shutdownCursor = min(max(0, len(hosts)-1), model.shutdownCursor+1)
		case "space":
			if len(hosts) > 0 {
				name := hosts[model.shutdownCursor].Name
				model.shutdownChosen[name] = !model.shutdownChosen[name]
			}
		case "a":
			selectAll := false
			for _, host := range hosts {
				if !model.shutdownChosen[host.Name] {
					selectAll = true
					break
				}
			}
			for _, host := range hosts {
				model.shutdownChosen[host.Name] = selectAll
			}
		case "enter":
			requested := model.shutdownRequested()
			if requested == "" {
				model.message = "Select at least one client computer. The controller is never selectable."
				return model, nil
			}
			return model.startShutdownPlan(requested)
		}
	case dashboardShutdownReview:
		switch key.String() {
		case "esc":
			model.confirmation = ""
			model.screen = dashboardShutdown
			model.message = "Shutdown cancelled; no request was sent."
		case "u":
			if !shutdownHasUnknown(model.shutdownPlan) {
				model.message = "No selected computer has an unknown session state."
				return model, nil
			}
			if model.shutdownPolicy == domain.ShutdownAcknowledgeUnknown {
				model.shutdownPolicy = domain.ShutdownProtectUnknown
			} else {
				model.shutdownPolicy = domain.ShutdownAcknowledgeUnknown
			}
			model.confirmation = ""
			return model.startShutdownPlan(model.shutdownRequested())
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.shutdownPlan.HasErrors() || model.shutdownPlan.State != "ready" {
				model.message = "No eligible shutdown request can be sent from this plan."
				return model, nil
			}
			if model.confirmation != model.shutdownPlan.Confirmation {
				model.confirmation = ""
				model.message = "Confirmation did not match; no shutdown request was sent."
				return model, nil
			}
			if model.actions.ApplyShutdown == nil {
				model.message = "Shutdown is not available in this deployment."
				return model, nil
			}
			model.busy = "Rechecking access and sending reviewed power-off requests"
			model.shutdownApplying = true
			model.confirmation = ""
			plan := model.shutdownPlan
			return model, func() tea.Msg { return dashboardShutdownApplyMsg{report: model.actions.ApplyShutdown(plan)} }
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardShutdownResult:
		switch key.String() {
		case "enter", "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "r":
			model.screen = dashboardShutdown
			model.shutdownPlan = domain.ShutdownPlanReport{}
			model.shutdownResult = domain.ShutdownApplyReport{}
			model.message = ""
		case "l":
			if model.actions.LoadLogs == nil {
				model.message = "Operation history is not available in this deployment."
				return model, nil
			}
			model.screen = dashboardLogs
			model.busy = "Loading private operation history"
			return model, func() tea.Msg { return dashboardLogsMsg{report: model.actions.LoadLogs()} }
		case "t":
			if shutdownHasTechnicalDetail(model.shutdownResult) {
				model.shutdownTechnical = !model.shutdownTechnical
			} else {
				model.message = "No additional technical detail was captured."
			}
		}
	}
	return model, nil
}

func (model dashboardModel) startShutdownPlan(requested string) (tea.Model, tea.Cmd) {
	if model.actions.PlanShutdown == nil {
		model.message = "Shutdown planning is not available in this deployment."
		return model, nil
	}
	model.busy = "Checking selected computers, user sessions and conflicting operations"
	model.message = ""
	policy := model.shutdownPolicy
	return model, func() tea.Msg { return dashboardShutdownPlanMsg{report: model.actions.PlanShutdown(requested, policy)} }
}

func (model dashboardModel) shutdownView() string {
	shell := tuiShell{path: []string{"Computers", "Shut down"}}
	if model.busy != "" {
		shell.body = model.busyView()
		if model.shutdownApplying {
			shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: "Power-off requests are being dispatched", detail: "Closing is disabled until the reviewed operation returns."}}
		}
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	switch model.screen {
	case dashboardShutdownReview:
		body, fixedBody := model.shutdownReviewView()
		shell.body = strings.Join(body, "\n")
		shell.fixedBody = strings.Join(fixedBody, "\n")
		if model.shutdownPlan.State == "ready" {
			shell.actions = []tuiAction{{key: "Enter", label: "Send requests"}, {key: "u", label: "Unknown sessions"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		} else {
			shell.actions = []tuiAction{{key: "u", label: "Unknown sessions"}, {key: "Esc", label: "Selection"}, {key: "F1", label: "Help"}}
		}
	case dashboardShutdownResult:
		shell.body = strings.Join(model.shutdownResultView(), "\n")
		shell.actions = []tuiAction{{key: "r", label: "New review"}, {key: "l", label: "History"}}
		if shutdownHasTechnicalDetail(model.shutdownResult) {
			shell.actions = append(shell.actions, tuiAction{key: "t", label: "Technical"})
		}
		shell.actions = append(shell.actions, tuiAction{key: "Enter", label: "Computers"}, tuiAction{key: "?", label: "Help"})
	default:
		shell.body = strings.Join(model.shutdownSelectionView(), "\n")
		shell.actions = []tuiAction{{key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Enter", label: "Check"}, {key: "Esc", label: "Computers"}, {key: "?", label: "Help"}}
	}
	if model.message != "" && model.message != model.shutdownPlan.Message {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(shell)
}

func (model dashboardModel) shutdownSelectionView() []string {
	hosts := model.report.Meta.Clients.Hosts
	selected := 0
	for _, host := range hosts {
		if model.shutdownChosen[host.Name] {
			selected++
		}
	}
	lines := []string{tuiSection("Which client computers should receive the request?", model.isDark), tuiMuted("Computers are checked only after you continue. The controller is never included.", model.isDark), "", fmt.Sprintf("%d of %d clients selected", selected, len(hosts)), ""}
	start, end := listWindow(len(hosts), model.shutdownCursor, max(3, model.height-20))
	for index := start; index < end; index++ {
		host := hosts[index]
		checked := " "
		if model.shutdownChosen[host.Name] {
			checked = "x"
		}
		lines = append(lines, tuiSelection(fmt.Sprintf("[%s] %-10s %s", checked, host.Name, host.IP), index == model.shutdownCursor, model.isDark))
	}
	if len(hosts) > end || start > 0 {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d", start+1, end, len(hosts)), model.isDark))
	}
	if len(hosts) == 0 {
		lines = append(lines, "No configured client computers.")
	}
	lines = append(lines, "", "No request is queued for a computer that is off or unreachable.")
	return lines
}

func (model dashboardModel) shutdownReviewView() ([]string, []string) {
	plan := model.shutdownPlan
	lines := []string{tuiSection(fmt.Sprintf("Shut down %d eligible client(s)?", plan.Eligible), model.isDark), "", fmt.Sprintf("Selected  %d", len(plan.Targets)), fmt.Sprintf("Eligible  %d", plan.Eligible), "Controller  excluded", "Session safety  " + shutdownPolicyLabel(plan.Policy), ""}
	start, end := listWindow(len(plan.Targets), 0, max(3, model.height-20))
	for _, target := range plan.Targets[start:end] {
		lines = append(lines, shutdownTargetStatus(target, model.isDark))
	}
	if end < len(plan.Targets) {
		lines = append(lines, tuiMuted(fmt.Sprintf("Showing %d of %d reviewed targets", end, len(plan.Targets)), model.isDark))
	}
	warning := "Selected computers will be shut down; unsaved user work may be lost."
	if shutdownHasActive(plan) {
		warning = "Active user sessions will be shut down; unsaved work may be lost."
	}
	lines = append(lines, "", tuiStatus(warning, tuiStatusAttention, model.isDark), "Access and session state are checked again immediately before requests are sent.", "An accepted request does not prove that a computer is physically off.")
	if shutdownHasUnknown(plan) {
		if plan.Policy == domain.ShutdownAcknowledgeUnknown {
			lines = append(lines, "", tuiStatus("Unknown session risk acknowledged", tuiStatusAttention, model.isDark), "Press u to protect unknown session states again.")
		} else {
			lines = append(lines, "", "Press u to explicitly acknowledge unknown session state and create a new plan.")
		}
	}
	if plan.State == "ready" {
		confirmationPrompt := "Type " + plan.Confirmation + " to continue:"
		if shutdownActiveCount(plan) > 0 {
			confirmationPrompt = "Type " + plan.Confirmation + " to confirm shutdown of active sessions:"
		}
		return lines, []string{tuiSection(confirmationPrompt, model.isDark), "> " + model.confirmation + "_"}
	} else {
		lines = append(lines, "", tuiStatus("No request can be sent from this plan", tuiStatusAttention, model.isDark), plan.Message)
	}
	return lines, nil
}

func (model dashboardModel) shutdownResultView() []string {
	report := model.shutdownResult
	success := report.State == "completed"
	title := "Shutdown requests need attention"
	if success {
		title = "Shutdown requests accepted"
	} else if report.State == "blocked" {
		title = "No shutdown request was sent"
	}
	lines := []string{tuiResult(title, success, model.isDark), "", fmt.Sprintf("Accepted  %d    Not sent  %d    Unconfirmed  %d", report.Accepted, report.NotSent, report.Unconfirmed), ""}
	for _, target := range report.Targets {
		marker := "!"
		if target.State == "accepted" {
			marker = "✓"
		} else if target.State == "not-sent" {
			marker = "○"
		}
		lines = append(lines, fmt.Sprintf("%s %-10s %s", marker, target.Name, target.State), tuiMuted("  "+target.Detail, model.isDark))
		if model.shutdownTechnical && target.TechnicalDetail != "" {
			lines = append(lines, tuiMuted("  Technical: "+target.TechnicalDetail, model.isDark))
		}
	}
	lines = append(lines, "", report.Message, "", "Network loss alone is not evidence of physical power state.")
	return lines
}

func shutdownHasTechnicalDetail(report domain.ShutdownApplyReport) bool {
	for _, target := range report.Targets {
		if target.TechnicalDetail != "" {
			return true
		}
	}
	return false
}

func shutdownTargetStatus(target domain.ShutdownTargetPlan, dark bool) string {
	switch {
	case target.Eligible && target.Session == domain.ShutdownSessionIdle:
		return tuiStatus(target.Name+" · Ready", tuiStatusSuccess, dark)
	case target.Eligible && target.Session == domain.ShutdownSessionActive:
		return tuiStatus(target.Name+" · Active user session · will shut down", tuiStatusAttention, dark)
	case target.Eligible:
		return tuiStatus(target.Name+" · Session unknown · risk acknowledged", tuiStatusAttention, dark)
	case target.Reachability != domain.ReachabilityReachable:
		return "○ " + target.Name + " · Not reachable · not sent"
	default:
		return tuiStatus(target.Name+" · Session unknown · not sent", tuiStatusAttention, dark)
	}
}

func shutdownHasActive(plan domain.ShutdownPlanReport) bool {
	return shutdownActiveCount(plan) > 0
}

func shutdownActiveCount(plan domain.ShutdownPlanReport) int {
	count := 0
	for _, target := range plan.Targets {
		if target.Eligible && target.Session == domain.ShutdownSessionActive {
			count++
		}
	}
	return count
}

func shutdownPolicyLabel(policy domain.ShutdownSessionPolicy) string {
	if policy == domain.ShutdownAcknowledgeUnknown {
		return "unknown states acknowledged"
	}
	return "unknown states protected"
}

func shutdownHasUnknown(plan domain.ShutdownPlanReport) bool {
	for _, target := range plan.Targets {
		if target.Session == domain.ShutdownSessionUnknown && target.Reachability == domain.ReachabilityReachable && target.SSH == domain.SSHAvailable {
			return true
		}
	}
	return false
}

func (model dashboardModel) shutdownRequested() string {
	names := []string{}
	hosts := model.report.Meta.Clients.Hosts
	for _, host := range hosts {
		if model.shutdownChosen[host.Name] {
			names = append(names, host.Name)
		}
	}
	if len(names) == len(hosts) && len(hosts) > 0 {
		return "@lab"
	}
	return strings.Join(names, ",")
}
