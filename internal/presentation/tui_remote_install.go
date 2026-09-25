package presentation

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

var installMethods = []struct {
	method      domain.RemoteInstallMethod
	title       string
	description string
}{
	{"pxe", "Network boot (PXE)", "Recommended for many computers; temporarily enables the managed network installer."},
	{domain.RemoteInstallUSBSSH, "USB over SSH", "Use the official NixOS Minimal ISO and install one physically identified computer."},
}

func (model dashboardModel) installMethodView() string {
	lines := []string{
		tuiTitle("Choose an installation method", model.isDark),
		"Both methods install the same reviewed NixOS closure and shared Disko layout.",
		"",
	}
	for index, method := range installMethods {
		lines = append(lines, tuiSelection(method.title, index == model.installation.methodCursor, model.isDark), tuiMuted("    "+method.description, model.isDark))
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
	}
	notices := []tuiNotice{}
	actions := []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Continue"}, {key: "Esc", label: "Installation"}, {key: "F1", label: "Help"}}
	if model.installation.remote.operationID != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "A USB installation operation is still known", detail: "Operation " + model.installation.remote.operationID + " can be refreshed without repeating apply."})
		actions = append([]tuiAction{{key: "r", label: "Reattach USB operation"}}, actions...)
	}
	return model.renderShell(tuiShell{
		path: []string{"Installation", "Install computers", "Method"}, body: strings.Join(lines, "\n"),
		notices: notices, actions: actions,
	})
}

func (model dashboardModel) openRemoteInstallHostSelection() (tea.Model, tea.Cmd) {
	model.busy = ""
	model.message = ""
	model.screen = dashboardUSBInstall
	model.installation.remote = remoteInstallationModel{stage: remoteInstallSelectHost}
	return model, nil
}

func (model dashboardModel) remoteInstallHosts() []domain.HostMeta {
	return model.report.Meta.Clients.Hosts
}

func (model dashboardModel) selectedRemoteInstallDisk() (domain.RemoteDisk, bool) {
	if model.installation.remote.response.Session == nil || model.installation.remote.response.Session.Preparation == nil {
		return domain.RemoteDisk{}, false
	}
	disks := model.installation.remote.response.Session.Preparation.Facts.Disks
	index := model.installation.remote.diskCursor
	if index < 0 || index >= len(disks) {
		return domain.RemoteDisk{}, false
	}
	return disks[index], true
}

