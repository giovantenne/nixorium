package presentation

import (
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) pxeView() string {
	preparation := tuiStatus("not prepared", tuiStatusNeutral, model.isDark)
	if model.report.PXEPreparation.Present {
		preparation = tuiStatus("out of date; prepare them again", tuiStatusAttention, model.isDark)
	}
	if model.report.PXEPreparation.Ready {
		preparation = tuiStatus("ready", tuiStatusSuccess, model.isDark)
	}
	path := []string{"Installation", "Network boot (PXE)"}
	title := "Network installation"
	if pxeRecoveryMode(model.report.PXE.Mode) && !model.installation.stateError {
		title = "Recover the controller network"
	}
	if model.installation.flow {
		path = []string{"Installation", "Install computers"}
		title = "Install computers"
	} else if model.setupMode {
		path = []string{"Installation", "Install computers"}
		title = "Install computers"
	}
	if model.installation.stateError {
		return model.renderShell(tuiShell{path: path, body: tuiTitle(title, model.isDark), notices: []tuiNotice{{kind: tuiStatusFailure, title: model.message}}, actions: model.pxeActions()})
	}
	lines := []string{
		tuiTitle(title, model.isDark),
		"",
	}
	fields := [][2]string{
		{"Network installation", tuiStatus(pxeModeLabel(model.report.PXE.Mode), pxeStatusKind(model.report.PXE.Mode), model.isDark)},
		{"Installation files", preparation},
	}
	// Interface and address come from the evaluated configuration, which is
	// loaded on demand; omit them rather than show empty values.
	if model.report.Meta.Network.Interface != "" {
		fields = append(fields, [2]string{"Network interface", model.report.Meta.Network.Interface})
	}
	// Prepared files carry the address the controller had when they were
	// built (preparation captures a changed DHCP lease); show the one PXE
	// serves rather than the saved hint.
	address := model.report.Meta.Controller.DHCPIP
	if model.report.PXEPreparation.Ready && model.report.PXEPreparation.DHCPAddress != "" {
		address = model.report.PXEPreparation.DHCPAddress
	} else if address != "" && address != domain.MasterDHCPPlaceholder {
		// Not prepared yet: the saved value; preparation reads the current one.
		address += " (saved; checked when files are prepared)"
	}
	if address == domain.MasterDHCPPlaceholder {
		address = "not set yet; laboratory settings ask for it"
	}
	if address != "" {
		fields = append(fields, [2]string{"Controller address", address})
	}
	lines = append(lines, tuiFields(model.isDark, fields...)...)
	if model.installation.flow {
		steps := []string{"Laboratory settings", "Save configuration", "Controller keys", "Activate controller", "Prepare clients", "Start PXE"}
		lines = append(lines, "")
		lines = append(lines, model.computerInstallationSteps(steps)...)
		if model.installation.failed {
			return model.renderShell(tuiShell{
				path:    path,
				body:    strings.Join(lines, "\n"),
				notices: []tuiNotice{{kind: tuiStatusFailure, title: "Computer installation could not continue", detail: model.message}},
				actions: []tuiAction{{key: "Esc", label: "Installation"}, {key: "F1", label: "Help"}},
			})
		}
	}
	if model.controller.applying {
		// Keep live progress ahead of static installation steps. At 80x24 the
		// safety footer must not push the actual running phase off screen.
		lines = []string{tuiTitle(title, model.isDark)}
		if model.controller.progress.State != "completed" {
			lines = append(lines, "", model.busyView())
		} else {
			lines = append(lines, "")
		}
		lines = append(lines, model.operationProgressView(model.controller.progress, "Controller progress")...)
		notice := tuiNotice{kind: tuiStatusAttention, title: "Controller activation is running", detail: "The managed job survives a lost terminal; services and networking may restart."}
		if model.controller.progress.State == "completed" {
			notice = tuiNotice{kind: tuiStatusSuccess, title: "Controller activation and verification completed", detail: "Continuing with client-system preparation."}
		}
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			notices: []tuiNotice{notice},
			actions: model.pxeActions(),
		})
	}
	if model.installation.pxePreparing {
		if model.installation.pxeProgress.State != "completed" {
			lines = append(lines, "", model.busyView())
		} else {
			lines = append(lines, "")
		}
		lines = append(lines, model.operationProgressView(model.installation.pxeProgress, "Current progress")...)
		notice := tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Preparation continues if this view closes",
			detail: "You can close Nixorium: the work continues and you can check it again later.",
		}
		if model.installation.pxeProgress.State == "completed" {
			notice = tuiNotice{kind: tuiStatusSuccess, title: "Client preparation completed", detail: "The client systems and network installation files are ready."}
		}
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			notices: []tuiNotice{notice},
			actions: model.pxeActions(),
		})
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			actions: model.pxeActions(),
		})
	}
	if model.screen == dashboardPXEStartReview {
		scope := model.installation.startPlan.Interface + " · controller network"
		body := strings.Join([]string{
			tuiTitle("Start network installation?", model.isDark),
			"",
			"Affects  " + scope,
		}, "\n")
		notices := []tuiNotice{{
			kind:   tuiStatusAttention,
			title:  "The controller's laboratory address (" + model.installation.startPlan.StaticCIDR + ") is paused until network installation is finished",
			detail: "Client computers on this network will be able to start the installer from the network, served by the controller at " + model.installation.startPlan.DHCPAddress + ". The school's DHCP keeps giving out addresses. Finish installation, or a restart, brings the laboratory address back. Remote connections to the controller may be interrupted meanwhile.",
		}}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{
			path:      path,
			body:      body,
			fixedBody: tuiSection("Enter starts network installation mode.", model.isDark),
			notices:   notices,
			actions:   model.pxeActions(),
		})
	}
	if model.screen == dashboardPXELeaveReview {
		leaveLines := []string{
			tuiTitle("Exit while installation mode is active?", model.isDark),
			"Closing Nixorium will not stop installation mode.",
			"It changes controller networking and can continue after Nixorium closes.",
			"Configured computers may continue to network-boot into the installer.",
			"",
			tuiSection("Recommended", model.isDark),
			"  x  Stop installation mode, verify normal networking, and exit",
		}
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{
			path:      path,
			body:      strings.Join(leaveLines, "\n"),
			fixedBody: tuiSection("Keep it active", model.isDark) + "\n  Enter closes Nixorium and leaves installation mode active.",
			notices:   notices,
			actions:   model.pxeActions(),
		})
	}
	lines = append(lines, "")
	lines = append(lines, model.pxeNextStepView()...)
	if model.installation.pxeProgress.Operation != "" {
		lines = append(lines, "")
		lines = append(lines, model.operationProgressView(model.installation.pxeProgress, "Last preparation")...)
	}
	notices := model.pxeLifecycleNotices()
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: model.pxeActions()})
}

