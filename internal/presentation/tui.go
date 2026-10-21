package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func RunLoadingDashboard(actions DashboardActions, setupMode bool) error {
	_, _, err := RunLoadingDashboardReload(actions, setupMode, "")
	return err
}

// RunLoadingDashboardReload runs the dashboard and reports whether it ended
// because the administrator asked to reopen it on the installed Nixorium, with
// the outcome to show once it has reopened. notice is such an outcome from the
// interface this one replaced. The terminal is already restored when it
// returns.
func RunLoadingDashboardReload(actions DashboardActions, setupMode bool, notice string) (bool, string, error) {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, actions, false)
	model.reopenNotice = notice
	model.setupMode = setupMode
	model.initializing = true
	model.busy = "Opening the laboratory and checking setup progress"
	if actions.ClassroomMode {
		model.busy = "Opening classroom controls"
	}
	if actions.LoadDisclaimer != nil && !actions.ClassroomMode {
		model.disclaimerChecking = true
		model.busy = "Reading local acknowledgement"
	}
	final, err := tea.NewProgram(model).Run()
	reload, outcome := false, ""
	if finished, ok := final.(dashboardModel); ok {
		reload = finished.reload.state == interfaceReloadRequested
		outcome = finished.reloadOutcome()
	}
	return reload && err == nil, outcome, err
}

func newDashboardModel(report domain.StatusReport, setup domain.SetupReport, actions DashboardActions, setupMode bool) dashboardModel {
	screen := dashboardHome
	if setupMode || setupNeedsImmediateAttention(setup) {
		screen = dashboardSetup
		setupMode = true
	}
	model := dashboardModel{
		report: report, setup: setup, setupMode: setupMode, screen: screen, actions: actions,
		activitySpinner: newTUISpinner(false), shutdown: newShutdownModel(),
	}
	model.homeMenu = newDashboardTaskMenu(model.homeAreas(), false, 80, 24)
	model.observeSetupController(setup)
	return model
}

func (model dashboardModel) Init() tea.Cmd {
	commands := []tea.Cmd{tea.RequestBackgroundColor, model.activitySpinner.Tick}
	if model.disclaimerChecking || model.screen == dashboardDisclaimer {
		commands = append(commands, model.loadDisclaimer())
	} else if model.initializing && model.actions.LoadInitial != nil {
		commands = append(commands, func() tea.Msg { return dashboardBeginInitialMsg{} })
	} else if model.actions.LoadManagedJobs != nil && !model.actions.ClassroomMode {
		commands = append(commands, model.loadManagedJobs(model.jobs.id))
	}
	return tea.Batch(commands...)
}

func (model dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if profile, ok := message.(tea.ColorProfileMsg); ok {
		model.assistantNoColors = profile.Profile <= colorprofile.ASCII
	}
	assistantTop := 0
	if model.screen == dashboardAssistant && model.assistant.historyScroll > 0 {
		before, _, _, capacity := model.assistantChatLayout()
		assistantTop = max(0, len(before)-capacity-model.assistant.historyScroll)
	}
	updated, command := model.updateState(message)
	next := updated.(dashboardModel)
	if next.screen != model.screen || next.message != model.message {
		next.noticeDetails = false
	}
	if model.screen == dashboardAssistant && next.screen == dashboardAssistant &&
		model.assistant.historyScroll > 0 && next.assistant.historyScroll == model.assistant.historyScroll &&
		next.assistant.id == model.assistant.id {
		// Keep the same first visible line while content arrives or the input shrinks.
		after, _, _, newCapacity := next.assistantChatLayout()
		next.assistant.historyScroll = max(0, len(after)-newCapacity-assistantTop)
	}
	if next.read.cancel != nil && next.read.id != model.read.id {
		next.read.returnScreen = model.screen
	}
	if next.busy != "" && model.busy == "" {
		next.busyStarted = time.Now()
	}
	if model.areaReturn != dashboardHome && model.screen != model.areaReturn && next.screen == dashboardHome {
		next.screen = model.areaReturn
	}
	if model.returnAdmin && model.screen != dashboardAdministration && next.screen == dashboardHome {
		next.screen = dashboardAdministration
	}
	if next.screen != model.screen {
		next.pageScroll = 0
	}
	if next.screen == dashboardHome && !next.initializing && !next.initialError {
		switch message.(type) {
		case dashboardInitialMsg, overviewRefreshMsg:
			next, command = next.withBackupSync(command)
		default:
			if model.screen != dashboardHome {
				next, command = next.withBackupSync(command)
				next, command = next.withQuietOverviewRefresh(command)
			}
		}
	}
	if next.screen == dashboardHome {
		next.returnAdmin = false
		next.areaReturn = dashboardHome
	}
	return next, command
}