func (model dashboardModel) remoteInstallView() string {
	remote := model.installation.remote
	lines := []string{tuiTitle("Install one computer from USB over SSH", model.isDark)}
	notices := []tuiNotice{}
	actions := []tuiAction{{key: "Esc", label: "Cancel safely"}, {key: "F1", label: "Help"}}
	fixedBody := ""
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		lines = append(lines, model.remoteInstallProgressLines()...)
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Controller work continues if this view closes", detail: "No disk is modified unless the content-bound review is confirmed."})
		actions = []tuiAction{{key: "q", label: "Detach"}, {key: "F1", label: "Help"}}
		return model.renderShell(tuiShell{path: []string{"Installation", "USB over SSH"}, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
	}

	switch remote.stage {
	case remoteInstallSelectHost:
		lines = append(lines,
			"Select the Nixorium identity that will be installed. Reachability alone never reserves an identity.", "")
		hosts := model.remoteInstallHosts()
		if len(hosts) == 0 {
			lines = append(lines, tuiStatus("No configured client identities are available", tuiStatusFailure, model.isDark))
		} else {
			for index, host := range hosts {
				detail := fmt.Sprintf("%s · %s", host.IP, host.Interface)
				lines = append(lines, tuiSelection(host.Name, index == remote.hostCursor, model.isDark), tuiMuted("    "+detail, model.isDark))
			}
		}
		actions = []tuiAction{{key: "↑/↓", label: "Select identity"}, {key: "Enter", label: "Prepare"}, {key: "Esc", label: "Method"}, {key: "F1", label: "Help"}}
	case remoteInstallConsole:
		consoleTitle := "Verify the fingerprint on the local console"
		consoleDetail := "The password is used once in memory, then replaced by an ephemeral key and cleared."
		if remote.recovery {
			consoleTitle = "Restore access only to reconcile the reserved operation"
			consoleDetail = "Re-enter the original live address and fingerprint. This cannot create or replay an apply."
		}
		lines = append(lines,
			"On the physical client, boot the official NixOS Minimal 26.05 ISO in UEFI mode and use:",
			"  passwd",
			"  systemctl is-active sshd",
			"  ip -4 -br address show scope global",
			"  ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub",
			"",
			fmt.Sprintf("Identity:           %s", remote.host),
			remoteInstallField("Live IPv4", remote.address, remote.formField == 0, false),
			remoteInstallField("Ed25519 fingerprint", remote.fingerprint, remote.formField == 1, false),
			remoteInstallField("Temporary password", remote.password, remote.formField == 2, true),
		)
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: consoleTitle, detail: consoleDetail})
		actions = []tuiAction{{key: "Tab/↑/↓", label: "Field"}, {key: "Enter", label: "Connect"}, {key: "Esc", label: "Cancel safely"}, {key: "F1", label: "Help"}}
	case remoteInstallSelectDisk:
		lines = append(lines,
			fmt.Sprintf("Verified live client: %s at %s", remote.host, remote.address),
			"Select a disk explicitly. The live medium and any disk in use remain visible but excluded.", "")
		preparation := remote.response.Session.Preparation
		for index, disk := range preparation.Facts.Disks {
			state := "eligible"
			if !disk.Eligible {
				state = "excluded: " + strings.Join(disk.ExclusionReasons, ", ")
			}
			detail := fmt.Sprintf("%s · %d bytes · %s · %s", state, disk.SizeBytes, disk.Model, disk.Serial)
			lines = append(lines, tuiSelection(disk.Path, index == remote.diskCursor, model.isDark), tuiMuted("    "+detail, model.isDark))
		}
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "The selected disk will be completely erased", detail: "Selection is checked again immediately before Disko; no excluded disk can be reviewed."})
		actions = []tuiAction{{key: "↑/↓", label: "Select disk"}, {key: "Enter", label: "Review disk"}, {key: "Esc", label: "Cancel safely"}, {key: "F1", label: "Help"}}
	case remoteInstallRotateHostKey:
		lines = append(lines,
			"A different host key is already known for the target static address.",
			"Only the reviewed entry is replaced, and only after post-boot verification succeeds.", "")
		notices = append(notices, tuiNotice{kind: tuiStatusFailure, title: "Host-key rotation requires separate consent", detail: "Type ROTATE HOST KEY to bind this decision to the installation review."})
		actions = []tuiAction{{key: "Enter", label: "Confirm rotation"}, {key: "Esc", label: "Cancel safely"}, {key: "F1", label: "Help"}}
	case remoteInstallReview:
		plan := remote.plan
		lines = append(lines,
			fmt.Sprintf("Logical identity:    %s", plan.Host.Name),
			fmt.Sprintf("Physical session:   %s · %s", plan.Host.LiveIP, remoteInstallPlanFingerprint(remote.response)),
			fmt.Sprintf("Installed address:  %s on %s", plan.Host.StaticIP, plan.Host.Interface),
			fmt.Sprintf("Disk to erase:      %s · %d bytes", plan.Disk.Path, plan.Disk.SizeBytes),
			fmt.Sprintf("Disk serial / WWN:  %s / %s", plan.Disk.Serial, plan.Disk.WWN),
			fmt.Sprintf("Revision:           %s", plan.Revision),
			fmt.Sprintf("System closure:     %s", plan.SystemPath),
			fmt.Sprintf("Signed cache:       %s", plan.CacheURL),
			fmt.Sprintf("Host-key rotation:  %t", plan.HostKeyRotation),
		)
		notices = append(notices, tuiNotice{kind: tuiStatusFailure, title: "This permanently erases only the reviewed disk", detail: "Type the exact confirmation below. The worker rechecks identity, cache, revision and disk before mutation."})
		actions = []tuiAction{{key: "Enter", label: "Erase and install"}, {key: "Esc", label: "Cancel safely"}, {key: "F1", label: "Help"}}
	case remoteInstallConfirmReboot, remoteInstallConfirmClose:
		word := "REBOOT"
		title := "Reboot the installed computer?"
		detail := "Remove or deprioritize the USB medium before continuing. The reboot is sent once and then the static identity is verified separately."
		if remote.stage == remoteInstallConfirmClose {
			word = "CLOSE"
			title = "Close without rebooting?"
			detail = "The live key is revoked and the controller reservation is released. Reboot locally when ready."
		}
		lines = append(lines, tuiTitle(title, model.isDark), detail)
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Type " + word + " to continue"})
		actions = []tuiAction{{key: "Enter", label: word}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	case remoteInstallResult:
		lines = append(lines, model.remoteInstallResultLines()...)
		kind := tuiStatusNeutral
		if remoteInstallResponseFailed(remote.response) {
			kind = tuiStatusFailure
		} else if remote.response.State == "verified" {
			kind = tuiStatusSuccess
		} else if remote.response.State == "ready-to-reboot" || remote.response.State == "reboot-requested" {
			kind = tuiStatusAttention
		}
		notices = append(notices, tuiNotice{kind: kind, title: sanitizeRemoteInstallText(remote.response.Message)})
		actions = model.remoteInstallResultActions()
	}

	if remote.stage == remoteInstallRotateHostKey || remote.stage == remoteInstallReview || remote.stage == remoteInstallConfirmReboot || remote.stage == remoteInstallConfirmClose {
		expected := ""
		switch remote.stage {
		case remoteInstallRotateHostKey:
			expected = "ROTATE HOST KEY"
		case remoteInstallReview:
			expected = remote.plan.Confirmation
		case remoteInstallConfirmReboot:
			expected = "REBOOT"
		case remoteInstallConfirmClose:
			expected = "CLOSE"
		}
		fixedBody = tuiSection("Type exactly", model.isDark) + "\n  " + expected + "\n> " + remote.confirmation + "_"
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: sanitizeRemoteInstallText(model.message)})
	}
	return model.renderShell(tuiShell{path: []string{"Installation", "USB over SSH"}, body: strings.Join(lines, "\n"), fixedBody: fixedBody, notices: notices, actions: actions})
}

