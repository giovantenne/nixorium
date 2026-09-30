package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type managedJobsModel struct {
	items  []domain.ManagedJob
	err    error
	id     uint64
	cursor int
}
type managedJobsTickMsg struct{ id uint64 }
type managedJobsMsg struct {
	id   uint64
	jobs []domain.ManagedJob
	err  error
}

func (model dashboardModel) observeManagedJobs(ctx context.Context) ([]domain.ManagedJob, error) {
	if model.actions.LoadManagedJobs == nil || model.actions.ClassroomMode {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return model.actions.LoadManagedJobs(ctx)
}

func (model dashboardModel) loadManagedJobs(id uint64) tea.Cmd {
	return func() tea.Msg {
		jobs, err := model.observeManagedJobs(context.Background())
		return managedJobsMsg{id: id, jobs: jobs, err: err}
	}
}

func (model dashboardModel) scheduleManagedJobsTick() tea.Cmd {
	if model.actions.LoadManagedJobs == nil || model.actions.ClassroomMode {
		return nil
	}
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return managedJobsTickMsg{id: model.jobs.id} })
}

func (model dashboardModel) managedJobConflict() string {
	if model.jobs.err != nil {
		return "Managed job state is unavailable. Wait for a successful check before starting conflicting work; use View progress from Overview."
	}
	for _, job := range model.jobs.items {
		if job.BlocksStart() {
			return fmt.Sprintf("%s is %s. Wait before starting conflicting work; use View progress from Overview.", managedJobTitle(job.Operation), job.State)
		}
	}
	return ""
}

func managedJobTitle(operation string) string {
	if operation == "pxe-prepare" {
		return "PXE preparation"
	}
	return "Controller configuration"
}

func (model dashboardModel) managedJobNotices() []tuiNotice {
	if model.jobs.err != nil {
		return []tuiNotice{{kind: tuiStatusAttention, title: "Managed job state unavailable", detail: "Cannot tell whether background work is running. Use v to view progress; checks retry automatically."}}
	}
	notices := []tuiNotice{}
	for _, job := range model.jobs.items {
		if job.State == "running" || job.State == "interrupted" || job.State == "unknown" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: managedJobTitle(job.Operation) + " — " + job.State, detail: "Press v to view progress and the journal unit. No job will be started."})
		}
	}
	return notices
}

// This view observes an independent job. It never sets applying/preparing flags,
// synthesizes execution results or resumes a guided installation follow-up.
func (model dashboardModel) managedJobsView() string {
	lines := []string{tuiTitle("Managed background work", model.isDark), tuiMuted("Read-only attachment. Leaving this view does not stop the job.", model.isDark), ""}
	if model.jobs.err != nil {
		lines = append(lines, "Unit state could not be checked. The job may still be running.", "Checks retry automatically; do not start conflicting work.")
	} else if len(model.jobs.items) == 0 {
		lines = append(lines, "Checking managed job state…")
	} else {
		job := model.jobs.items[min(model.jobs.cursor, len(model.jobs.items)-1)]
		lines = append(lines, managedJobTitle(job.Operation)+" — "+job.State, job.Detail, "")
		if job.State == "interrupted" && job.Progress.Operation != "" {
			lines = append(lines, "Last recorded phase: "+job.Progress.Phase)
			lines = append(lines, job.Progress.Recent...)
		} else if job.Progress.Operation != "" {
			lines = append(lines, model.operationProgressView(job.Progress, "Observed progress")...)
		} else {
			lines = append(lines, "No matching progress record is available.")
		}
		lines = append(lines, "", "Journal unit: "+job.Unit)
	}
	return model.renderShell(tuiShell{path: []string{"Overview", "Background work"}, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "Tab", label: "Other job"}, {key: "Esc", label: "Overview"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}}})
}
