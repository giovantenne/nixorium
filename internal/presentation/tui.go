package presentation

import (
	"context"
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