func (model dashboardModel) runAction(operation func() string, screen dashboardScreen) tea.Cmd {
	return func() tea.Msg {
		message := operation()
		ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer cancel()
		report, err := model.actions.Refresh(ctx)
		return dashboardOperationMsg{message: message, report: report, err: err, screen: screen}
	}
}

func setupNeedsImmediateAttention(report domain.SetupReport) bool {
	switch report.CurrentStage {
	case domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials:
		return true
	default:
		return false
	}
}

func (model dashboardModel) openSetupSettings() (tea.Model, tea.Cmd) {
	if model.actions.LoadSettings == nil {
		model.message = "Laboratory settings are not available in this session."
		return model, nil
	}
	model.screen = dashboardSettings
	model.settings.returnScreen = dashboardSetup
	model.settings.collectPasswords = model.setup.CurrentStage != domain.SetupStageValidate
	model.settings.result = domain.ConfigurationSaveReport{}
	model.settings.plan = domain.ConfigPlanReport{}
	model.settings.candidate = domain.LabSettingsFile{}
	model.busy = "Loading laboratory settings"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return model.loadSettingsMsg(ctx)
	})
}

func (model dashboardModel) startComputerInstallation() (tea.Model, tea.Cmd) {
	remote := model.installation.remote
	model.installation = installationModel{flow: true, remote: remote}
	model.screen = dashboardInstallationArea
	model.message = ""
	model.busy = ""
	return model, nil
}

func (model dashboardModel) openUSBInstallation() (tea.Model, tea.Cmd) {
	if model.actions.LoadRemoteInstall == nil {
		return model.beginComputerInstallation(domain.RemoteInstallUSBSSH)
	}
	model.areaReturn = dashboardInstallationArea
	model.busy = "Checking for an existing USB installation operation"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		response, err := model.actions.LoadRemoteInstall(ctx)
		return dashboardRemoteInstallMsg{action: "open-usb", response: response, err: err}
	})
}

func (model dashboardModel) beginComputerInstallation(method domain.RemoteInstallMethod) (tea.Model, tea.Cmd) {
	if reason := model.managedJobConflict(); reason != "" {
		model.message = reason
		return model, nil
	}
	if model.actions.LoadSettings == nil {
		model.message = "Laboratory settings are not available in this session."
		return model, nil
	}
	model.installation.flow = true
	model.installation.method = method
	model.installation.stage = 0
	model.installation.failed = false
	model.setupMode = false
	model.installation.startingLabSetup = true
	model.installation.savedSummary = false
	model.settings.returnScreen = dashboardHome
	model.settings.collectPasswords = false
	model.settings.result = domain.ConfigurationSaveReport{}
	model.settings.plan = domain.ConfigPlanReport{}
	model.settings.candidate = domain.LabSettingsFile{}
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
	model.installation.pxeProgress = domain.OperationProgress{}
	model.areaReturn = dashboardInstallationArea
	model.screen = dashboardSettings
	model.busy = "Loading laboratory settings"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return model.loadSettingsMsg(ctx)
	})
}

func (model dashboardModel) failComputerInstallation(message string) (tea.Model, tea.Cmd) {
	model.busy = ""
	model.installation.failed = true
	model.controller.applying = false
	model.installation.pxePreparing = false
	model.message = message
	model.screen = dashboardPXE
	if model.installation.method == domain.RemoteInstallUSBSSH {
		model.screen = dashboardUSBInstall
		model.installation.remote.stage = remoteInstallResult
		model.installation.remote.response = domain.RemoteInstallResponse{
			OperationID: model.installation.remote.operationID, State: "failed", Message: message,
		}
	}
	return model, nil
}