func remoteInstallField(label, value string, focused, secret bool) string {
	display := value
	if secret {
		display = strings.Repeat("•", min(24, utf8.RuneCountInString(value)))
	}
	prefix := "  "
	if focused {
		prefix = "> "
	}
	return fmt.Sprintf("%s%-22s %s", prefix, label+":", display)
}

func remoteInstallPlanFingerprint(response domain.RemoteInstallResponse) string {
	if response.Session != nil && response.Session.Preparation != nil {
		return response.Session.Preparation.HostFingerprint
	}
	return "unavailable"
}

func (model dashboardModel) remoteInstallProgressLines() []string {
	remote := model.installation.remote
	lines := []string{""}
	for _, item := range []struct {
		stage remoteInstallationStage
		label string
	}{{remoteInstallPreparing, "Build pinned artifacts"}, {remoteInstallBootstrap, "Verify live ISO and replace password"}, {remoteInstallSelectDisk, "Transfer signed bundle and probe disks"}, {remoteInstallApplying, "Dispatch independent installer job"}} {
		kind := tuiStatusAttention
		if remote.stage > item.stage {
			kind = tuiStatusSuccess
		}
		lines = append(lines, tuiStatus(item.label, kind, model.isDark))
	}
	return lines
}

func (model dashboardModel) remoteInstallResultLines() []string {
	response := model.installation.remote.response
	lines := []string{
		fmt.Sprintf("State:               %s", response.State),
		fmt.Sprintf("Operation ID:        %s", response.OperationID),
	}
	if response.Execution != nil {
		lines = append(lines,
			fmt.Sprintf("Phase:               %s", response.Execution.Phase),
			fmt.Sprintf("Disk may be changed: %t", response.Execution.DiskMayBeModified),
			fmt.Sprintf("Installed:           %t", response.Execution.Installed),
			fmt.Sprintf("Reboot requested:    %t", response.Execution.RebootRequested),
			fmt.Sprintf("Boot verified:       %t", response.Execution.BootVerified),
			fmt.Sprintf("Dispatch uncertain:  %t", response.Execution.DispatchUncertain),
			fmt.Sprintf("Cleanup unconfirmed: %t", response.Execution.CleanupUnconfirmed),
		)
		if response.Execution.LogID != "" {
			lines = append(lines, "Operation log:       "+response.Execution.LogID)
		}
	}
	if response.Session != nil && response.Session.Receipt != nil {
		lines = append(lines,
			fmt.Sprintf("Remote phase:        %s", response.Session.Receipt.Phase),
			fmt.Sprintf("Disk may be changed: %t", response.Session.Receipt.DiskMayBeModified),
			fmt.Sprintf("Installed:           %t", response.Session.Receipt.Installed),
		)
	}
	if response.Session != nil && response.Session.LogID != "" && (response.Execution == nil || response.Execution.LogID == "") {
		lines = append(lines, "Operation log:       "+response.Session.LogID)
	}
	if response.Session != nil && response.Session.DispatchUncertain {
		lines = append(lines, "Dispatch is uncertain: do not repeat apply; reconcile the same operation ID.")
	}
	if response.State == "ready-to-reboot" {
		lines = append(lines, "", "Remove or deprioritize the USB medium before authorizing reboot.")
	}
	if response.State == "reconciliation-required" {
		lines = append(lines, "", "Do not start another installation. Refresh this operation until its remote receipt is known.")
	}
	return lines
}

