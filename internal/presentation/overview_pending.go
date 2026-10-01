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
	if model.usbReserved {
		add("pending-usb", "A USB installation is unfinished", "Open it to resume, verify or close it. Other computer operations wait until then.")
	}
	switch model.report.PXE.Mode {
	case "active":
		add("pending-pxe", "Network installation is active", "Review the installation session before restoring normal networking.")
	case "recovery-required", "degraded":
		add("pending-pxe", "Controller network recovery required", "Inspect the installation session; recovery remains separately confirmed.")
	}
	if notice, ok := setupOverviewNotice(model.setup); ok {
		id := "pending-setup"
		switch model.setup.CurrentStage {
		case domain.SetupStageReview:
			id = "pending-git"
		case domain.SetupStageApply:
			id = "pending-controller"
		case domain.SetupStageArtifacts:
			id = "pending-pxe"
		}
		if id != "pending-controller" || !controllerVerifiedForSave(model.pendingRevision, model.controller.result) {
			add(id, notice.title, notice.detail)
		}
	}
	if model.report.Git.Dirty {
		add("pending-git", "Configuration has uncommitted changes", "Last local Git observation; inspect paths before recording anything.")
	}
	if model.report.StoreSpaceLow && model.report.StoreFreeBytes != nil {
		add("pending-disk", "Low Nix store disk space", fmt.Sprintf("%.1f GiB available at the last local check. Open Free disk space to remove old system versions after review.", float64(*model.report.StoreFreeBytes)/(1<<30)))
	}
	if (model.pendingRevision != "" && !controllerVerifiedForSave(model.pendingRevision, model.controller.result)) || (model.controller.plan.Operation != "" && !model.controller.plan.HasErrors() && !model.controller.plan.Current && !controllerVerifiedForSave(model.controller.plan.Revision, model.controller.result)) {
		add("pending-controller", "Saved configuration needs applying to this controller", "Observed in this session. Open a fresh controller review.")
	}
	if model.report.PXEPreparation.Present && !model.report.PXEPreparation.Ready {
		add("pending-pxe", "Installation files need preparing", "Previously checked installation files are stale or invalid; review them again.")
	}
	hosts := model.computers.hosts
	if !hosts.GeneratedAt.IsZero() {
		add("pending-clients", fmt.Sprintf("Clients last checked %s", hosts.GeneratedAt.Local().Format("15:04:05")), fmt.Sprintf("Observation from %s: %d current, %d need updates, %d unknown. Not live state; open inventory to check again.", hosts.GeneratedAt.Format("2006-01-02 15:04:05 MST"), hosts.Deployment.Current, hosts.Deployment.Outdated, hosts.Deployment.Unknown))
	}
	return tasks
}

func (model dashboardModel) usbReservationPresent() bool {
	return !model.actions.ClassroomMode && model.actions.RemoteReservationPresent != nil && model.actions.RemoteReservationPresent()
}

func (model dashboardModel) overviewTasks() []dashboardTask {
	return append(model.pendingTasks(), dashboardTasks...)
}

type overviewRefreshMsg struct {
	usbReserved bool
	report      domain.StatusReport
	setup  domain.SetupReport
	err    error
}

func (model dashboardModel) refreshOverview() (tea.Model, tea.Cmd) {
	if model.actions.LoadInitial == nil {
		model.message = "Local refresh is unavailable in this session."
		return model, nil
	}
	model.busy = "Refreshing local configuration and service observations"
	return model.startRead(func(ctx context.Context) tea.Msg {
		report, setup, err := model.actions.LoadInitial(ctx)
		return overviewRefreshMsg{report: report, setup: setup, err: err, usbReserved: model.usbReservationPresent()}
	})
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
	case "pending-git":
		if model.actions.LoadGitReview != nil {
			return model.openMaintenanceTask("g")
		}
	case "pending-controller":
		if model.actions.PlanController != nil {
			return model.openControllerReview()
		}
	case "pending-disk":
		return model.openCleanup()
	case "pending-clients":
		if model.actions.LoadHosts != nil {
			return model.openComputerTask("h")
		}
	case "pending-setup":
		return model.startComputerInstallation()
	}
	if strings.HasPrefix(id, "pending-") && id != "pending-jobs" {
		model.message = "This follow-up is unavailable in this session."
	}
	return model, nil
}
