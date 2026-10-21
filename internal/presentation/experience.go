package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func statusLevel(level domain.Level) tuiStatusKind {
	switch level {
	case domain.LevelOK:
		return tuiStatusSuccess
	case domain.LevelWarning:
		return tuiStatusAttention
	case domain.LevelError:
		return tuiStatusFailure
	default:
		return tuiStatusNeutral
	}
}

func listWindow(total, cursor, capacity int) (int, int) {
	capacity = max(1, capacity)
	start := max(0, min(cursor-capacity/2, total-capacity))
	return start, min(total, start+capacity)
}

func (model dashboardModel) rowCapacity() int { return max(3, model.height-ctaRows(model.height)-15) }

func (model dashboardModel) filteredHosts() []domain.HostStatus {
	result := []domain.HostStatus{}
	query := strings.ToLower(model.computers.hostQuery)
	for _, h := range model.computers.hosts.Hosts {
		_, label, _ := domain.ComputerCondition(h)
		if strings.Contains(strings.ToLower(h.Name+" "+h.IP+" "+label), query) {
			result = append(result, h)
		}
	}
	return result
}

func (model dashboardModel) computerDetail(h domain.HostStatus) string {
	level, label, guidance := domain.ComputerCondition(h)
	lines := []string{tuiSection(h.Name, model.isDark), tuiStatus(label, statusLevel(level), model.isDark), "", guidance, "", tuiMuted("Address  "+h.IP, model.isDark)}
	if !model.computers.hosts.GeneratedAt.IsZero() {
		lines = append(lines, tuiMuted("Checked  "+model.computers.hosts.GeneratedAt.Local().Format("15:04:05"), model.isDark))
	}
	if model.computers.hostTechnical {
		lines = append(lines, "", tuiSection("Technical details", model.isDark), "Network: "+string(h.Reachability)+" · SSH: "+string(h.SSH), "Current revision: "+h.CurrentRevision, "Desired revision: "+h.DesiredRevision, "System: "+h.CurrentSystem, h.Detail, h.DeploymentDetail)
		if h.LastSuccessfulDeploy != nil {
			lines = append(lines, "Last verified deployment: "+h.LastSuccessfulDeploy.VerifiedAt.Format(time.RFC3339))
		}
		if model.computers.hosts.HistoryDetail != "" {
			lines = append(lines, "History warning: "+model.computers.hosts.HistoryDetail)
		}
	}
	return strings.Join(lines, "\n")
}