func (model dashboardModel) computerInstallationSteps(labels []string) []string {
	lines := []string{tuiSection("Preparation progress", model.isDark), ""}
	for index, label := range labels {
		label = fmt.Sprintf("%d. %s", index+1, label)
		switch {
		case index < model.installation.stage:
			lines = append(lines, tuiStatus(label, tuiStatusSuccess, model.isDark))
		case index == model.installation.stage && model.screen == dashboardPXEStartReview:
			lines = append(lines, tuiMuted("○ "+label+" · Awaiting confirmation", model.isDark))
		case index == model.installation.stage:
			lines = append(lines, tuiTitle("● "+label+" · Running", model.isDark))
		default:
			lines = append(lines, tuiMuted("○ "+label+" · Waiting", model.isDark))
		}
	}
	return lines
}

func (model dashboardModel) pxeActions() []tuiAction {
	if model.screen == dashboardPXEStartReview {
		return []tuiAction{{key: "Enter", label: "Start PXE"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	if model.screen == dashboardPXELeaveReview {
		return []tuiAction{{key: "x", label: "Stop and exit"}, {key: "Enter", label: "Leave active"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	if model.installation.pxePreparing {
		actions := []tuiAction{{key: "q", label: "Quit Nixorium; work continues"}, {key: "F1", label: "Help"}}
		if operationDetailsAvailable(model.installation.pxeProgress) {
			actions = append([]tuiAction{{key: "l", label: "Progress details"}}, actions...)
		}
		return actions
	}
	if model.controller.applying {
		actions := []tuiAction{{key: "F1", label: "Help"}}
		if model.controllerDetailsAvailable() {
			actions = append([]tuiAction{{key: "l", label: "Progress details"}}, actions...)
		}
		return actions
	}
	if model.busy != "" {
		return []tuiAction{{key: "q", label: "Quit Nixorium; work continues"}, {key: "F1", label: "Help"}}
	}
	if model.installation.flow && model.installation.failed {
		return []tuiAction{{key: "Esc", label: "Installation"}, {key: "F1", label: "Help"}}
	}
	primary := model.pxePrimaryAction()
	actions := []tuiAction{{key: "Enter", label: primary.label}}
	// The primary action is offered once, on Enter, not again on its letter.
	add := func(action tuiAction) {
		if action.key != primary.key {
			actions = append(actions, action)
		}
	}
	if model.installation.stateError {
		return append(actions, tuiAction{key: "r", label: "Refresh"}, tuiAction{key: "Esc", label: "Installation"}, tuiAction{key: "F1", label: "Help"})
	}
	recovery := model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required"
	if model.report.PXE.Mode != "active" && !recovery {
		add(tuiAction{key: "p", label: "Settings and preparation"})
		if model.report.PXEPreparation.Ready {
			add(tuiAction{key: "s", label: "Start"})
		}
	}
	if model.report.PXE.Mode == "active" {
		add(tuiAction{key: "x", label: "Stop"})
	}
	if model.pxeJustFinished() && model.actions.LoadHosts != nil {
		add(tuiAction{key: "h", label: "Check computers"})
	}
	if recovery {
		add(tuiAction{key: "x", label: "Stop the network installation"})
	}
	if result := model.installation.pxeResult; result.HasErrors() && result.Message != "" {
		label := "Details"
		if model.noticeDetails {
			label = "Hide details"
		}
		add(tuiAction{key: "d", label: label})
	}
	actions = append(actions, tuiAction{key: "r", label: "Refresh"}, tuiAction{key: "Esc", label: "Installation"})
	if model.report.PXE.Mode == "active" {
		// Quitting reviews whether to leave network installation on.
		actions = append(actions, tuiAction{key: "q", label: "Quit"})
	}
	return append(actions, tuiAction{key: "F1", label: "Help"})
}

func (model dashboardModel) pxeNextStepView() []string {
	switch model.report.PXE.Mode {
	case "active":
		return []string{
			tuiSection("Next: install computers", model.isDark), "",
			tuiInstruction(1, "At the client PC", "Start UEFI network boot. The guided installer opens automatically.", model.isDark), "",
			tuiInstruction(2, "Review the computer and disk", "Choose its configured identity and type ERASE only for the right disk.", model.isDark), "",
			tuiInstruction(3, "After installation", "Reboot from the installed disk and verify the computer.", model.isDark), "",
			tuiInstruction(4, "Back on this controller", "Press Enter when finished to stop network installation and restore normal networking.", model.isDark),
		}
	case "degraded", "recovery-required":
		return model.pxeRecoveryLines()
	}
	if model.pxeJustFinished() {
		return []string{
			tuiSection("Next: check the installed computers", model.isDark),
			"  Press Enter to open the computer inventory: each installed computer should answer and be up to date.",
			"  A computer installed again offers a new SSH key; review it there before updating it.",
		}
	}
	if !model.report.PXEPreparation.Ready {
		return []string{
			tuiSection("Next: prepare the installation files", model.isDark),
			"  Press Enter to review lab settings, prepare the controller and build installation files.",
		}
	}
	return []string{
		tuiSection("Next: start network installation", model.isDark),
		"  Press Enter to review the temporary network change and start.",
	}
}

// pxeJustFinished reports that this session has just stopped network
// installation without error, so the installed computers are the next step.
func (model dashboardModel) pxeJustFinished() bool {
	result := model.installation.pxeResult
	return result.Operation == "pxe-stop" && !result.HasErrors() && model.report.PXE.Mode != "active"
}

// pxeModeLabel names the network installation state in plain words.
func pxeModeLabel(mode string) string {
	switch mode {
	case "ready", "stopped":
		return "off"
	case "active":
		return "on; client PCs can start from the network"
	case "preparing":
		return "starting"
	case "degraded", "recovery-required":
		return "interrupted; recover the network"
	case "":
		return "unknown"
	}
	return mode
}

func pxeStatusKind(mode string) tuiStatusKind {
	switch mode {
	case "ready", "stopped":
		return tuiStatusNeutral
	case "active", "preparing":
		return tuiStatusAttention
	case "degraded", "recovery-required":
		return tuiStatusFailure
	default:
		return tuiStatusNeutral
	}
}

func (model dashboardModel) operationProgressView(operation domain.OperationProgress, title string) []string {
	if operation.Operation == "" {
		return []string{"  Waiting for managed progress…"}
	}
	phaseLabels := map[string]string{
		"starting":  "Starting",
		"validate":  "Validating configuration",
		"network":   "Checking network and cache",
		"artifacts": "Building the network installer",
		"clients":   "Building client systems",
		"publish":   "Publishing preparation",
		"complete":  "Complete",
		"build":     "Building system",
		"activate":  "Activating system",
		"verify":    "Verifying activation",
	}
	lines := []string{title, "  Phase: " + phaseLabels[operation.Phase]}
	if !(model.progressDetails && operationDetailsAvailable(operation)) && !model.controller.details {
		kind := tuiStatusNeutral
		label := "● " + phaseLabels[operation.Phase] + " · Running"
		if operation.State == "completed" {
			kind = tuiStatusSuccess
			label = "Preparation completed"
			if operation.Operation == "controller-apply" {
				label = "Controller operation completed"
			}
		}
		if operation.State == "failed" {
			kind = tuiStatusFailure
			label = phaseLabels[operation.Phase] + " failed"
		}
		if kind == tuiStatusNeutral {
			lines = []string{tuiTitle(label, model.isDark)}
		} else {
			lines = []string{tuiStatus(label, kind, model.isDark)}
		}
		if operation.Total > 0 {
			lines = append(lines, fmt.Sprintf("%d/%d steps complete", operation.Current, operation.Total))
		}
		if len(operation.Recent) > 0 {
			lines = append(lines, tuiMuted(operation.Recent[len(operation.Recent)-1], model.isDark))
		}
		return lines
	}
	if operation.Total > 0 {
		barWidth := model.width - 8
		if barWidth < 24 {
			barWidth = 24
		}
		if barWidth > 64 {
			barWidth = 64
		}
		bar := tuiProgress(barWidth, model.isDark)
		percentage := float64(operation.Current) / float64(operation.Total)
		lines = append(lines,
			fmt.Sprintf("  %d/%d", operation.Current, operation.Total),
			"  "+bar.ViewAs(percentage),
		)
	}
	if len(operation.Recent) > 0 {
		lines = append(lines, "  Recent activity:")
		for _, activity := range operation.Recent {
			lines = append(lines, "    • "+activity)
		}
	}
	return lines
}
