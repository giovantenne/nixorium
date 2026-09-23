package presentation

import (
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

func (model dashboardModel) rowCapacity() int { return max(3, model.height-15) }

func (model dashboardModel) filteredHosts() []domain.HostStatus {
	result := []domain.HostStatus{}
	query := strings.ToLower(model.hostQuery)
	for _, h := range model.hosts.Hosts {
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
	if !model.hosts.GeneratedAt.IsZero() {
		lines = append(lines, tuiMuted("Checked  "+model.hosts.GeneratedAt.Local().Format("15:04:05"), model.isDark))
	}
	if model.hostTechnical {
		lines = append(lines, "", tuiSection("Technical details", model.isDark), "Network: "+string(h.Reachability)+" · SSH: "+string(h.SSH), "Current revision: "+h.CurrentRevision, "Desired revision: "+h.DesiredRevision, "System: "+h.CurrentSystem, h.Detail, h.DeploymentDetail)
		if h.LastSuccessfulDeploy != nil {
			lines = append(lines, "Last verified deployment: "+h.LastSuccessfulDeploy.VerifiedAt.Format(time.RFC3339))
		}
		if model.hosts.HistoryDetail != "" {
			lines = append(lines, "History warning: "+model.hosts.HistoryDetail)
		}
	}
	return strings.Join(lines, "\n")
}

func (model dashboardModel) computersView() string {
	lines := []string{tuiTitle("Computer inventory", model.isDark), ""}
	if model.busy != "" {
		return model.renderShell(tuiShell{
			path:    []string{"Computers", "Inventory"},
			body:    model.busyView(),
			actions: []tuiAction{{key: "Esc", label: "Computers"}, {key: "F1", label: "Help"}},
		})
	}
	hosts := model.filteredHosts()
	var actions []tuiAction
	if model.hostDetail && len(hosts) > 0 {
		lines = append(lines, model.computerDetail(hosts[min(model.hostCursor, len(hosts)-1)]))
		actions = []tuiAction{{key: "d", label: "Deploy"}, {key: "t", label: "Technical"}, {key: "i", label: "Diagnostics"}, {key: "Esc", label: "Back"}, {key: "?", label: "Help"}}
	} else {
		available, total := hostAvailability(model.hosts.Hosts)
		lines = append(lines, fmt.Sprintf("%d / %d reachable · %d up to date · %d update ready", available, total, model.hosts.Deployment.Current, model.hosts.Deployment.Outdated))
		if !model.hosts.GeneratedAt.IsZero() {
			lines = append(lines, tuiMuted("Observation: "+model.hosts.GeneratedAt.Local().Format("15:04:05")+" · r refresh", model.isDark))
		}
		lines = append(lines, "")
		if model.hostSearching || model.hostQuery != "" {
			lines = append(lines, "Search  / "+model.hostQuery+"_")
		}
		start, end := listWindow(len(hosts), model.hostCursor, max(3, model.height-18))
		rows := []string{}
		for index := start; index < end; index++ {
			h := hosts[index]
			level, label, _ := domain.ComputerCondition(h)
			marker := tuiSelectionMarker(index == model.hostCursor, model.isDark)
			name := fmt.Sprintf("%-10s", h.Name)
			if index == model.hostCursor {
				name = lipgloss.NewStyle().Bold(true).Foreground(tuiAccent(model.isDark)).Render(name)
			}
			rows = append(rows, marker+name+" "+tuiStatus(label, statusLevel(level), model.isDark))
		}
		if len(hosts) == 0 {
			if model.hostQuery != "" {
				rows = append(rows, "No computers match your search.", "Press Esc to clear it.")
			} else {
				rows = append(rows, "No computer observations yet.", "Refresh to check the configured computers.")
			}
		}
		body := strings.Join(rows, "\n")
		if model.width >= 108 && len(hosts) > 0 {
			left := lipgloss.NewStyle().Width(43).Render(body)
			right := lipgloss.NewStyle().Width(min(62, model.width-52)).Render(model.computerDetail(hosts[min(model.hostCursor, len(hosts)-1)]))
			body = lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
		}
		lines = append(lines, body, "", tuiMuted(fmt.Sprintf("%d–%d of %d computers", displayedLineStart(start, len(hosts)), end, len(hosts)), model.isDark))
		actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Details"}, {key: "/", label: "Search"}, {key: "r", label: "Refresh"}, {key: "Esc", label: "Computers"}, {key: "?", label: "Help"}}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{
		path:    []string{"Computers", "Inventory"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: actions,
	})
}

func (model dashboardModel) administrationView() string {
	lines := []string{
		tuiTitle("Choose a maintenance task", model.isDark),
		tuiMuted("Configuration, controller operations and technical evidence", model.isDark),
		"",
	}
	start, end := listWindow(len(administrationTasks), model.adminCursor, max(3, (model.height-15)/2))
	for i := start; i < end; i++ {
		task := administrationTasks[i]
		lines = append(lines, tuiSelection(task.title+"  ["+task.shortcut+"]", i == model.adminCursor, model.isDark), tuiMuted("    "+task.description, model.isDark))
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return model.renderShell(tuiShell{
		path:    []string{"Maintenance"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}, {key: "?", label: "Help"}},
	})
}

func (model dashboardModel) helpView() string {
	lines := []string{tuiTitle("Keyboard help", model.isDark), "", "↑ ↓ / j k   Move through lists", "Enter       Open, review, or confirm the exact phrase", "Esc         Back / cancel / clear search", "/           Search Computers or a settings list", "?           Open or close help (F1 also works in text fields)", "q           Quit outside text entry", "Shift ↑/↓   Scroll a page that exceeds the terminal", "", tuiSection("In this view", model.isDark)}
	switch model.screen {
	case dashboardHome:
		lines = append(lines, "Choose Computers, Installation, Software or Maintenance.", "Use the visible list with arrows and Enter; direct routes are not required.")
	case dashboardComputersArea:
		lines = append(lines, "Choose inventory, distribute, restore or shut down.", "Observed state is loaded only by the task that needs it.")
	case dashboardInstallationArea:
		lines = append(lines, "Install computers validates and saves laboratory settings, prepares every configured client, then asks once before starting PXE.", "PXE mode and network recovery is the advanced controller-side view.")
	case dashboardRestore:
		lines = append(lines, "Choose reapply to keep the disk, or reinstall to start PXE.", "Identity and disk erasure are confirmed locally on each computer.")
	case dashboardAdministration:
		lines = append(lines, "u update Nixorium   e settings   c controller", "s controller services   g changes   l history   i diagnostics")
	case dashboardHosts:
		lines = append(lines, "r refresh computers   / search names, addresses or status", "Enter open details   t technical detail   i diagnostics", "d review a deployment for the focused computer", "Search owns all text keys until Enter or Esc.")
	case dashboardDeploy:
		lines = append(lines, "Space select   a select/deselect all   Enter review", "During deployment: l progress details; q cannot interrupt", "After result: l logs   r new review   Enter overview")
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		lines = append(lines, "Space select   a select/deselect all   Enter check/review", "u acknowledge unknown sessions in review   Esc cancel", "An accepted request does not prove physical power state.")
	case dashboardSetup:
		lines = append(lines, "Enter continue the observed stage   t full checklist")
	case dashboardPXE:
		lines = append(lines, "p prepare   s review start   x stop   r recover", "l progress details while preparing; q closes only the view")
	case dashboardController:
		lines = append(lines, "r new review   d result details   l logs / progress detail", "q closes the view; systemd-owned work continues")
	case dashboardServices:
		lines = append(lines, "r review cache restart   f refresh   l logs after result")
	case dashboardLogs, dashboardLogDetail:
		lines = append(lines, "Enter open log   f refresh list   ↑/↓/pg scroll detail")
	case dashboardGitReview, dashboardGitCommitSelect, dashboardGitCommitReview:
		lines = append(lines, "c select commit paths   Space select   a all safe paths", "f refresh review   ↑/↓/pg scroll patch", "Exact confirmation creates a local commit; nothing is pushed.")
	case dashboardUpdate, dashboardUpdateReview:
		if model.baseUpdate {
			lines = append(lines, "Enter check current channel   m change channel   r inspect pin", "In the channel form, type nixos-YY.MM and Space to acknowledge unverified compatibility.", "Review/build precedes saving and controller activation. Client distribution is separate.", "Build success does not verify runtime or hardware; check boot and services afterwards.")
		} else {
			lines = append(lines, "↑/↓ select release   Enter validate   p show prereleases", "Validation prepares the selected version, checks the lab configuration and tests required systems without saving it.", "On the review, Enter saves and activates; Esc cancels.", "After result: r new update")
		}
	case dashboardDiagnostics:
		lines = append(lines, "↑/↓ move   Enter technical evidence   r run checks again")
	default:
		lines = append(lines, "Follow the contextual controls and review before applying.", "Text fields keep their normal typing keys; F1 opens help.")
	}
	return strings.Join(append(lines, "", "Esc / ? / F1 close help"), "\n")
}

func (model dashboardModel) diagnosticsView() string {
	path := []string{"Maintenance", "Diagnostics"}
	if model.diagnosticReturn == dashboardHosts {
		path = []string{"Computers", "Inventory", "Diagnostics"}
	}
	lines := []string{tuiTitle("Diagnostics", model.isDark), ""}
	if model.busy != "" {
		return model.renderShell(tuiShell{path: path, body: strings.Join(append(lines, model.busyView()), "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	findings := model.doctor.Findings
	if len(findings) == 0 {
		lines = append(lines, "No diagnostic observations available.", "Press r to run the laboratory checks.")
	}
	start, end := listWindow(len(findings), model.diagnosticCursor, max(2, (model.height-12)/3))
	for i := start; i < end; i++ {
		f := findings[i]
		lines = append(lines, tuiSelectionMarker(i == model.diagnosticCursor, model.isDark)+tuiStatus(f.Summary, statusLevel(f.Level), model.isDark))
		if f.Remediation != "" {
			lines = append(lines, "  "+f.Remediation)
		}
		if model.diagnosticDetails && i == model.diagnosticCursor {
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
	if model.diagnosticReturn == dashboardHosts {
		back = "Inventory"
	}
	actions = append(actions, tuiAction{key: "r", label: "Check again"}, tuiAction{key: "Esc", label: back}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) restoreView() string {
	options := []struct {
		title       string
		description string
	}{
		{"Reapply the intended system", "Keeps the disk and deploys the declared configuration again."},
		{"Reinstall from scratch", "Starts PXE; each computer chooses its configured identity and target disk locally."},
	}
	lines := []string{tuiTitle("Restore computers", model.isDark), tuiMuted("Choose whether to keep or replace the installed system.", model.isDark), ""}
	for index, option := range options {
		lines = append(lines, tuiSelection(option.title, index == model.restoreCursor, model.isDark), tuiMuted("    "+option.description, model.isDark), "")
	}
	return model.renderShell(tuiShell{
		path: []string{"Computers", "Restore"},
		body: strings.Join(lines, "\n"),
		notices: []tuiNotice{{
			kind:   tuiStatusNeutral,
			title:  "Disk erasure is always confirmed locally",
			detail: "Starting PXE does not erase or reserve a computer. The downloaded installer asks for identity and disk confirmation on each machine.",
		}},
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Continue"}, {key: "Esc", label: "Computers"}, {key: "?", label: "Help"}},
	})
}

// frame bounds plain content such as contextual help. Routine screens use
// renderShell so their body, notices, confirmations, and actions stay distinct.
func (model dashboardModel) frame(content string) string {
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
		start := min(model.pageScroll, len(lines)-height+1)
		lines = append(lines[start:min(len(lines), start+height-1)], tuiMuted("Shift ↑/↓ scroll · ? help", model.isDark))
	}
	return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(lines, "\n"))
}

func (model dashboardModel) renderShell(shell tuiShell) string {
	width := min(116, max(20, model.width-6))
	if model.width == 0 {
		width = 100
	}
	regions := buildTUIShellRegions(shell, model.width, model.isDark)
	wrappedLines := func(content string) []string {
		if content == "" {
			return nil
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
			start := min(model.pageScroll, max(0, len(body)-bodyHeight))
			lines = compose(body[start:min(len(body), start+bodyHeight)], true)
		} else {
			return model.frame(renderTUIShell(shell, model.width, model.isDark))
		}
	}
	return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(lines, "\n"))
}

func (model dashboardModel) textEntry() bool {
	if model.screen == dashboardUpdate && model.baseUpdate && model.baseEditing {
		return true
	}
	if model.screen == dashboardSetupKeys && model.setupKeyImporting {
		return true
	}
	switch model.screen {
	case dashboardDeployReview, dashboardControllerReview, dashboardServicesRestartReview,
		dashboardGitCommitReview, dashboardUpdateReview, dashboardShutdownReview,
		dashboardSettingsEdit, dashboardPXEStartReview, dashboardPXELeaveReview:
		return true
	case dashboardHosts:
		return model.hostSearching
	case dashboardSoftware:
		return model.software.acceptsText()
	default:
		return false
	}
}

func (model *dashboardModel) startDiagnostics() tea.Cmd {
	if model.actions.LoadDoctor == nil {
		model.message = "Diagnostics are not available in this session."
		return nil
	}
	model.busy = "Checking configuration, network and services"
	model.message = ""
	action := model.actions.LoadDoctor
	return func() tea.Msg { report, err := action(); return dashboardDoctorMsg{report: report, err: err} }
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
	lines := []string{tuiTitle("Review Nixorium update", model.isDark), "", tuiSection("Validated release", model.isDark), fmt.Sprintf("%s → %s (%s)", model.updatePlan.CurrentRef, model.updatePlan.Target, model.updatePlan.TargetChannel), "Save flake.nix and flake.lock, then build and activate this controller", "No push, PXE action, or client deployment is included", fmt.Sprintf("Candidate checks: %d reviewed · F4 details", len(model.updatePlan.Checks))}
	if model.baseUpdate {
		lines[0] = tuiTitle("Review system and package update", model.isDark)
		lines[2] = "Kernel, services and application versions may all change"
		if base := model.updatePlan.PackageBase; base != nil {
			lines[3] = fmt.Sprintf("%s → %s · %s → %s", base.CurrentChannel, base.TargetChannel, shortRevision(base.CurrentRevision), shortRevision(base.TargetRevision))
		}
	}
	if model.updateDetails || model.height == 0 {
		lines = append(lines, "Revision: "+model.updatePlan.Revision, fmt.Sprintf("Downgrade: %t", model.updatePlan.Downgrade))
		for _, check := range model.updatePlan.Checks {
			lines = append(lines, check.ID+" · "+check.State+" · "+check.Message)
		}
	}
	patch := strings.Split(strings.TrimSuffix(model.updatePlan.Diff.Content, "\n"), "\n")
	start := min(model.updateScroll, max(0, len(patch)-model.updateReviewHeight()))
	end := min(len(patch), start+model.updateReviewHeight())
	lines = append(lines, "", fmt.Sprintf("Diff lines %d-%d of %d", start+1, end, len(patch)))
	lines = append(lines, patch[start:end]...)
	lines = append(lines, "", "Press Enter to save this validated update and activate the controller.")
	notices := []tuiNotice{}
	if model.baseUpdate {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Builds passed; runtime and hardware remain unverified", detail: "Check the controller and a selected client before distributing to the fleet."})
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", model.updateTitle(), "Review"}, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "↑/↓", label: "Scroll diff"}, {key: "F4", label: "Details"}, {key: "Enter", label: "Apply update"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}})
}