func (model dashboardModel) computersView() string {
	configurationView := model.computers.configurationState.Operation != ""
	title := "Computer inventory"
	path := []string{"Computers", "Inventory"}
	back := "Computers"
	if configurationView {
		title = "Current system configuration"
		path = []string{"Software", "System status"}
		back = "Software"
	}
	lines := []string{tuiTitle(title, model.isDark), ""}
	if model.busy != "" {
		return model.renderShell(tuiShell{
			path:    path,
			body:    model.busyView(),
			actions: []tuiAction{{key: "Esc", label: back}, {key: "F1", label: "Help"}},
		})
	}
	if configurationView {
		desired := model.computers.configurationState.DesiredRevision
		if desired == "" {
			desired = "changed while this snapshot was collected"
		} else {
			desired = shortRevision(desired)
		}
		controllerLabel := "Not verified for the desired revision"
		controllerKind := tuiStatusAttention
		if model.computers.configurationState.ControllerVerified() {
			controllerLabel = "Verified at the desired revision"
			controllerKind = tuiStatusSuccess
		}
		freshness := "not recorded"
		if !model.computers.configurationState.GeneratedAt.IsZero() {
			freshness = model.computers.configurationState.GeneratedAt.Local().Format("2006-01-02 15:04:05 MST") + " · r refreshes this snapshot"
		}
		lines = append(lines,
			tuiSection("Desired and observed state", model.isDark),
			"Desired revision  "+desired,
			"Controller        "+tuiStatus(controllerLabel, controllerKind, model.isDark),
			fmt.Sprintf("Clients           %d up to date · %d update ready · %d not verified", model.computers.hosts.Deployment.Current, model.computers.hosts.Deployment.Outdated, model.computers.hosts.Deployment.Unknown),
			tuiMuted("Freshness         "+freshness, model.isDark),
			"",
			tuiSection("Client observations", model.isDark),
		)
	}
	hosts := model.filteredHosts()
	var actions []tuiAction
	if model.computers.hostDetail && len(hosts) > 0 {
		lines = append(lines, model.computerDetail(hosts[min(model.computers.hostCursor, len(hosts)-1)]))
		actions = []tuiAction{{key: "t", label: "Technical"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		if !model.actions.ClassroomMode {
			keyChanged := hosts[min(model.computers.hostCursor, len(hosts)-1)].HostKeyCondition == domain.HostKeyChanged
			if keyChanged {
				// The detail says not to update it until the key is reviewed.
				actions = append([]tuiAction{{key: "i", label: "Diagnostics"}}, actions...)
			} else {
				actions = append([]tuiAction{{key: "d", label: "Update this computer"}, {key: "i", label: "Diagnostics"}}, actions...)
			}
			if keyChanged && model.actions.PlanHostTrust != nil && model.actions.ApplyHostTrust != nil {
				actions = append([]tuiAction{{key: "h", label: "Review changed SSH key"}}, actions...)
			}
		}
	} else {
		available, total := hostAvailability(model.computers.hosts.Hosts)
		lines = append(lines, fmt.Sprintf("%d / %d reachable · %d up to date · %d update ready", available, total, model.computers.hosts.Deployment.Current, model.computers.hosts.Deployment.Outdated))
		if !model.computers.hosts.GeneratedAt.IsZero() {
			lines = append(lines, tuiMuted("Observation: "+model.computers.hosts.GeneratedAt.Local().Format("15:04:05")+" · r refresh", model.isDark))
		}
		lines = append(lines, "")
		if model.computers.hostSearching || model.computers.hostQuery != "" {
			lines = append(lines, "Search  / "+model.computers.hostQuery+"_")
		}
		start, end := listWindow(len(hosts), model.computers.hostCursor, max(3, model.height-ctaRows(model.height)-18))
		rows := []string{}
		for index := start; index < end; index++ {
			h := hosts[index]
			level, label, _ := domain.ComputerCondition(h)
			marker := tuiSelectionMarker(index == model.computers.hostCursor, model.isDark)
			name := fmt.Sprintf("%-10s", h.Name)
			if index == model.computers.hostCursor {
				name = tuiFocusStyle(model.isDark).Render(name)
			}
			rows = append(rows, marker+name+" "+tuiStatus(label, statusLevel(level), model.isDark))
		}
		if len(hosts) == 0 {
			if model.computers.hostQuery != "" {
				rows = append(rows, "No computers match your search.", "Press Esc to clear it.")
			} else {
				rows = append(rows, "No computer observations yet.", "Refresh to check the configured computers.")
			}
		}
		body := strings.Join(rows, "\n")
		if model.width >= 108 && len(hosts) > 0 {
			left := lipgloss.NewStyle().Width(43).Render(body)
			right := lipgloss.NewStyle().Width(min(62, model.width-52)).Render(model.computerDetail(hosts[min(model.computers.hostCursor, len(hosts)-1)]))
			body = lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
		}
		lines = append(lines, body, "", tuiMuted(fmt.Sprintf("%d–%d of %d computers", displayedLineStart(start, len(hosts)), end, len(hosts)), model.isDark))
		actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Details"}, {key: "/", label: "Search"}, {key: "r", label: "Refresh"}, {key: "Esc", label: back}, {key: "F1", label: "Help"}}
		if !model.actions.ClassroomMode && model.computers.hosts.Deployment.Outdated > 0 {
			actions = append([]tuiAction{{key: "u", label: "Update those that need it"}}, actions...)
		}
	}
	notices := []tuiNotice{}
	if configurationView && model.computers.configurationState.HasErrors() {
		details := make([]string, 0, len(model.computers.configurationState.Issues))
		for _, issue := range model.computers.configurationState.Issues {
			details = append(details, issue.Field+": "+issue.Message)
		}
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "The state snapshot is incomplete", detail: strings.Join(details, "; ")})
	} else if configurationView && !model.computers.configurationState.ControllerVerified() && model.computers.configurationState.Controller.CurrentDetail != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Controller is not verified for the desired revision", detail: model.computers.configurationState.Controller.CurrentDetail})
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{
		path:    path,
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: actions,
	})
}

