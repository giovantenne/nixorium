package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// shutdownModel owns the transient selection, review and result state for
// client shutdown. Navigation and application callbacks remain in the root.
type shutdownModel struct {
	cursor       int
	chosen       map[string]bool
	policy       domain.ShutdownSessionPolicy
	plan         domain.ShutdownPlanReport
	result       domain.ShutdownApplyReport
	applying     bool
	technical    bool
	confirmation string
}

func newShutdownModel() shutdownModel {
	return shutdownModel{
		chosen: map[string]bool{},
		policy: domain.ShutdownProtectUnknown,
	}
}

type shutdownIntentKind int

const (
	shutdownNoIntent shutdownIntentKind = iota
	shutdownCloseIntent
	shutdownSelectionIntent
	shutdownPlanIntent
	shutdownApplyIntent
	shutdownHistoryIntent
)

type shutdownIntent struct {
	kind      shutdownIntentKind
	requested string
	message   string
}

func (model shutdownModel) update(screen dashboardScreen, key tea.KeyPressMsg, hosts []domain.HostMeta) (shutdownModel, shutdownIntent) {
	switch screen {
	case dashboardShutdown:
		switch key.String() {
		case "esc", "left":
			return model, shutdownIntent{kind: shutdownCloseIntent}
		case "up", "k":
			model.cursor = max(0, model.cursor-1)
		case "down", "j":
			model.cursor = min(max(0, len(hosts)-1), model.cursor+1)
		case "space":
			if len(hosts) > 0 {
				name := hosts[model.cursor].Name
				model.chosen[name] = !model.chosen[name]
			}
		case "a":
			selectAll := false
			for _, host := range hosts {
				if !model.chosen[host.Name] {
					selectAll = true
					break
				}
			}
			for _, host := range hosts {
				model.chosen[host.Name] = selectAll
			}
		case "enter":
			requested := model.requested(hosts)
			if requested == "" {
				return model, shutdownIntent{message: "Select at least one client computer. The controller is never selectable."}
			}
			return model, shutdownIntent{kind: shutdownPlanIntent, requested: requested}
		}
	case dashboardShutdownReview:
		switch key.String() {
		case "esc":
			model.confirmation = ""
			return model, shutdownIntent{kind: shutdownSelectionIntent, message: "Shutdown cancelled; no request was sent."}
		case "u":
			if !shutdownHasUnknown(model.plan) {
				return model, shutdownIntent{message: "No selected computer has an unknown session state."}
			}
			if model.policy == domain.ShutdownAcknowledgeUnknown {
				model.policy = domain.ShutdownProtectUnknown
			} else {
				model.policy = domain.ShutdownAcknowledgeUnknown
			}
			model.confirmation = ""
			return model, shutdownIntent{kind: shutdownPlanIntent, requested: model.requested(hosts)}
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.plan.HasErrors() || model.plan.State != "ready" {
				return model, shutdownIntent{message: "No eligible shutdown request can be sent from this plan."}
			}
			if model.confirmation != model.plan.Confirmation {
				model.confirmation = ""
				return model, shutdownIntent{message: "Confirmation did not match; no shutdown request was sent."}
			}
			model.applying = true
			model.confirmation = ""
			return model, shutdownIntent{kind: shutdownApplyIntent}
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardShutdownResult:
		switch key.String() {
		case "enter", "esc", "left":
			return model, shutdownIntent{kind: shutdownCloseIntent}
		case "r":
			model.plan = domain.ShutdownPlanReport{}
			model.result = domain.ShutdownApplyReport{}
			return model, shutdownIntent{kind: shutdownSelectionIntent}
		case "l":
			return model, shutdownIntent{kind: shutdownHistoryIntent}
		case "t":
			if shutdownHasTechnicalDetail(model.result) {
				model.technical = !model.technical
			} else {
				return model, shutdownIntent{message: "No additional technical detail was captured."}
			}
		}
	}
	return model, shutdownIntent{}
}

func (model dashboardModel) updateShutdown(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	shutdown, intent := model.shutdown.update(model.screen, key, model.report.Meta.Clients.Hosts)
	model.shutdown = shutdown
	if intent.message != "" {
		model.message = intent.message
	}
	switch intent.kind {
	case shutdownCloseIntent:
		model.screen = dashboardHome
		model.message = ""
	case shutdownSelectionIntent:
		model.screen = dashboardShutdown
	case shutdownPlanIntent:
		return model.startShutdownPlan(intent.requested)
	case shutdownApplyIntent:
		if model.actions.ApplyShutdown == nil {
			model.shutdown.applying = false
			model.message = "Shutdown is not available in this deployment."
			return model, nil
		}
		model.busy = "Rechecking access and sending reviewed power-off requests"
		plan := model.shutdown.plan
		return model, func() tea.Msg { return dashboardShutdownApplyMsg{report: model.actions.ApplyShutdown(plan)} }
	case shutdownHistoryIntent:
		if model.actions.LoadLogs == nil {
			model.message = "Operation history is not available in this deployment."
			return model, nil
		}
		model.screen = dashboardLogs
		model.busy = "Loading private operation history"
		return model, func() tea.Msg { return dashboardLogsMsg{report: model.actions.LoadLogs()} }
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
	policy := model.shutdown.policy
	return model, func() tea.Msg { return dashboardShutdownPlanMsg{report: model.actions.PlanShutdown(requested, policy)} }
}

func (model dashboardModel) shutdownView() string {
	context := shutdownViewContext{
		height:   model.height,
		dark:     model.isDark,
		message:  model.message,
		busy:     model.busy,
		busyView: model.busyView(),
	}
	return model.renderShell(model.shutdown.view(model.screen, model.report.Meta.Clients.Hosts, context))
}

type shutdownViewContext struct {
	height   int
	dark     bool
	message  string
	busy     string
	busyView string
}

func (model shutdownModel) view(screen dashboardScreen, hosts []domain.HostMeta, context shutdownViewContext) tuiShell {
	shell := tuiShell{path: []string{"Computers", "Shut down"}}
	if context.busy != "" {
		shell.body = context.busyView
		if model.applying {
			shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: "Power-off requests are being dispatched", detail: "Closing is disabled until the reviewed operation returns."}}
		}
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return shell
	}
	switch screen {
	case dashboardShutdownReview:
		body, fixedBody := model.reviewView(context)
		shell.body = strings.Join(body, "\n")
		shell.fixedBody = strings.Join(fixedBody, "\n")
		if model.plan.State == "ready" {
			shell.actions = []tuiAction{{key: "Enter", label: "Send requests"}, {key: "u", label: "Unknown sessions"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		} else {
			shell.actions = []tuiAction{{key: "u", label: "Unknown sessions"}, {key: "Esc", label: "Selection"}, {key: "F1", label: "Help"}}
		}
	case dashboardShutdownResult:
		shell.body = strings.Join(model.resultView(context), "\n")
		shell.actions = []tuiAction{{key: "r", label: "New review"}, {key: "l", label: "History"}}
		if shutdownHasTechnicalDetail(model.result) {
			shell.actions = append(shell.actions, tuiAction{key: "t", label: "Technical"})
		}
		shell.actions = append(shell.actions, tuiAction{key: "Enter", label: "Computers"}, tuiAction{key: "?", label: "Help"})
	default:
		shell.body = strings.Join(model.selectionView(hosts, context), "\n")
		shell.actions = []tuiAction{{key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Enter", label: "Check"}, {key: "Esc", label: "Computers"}, {key: "?", label: "Help"}}
	}
	if context.message != "" && context.message != model.plan.Message {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: context.message})
	}
	return shell
}

func (model shutdownModel) selectionView(hosts []domain.HostMeta, context shutdownViewContext) []string {
	selected := 0
	for _, host := range hosts {
		if model.chosen[host.Name] {
			selected++
		}
	}
	lines := []string{tuiSection("Which client computers should receive the request?", context.dark), tuiMuted("Computers are checked only after you continue. The controller is never included.", context.dark), "", fmt.Sprintf("%d of %d clients selected", selected, len(hosts)), ""}
	start, end := listWindow(len(hosts), model.cursor, max(3, context.height-20))
	for index := start; index < end; index++ {
		host := hosts[index]
		checked := " "
		if model.chosen[host.Name] {
			checked = "x"
		}
		lines = append(lines, tuiSelection(fmt.Sprintf("[%s] %-10s %s", checked, host.Name, host.IP), index == model.cursor, context.dark))
	}
	if len(hosts) > end || start > 0 {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d", start+1, end, len(hosts)), context.dark))
	}
	if len(hosts) == 0 {
		lines = append(lines, "No configured client computers.")
	}
	lines = append(lines, "", "No request is queued for a computer that is off or unreachable.")
	return lines
}

func (model shutdownModel) reviewView(context shutdownViewContext) ([]string, []string) {
	plan := model.plan
	lines := []string{tuiSection(fmt.Sprintf("Shut down %d eligible client(s)?", plan.Eligible), context.dark), "", fmt.Sprintf("Selected  %d", len(plan.Targets)), fmt.Sprintf("Eligible  %d", plan.Eligible), "Controller  excluded", "Session safety  " + shutdownPolicyLabel(plan.Policy), ""}
	start, end := listWindow(len(plan.Targets), 0, max(3, context.height-20))
	for _, target := range plan.Targets[start:end] {
		lines = append(lines, shutdownTargetStatus(target, context.dark))
	}
	if end < len(plan.Targets) {
		lines = append(lines, tuiMuted(fmt.Sprintf("Showing %d of %d reviewed targets", end, len(plan.Targets)), context.dark))
	}
	warning := "Selected computers will be shut down; unsaved user work may be lost."
	if shutdownHasActive(plan) {
		warning = "Active user sessions will be shut down; unsaved work may be lost."
	}
	lines = append(lines, "", tuiStatus(warning, tuiStatusAttention, context.dark), "Access and session state are checked again immediately before requests are sent.", "An accepted request does not prove that a computer is physically off.")
	if shutdownHasUnknown(plan) {
		if plan.Policy == domain.ShutdownAcknowledgeUnknown {
			lines = append(lines, "", tuiStatus("Unknown session risk acknowledged", tuiStatusAttention, context.dark), "Press u to protect unknown session states again.")
		} else {
			lines = append(lines, "", "Press u to explicitly acknowledge unknown session state and create a new plan.")
		}
	}
	if plan.State == "ready" {
		confirmationPrompt := "Type " + plan.Confirmation + " to continue:"
		if shutdownActiveCount(plan) > 0 {
			confirmationPrompt = "Type " + plan.Confirmation + " to confirm shutdown of active sessions:"
		}
		return lines, []string{tuiSection(confirmationPrompt, context.dark), "> " + model.confirmation + "_"}
	} else {
		lines = append(lines, "", tuiStatus("No request can be sent from this plan", tuiStatusAttention, context.dark), plan.Message)
	}
	return lines, nil
}

func (model shutdownModel) resultView(context shutdownViewContext) []string {
	report := model.result
	success := report.State == "completed"
	title := "Shutdown requests need attention"
	if success {
		title = "Shutdown requests accepted"
	} else if report.State == "blocked" {
		title = "No shutdown request was sent"
	}
	lines := []string{tuiResult(title, success, context.dark), "", fmt.Sprintf("Accepted  %d    Not sent  %d    Unconfirmed  %d", report.Accepted, report.NotSent, report.Unconfirmed), ""}
	for _, target := range report.Targets {
		marker := "!"
		if target.State == "accepted" {
			marker = "✓"
		} else if target.State == "not-sent" {
			marker = "○"
		}
		lines = append(lines, fmt.Sprintf("%s %-10s %s", marker, target.Name, target.State), tuiMuted("  "+target.Detail, context.dark))
		if model.technical && target.TechnicalDetail != "" {
			lines = append(lines, tuiMuted("  Technical: "+target.TechnicalDetail, context.dark))
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

func (model shutdownModel) requested(hosts []domain.HostMeta) string {
	names := []string{}
	for _, host := range hosts {
		if model.chosen[host.Name] {
			names = append(names, host.Name)
		}
	}
	if len(names) == len(hosts) && len(hosts) > 0 {
		return "@lab"
	}
	return strings.Join(names, ",")
}