func (model dashboardModel) remoteInstallResultActions() []tuiAction {
	state := model.installation.remote.response.State
	actions := []tuiAction{{key: "r", label: "Refresh status"}}
	if state == "ready-to-reboot" {
		actions = append(actions, tuiAction{key: "b", label: "Reboot"}, tuiAction{key: "c", label: "Close without reboot"})
	}
	if state == "reboot-requested" || state == "verified" || state == "reconciliation-required" {
		actions = append(actions, tuiAction{key: "v", label: "Verify installed system"})
	}
	if state == "reconciliation-required" {
		actions = append(actions, tuiAction{key: "n", label: "Reconcile remote receipt"}, tuiAction{key: "a", label: "Restore live recovery access"})
	}
	if state == "artifacts-ready" || state == "bootstrapped" || state == "bootstrapped-artifacts" || state == "prepared" || state == "review-ready" {
		actions = append(actions, tuiAction{key: "x", label: "Cancel before apply"})
	}
	actions = append(actions, tuiAction{key: "Esc", label: "Detach"}, tuiAction{key: "F1", label: "Help"})
	return actions
}

func remoteInstallResponseFailed(response domain.RemoteInstallResponse) bool {
	return strings.Contains(response.State, "failed") || response.State == "blocked" || response.State == "unavailable" || response.State == "reconciliation-required"
}

func sanitizeRemoteInstallText(value string) string {
	var result strings.Builder
	for _, character := range value {
		if result.Len() >= 4096 {
			break
		}
		if character == '\n' || character == '\t' || (unicode.IsPrint(character) && !unicode.In(character, unicode.Cf)) {
			result.WriteRune(character)
		}
	}
	return strings.TrimSpace(result.String())
}