func (model dashboardModel) administrationView() string {
	if model.navigation.menuRows == 0 {
		return model.fitMenu(len(administrationTasks), func(fitted dashboardModel) string { return fitted.administrationView() })
	}
	lines := []string{
		tuiTitle("Choose a maintenance task", model.isDark),
		tuiMuted("Configuration, controller operations and technical evidence", model.isDark),
		"",
	}
	lines = append(lines, model.taskMenu(administrationTasks, model.navigation.adminCursor))
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	if model.report.StoreSpaceLow && model.report.StoreFreeBytes != nil {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: fmt.Sprintf("Low disk space: %.1f GiB free at the last check.", float64(*model.report.StoreFreeBytes)/(1<<30)), detail: "Open Free disk space (f) to remove old system versions after review."})
	}
	return model.renderShell(tuiShell{
		path:    []string{"Maintenance"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}},
	})
}

func (model dashboardModel) helpView() string {
	if model.screen == dashboardAssistant && (model.assistant.state == "chat" || model.assistant.state == "running") {
		return assistantHelpText
	}
	lines := []string{tuiTitle("Keyboard help", model.isDark), "", "↑ ↓ / j k   Move through lists", "Enter       Open, review, or confirm what is on screen", "Esc         Back / cancel / clear search", "/           Search Computers or a settings list", "?           Open or close help (F1 also works in text fields)", "q           Quit outside text entry", "Shift ↑/↓   Scroll a page that exceeds the terminal", "", tuiSection("In this view", model.isDark)}
	text, known := screenHelp[model.screen]
	if !known {
		text = helpText{head: []string{"Follow the contextual controls and review before applying.", "Text fields keep their normal typing keys; F1 opens help."}}
	}
	lines = append(lines, text.head...)
	lines = append(lines, model.dynamicHelp()...)
	lines = append(lines, text.tail...)
	lines = append(lines, "", tuiMuted("Questions or feedback: hello@nixorium.org · nixorium.org", model.isDark))
	return strings.Join(append(lines, "", "Esc / ? / F1 close help"), "\n")
}

