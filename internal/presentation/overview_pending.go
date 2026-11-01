package presentation

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Pending rows summarize observations, never authorize operations. Opening a
// row follows the ordinary read/review path and never starts a mutation.
func (model dashboardModel) pendingTasks() []dashboardTask {
	var tasks []dashboardTask
	add := func(id, title, detail string) {
		for _, task := range tasks {
			if task.id == id {
				return
			}
		}
		tasks = append(tasks, dashboardTask{id: id, shortcut: fmt.Sprint(len(tasks) + 1), title: title, description: detail})
	}
	if model.jobs.err != nil {
		add("pending-jobs", "Background work needs checking", "Open read-only progress; managed job state is unavailable.")
	}
	for _, job := range model.jobs.items {
		if job.State == "running" || job.State == "interrupted" || job.State == "unknown" {
			add("pending-jobs", managedJobTitle(job.Operation)+" — "+job.State, "View observed progress and its journal. No job will be started.")
		}
	}
	for _, condition := range model.recovery.report.Conditions {
		switch condition.Kind {
		case domain.RecoveryDeploymentPending:
			add("pending-deploy-recovery", condition.Title, condition.Detail)
		case domain.RecoveryResetPending:
			add("pending-reset-recovery", condition.Title, condition.Detail)
		case domain.RecoveryOperationBusy:
			add("pending-busy", condition.Title, condition.Detail)
		case domain.RecoverySettingsInvalid:
			add("pending-settings", condition.Title, condition.Detail)
		case domain.RecoveryControllerChanged:
			add("pending-controller", condition.Title, condition.Detail)
		case domain.RecoveryBackupDue:
			add("pending-backup", condition.Title, condition.Detail)
		case domain.RecoveryDeferredUpdates:
			add("pending-deferred", condition.Title, condition.Detail)
		case domain.RecoveryNetworkChanging:
			add("pending-network", condition.Title, condition.Detail)
		}
	}
	if model.usbReserved {
		add("pending-usb", situationTitle("USB-RESERVED"), "Open it to resume, verify or close it. Other computer operations wait until then.")
	}
	switch model.report.PXE.Mode {
	case "active":
		add("pending-pxe", situationTitle("PXE-ACTIVE"), "Open it to finish the network installation and restore the controller's normal network.")
	case "recovery-required", "degraded":
		add("pending-pxe", situationTitle("PXE-ACTIVE"), "It was interrupted. Open it to recover the controller network; nothing changes on the student computers.")
	}
	// Controller setup evidence enters the same ordered observation as saves
	// and reviews; never revive it by reading the older setup report again.
	if notice, ok := setupOverviewNotice(model.setup); ok && model.setup.CurrentStage != domain.SetupStageApply {
		id := "pending-setup"
		if model.setup.CurrentStage == domain.SetupStageReview {
			id = "pending-git"
		}
		add(id, notice.title, notice.detail)
	}
	if model.report.Git.Dirty {
		add("pending-git", situationTitle("GIT-DIRTY"), "Last local Git observation; inspect paths before recording anything.")
	}
	// Low disk space is shown in Maintenance, next to Free disk space.
	if model.controllerObservation == controllerOverviewPending {
		add("pending-controller", situationTitle("CONTROLLER-NOT-APPLIED"), "Observed in this session. Open a fresh controller review.")
	}
	if model.report.PXEPreparation.Present && !model.report.PXEPreparation.Ready {
		add("pending-pxe", "Installation files need preparing", "Previously checked installation files are stale or invalid; review them again.")
	}
	// The last client check is a reminder, not work: Computer inventory shows it.
	return tasks
}

// situationTitle is the catalogue sentence for a situation code.
func situationTitle(code string) string {
	step, _ := domain.NextStepByCode(code)
	return step.Title
}

func (model dashboardModel) usbReservationPresent() bool {
	return !model.actions.ClassroomMode && model.actions.RemoteReservationPresent != nil && model.actions.RemoteReservationPresent()
}

func (model dashboardModel) overviewTasks() []dashboardTask {
	return append(model.pendingTasks(), model.homeAreas()...)
}

type overviewRefreshMsg struct {
	usbReserved bool
	recovery    domain.RecoveryReport
	report      domain.StatusReport
	setup       domain.SetupReport
	err         error
}

func (model dashboardModel) refreshOverview() (tea.Model, tea.Cmd) {
	if model.actions.LoadInitial == nil {
		model.message = "Local refresh is unavailable in this session."
		return model, nil
	}
	model.overviewReads++
	model.busy = "Refreshing local configuration and service observations"
	return model.startRead(func(ctx context.Context) tea.Msg {
		report, setup, err := model.actions.LoadInitial(ctx)
		return overviewRefreshMsg{report: report, setup: setup, err: err, usbReserved: model.usbReservationPresent(), recovery: model.observeRecovery(ctx)}
	})
}