func (model dashboardModel) updateRemoteInstallKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	remote := &model.installation.remote
	switch remote.stage {
	case remoteInstallSelectHost:
		hosts := model.remoteInstallHosts()
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardInstallMethod
		case "up", "k":
			remote.hostCursor = max(0, remote.hostCursor-1)
		case "down", "j":
			remote.hostCursor = min(max(0, len(hosts)-1), remote.hostCursor+1)
		case "enter":
			if len(hosts) == 0 || model.actions.PrepareRemoteInstall == nil {
				model.message = "Remote installation preparation is not available."
				return model, nil
			}
			remote.host = hosts[remote.hostCursor].Name
			remote.stage = remoteInstallPreparing
			model.busy = "Building the selected client closure and immutable installer bundle"
			host := remote.host
			return model, func() tea.Msg {
				response, err := model.actions.PrepareRemoteInstall(host)
				return dashboardRemoteInstallMsg{action: "prepare", response: response, err: err}
			}
		}
	case remoteInstallConsole:
		switch key.String() {
		case "esc":
			remote.password = ""
			if remote.recovery {
				remote.recovery = false
				remote.stage = remoteInstallResult
				return model, nil
			}
			return model.cancelRemoteInstall()
		case "tab", "down":
			remote.formField = min(2, remote.formField+1)
		case "shift+tab", "up":
			remote.formField = max(0, remote.formField-1)
		case "backspace":
			model.remoteInstallRemoveInputRune()
		case "enter":
			if remote.formField < 2 {
				remote.formField++
				return model, nil
			}
			if remote.address == "" || remote.fingerprint == "" || remote.password == "" || model.actions.BootstrapRemoteInstall == nil {
				model.message = "IPv4 address, console fingerprint and temporary password are required."
				return model, nil
			}
			secret := []byte(remote.password)
			remote.password = ""
			remote.stage = remoteInstallBootstrap
			model.busy = "Verifying the physical console fingerprint and supported live ISO"
			host, address, fingerprint := remote.host, remote.address, remote.fingerprint
			return model, func() tea.Msg {
				response, err := model.actions.BootstrapRemoteInstall(host, address, fingerprint, secret)
				return dashboardRemoteInstallMsg{action: "bootstrap", response: response, err: err}
			}
		default:
			model.remoteInstallAppendInput(key.Text)
		}
	case remoteInstallSelectDisk:
		disks := remote.response.Session.Preparation.Facts.Disks
		switch key.String() {
		case "esc":
			return model.cancelRemoteInstall()
		case "up", "k":
			remote.diskCursor = max(0, remote.diskCursor-1)
		case "down", "j":
			remote.diskCursor = min(max(0, len(disks)-1), remote.diskCursor+1)
		case "enter":
			disk, ok := model.selectedRemoteInstallDisk()
			if !ok || !disk.Eligible {
				model.message = "Select an eligible disk; excluded disks cannot be reviewed."
				return model, nil
			}
			if remote.response.Session.Preparation.KnownHostConflict {
				remote.stage = remoteInstallRotateHostKey
				remote.confirmation = ""
				return model, nil
			}
			return model.planRemoteInstall(disk.Path, false)
		}
	case remoteInstallRotateHostKey:
		return model.updateRemoteInstallConfirmation(key, "ROTATE HOST KEY", func(model dashboardModel) (tea.Model, tea.Cmd) {
			disk, ok := model.selectedRemoteInstallDisk()
			if !ok {
				model.message = "The reviewed disk is no longer available."
				return model, nil
			}
			return model.planRemoteInstall(disk.Path, true)
		})
	case remoteInstallReview:
		return model.updateRemoteInstallConfirmation(key, remote.plan.Confirmation, func(model dashboardModel) (tea.Model, tea.Cmd) {
			remote := &model.installation.remote
			remote.stage = remoteInstallApplying
			model.busy = "Revalidating the reviewed identity and dispatching the independent installer job"
			request := domain.RemoteInstallRequest{Operation: domain.RemoteInstallApplyOperation, OperationID: remote.operationID, ReviewToken: remote.plan.ReviewToken, Confirmation: remote.plan.Confirmation}
			return model.remoteInstallCommand("apply", request)
		})
	case remoteInstallConfirmReboot:
		return model.updateRemoteInstallConfirmation(key, "REBOOT", func(model dashboardModel) (tea.Model, tea.Cmd) {
			model.busy = "Sending the one-time reboot request"
			return model.remoteInstallCommand("reboot", domain.RemoteInstallRequest{Operation: domain.RemoteInstallRebootOperation, OperationID: model.installation.remote.operationID})
		})
	case remoteInstallConfirmClose:
		return model.updateRemoteInstallConfirmation(key, "CLOSE", func(model dashboardModel) (tea.Model, tea.Cmd) {
			model.busy = "Revoking the live key and closing the installed session"
			return model.remoteInstallCommand("close", domain.RemoteInstallRequest{Operation: domain.RemoteInstallCloseOperation, OperationID: model.installation.remote.operationID})
		})
	case remoteInstallResult:
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardInstallationArea
			model.installation.flow = false
			model.message = "Remote installation remains available by its operation ID."
		case "r":
			model.busy = "Refreshing the independent remote installation job"
			return model.remoteInstallCommand("status", domain.RemoteInstallRequest{Operation: domain.RemoteInstallStatusOperation, OperationID: remote.operationID})
		case "v":
			model.busy = "Verifying the installed static identity, system closure and revision"
			return model.remoteInstallCommand("verify", domain.RemoteInstallRequest{Operation: domain.RemoteInstallVerifyOperation, OperationID: remote.operationID})
		case "n":
			if remote.response.State == "reconciliation-required" {
				model.busy = "Reconciling the recorded operation with the remote receipt"
				return model.remoteInstallCommand("reconcile", domain.RemoteInstallRequest{Operation: domain.RemoteInstallReconcileOperation, OperationID: remote.operationID})
			}
		case "a":
			if remote.response.State == "reconciliation-required" && remote.response.Session != nil && remote.response.Session.Bootstrap != nil {
				remote.host = remote.response.Session.Bootstrap.Host
				remote.address = ""
				remote.fingerprint = ""
				remote.password = ""
				remote.formField = 0
				remote.recovery = true
				remote.stage = remoteInstallConsole
				model.message = "Read the original address and Ed25519 fingerprint again from the live console."
			}
		case "b":
			if remote.response.State == "ready-to-reboot" {
				remote.stage = remoteInstallConfirmReboot
				remote.confirmation = ""
			}
		case "c":
			if remote.response.State == "ready-to-reboot" {
				remote.stage = remoteInstallConfirmClose
				remote.confirmation = ""
			}
		case "x":
			if remote.response.State == "artifacts-ready" || remote.response.State == "bootstrapped" || remote.response.State == "bootstrapped-artifacts" || remote.response.State == "prepared" || remote.response.State == "review-ready" {
				return model.cancelRemoteInstall()
			}
		}
	}
	return model, nil
}

