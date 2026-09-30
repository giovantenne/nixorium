package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestManagedJobsAttachOldProgressWithoutStartingOrFollowingUp(t *testing.T) {
	started := time.Now().Add(-time.Hour)
	job := domain.ManagedJob{Operation: "controller-apply", State: "running", Unit: "nixorium-apply-controller@" + strings.Repeat("a", 40) + ".service", Progress: domain.OperationProgress{Operation: "controller-apply", State: "running", Phase: "build", StartedAt: started, Recent: []string{"Existing build"}}}
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{
		LoadManagedJobs: func(context.Context) ([]domain.ManagedJob, error) { return []domain.ManagedJob{job}, nil },
		ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			t.Fatal("attachment started a controller job")
			return domain.ControllerRebuildExecutionReport{}
		},
		PreparePXE: func() domain.ActionReport {
			t.Fatal("attachment resumed guided installation")
			return domain.ActionReport{}
		},
	}, false)
	model.width, model.height = 120, 30
	updated, command := model.Update(model.loadManagedJobs(0)())
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View().Content, "Controller configuration — running") {
		t.Fatal("no running banner or poll")
	}
	updated, _ = model.Update(demoText("v"))
	model = updated.(dashboardModel)
	if model.screen != dashboardManagedJobs || model.controller.applying || model.installation.pxePreparing || model.busy != "" {
		t.Fatal("attachment acquired mutation ownership")
	}
	if !strings.Contains(model.View().Content, "Existing build") || !model.jobs.items[0].Progress.StartedAt.Equal(started) {
		t.Fatal("old progress discarded")
	}
	job.State = "completed"
	job.Progress.State = "completed"
	job.Progress.Phase = "complete"
	updated, _ = model.Update(managedJobsMsg{id: 0, jobs: []domain.ManagedJob{job}})
	model = updated.(dashboardModel)
	if model.controller.result.Verified || model.installation.flow || model.screen != dashboardManagedJobs {
		t.Fatal("observation fabricated verification/follow-up")
	}
	updated, _ = model.Update(demoCode(tea.KeyEscape))
	model = updated.(dashboardModel)
	if model.screen != dashboardHome {
		t.Fatal("cannot leave attachment")
	}
}

func TestManagedJobsInterruptedAndConflict(t *testing.T) {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{}, false)
	model.width, model.height = 120, 30
	job := domain.ManagedJob{Operation: "pxe-prepare", State: "running", Unit: "nixorium-prepare-pxe.service"}
	model.jobs.items = []domain.ManagedJob{job}
	for name, attempt := range map[string]func() (interface{}, bool){
		"controller":   func() (interface{}, bool) { m, c := model.openControllerReview(); return m, c != nil },
		"installation": func() (interface{}, bool) { m, c := model.beginComputerInstallation("pxe"); return m, c != nil },
	} {
		t.Run(name, func(t *testing.T) {
			m, command := attempt()
			if command || !strings.Contains(m.(dashboardModel).message, "Wait before starting conflicting work") {
				t.Fatal("conflict was not explained/blocked")
			}
		})
	}
	job.State = "interrupted"
	model.jobs.items = []domain.ManagedJob{job}
	if model.managedJobConflict() != "" || !strings.Contains(model.View().Content, "interrupted") {
		t.Fatal("interrupted is not distinguished from running")
	}
	model.screen = dashboardManagedJobs
	if !strings.Contains(model.View().Content, job.Unit) {
		t.Fatal("missing interrupted journal unit")
	}
	model.jobs.err = errors.New("offline")
	if model.managedJobConflict() == "" || !strings.Contains(model.View().Content, "may still be running") {
		t.Fatal("unknown state treated as idle")
	}
}

func TestManagedJobsStalePollAndOwnedProgressFilters(t *testing.T) {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{State: "ready"}, DashboardActions{}, false)
	model.jobs.id = 3
	updated, command := model.Update(managedJobsMsg{id: 2, jobs: []domain.ManagedJob{{State: "running"}}})
	if command != nil || len(updated.(dashboardModel).jobs.items) != 0 {
		t.Fatal("stale job observation accepted")
	}
	model.controller.started = time.Now()
	model.controller.progressID = 1
	updated, _ = model.Update(dashboardControllerProgressMsg{id: 1, progress: domain.OperationProgress{StartedAt: time.Now().Add(-time.Hour), Phase: "build"}})
	if updated.(dashboardModel).controller.progress.Phase != "" {
		t.Fatal("owned controller accepted old progress")
	}
	model.installation.pxeStarted = time.Now()
	model.installation.pxeProgressID = 1
	updated, _ = model.Update(dashboardPXEProgressMsg{id: 1, progress: domain.OperationProgress{StartedAt: time.Now().Add(-time.Hour), Phase: "artifacts"}})
	if updated.(dashboardModel).installation.pxeProgress.Phase != "" {
		t.Fatal("owned PXE accepted old progress")
	}
}

func TestManagedJobsCachedObservationDoesNotBlockOwnedGuidedFollowup(t *testing.T) {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		PreparePXE: func() domain.ActionReport { return domain.ActionReport{} },
	}, false)
	model.jobs.items = []domain.ManagedJob{{Operation: "controller-apply", State: "running"}}
	model.controller.result = domain.ControllerRebuildExecutionReport{Applied: true, Verified: true}
	updated, command := model.startComputerInstallationPreparation()
	if command == nil || !updated.(dashboardModel).installation.pxePreparing {
		t.Fatal("stale background poll blocked an owned continuation before its live adapter recheck")
	}
}

func TestManagedJobsStartupDoesNotResumeSetupOrExposeClassroomAdmin(t *testing.T) {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{LoadManagedJobs: func(ctx context.Context) ([]domain.ManagedJob, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded observation")
		}
		return []domain.ManagedJob{{State: "running"}}, nil
	}}, true)
	updated, _ := model.Update(dashboardInitialMsg{setup: domain.SetupReport{CurrentStage: domain.SetupStageApply}, jobs: []domain.ManagedJob{{Operation: "controller-apply", State: "running"}}})
	if updated.(dashboardModel).screen != dashboardHome || updated.(dashboardModel).installation.flow {
		t.Fatal("startup resumed setup while job runs")
	}
	failed, _ := model.Update(dashboardInitialMsg{err: errors.New("repository unavailable"), jobs: []domain.ManagedJob{{Operation: "pxe-prepare", State: "running"}}})
	failedModel := failed.(dashboardModel)
	failedModel.width, failedModel.height = 120, 30
	if !strings.Contains(failedModel.View().Content, "PXE preparation — running") || !strings.Contains(failedModel.View().Content, "View progress") {
		t.Fatal("repository failure hid surviving managed work")
	}
	if _, err := model.observeManagedJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	model.actions.ClassroomMode = true
	model.actions.LoadManagedJobs = func(context.Context) ([]domain.ManagedJob, error) {
		t.Fatal("classroom inspected administrative jobs")
		return nil, nil
	}
	if jobs, err := model.observeManagedJobs(context.Background()); err != nil || jobs != nil {
		t.Fatal("classroom has administrative jobs")
	}
}