func (model dashboardModel) continueComputerInstallation(report domain.SetupReport) (tea.Model, tea.Cmd) {
	model.setup = report
	model.screen = dashboardPXE
	model.message = ""
	switch report.CurrentStage {
	case domain.SetupStageKeys:
		if !model.setupKeyActionsAvailable() {
			return model.failComputerInstallation("Controller key preparation is not available in this session.")
		}
		model.installation.stage = 2
		model.busy = "Preparing and installing controller keys"
		return model, model.prepareSetupKeys()
	case domain.SetupStageReview:
		if model.actions.SaveSetupConfiguration == nil {
			return model.failComputerInstallation("Local configuration saving is not available in this session.")
		}
		model.installation.stage = 1
		model.busy = "Saving generated laboratory files locally"
		return model, func() tea.Msg {
			return dashboardSetupSaveMsg{report: model.actions.SaveSetupConfiguration()}
		}
	case domain.SetupStageApply:
		if model.actions.PlanController == nil || model.actions.ApplyController == nil {
			return model.failComputerInstallation("Controller activation is not available in this session.")
		}
		model.installation.stage = 3
		model.busy = "Checking the controller configuration"
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardControllerPlanMsg{report: model.actions.PlanController(ctx)}
		})
	case domain.SetupStageArtifacts:
		if model.installation.method == "" {
			return model.startComputerInstallation()
		}
		if model.installation.method == domain.RemoteInstallUSBSSH {
			return model.openRemoteInstallHostSelection()
		}
		return model.startComputerInstallationPreparation()
	case "":
		if model.installation.method == "" {
			return model.startComputerInstallation()
		}
		if model.installation.method == domain.RemoteInstallUSBSSH {
			return model.openRemoteInstallHostSelection()
		}
		if model.actions.PlanPXEStart == nil {
			return model.failComputerInstallation("PXE start validation is not available in this session.")
		}
		model.installation.stage = 4
		model.busy = "Checking PXE readiness"
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardPlanMsg{report: model.actions.PlanPXEStart(ctx)}
		})
	default:
		return model.failComputerInstallation("Laboratory configuration is still incomplete: " + setupCurrentAction(report) + ".")
	}
}

func (model dashboardModel) startComputerInstallationPreparation() (tea.Model, tea.Cmd) {
	// A guided follow-up can arrive before the background poll observes our
	// completed controller job. The mutation adapter rechecks live conflicts;
	// never fail this continuation from an older observational snapshot.
	if model.actions.PreparePXE == nil {
		return model.failComputerInstallation("Client preparation is not available in this session.")
	}
	model.installation.stage = 4
	model.busy = "Preparing client systems and network installation files"
	model.installation.pxePreparing = true
	model.installation.pxeProgress = domain.OperationProgress{}
	model.installation.pxeStarted = time.Now().UTC()
	model.installation.pxeProgressID++
	model.message = ""
	operation := func() tea.Msg {
		report := model.actions.PreparePXE()
		status := domain.StatusReport{}
		var err error
		if model.actions.Refresh != nil {
			ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
			defer cancel()
			status, err = model.actions.Refresh(ctx)
		}
		return dashboardInstallationPrepareMsg{report: report, status: status, statusErr: err}
	}
	return model, tea.Batch(operation, schedulePXEProgressTick(model.installation.pxeProgressID))
}

func (model dashboardModel) returnFromSettings() (tea.Model, tea.Cmd) {
	returnTo := model.settings.returnScreen
	model.settings.returnScreen = dashboardHome
	model.settings.collectPasswords = false
	model.settings.fromAccounts = false
	model.message = ""
	if returnTo == dashboardSetup {
		model.screen = dashboardSetup
		if model.actions.LoadSetup != nil && (model.settings.result.State == "saved" || model.settings.result.State == "unchanged") {
			model.busy = "Refreshing setup progress"
			return model.loadSetup()
		}
		return model, nil
	}
	model.screen = dashboardHome
	return model, nil
}

// deploymentEventBuffer lets the deployment goroutine hand over its last
// messages without waiting for the interface.
const deploymentEventBuffer = 4