func (model *dashboardModel) remoteInstallAppendInput(text string) {
	if text == "" {
		return
	}
	remote := &model.installation.remote
	target := &remote.address
	limit := 64
	if remote.formField == 1 {
		target, limit = &remote.fingerprint, 128
	} else if remote.formField == 2 {
		target, limit = &remote.password, 1024
	}
	for _, character := range text {
		if utf8.RuneCountInString(*target) >= limit {
			break
		}
		if unicode.IsPrint(character) && !unicode.In(character, unicode.Cf) && character != '\x00' {
			*target += string(character)
		}
	}
}

func (model *dashboardModel) remoteInstallRemoveInputRune() {
	remote := &model.installation.remote
	target := &remote.address
	if remote.formField == 1 {
		target = &remote.fingerprint
	} else if remote.formField == 2 {
		target = &remote.password
	}
	runes := []rune(*target)
	if len(runes) > 0 {
		*target = string(runes[:len(runes)-1])
	}
}

func (model dashboardModel) updateRemoteInstallConfirmation(key tea.KeyPressMsg, expected string, accepted func(dashboardModel) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	remote := &model.installation.remote
	switch key.String() {
	case "esc":
		if remote.stage == remoteInstallConfirmReboot || remote.stage == remoteInstallConfirmClose {
			remote.stage = remoteInstallResult
			remote.confirmation = ""
			return model, nil
		}
		return model.cancelRemoteInstall()
	case "backspace":
		runes := []rune(remote.confirmation)
		if len(runes) > 0 {
			remote.confirmation = string(runes[:len(runes)-1])
		}
	case "space":
		if len(remote.confirmation) < 512 {
			remote.confirmation += " "
		}
	case "enter":
		if remote.confirmation != expected {
			remote.confirmation = ""
			model.message = "Confirmation did not match; no action was sent."
			return model, nil
		}
		remote.confirmation = ""
		model.message = ""
		return accepted(model)
	default:
		if key.Text != "" && len(remote.confirmation)+len(key.Text) <= 512 {
			remote.confirmation += key.Text
		}
	}
	return model, nil
}