// overviewQuietRefreshMsg re-reads the local observations the Overview lists
// when the administrator returns to it, without a busy screen or a message.
type overviewQuietRefreshMsg struct {
	report      domain.StatusReport
	setup       domain.SetupReport
	err         error
	usbReserved bool
	recovery    domain.RecoveryReport
	jobs        []domain.ManagedJob
	jobsErr     error
	jobsID      uint64
	read        uint64
}

// withQuietOverviewRefresh re-reads, in the background, what the Overview
// lists, so items about finished work disappear without pressing r.
func (model dashboardModel) withQuietOverviewRefresh(command tea.Cmd) (dashboardModel, tea.Cmd) {
	if model.actions.LoadInitial == nil || model.actions.ClassroomMode || model.busy != "" {
		return model, command
	}
	model.jobs.id++
	model.overviewReads++
	id, read := model.jobs.id, model.overviewReads
	return model, tea.Batch(command, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer cancel()
		jobs, jobsErr := model.observeManagedJobs(ctx)
		report, setup, err := model.actions.LoadInitial(ctx)
		return overviewQuietRefreshMsg{report: report, setup: setup, err: err, usbReserved: model.usbReservationPresent(), recovery: model.observeRecovery(ctx), jobs: jobs, jobsErr: jobsErr, jobsID: id, read: read}
	})
}

func (model dashboardModel) finishQuietOverviewRefresh(message overviewQuietRefreshMsg) (tea.Model, tea.Cmd) {
	// Starting the refresh took a new identity for the job observations and
	// dropped the pending tick of the previous polling chain. This is the only
	// place that can start the new chain, whatever happens to the rest of the
	// result: without it a finished job would stay "running" forever.
	var poll tea.Cmd
	if message.jobsID == model.jobs.id {
		poll = model.scheduleManagedJobsTick()
	}
	if message.err != nil || model.busy != "" || message.read != model.overviewReads {
		return model, poll
	}
	if message.jobsID == model.jobs.id {
		model.jobs.items, model.jobs.err = message.jobs, message.jobsErr
	}
	message.report.PXEPreparation = model.report.PXEPreparation
	message.report.Meta = model.report.Meta
	model.report, model.setup = message.report, message.setup
	model.usbReserved = message.usbReserved
	model.observeSetupController(message.setup)
	model.observeRecoveryController(message.recovery)
	if model.screen == dashboardHome {
		model.syncHomeTasks()
	}
	return model, poll
}

func (model *dashboardModel) syncHomeTasks() {
	model.ensureHomeMenu()
	selected, hadSelection := model.homeMenu.selected()
	tasks := model.overviewTasks()
	items := make([]list.Item, len(tasks))
	for index := range tasks {
		items[index] = tasks[index]
	}
	model.homeMenu.list.SetItems(items)
	if hadSelection {
		for index, task := range tasks {
			if task.id == selected.id {
				model.homeMenu.list.Select(index)
				return
			}
		}
	}
	model.homeMenu.list.Select(0)
}

func (model dashboardModel) openPendingTask(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "pending-jobs":
		model.screen = dashboardManagedJobs
	case "pending-pxe":
		return model.openNetworkInstallation()
	case "pending-usb":
		return model.openUSBInstallation()
	case "pending-deploy-recovery":
		if model.actions.PlanDeploymentRecovery != nil {
			return model.openDeploymentRecovery()
		}
		return model.openRecovery()
	case "pending-reset-recovery":
		if model.actions.PlanResetRecovery != nil {
			return model.openResetRecovery()
		}
		return model.openRecovery()
	case "pending-backup":
		return model.openBackup()
	case "pending-deferred":
		return model.openDeferredUpdates()
	case "pending-network":
		return model.openNetworkChange()
	case "pending-settings":
		return model.openMaintenanceTask("e")
	case "pending-busy":
		return model.openRecovery()
	case "pending-git":
		if model.actions.LoadGitReview != nil {
			return model.openMaintenanceTask("g")
		}
	case "pending-controller":
		if model.actions.PlanController != nil {
			return model.openControllerReview()
		}
	case "pending-setup":
		return model.startComputerInstallation()
	}
	if strings.HasPrefix(id, "pending-") && id != "pending-jobs" {
		model.message = "This follow-up is unavailable in this session."
	}
	return model, nil
}
