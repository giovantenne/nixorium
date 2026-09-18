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
		return renderTUIShell(tuiShell{
			path:    []string{"Computers", "Inventory"},
			body:    model.busyView(),
			actions: []tuiAction{{key: "Esc", label: "Computers"}, {key: "F1", label: "Help"}},
		}, model.width, model.isDark)
	}
	hosts := model.filteredHosts()
	actions := []tuiAction{}
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
			marker := "  "
			if index == model.hostCursor {
				marker = "› "
			}
			rows = append(rows, fmt.Sprintf("%s%-10s %s", marker, h.Name, tuiStatus(label, statusLevel(level), model.isDark)))
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
	return renderTUIShell(tuiShell{
		path:    []string{"Computers", "Inventory"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: actions,
	}, model.width, model.isDark)
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
		marker := "  "
		if i == model.adminCursor {
			marker = "› "
		}
		lines = append(lines, tuiSection(marker+task.title+"  ["+task.shortcut+"]", model.isDark), tuiMuted("  "+task.description, model.isDark))
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return renderTUIShell(tuiShell{
		path:    []string{"Maintenance"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}, {key: "?", label: "Help"}},
	}, model.width, model.isDark)
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
		lines = append(lines, "Choose reapply to keep the disk, or reinstall to erase", "the disk confirmed locally on each selected computer.")
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
		lines = append(lines, "↑/↓ select release   Enter validate   p show prereleases", "Validation shows candidate-lock, evaluation and representative-build phases.", "On the review, Enter saves and activates; Esc cancels.", "After result: r new update")
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
		return renderTUIShell(tuiShell{path: path, body: strings.Join(append(lines, model.busyView()), "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}}, model.width, model.isDark)
	}
	findings := model.doctor.Findings
	if len(findings) == 0 {
		lines = append(lines, "No diagnostic observations available.", "Press r to run the laboratory checks.")
	}
	start, end := listWindow(len(findings), model.diagnosticCursor, max(2, (model.height-12)/3))
	for i := start; i < end; i++ {
		f := findings[i]
		marker := "  "
		if i == model.diagnosticCursor {
			marker = "› "
		}
		lines = append(lines, marker+tuiStatus(f.Summary, statusLevel(f.Level), model.isDark))
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
	return renderTUIShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions}, model.width, model.isDark)
}

func (model dashboardModel) restoreView() string {
	options := []struct {
		title       string
		description string
	}{
		{"Reapply the intended system", "Keeps the disk and deploys the declared configuration again."},
		{"Reinstall from scratch", "Choose this physical computer's configured identity, then start PXE. Its disk is erased only after local confirmation."},
	}
	lines := []string{tuiTitle("Restore computers", model.isDark), tuiMuted("Choose whether to keep or replace the installed system.", model.isDark), ""}
	for index, option := range options {
		marker := "  "
		if index == model.restoreCursor {
			marker = "› "
		}
		lines = append(lines, tuiSection(marker+option.title, model.isDark), tuiMuted("  "+option.description, model.isDark), "")
	}
	return renderTUIShell(tuiShell{
		path: []string{"Computers", "Restore"},
		body: strings.Join(lines, "\n"),
		notices: []tuiNotice{{
			kind:   tuiStatusNeutral,
			title:  "Why reinstall asks for a computer",
			detail: "The choice binds the checklist to one configured identity; it does not remotely erase or reserve that machine. Disk erasure is confirmed locally in the installer.",
		}},
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Continue"}, {key: "Esc", label: "Computers"}, {key: "?", label: "Help"}},
	}, model.width, model.isDark)
}