// dynamicHelp returns the help lines that depend on model state; the static
// text of every screen lives in screenHelp.
func (model dashboardModel) dynamicHelp() []string {
	lines := []string{}
	switch model.screen {
	case dashboardHome:
		lines = append(lines, taskHelp(model.homeAreas())...)
		if model.updateNotification.Key != "" {
			lines = append(lines, "u  Review available updates", "x  Dismiss this update version")
		}
		if model.actions.LoadManagedJobs != nil && !model.actions.ClassroomMode {
			lines = append(lines, "v  See work already running or finished (nothing is started)")
		}
	case dashboardAssistant:
		switch model.assistant.state {
		case "welcome":
			lines = append(lines, "Up/Down Choose free trial, activation code or subscription; Enter Confirm; Esc Back")
		case "activate":
			lines = append(lines, "Enter Activate; Esc Cancel")
		case "consent":
			lines = append(lines, "Space Switch saving; Enter Continue without sending; Esc Back")
		case "name", "name-saving":
			lines = append(lines, "Type the laboratory name (60 characters); Enter Save; an empty name removes it; Esc Cancel")
		case "sharing":
			lines = append(lines, "Enter Switch conversation saving; Esc Cancel")
		case "report":
			lines = append(lines, "Up/Down Helpful or Wrong; type an optional comment (500 characters); Enter Send report; Esc Cancel")
		case "report-sending":
			lines = append(lines, "Esc Cancel report submission")
		case "running":
			lines = append(lines, "Esc Stop; the deployment remains unchanged")
		default:
			lines = append(lines, "Enter Send; Alt+Enter or Ctrl+J New line; PgUp/PgDn History", "Ctrl+N New chat; Ctrl+R Review changed files; Esc Back", "/review Review proposal; /new New chat; /report Report this answer", "/share Save conversations; /name Laboratory name; /help Help; /code Activation code")
		}
	case dashboardComputersArea:
		lines = append(lines, taskHelp(model.availableComputerTasks())...)
	case dashboardInstallationArea:
		lines = append(lines, taskHelp(installationAreaTasks)...)
	case dashboardAdministration:
		lines = append(lines, taskHelp(administrationTasks)...)
	case dashboardHosts:
		if !model.actions.ClassroomMode {
			lines = append(lines, "d review a deployment for the focused computer", "u review updating every computer that needed it at the last check")
			lines = append(lines, "h review changed SSH trust in a computer's details, after verifying its physical fingerprint")
		}
	case dashboardDeploy:
		if model.deployment.usbRecovery != nil {
			lines = append(lines, "Enter runs the visible recovery action; it never starts deployment.",
				"For an installed computer awaiting its final check: turn it on, boot from its disk, and connect the network cable.",
				"r checks installation status again; d shows technical details; i opens the existing installation when available.",
				"Esc keeps your selection. Successful recovery creates a fresh review that Enter confirms again.")
			break
		}
		lines = append(lines, "Space select   a select/deselect all   n select those needing the update   r check computers   Enter review", "States come from the last check in this session, shown with its time; the review probes again.", "During deployment: l private output details when there are any; s review stopping local supervision; q cannot interrupt", "Stopping asks for Enter on its own review; remote activation may continue and require recovery.", "After result: l logs   n new review when no recovery is required   Enter Computers, or the review of an interrupted update   d technical details")
	case dashboardCleanup:
		lines = append(lines, fmt.Sprintf("Each computer keeps its newest %d system versions plus the running and booted ones; the review lists what goes.", domain.CleanupKeepGenerations))
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		if model.actions.PlanWake != nil {
			lines = append(lines, "Tab also chooses Switch on: a Wake-on-LAN packet for the selected computers that are off and whose network address the controller has learned; n then selects those off at the last check.", "A sent packet does not prove that a computer starts: Wake-on-LAN must be allowed in its firmware.")
		}
	case dashboardSettings:
		for _, group := range routineSettingsGroups {
			lines = append(lines, group.shortcut+"  "+group.label)
		}
	case dashboardSettingsPasswords:
		if model.settings.returnScreen != dashboardSetup && !model.installation.flow {
			lines = append(lines, "After review and save, apply the configuration to this controller, then update the computers. Esc returns to Accounts.")
		}
	case dashboardPXE:
		if pxeRecoveryMode(model.report.PXE.Mode) {
			lines = append(lines, "After an interrupted network installation, Enter recovers the network: it puts back the controller's normal address and turns network installation off. Nothing changes on the student computers, so no word needs typing.",
				"x stops both network installation services instead and checks the normal address. d shows the technical text of a failed attempt.")
		}
	case dashboardRecoveryReview:
		if model.recovery.review.kind == "deploy" {
			lines = append(lines, "r check every computer again   c continue without the computers that do not answer (c again includes them)",
				"Enter marks the update as reviewed once every computer was checked   d technical details of the failure   Esc back",
				"Marking it as reviewed changes nothing on the computers: it archives the record and unblocks other operations. Then update the computers again.")
		} else {
			lines = append(lines, "r check the repository again   Enter confirm   Esc back without changing anything",
				"When the reset had finished, only the branch move is missing, or nothing was replaced, Enter confirms: no file changes and the marker is archived.",
				"When files from before and after the reset are mixed, type RESTORE: the current files are saved under a recovery reference and the previous configuration comes back; untracked and ignored files are not removed.")
		}
	case dashboardUpdate, dashboardUpdateReview:
		if model.updates.packageBase {
			lines = append(lines, "Enter check current channel   m change channel   r inspect pin", "In the channel form, type nixos-YY.MM and Space to acknowledge unverified compatibility.", "Review/build precedes saving and controller activation. Client distribution is separate.", "Build success does not verify runtime or hardware; check boot and services afterwards.")
		} else {
			lines = append(lines, "↑/↓ select release   Enter validate   p show prereleases", "Validation prepares the selected version, checks the lab configuration and tests required systems without saving it.", "On the review, Enter saves and activates; Esc cancels.", "After result: n new update; s completes an interrupted save")
		}
	case dashboardDiagnostics:
		if model.actions.PreviewSupport != nil && !model.actions.ClassroomMode {
			lines = append(lines, "e  Preview a minimized support report; no save or upload yet")
		}
	}
	return lines
}