func (model dashboardModel) planRemoteInstall(disk string, rotation bool) (tea.Model, tea.Cmd) {
	model.busy = "Creating a content-bound review for the exact client and disk"
	return model.remoteInstallCommand("plan", domain.RemoteInstallRequest{Operation: domain.RemoteInstallPlanOperation, OperationID: model.installation.remote.operationID, Disk: disk, HostKeyRotation: rotation})
}

func (model dashboardModel) cancelRemoteInstall() (tea.Model, tea.Cmd) {
	model.installation.remote.password = ""
	if model.installation.remote.operationID == "" || model.actions.RemoteInstallRequest == nil {
		model.screen = dashboardInstallMethod
		return model, nil
	}
	model.busy = "Cancelling before apply and confirming credential and GC-root cleanup"
	return model.remoteInstallCommand("cancel", domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: model.installation.remote.operationID})
}

func (model dashboardModel) remoteInstallCommand(action string, request domain.RemoteInstallRequest) (tea.Model, tea.Cmd) {
	if model.actions.RemoteInstallRequest == nil {
		model.busy = ""
		model.message = "Remote installation control is not available in this session."
		return model, nil
	}
	return model, func() tea.Msg {
		response, err := model.actions.RemoteInstallRequest(request)
		return dashboardRemoteInstallMsg{action: action, response: response, err: err}
	}
}

func (model dashboardModel) handleRemoteInstallMessage(message dashboardRemoteInstallMsg) (tea.Model, tea.Cmd) {
	model.busy = ""
	remote := &model.installation.remote
	remote.password = ""
	if message.action == "probe" {
		if message.err != nil {
			model.message = "Existing USB installation state could not be checked: " + sanitizeRemoteInstallText(message.err.Error())
			return model, nil
		}
		if message.response.OperationID != "" && message.response.Session != nil {
			remote.operationID = message.response.OperationID
			remote.response = message.response
			remote.stage = remoteInstallResult
		} else {
			*remote = remoteInstallationModel{}
		}
		return model, nil
	}
	if message.err != nil {
		remote.stage = remoteInstallResult
		remote.response = domain.RemoteInstallResponse{OperationID: remote.operationID, State: "failed", Message: sanitizeRemoteInstallText(message.err.Error())}
		model.message = ""
		return model, nil
	}
	remote.response = message.response
	if message.response.OperationID != "" {
		remote.operationID = message.response.OperationID
	}
	if remoteInstallResponseFailed(message.response) {
		remote.stage = remoteInstallResult
		model.message = ""
		return model, nil
	}
	switch message.action {
	case "prepare":
		remote.stage = remoteInstallConsole
		remote.formField = 0
	case "bootstrap":
		if message.response.State == "recovery-attached" {
			remote.recovery = false
			model.busy = "Reconciling the reserved operation without replaying apply"
			return model.remoteInstallCommand("reconcile", domain.RemoteInstallRequest{Operation: domain.RemoteInstallReconcileOperation, OperationID: remote.operationID})
		}
		remote.stage = remoteInstallBootstrap
		model.busy = "Verifying signed cache access, importing the installer bundle and probing disks"
		return model.remoteInstallCommand("finalize", domain.RemoteInstallRequest{Operation: domain.RemoteInstallPrepareOperation, OperationID: remote.operationID, Host: remote.host})
	case "finalize":
		if message.response.Session == nil || message.response.Session.Preparation == nil {
			remote.stage = remoteInstallResult
			remote.response.State = "failed"
			remote.response.Message = "The worker did not return a final disk inventory."
			return model, nil
		}
		remote.stage = remoteInstallSelectDisk
		remote.diskCursor = 0
	case "plan":
		if message.response.Plan == nil {
			remote.stage = remoteInstallResult
			remote.response.State = "failed"
			remote.response.Message = "The worker did not return a destructive review."
			return model, nil
		}
		remote.plan = *message.response.Plan
		remote.stage = remoteInstallReview
		remote.confirmation = ""
	case "apply", "status", "reconcile", "verify", "reboot", "close":
		remote.stage = remoteInstallResult
	case "cancel":
		remote.stage = remoteInstallResult
		if message.response.State == "cancelled" {
			model.message = "Cancellation and pre-apply cleanup were confirmed."
		}
	}
	return model, nil
}