// frame bounds reading width and provides an explicit scroll surface. It never
// truncates source data; the same content remains reachable after a resize.
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
		// Keep the exact phrase, input, mismatch feedback and cancellation visible
		// while long impact/diff content scrolls independently above it.
		if model.textEntry() && !model.helpOpen {
			for i, line := range lines {
				if strings.Contains(line, "to continue:") && len(lines)-i < height-2 {
					space := height - (len(lines) - i) - 1
					start := min(model.pageScroll, max(0, i-space))
					body := append([]string{}, lines[start:min(i, start+space)]...)
					body = append(body, tuiMuted("Shift ↑/↓ review more · F1 help", model.isDark))
					body = append(body, lines[i:]...)
					return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(body, "\n"))
				}
			}
		}
		// Screens using the common shell keep their breadcrumb and action bar
		// fixed. Only the body between those regions scrolls.
		if model.usesTUIShell() {
			footerStart := -1
			for index := len(lines) - 1; index >= 2; index-- {
				if strings.TrimSpace(lines[index]) == "" {
					footerStart = index + 1
					break
				}
			}
			if footerStart > 2 && footerStart < len(lines) {
				footer := lines[footerStart:]
				bodyEnd := footerStart - 1
				fixed := []string{}
				for index := 2; index < bodyEnd; index++ {
					if strings.Contains(lines[index], "NOTICE") {
						fixedStart := index
						if index > 2 && strings.TrimSpace(lines[index-1]) == "" {
							fixedStart--
						}
						fixed = append(fixed, lines[fixedStart:bodyEnd]...)
						bodyEnd = fixedStart
						break
					}
				}
				body := lines[2:bodyEnd]
				bodyHeight := height - 2 - len(fixed) - len(footer) - 2
				if bodyHeight >= 2 {
					start := min(model.pageScroll, max(0, len(body)-bodyHeight))
					visible := append([]string{}, lines[:2]...)
					visible = append(visible, body[start:min(len(body), start+bodyHeight)]...)
					visible = append(visible, tuiMuted("Shift ↑/↓ scroll · ? help", model.isDark))
					visible = append(visible, fixed...)
					visible = append(visible, "")
					visible = append(visible, footer...)
					return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(visible, "\n"))
				}
			}
		}
		start := min(model.pageScroll, len(lines)-height+1)
		lines = append(lines[start:min(len(lines), start+height-1)], tuiMuted("Shift ↑/↓ scroll · ? help", model.isDark))
	}
	return lipgloss.NewStyle().Padding(1, 3).Render(strings.Join(lines, "\n"))
}

func (model dashboardModel) usesTUIShell() bool {
	switch model.screen {
	case dashboardHome, dashboardComputersArea, dashboardInstallationArea,
		dashboardSetup, dashboardSetupKeys, dashboardRestore, dashboardHosts,
		dashboardDeploy, dashboardDeployReview, dashboardPXE, dashboardPXEStartReview,
		dashboardPXELeaveReview, dashboardAdministration, dashboardShutdown,
		dashboardShutdownReview, dashboardShutdownResult, dashboardSoftware,
		dashboardSoftwareScope, dashboardSoftwareReview, dashboardSoftwareResult,
		dashboardController, dashboardControllerReview, dashboardServices,
		dashboardServicesRestartReview, dashboardDiagnostics, dashboardLogs,
		dashboardLogDetail, dashboardSettings, dashboardSettingsPasswords,
		dashboardSettingsReview, dashboardGitReview, dashboardGitCommitSelect,
		dashboardGitCommitReview, dashboardUpdate, dashboardUpdateReview:
		return true
	default:
		return false
	}
}

func (model dashboardModel) textEntry() bool {
	if model.screen == dashboardSetupKeys && model.setupKeyImporting {
		return true
	}
	switch model.screen {
	case dashboardDeployReview, dashboardControllerReview, dashboardServicesRestartReview,
		dashboardGitCommitReview, dashboardUpdateReview, dashboardSoftwareReview, dashboardShutdownReview,
		dashboardSettingsEdit, dashboardPXEStartReview, dashboardPXELeaveReview:
		return true
	case dashboardHosts:
		return model.hostSearching
	case dashboardSoftware:
		return model.softwareSearching
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

func (model dashboardModel) confirmationView(title, scope, impact, recovery, revision, phrase string) string {
	lines := []string{tuiTitle("Nixorium  /  Review", model.isDark), "", tuiSection(title, model.isDark), "", "Affects  " + scope, "", tuiStatus(impact, tuiStatusAttention, model.isDark), recovery}
	if revision != "" {
		lines = append(lines, "", tuiMuted("Reviewed revision  "+revision, model.isDark))
	}
	lines = append(lines, "", tuiSection("Type "+phrase+" to continue:", model.isDark), "> "+model.confirmation+"_", "", "enter confirm   esc cancel   F1 help")
	if model.message != "" {
		lines = append(lines, "", tuiStatus(model.message, tuiStatusAttention, model.isDark))
	}
	return strings.Join(lines, "\n")
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
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return renderTUIShell(tuiShell{path: []string{"Maintenance", "Update Nixorium", "Review"}, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "↑/↓", label: "Scroll diff"}, {key: "F4", label: "Details"}, {key: "Enter", label: "Apply update"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}}, model.width, model.isDark)
}