func (model dashboardModel) diagnosticsView() string {
	path := []string{"Maintenance", "Diagnostics"}
	if model.diagnostics.back == dashboardHosts {
		path = []string{"Computers", "Inventory", "Diagnostics"}
	}
	lines := []string{tuiTitle("Diagnostics", model.isDark), ""}
	if model.busy != "" {
		return model.renderShell(tuiShell{path: path, body: strings.Join(append(lines, model.busyView()), "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	findings := model.diagnostics.report.Findings
	if len(findings) == 0 {
		lines = append(lines, "No diagnostic observations available.", "Press r to run the laboratory checks.")
	}
	start, end := listWindow(len(findings), model.diagnostics.cursor, max(2, (model.height-ctaRows(model.height)-12)/3))
	for i := start; i < end; i++ {
		f := findings[i]
		lines = append(lines, tuiSelectionMarker(i == model.diagnostics.cursor, model.isDark)+tuiStatus(f.Summary, statusLevel(f.Level), model.isDark))
		if f.Remediation != "" {
			lines = append(lines, "  "+f.Remediation)
		}
		if model.diagnostics.details && i == model.diagnostics.cursor {
			lines = append(lines, tuiMuted("  "+f.ID+": "+f.Evidence, model.isDark))
		}
		lines = append(lines, "")
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if len(findings) > 0 {
		actions = append(actions, tuiAction{key: "↑/↓", label: "Select"}, tuiAction{key: "Enter", label: "Evidence"})
	}
	back := "Maintenance"
	if model.diagnostics.back == dashboardHosts {
		back = "Inventory"
	}
	actions = append(actions, tuiAction{key: "r", label: "Check again"}, tuiAction{key: "Esc", label: back}, tuiAction{key: "F1", label: "Help"})
	if model.actions.PreviewSupport != nil && !model.actions.ClassroomMode {
		actions = append([]tuiAction{{key: "e", label: "Support report"}}, actions...)
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

// frame bounds plain content such as contextual help. Routine screens use
// renderShell so their body, notices, confirmations, and actions stay distinct.
func (model dashboardModel) frame(content string) string {
	if model.screen == dashboardAssistant || model.screen == dashboardAssistantReview {
		content = assistantLinkify(content)
	}
	width := min(116, max(20, model.width-6))
	if model.width == 0 {
		width = 100
	}
	content = lipgloss.NewStyle().Width(width).Render(strings.TrimRight(content, "\n"))
	lines := strings.Split(content, "\n")
	height := model.height - 4
	if model.height == 0 {
		height = len(lines)
	}
	height = max(2, height)
	if len(lines) > height {
		start := min(model.navigation.pageScroll, len(lines)-height+1)
		lines = append(lines[start:min(len(lines), start+height-1)], tuiMuted("Shift ↑/↓ scroll · ? help", model.isDark))
	}
	return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(lines, "\n"))
}

func (model dashboardModel) renderShell(shell tuiShell) string {
	shell = model.withResultSituation(shell)
	if model.busy != "" && model.screen != dashboardDisclaimer {
		if !strings.Contains(shell.body, model.busy) && !strings.Contains(shell.fixedBody, model.busy) {
			shell.fixedBody = strings.TrimSpace(model.busyView() + "\n" + shell.fixedBody)
		}
		if model.height > 0 && model.height < 28 {
			lines := []string{}
			for _, line := range strings.Split(shell.body, "\n") {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
			}
			shell.body = strings.Join(lines, "\n")
		}
		if shell.fixedBody != "" {
			shell.fixedBody += "\n"
		}
		shell.fixedBody += model.activityStatus()
		if model.message != "" {
			present := false
			for _, notice := range shell.notices {
				present = present || strings.Contains(notice.title+notice.detail, model.message)
			}
			if !present {
				shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
			}
		}
		if model.read.cancel != nil {
			shell.actions = []tuiAction{{key: "Esc", label: "Cancel read"}, {key: "F1", label: "Help"}}
			if model.updates.planning && model.updatePlanDetailsAvailable() {
				shell.actions = append([]tuiAction{{key: "l", label: "Progress details"}}, shell.actions...)
			}
		} else if !model.deployment.stopReview {
			// The footer must not advertise navigation or repeat a mutation
			// while its result is pending. Reviewed deployment stop is separate.
			shell.actions = []tuiAction{{key: "F1", label: "Help"}}
			if model.operationDetailsActive() {
				shell.actions = append([]tuiAction{{key: "l", label: "Progress details"}}, shell.actions...)
			}
			if model.deployment.applying && model.deployment.cancel != nil && !model.deployment.stopRequested {
				shell.actions = append(shell.actions, tuiAction{key: "s", label: "Stop waiting"})
			}
			if model.installation.pxePreparing {
				shell.actions = append(shell.actions, tuiAction{key: "q", label: "Quit Nixorium; work continues"})
			}
		}
	}
	width := min(116, max(20, model.width-6))
	if model.width == 0 {
		width = 100
	}
	shell.compactPrimary = compactTerminal(model.height)
	regions := buildTUIShellRegions(shell, model.width, model.isDark)
	wrappedLines := func(content string) []string {
		if content == "" {
			return nil
		}
		if model.screen == dashboardAssistant || model.screen == dashboardAssistantReview {
			content = assistantLinkify(content)
		}
		rendered := lipgloss.NewStyle().Width(width).Render(content)
		return strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	}
	header := wrappedLines(regions.header)
	body := wrappedLines(regions.body)
	fixed := wrappedLines(regions.fixedBody)
	notices := wrappedLines(regions.notices)
	actions := wrappedLines(regions.actions)

	compose := func(bodyLines []string, showScrollHint bool) []string {
		lines := append([]string{}, header...)
		lines = append(lines, "")
		lines = append(lines, bodyLines...)
		if showScrollHint {
			lines = append(lines, tuiMuted("Shift ↑/↓ scroll · ? help", model.isDark))
		}
		for _, region := range [][]string{fixed, notices, actions} {
			if len(region) > 0 {
				lines = append(lines, "")
				lines = append(lines, region...)
			}
		}
		return lines
	}

	height := model.height - 4
	if model.height == 0 {
		height = len(compose(body, false))
	}
	height = max(2, height)
	lines := compose(body, false)
	if len(lines) > height {
		fixedHeight := len(compose(nil, true))
		bodyHeight := height - fixedHeight
		if bodyHeight > 0 {
			start := min(model.navigation.pageScroll, max(0, len(body)-bodyHeight))
			lines = compose(body[start:min(len(body), start+bodyHeight)], true)
		} else {
			return model.frame(renderTUIShell(shell, model.width, model.isDark))
		}
	}
	return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(lines, "\n"))
}

func (model dashboardModel) textEntry() bool {
	if model.screen == dashboardBackup {
		return !model.backup.done
	}
	if model.screen == dashboardAssistantReview {
		return false
	}
	if model.screen == dashboardAssistant {
		return model.assistant.state == "chat" || model.assistant.state == "activate" || model.assistant.state == "name"
	}
	if model.screen == dashboardHostTrust {
		return model.hostTrust.cancel == nil && !model.hostTrust.applying && model.hostTrust.result.Operation == "" && !model.hostTrust.plan.HasErrors()
	}
	if model.screen == dashboardGitUndoReview {
		return !model.maintenance.gitUndoPlan.HasErrors()
	}
	if model.screen == dashboardUSBInstall {
		switch model.installation.remote.stage {
		case remoteInstallConsole, remoteInstallRotateHostKey, remoteInstallPassword, remoteInstallReview, remoteInstallConfirmReboot, remoteInstallConfirmClose, remoteInstallConfirmCancel:
			return true
		}
	}
	if model.screen == dashboardUpdate && model.updates.packageBase && model.updates.baseEditing {
		return true
	}
	if model.screen == dashboardSetupKeys && model.keys.importing {
		return true
	}
	if model.screen == dashboardRecoveryReview && model.recovery.review.kind == "reset" && !model.recovery.review.done {
		ready, word := model.recovery.review.ready()
		return ready && word != ""
	}
	switch model.screen {
	case dashboardSettings:
		return model.settings.menu.filtering()
	case dashboardDeployReview, dashboardControllerReview, dashboardServicesRestartReview,
		dashboardGitCommitReview, dashboardUpdateReview, dashboardShutdownReview,
		dashboardSettingsEdit, dashboardPXEStartReview, dashboardPXELeaveReview:
		return true
	case dashboardHosts:
		return model.computers.hostSearching
	case dashboardSoftware:
		return model.software.acceptsText()
	case dashboardWorkspace:
		return model.workspace.textEntry()
	case dashboardHomeFiles:
		return model.homeFiles.textEntry()
	case dashboardTemplateReset:
		return model.templateReset.stage == "review"
	default:
		return false
	}
}

func (model dashboardModel) startDiagnostics() (tea.Model, tea.Cmd) {
	if model.actions.LoadDoctor == nil {
		model.message = "Diagnostics are not available in this session."
		return model, nil
	}
	model.busy = "Checking configuration, network and services"
	model.message = ""
	action := model.actions.LoadDoctor
	return model.startRead(func(ctx context.Context) tea.Msg {
		report, err := action(ctx)
		return dashboardDoctorMsg{report: report, err: err}
	})
}

func phaseSteps(labels []string, current int, complete bool, dark bool) []string {
	lines := []string{""}
	for index, label := range labels {
		if complete || index < current {
			lines = append(lines, tuiStatus(label, tuiStatusSuccess, dark))
		} else if index == current {
			lines = append(lines, tuiTitle("● "+label+" · Running", dark))
		} else {
			lines = append(lines, tuiMuted("○ "+label+" · Waiting", dark))
		}
	}
	return lines
}

func (model dashboardModel) releaseReviewView() string {
	if model.updates.plan.Workspace != nil {
		return model.workspaceUpdateReviewView()
	}
	lines := []string{tuiTitle("Review Nixorium update", model.isDark), "", tuiSection("Validated release", model.isDark), fmt.Sprintf("%s → %s (%s)", model.updates.plan.CurrentRef, model.updates.plan.Target, model.updates.plan.TargetChannel), "Save flake.nix and flake.lock, then build and activate this controller", "No push, PXE action, or client deployment is included", fmt.Sprintf("Candidate checks: %d reviewed · F4 details", len(model.updates.plan.Checks))}
	if model.updates.packageBase {
		lines[0] = tuiTitle("Review system and package update", model.isDark)
		lines[2] = "Kernel, services and application versions may all change"
		if base := model.updates.plan.PackageBase; base != nil {
			lines[3] = fmt.Sprintf("%s → %s · %s → %s", base.CurrentChannel, base.TargetChannel, shortRevision(base.CurrentRevision), shortRevision(base.TargetRevision))
		}
	}
	if model.updateDetails || model.height == 0 {
		lines = append(lines, "Revision: "+model.updates.plan.Revision, fmt.Sprintf("Downgrade: %t", model.updates.plan.Downgrade))
		for _, check := range model.updates.plan.Checks {
			lines = append(lines, check.ID+" · "+check.State+" · "+check.Message)
		}
	}
	patch := updateReviewLines(model.updates.plan, model.updateDetails)
	start := min(model.updates.scroll, max(0, len(patch)-model.updateReviewHeight()))
	end := min(len(patch), start+model.updateReviewHeight())
	lines = append(lines, "", fmt.Sprintf("Diff lines %d-%d of %d", start+1, end, len(patch)))
	lines = append(lines, patch[start:end]...)
	lines = append(lines, "", "Press Enter to save this validated update and activate the controller.")
	notices := []tuiNotice{}
	if model.updates.packageBase {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Builds passed; runtime and hardware remain unverified", detail: "Check the controller and a selected client before distributing to the fleet."})
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", model.updateTitle(), "Review"}, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "↑/↓", label: "Scroll diff"}, {key: "F4", label: "Details"}, {key: "Enter", label: "Apply update"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}})
}
