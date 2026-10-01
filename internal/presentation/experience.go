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

func (model dashboardModel) rowCapacity() int { return max(3, model.height-15) }

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
			actions = append([]tuiAction{{key: "d", label: "Update this computer"}, {key: "i", label: "Diagnostics"}}, actions...)
			if hosts[min(model.computers.hostCursor, len(hosts)-1)].HostKeyCondition == domain.HostKeyChanged && model.actions.PlanHostTrust != nil && model.actions.ApplyHostTrust != nil && !model.actions.ClassroomMode {
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
		start, end := listWindow(len(hosts), model.computers.hostCursor, max(3, model.height-18))
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
	lines := []string{
		tuiTitle("Choose a maintenance task", model.isDark),
		tuiMuted("Configuration, controller operations and technical evidence", model.isDark),
		"",
	}
	lines = append(lines, model.taskMenu(administrationTasks, model.adminCursor))
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return model.renderShell(tuiShell{
		path:    []string{"Maintenance"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}},
	})
}

func (model dashboardModel) helpView() string {
	lines := []string{tuiTitle("Keyboard help", model.isDark), "", "↑ ↓ / j k   Move through lists", "Enter       Open, review, or confirm the exact phrase", "Esc         Back / cancel / clear search", "/           Search Computers or a settings list", "?           Open or close help (F1 also works in text fields)", "q           Quit outside text entry", "Shift ↑/↓   Scroll a page that exceeds the terminal", "", tuiSection("In this view", model.isDark)}
	switch model.screen {
	case dashboardManagedJobs:
		lines = append(lines, "Tab switches between controller and PXE jobs; status refreshes automatically.", "Esc leaves this read-only view; quitting does not cancel a managed job.", "Interrupted means a running progress record has no running unit.", "Inspect the named journal unit, then request a fresh review before retrying.", "Completion here is not verification of the current configuration.")
	case dashboardHostTrust:
		lines = append(lines, "Compare the offered fingerprint with this computer's physical console.", "Only a deliberately reinstalled client is eligible for reviewed key rotation.", "Esc cancels the read or review without changes; saving cannot be interrupted.")
	case dashboardHome:
		lines = append(lines, taskHelp(dashboardTasks)...)
		if model.actions.LoadManagedJobs != nil && !model.actions.ClassroomMode {
			lines = append(lines, "v  View existing managed work without starting or resuming it")
		}
	case dashboardComputersArea:
		lines = append(lines, taskHelp(model.availableComputerTasks())...)
	case dashboardInstallationArea:
		lines = append(lines, taskHelp(installationAreaTasks)...)
		lines = append(lines, "r  Reattach the recorded USB operation when available")
	case dashboardUSBInstall:
		lines = append(lines, "Tab or arrows move through console fields; the password is masked and cleared after use.", "Esc before apply requests confirmed cleanup. After apply, Esc detaches and never retries Disko.", "Use r for status, n for reconciliation, b for reviewed reboot, c to close without reboot, and v for post-boot verification when visible.")
		lines = append(lines, "While the installer job runs the view refreshes by itself with read-only status requests. d shows the raw result fields.")
		lines = append(lines, "Artifacts-ready means the live session is not verified. Use a to connect with fresh physical key confirmation, or x to cancel safely, when visible.", "A failed connection's original error remains visible across status refreshes in this TUI session.")
	case dashboardAdministration:
		lines = append(lines, taskHelp(administrationTasks)...)
	case dashboardHosts:
		lines = append(lines, "r refresh computers   / search names, addresses or status", "Enter open details   t technical detail", "Search owns all text keys until Enter or Esc.")
		if !model.actions.ClassroomMode {
			lines = append(lines, "d review a deployment for the focused computer", "u review updating every computer that needed it at the last check")
			lines = append(lines, "h review changed SSH trust in a computer's details, after verifying its physical fingerprint")
		}
	case dashboardDeploy:
		lines = append(lines, "Review probes only selected computers; it does not prove power state or authenticated identity.", "Results list each computer; Shift+arrows scroll long results and l opens private logs.")
		if model.deployment.usbRecovery != nil {
			lines = append(lines, "Enter runs the visible recovery action; it never starts deployment.",
				"For an installed computer awaiting its final check: turn it on, boot from its disk, and connect the network cable.",
				"r checks installation status again; d shows technical details; i opens the existing installation when available.",
				"Esc keeps your selection. Successful recovery creates a fresh review requiring DEPLOY again.")
			break
		}
		lines = append(lines, "Space select   a select/deselect all   n select those needing the update   r check computers   Enter review", "States come from the last check in this session, shown with its time; the review probes again.", "During deployment: l private output details; s review stopping local supervision; q cannot interrupt", "Stopping requires STOP WAITING; remote activation may continue and require recovery.", "After result: l logs   n new review when no recovery is required   Enter Computers")
	case dashboardBackup:
		lines = append(lines, "Tab next field   Enter write the backup   Esc cancel",
			"The file is encrypted with the passphrase; keep both away from this controller.",
			"Restore: nixorium backup restore FILE --to EMPTY-DIRECTORY, then follow the controller replacement steps in the troubleshooting guide.")
	case dashboardCleanup:
		lines = append(lines, "Space select   a select all   Enter review   Esc Maintenance",
			fmt.Sprintf("Each computer keeps its newest %d system versions plus the running and booted ones; the review lists what goes.", domain.CleanupKeepGenerations),
			"Type CLEAN on the review to start. Computers that are off are never queued. After the result: n new review.")
	case dashboardInternet:
		lines = append(lines, "Space select clients; a select all; Tab choose block or unblock; Enter review.", "r checks Internet access on all clients (read-only); n selects those the chosen action would change.", "Enter applies the reviewed change. Reboot restores Internet; offline clients are never queued.")
	case dashboardDeployReview:
		lines = append(lines, "The brief availability check is not authentication or proof of power state.", "F2, when offered, creates a new review for only the reachable computers with an open SSH port.", "The new review requires a fresh DEPLOY confirmation; it never starts automatically.", "Esc during replanning keeps the original review and clears its confirmation.")
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		lines = append(lines, "Space select   a select/deselect all   n select those on at the last check   r check computers   Tab shutdown/restart   Enter check/review", "u acknowledge unknown sessions in review   Esc cancel", "An accepted request does not prove physical power state or a completed restart.")
	case dashboardSetup:
		lines = append(lines, "Enter continue the observed stage   t full checklist")
	case dashboardSettings:
		for _, group := range routineSettingsGroups {
			lines = append(lines, group.shortcut+"  "+group.label)
		}
		lines = append(lines, "p  Account passwords", "y  Controller keys", "/  Search categories; Esc clears the search before leaving")
		lines = append(lines, "After saving, Enter opens a separate controller review; after verified application it opens client selection. Esc leaves the result.")
	case dashboardWorkspace:
		lines = append(lines,
			"Choose Desktop, Dock, VSCode or Browser, then a supported field.",
			"Inherit keeps the deployment baseline; Clear means an explicit empty list.",
			"In a list: Space toggles, i inherits, c clears; Shift arrows reorder favorites.",
			"Extensions: / searches every packaged extension of the pinned package set, not only the catalog.",
			"Dependencies of searched extensions are checked when the system is built.",
			"m downloads one Marketplace extension (publisher.name) into the controller's store and shows it before adding;",
			"u checks pinned Marketplace extensions for newer versions the pinned VS Code accepts. The controller needs Internet for both.",
			"Enter keeps a field in the draft; Esc cancels that field edit.",
			"Other settings: a adds a name and value, e edits, d removes, p takes a pasted settings file.",
			"Guided fields, update settings and settings that can start programs cannot be added there.",
			"v reviews the complete draft, dependencies and student destinations.",
			"Type SAVE and Enter only after review to save and record just the workspace profile locally.",
			"Enter then opens a separate controller review. Runtime opt-in, deployment and home reset are never automatic.",
			"A partial save opens Git review for inspection; do not replay the old save or apply systems before recovery.")
		lines = append(lines, "Packaged extension updates use Maintenance → Update system and packages, which can also change the editor, desktop and operating system.")
	case dashboardTemplateReset:
		lines = append(lines,
			"Choose a software preset from the exact framework revision already locked in this deployment.",
			"Review all removed/replaced paths with arrows or Page Up/Down; no file contents are exposed.",
			"Existing custom software, home preferences, assets and modules are replaced, not merged.",
			"Settings, lock, keys, ignore rules and untracked files are preserved; collisions block the reset.",
			"Type RESET DEPLOYMENT and Enter to create a backup and a local reset commit. Esc cancels review.",
			"The guided home is enabled in configuration only. Apply systems and reboot separately; no push.",
			"The result offers a separate controller review, then client selection after verified application. Esc leaves without applying.",
			"An interrupted reset requires recovery; preserve the backup ref and .git/nixorium-template-reset.json.")
	case dashboardSettingsPasswords:
		lines = append(lines, "a  Administrator", "t  Teacher", "s  Student", "Selecting an account opens protected password input; it does not save changes.")
	case dashboardPXE:
		lines = append(lines, "Enter next step   p configure/prepare   s review start   x stop   n recover network   r refresh", "l progress details while preparing; q closes only the view")
	case dashboardController:
		lines = append(lines, "n new review   d result details   l logs / progress detail", "q closes the view; systemd-owned work continues")
	case dashboardServices:
		lines = append(lines, "c review cache restart   r refresh   l logs after result")
	case dashboardLogs, dashboardLogDetail:
		lines = append(lines, "Enter open log   r refresh list   ↑/↓/pg scroll detail")
	case dashboardGitReview, dashboardGitCommitSelect, dashboardGitCommitReview:
		lines = append(lines, "c select commit paths   Space select   a all safe paths", "r refresh review   ↑/↓/pg scroll patch", "Exact confirmation creates a local commit; nothing is pushed.")
	case dashboardUpdate, dashboardUpdateReview:
		lines = append(lines, "During validation: l expands or collapses current check details in place.")
		if model.updates.packageBase {
			lines = append(lines, "Enter check current channel   m change channel   r inspect pin", "In the channel form, type nixos-YY.MM and Space to acknowledge unverified compatibility.", "Review/build precedes saving and controller activation. Client distribution is separate.", "Build success does not verify runtime or hardware; check boot and services afterwards.")
		} else {
			lines = append(lines, "↑/↓ select release   Enter validate   p show prereleases", "Validation prepares the selected version, checks the lab configuration and tests required systems without saving it.", "On the review, Enter saves and activates; Esc cancels.", "After result: n new update; s completes an interrupted save")
		}
	case dashboardDiagnostics:
		lines = append(lines, "↑/↓ move   Enter technical evidence   r run checks again")
		if model.actions.PreviewSupport != nil && !model.actions.ClassroomMode {
			lines = append(lines, "e  Preview a minimized support report; no save or upload yet")
		}
	case dashboardSupport:
		lines = append(lines, "↑/↓, PgUp/PgDown, Home/End scroll the exact filtered JSON.",
			"Enter saves this preview to a private local file; nothing is uploaded.",
			"Esc cancels collection or leaves without saving. r collects a fresh preview.",
			"Versions, revision, time and counts remain visible. Review before sharing.",
			"Unavailable sections are not healthy results. Detailed logs are not included.")
	case dashboardSoftware:
		lines = append(lines, "F2 selected   F3 package search   F4 suggestions   Tab next view", "p add a deployment-owned profile   / search",
			"Selected software: Enter opens details; c changes where it applies, x reviews its removal.", "Profile packages: Space include/exclude   Enter choose scope and review", "A profile adds missing declarations together; existing package scopes are preserved.")
		lines = append(lines, "After saving: Enter takes the displayed next step; Esc leaves it for later. Client deployment always has its own review and confirmation.")
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
	if model.actions.PreviewSupport != nil && !model.actions.ClassroomMode {
		actions = append([]tuiAction{{key: "e", label: "Support report"}}, actions...)
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
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
	if model.busy != "" {
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
			if model.updates.planning {
				shell.actions = append([]tuiAction{{key: "l", label: "Progress details"}}, shell.actions...)
			}
		} else if !model.deployment.stopReview {
			// The footer must not advertise navigation or repeat a mutation
			// while its result is pending. Reviewed deployment stop is separate.
			shell.actions = []tuiAction{{key: "F1", label: "Help"}}
			if model.deployment.applying || model.controller.applying || model.installation.pxePreparing {
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
	if model.screen == dashboardHostTrust {
		return model.hostTrust.cancel == nil && !model.hostTrust.applying && model.hostTrust.result.Operation == "" && !model.hostTrust.plan.HasErrors()
	}
	if model.screen == dashboardUSBInstall {
		switch model.installation.remote.stage {
		case remoteInstallConsole, remoteInstallRotateHostKey, remoteInstallPassword, remoteInstallReview, remoteInstallConfirmReboot, remoteInstallConfirmClose:
			return true
		}
	}
	if model.screen == dashboardUpdate && model.updates.packageBase && model.updates.baseEditing {
		return true
	}
	if model.screen == dashboardSetupKeys && model.setupKeyImporting {
		return true
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
	patch := updateReviewLines(model.updates.plan)
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