// startDeployment runs the deployment in the background. The deployment holds
// the controller operation lock, so it must never wait for the interface:
// progress is a best-effort snapshot dropped while the interface is behind,
// and the final result displaces any queued progress instead of blocking.
func startDeployment(ctx context.Context, action func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport, plan domain.DeploymentPlanReport, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			report := action(ctx, plan, func(progress domain.DeploymentProgress) {
				select {
				case events <- dashboardDeploymentProgressMsg{progress: progress}:
				default:
				}
			})
			sendDeploymentResult(events, dashboardDeploymentResultMsg{report: report})
			close(events)
		}()
		return <-events
	}
}

// sendDeploymentResult delivers the last message of a deployment without
// blocking: when the buffer is full of older progress, the oldest is dropped.
func sendDeploymentResult(events chan tea.Msg, result tea.Msg) {
	for {
		select {
		case events <- result:
			return
		default:
		}
		select {
		case <-events:
		default:
		}
	}
}

func waitForDeploymentEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return nil
		}
		return message
	}
}

func (model dashboardModel) startUpdatePlan(action func(context.Context, string, bool, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport, target string, allowPrerelease, allowDowngrade bool) (tea.Model, tea.Cmd) {
	ctx, id := model.beginRead(dashboardBuildTimeout)
	events := make(chan tea.Msg, 8)
	model.updates.planEvents = events
	model.updates.planning = true
	model.updates.planStarted = time.Now().UTC()
	return model, func() tea.Msg {
		go func() {
			defer close(events)
			report := action(ctx, target, allowPrerelease, allowDowngrade, func(progress domain.UpdatePlanProgress) {
				select {
				case events <- dashboardUpdatePlanProgressMsg{progress: progress}:
				case <-ctx.Done():
				}
			})
			select {
			case events <- dashboardUpdatePlanMsg{report: report}:
			case <-ctx.Done():
			}
		}()
		return waitForActivityEvent(ctx, id, events)()
	}
}

func waitForActivityEvent(ctx context.Context, id uint64, events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case message, ok := <-events:
			if !ok {
				return activityResultMsg{id: id, err: ctx.Err()}
			}
			progress := false
			switch message.(type) {
			case dashboardUpdatePlanProgressMsg, templateResetProgressMsg:
				progress = true
			}
			return activityResultMsg{id: id, message: message, more: progress, err: ctx.Err()}
		case <-ctx.Done():
			return activityResultMsg{id: id, err: ctx.Err()}
		}
	}
}

func appendBoundedActivity(recent []string, activity string, maximum int) []string {
	if activity == "" || maximum < 1 {
		return recent
	}
	recent = append(recent, activity)
	if len(recent) > maximum {
		recent = append([]string(nil), recent[len(recent)-maximum:]...)
	}
	return recent
}

func (model dashboardModel) loadHosts() (tea.Model, tea.Cmd) {
	return model.startRead(func(ctx context.Context) tea.Msg {
		report, err := model.actions.LoadHosts(ctx)
		return dashboardHostsMsg{report: report, err: err}
	})
}

func (model dashboardModel) loadConfigurationState() (tea.Model, tea.Cmd) {
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardConfigurationStateMsg{report: model.actions.LoadConfigurationState(ctx)}
	})
}

func (model *dashboardModel) ensureHomeMenu() {
	if len(model.homeMenu.list.Items()) == 0 {
		model.homeMenu = newDashboardTaskMenu(model.homeAreas(), model.isDark, model.width, model.height)
	}
}

func (model *dashboardModel) ensureActivitySpinner() {
	if len(model.activitySpinner.Spinner.Frames) == 0 {
		model.activitySpinner = newTUISpinner(model.isDark)
	}
}

func (model dashboardModel) busyView() string {
	model.ensureActivitySpinner()
	return model.activitySpinner.View() + " " + model.busy
}

func (model dashboardModel) loadSetup() (tea.Model, tea.Cmd) {
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardSetupMsg{report: model.actions.LoadSetup(ctx)}
	})
}

func (model dashboardModel) loadInitial() (tea.Model, tea.Cmd) {
	model.jobs.id++
	return model.startRead(func(ctx context.Context) tea.Msg {
		jobs, jobsErr := model.observeManagedJobs(ctx)
		report, setup, err := model.actions.LoadInitial(ctx)
		return dashboardInitialMsg{report: report, setup: setup, err: err, jobs: jobs, jobsErr: jobsErr, jobsID: model.jobs.id, usbReserved: model.usbReservationPresent(), recovery: model.observeRecovery(ctx)}
	})
}

func schedulePXEProgressTick(id uint64) tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return dashboardPXEProgressTickMsg{id: id}
	})
}

func (model dashboardModel) loadPXEProgress(id uint64) tea.Cmd {
	return func() tea.Msg {
		progress, err := model.actions.LoadPXEProgress()
		return dashboardPXEProgressMsg{id: id, progress: progress, err: err}
	}
}

func scheduleControllerProgressTick(id uint64) tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return dashboardControllerProgressTickMsg{id: id}
	})
}

func (model dashboardModel) loadControllerProgress(id uint64) tea.Cmd {
	return func() tea.Msg {
		progress, err := model.actions.LoadControllerProgress()
		return dashboardControllerProgressMsg{id: id, progress: progress, err: err}
	}
}

func (model dashboardModel) View() tea.View {
	if model.inventory.cancel != nil && !model.helpOpen {
		view := tea.NewView(model.frame(model.renderShell(tuiShell{
			path:    []string{"Computers"},
			body:    model.busyView(),
			actions: []tuiAction{{key: "Esc", label: "Cancel"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}},
		})))
		view.AltScreen = true
		return view
	}
	if model.helpOpen {
		view := tea.NewView(model.frame(model.helpView()))
		view.AltScreen = true
		return view
	}
	content := ""
	switch model.screen {
	case dashboardManagedJobs:
		content = model.managedJobsView()
	case dashboardHostTrust:
		content = model.hostTrustView()
	case dashboardComputersArea:
		content = model.computersAreaView()
	case dashboardInstallationArea:
		content = model.installationAreaView()
	case dashboardSetup:
		content = model.setupView()
	case dashboardSetupKeys:
		content = model.setupKeysView()
	case dashboardHosts:
		content = model.computersView()
	case dashboardAdministration:
		content = model.administrationView()
	case dashboardDiagnostics:
		content = model.diagnosticsView()
	case dashboardDisclaimer:
		content = model.disclaimerView()
	case dashboardAssistantReview:
		content = model.assistantReviewView()
	case dashboardAssistant:
		content = model.assistantView()
	case dashboardTelemetry:
		content = model.telemetryView()
	case dashboardSupport:
		content = model.supportView()
	case dashboardTemplateReset:
		content = model.templateResetView()
	case dashboardSoftware:
		content = model.softwareView()
	case dashboardWorkspace:
		content = model.workspaceView()
	case dashboardHomeFiles:
		content = model.homeFilesView()
	case dashboardInternet:
		content = model.internetView()
	case dashboardLock:
		content = model.lockView()
	case dashboardShare:
		content = model.shareView()
	case dashboardDeferredUpdates:
		content = model.deferredUpdatesView()
	case dashboardNetworkChange:
		content = model.networkChangeView()
	case dashboardCleanup:
		content = model.cleanupView()
	case dashboardRecovery:
		content = model.recoveryView()
	case dashboardRecoveryReview:
		content = model.recoveryReviewView()
	case dashboardBackup:
		content = model.backupView()
	case dashboardShutdown, dashboardShutdownReview, dashboardShutdownResult:
		content = model.shutdownView()
	case dashboardDeploy, dashboardDeployReview:
		content = model.deployView()
	case dashboardController, dashboardControllerReview:
		content = model.controllerView()
	case dashboardServices, dashboardServicesRestartReview:
		content = model.servicesView()
	case dashboardLogs:
		content = model.logsView()
	case dashboardLogDetail:
		content = model.logDetailView()
	case dashboardGitReview, dashboardGitCommitSelect, dashboardGitCommitReview, dashboardGitUndoReview:
		content = model.gitReviewView()
	case dashboardUpdate, dashboardUpdateReview:
		content = model.updateView()
	case dashboardSettings, dashboardSettingsEdit, dashboardSettingsPasswords, dashboardSettingsReview, dashboardSettingsAccounts:
		content = model.settingsView()
	case dashboardPXE, dashboardPXEStartReview, dashboardPXELeaveReview:
		content = model.pxeView()
	case dashboardUSBInstall:
		content = model.remoteInstallView()
	default:
		content = model.homeView()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

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
