package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestControllerFollowUpsExposeProgressAndDetails(t *testing.T) {
	for _, entry := range []string{"update", "package-base", "software"} {
		t.Run(entry, func(t *testing.T) {
			applies := 0
			model := dashboardModel{width: 100, height: 30, screen: dashboardUpdate, actions: DashboardActions{
				PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
					return domain.ControllerRebuildPlanReport{State: "ready"}
				},
				ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
					applies++
					return domain.ControllerRebuildExecutionReport{State: "completed", Applied: true, Verified: true}
				},
				LoadControllerProgress: func() (domain.OperationProgress, error) { return domain.OperationProgress{}, nil },
			}}
			model.updates.packageBase = entry == "package-base"
			var updated tea.Model
			var command tea.Cmd
			if entry == "software" {
				model.screen = dashboardSoftware
				updated, command = model.startSoftwareControllerApply()
			} else {
				updated, command = model.startUpdateControllerApply()
			}
			model = updated.(dashboardModel)
			batch, ok := command().(tea.BatchMsg)
			if !ok || len(batch) != 2 || !model.controller.applying {
				t.Fatal("controller follow-up did not start progress polling")
			}
			id := model.controller.progressID
			updated, poll := model.Update(dashboardControllerProgressMsg{id: id, progress: domain.OperationProgress{
				Operation: "controller-apply", State: "running", Phase: "build", Current: 1, Total: 4,
				StartedAt: model.controller.started.Add(time.Millisecond), Recent: []string{"Validated prerequisites", "Building the reviewed controller system"},
			}})
			model = updated.(dashboardModel)
			if poll == nil || !strings.Contains(model.View().Content, "Progress details") || strings.Contains(model.View().Content, "atomic two-file") {
				t.Fatalf("missing or misleading controller view:\n%s", model.View().Content)
			}
			updated, _ = model.Update(keyPress("l"))
			model = updated.(dashboardModel)
			if !strings.Contains(model.View().Content, "Recent activity") || !strings.Contains(model.View().Content, "Validated prerequisites") {
				t.Fatalf("l did not reveal controller details:\n%s", model.View().Content)
			}
			updated, _ = model.Update(keyPress("l"))
			model = updated.(dashboardModel)
			if strings.Contains(model.View().Content, "Recent activity") {
				t.Fatal("l did not collapse controller details")
			}
			updated, _ = model.Update(batch[0]())
			model = updated.(dashboardModel)
			if applies != 1 || model.controller.applying {
				t.Fatal("completion left controller progress running")
			}
			_, poll = model.Update(dashboardControllerProgressTickMsg{id: id})
			if poll != nil {
				t.Fatal("late tick restarted polling after completion")
			}
		})
	}
}

func TestControllerProgressReadFailureIsVisibleAndCanRecover(t *testing.T) {
	model := dashboardModel{screen: dashboardController, busy: "Building", width: 80, height: 24,
		controller: controllerModel{applying: true, progressID: 4, started: time.Now()}, progressDetails: true}
	updated, _ := model.Update(dashboardControllerProgressMsg{id: 4, err: errors.New("private filesystem error")})
	model = updated.(dashboardModel)
	if strings.Contains(model.View().Content, "cannot be read") {
		t.Fatal("a single unreadable poll was reported")
	}
	for range controllerProgressMissesShown - 1 {
		updated, _ = model.Update(dashboardControllerProgressMsg{id: 4, err: errors.New("private filesystem error")})
		model = updated.(dashboardModel)
	}
	view := model.View().Content
	if !strings.Contains(view, "cannot be read") || strings.Contains(view, "private filesystem error") || !strings.Contains(view, "journalctl") {
		t.Fatalf("missing safe progress fallback:\n%s", view)
	}
	updated, _ = model.Update(dashboardControllerProgressMsg{id: 3})
	model = updated.(dashboardModel)
	if !model.controller.progressUnavailable {
		t.Fatal("stale response cleared current error")
	}
	updated, _ = model.Update(dashboardControllerProgressMsg{id: 4, progress: domain.OperationProgress{
		Operation: "controller-apply", State: "running", Phase: "build", StartedAt: model.controller.started,
	}})
	if updated.(dashboardModel).controller.progressUnavailable {
		t.Fatal("successful poll did not clear the warning")
	}
}

func TestCandidateControllerBuildExposesItsOwnDetails(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, packageBase := range []bool{false, true} {
			model := dashboardModel{screen: dashboardUpdate, busy: "Building candidate", width: size[0], height: size[1],
				updates: updateModel{planning: true, packageBase: packageBase, target: "master", planStarted: time.Now(), planProgress: domain.UpdatePlanProgress{
					Phase: domain.UpdatePlanPhaseBuild, Detail: "Building the candidate controller", Current: 2, Total: 5,
				}}}
			model.beginRead(dashboardBuildTimeout)
			t.Cleanup(model.read.cancel)
			for _, expanded := range []bool{true, false, true} {
				updated, command := model.Update(keyPress("l"))
				model = updated.(dashboardModel)
				view := demoANSI.ReplaceAllString(model.View().Content, "")
				if command != nil || model.screen != dashboardUpdate || !model.updates.planning || model.controller.applying || model.updates.applying {
					t.Fatal("toggling details changed the running operation")
				}
				for _, expected := range []string{model.updateTitle(), "Target: master", "Test systems before saving", "Testing the controller system", "Safety check 2/5", "Elapsed:", "current deployment remains unchanged", "Progress details", "Help"} {
					if !strings.Contains(view, expected) {
						t.Fatalf("%v details=%t lost %q:\n%s", size, expanded, expected, view)
					}
				}
				for _, detail := range []string{"Current check details", "Building the candidate controller"} {
					if strings.Contains(view, detail) != expanded {
						t.Fatalf("%v details=%t did not toggle %q in place:\n%s", size, expanded, detail, view)
					}
				}
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("update details overflow at %v:\n%s", size, view)
				}
			}
		}
	}
}

func TestGroupedUpdateBuildRetainsDetailsWithoutInventingCompletion(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		model := dashboardModel{screen: dashboardUpdate, busy: "Checking systems", width: size[0], height: size[1], progressDetails: true,
			updates: updateModel{planning: true, target: "master", planStarted: time.Now(), planProgress: domain.UpdatePlanProgress{
				Phase: domain.UpdatePlanPhaseBuild, Detail: "Building all 5 required outputs together", Total: 5,
			}}}
		model.beginRead(dashboardBuildTimeout)
		t.Cleanup(model.read.cancel)
		view := demoANSI.ReplaceAllString(model.View().Content, "")
		for _, expected := range []string{"Target: master", "Required outputs: 5", "checking together", "Current check details", "Building all 5 required outputs together", "Elapsed:", "current deployment remains unchanged", "Help"} {
			if !strings.Contains(view, expected) {
				t.Fatalf("%v grouped build lost %q:\n%s", size, expected, view)
			}
		}
		if strings.Contains(view, "Safety check 0/") || strings.Contains(view, "100%") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("%v grouped build has misleading progress or overflows:\n%s", size, view)
		}
	}
}

func TestControllerReviewStatesChangesOrUpToDate(t *testing.T) {
	m := experienceFixture(t, 1)
	m.screen = dashboardControllerReview
	m.controller.plan = domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: strings.Repeat("a", 40), ChangesKnown: true, Changes: []string{"Saved settings", "Nixorium version"}}
	view := m.View().Content
	for _, want := range []string{"Apply the saved configuration", "• Saved settings", "• Nixorium version", "Technical details", strings.Repeat("a", 40), "Apply to controller"} {
		if !strings.Contains(view, want) {
			t.Fatalf("review lacks %q:\n%s", want, view)
		}
	}
	m.controller.plan.ChangesKnown, m.controller.plan.Changes = false, nil
	if !strings.Contains(m.View().Content, "Unknown: no readable record") {
		t.Fatal("unknown changes are not explained")
	}
	m.controller.plan.State, m.controller.plan.Current = "current", true
	view = m.View().Content
	if !strings.Contains(view, "This controller is up to date") || strings.Contains(view, "Apply to controller") || strings.Contains(view, "Press Enter") {
		t.Fatalf("current controller still offers application:\n%s", view)
	}
	applied := 0
	m.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		applied++
		return domain.ControllerRebuildExecutionReport{}
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd != nil || applied != 0 || m.controller.applying || !strings.Contains(m.message, "already up to date") {
		t.Fatalf("Enter started an unnecessary application: %q", m.message)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(dashboardModel).screen == dashboardControllerReview {
		t.Fatal("Esc did not leave the up-to-date review")
	}
}

func TestDashboardReviewsAndRunsControllerRebuild(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	applied := 0
	refreshed := 0
	actions := DashboardActions{
		Refresh: func(ctx context.Context) (domain.StatusReport, error) {
			refreshed++
			return domain.StatusReport{Deployment: domain.DeploymentStatus{Ready: true}}, nil
		},
		PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-plan",
				State:         "ready",
				Controller:    "pc99",
				Revision:      revision,
				Confirmation:  "REBUILD",
				Issues:        []domain.ValidationIssue{},
			}
		},
		ApplyController: func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			applied++
			return domain.ControllerRebuildExecutionReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-apply",
				State:         "completed",
				Controller:    plan.Controller,
				Revision:      plan.Revision,
				Phase:         domain.ControllerRebuildPhaseComplete,
				Applied:       true,
				Verified:      true,
			}
		},
		LoadControllerProgress: func() (domain.OperationProgress, error) {
			return domain.OperationProgress{}, nil
		},
	}
	model := dashboardModel{actions: actions}
	updated, command := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardAdministration {
		t.Fatalf("maintenance did not open: screen=%d", model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardControllerReview || !strings.Contains(model.View().Content, revision) || strings.Contains(model.View().Content, "REBUILD pc99") || !strings.Contains(model.View().Content, "Press Enter") {
		t.Fatalf("controller review missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller apply did not start: %+v", model)
	}
	if !strings.Contains(model.View().Content, "Waiting for managed progress") {
		t.Fatalf("controller apply omits managed progress feedback:\n%s", model.View().Content)
	}
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("controller apply did not schedule action and progress polling")
	}
	operation, ok := batch[0]().(tea.BatchMsg)
	if !ok || len(operation) != 2 {
		t.Fatal("controller result and status refresh are not separate")
	}
	updated, _ = model.Update(operation[0]())
	model = updated.(dashboardModel)
	if refreshed != 0 || model.busy != "" {
		t.Fatal("result waited for the status refresh")
	}
	updated, _ = model.Update(operation[1]())
	model = updated.(dashboardModel)
	if applied != 1 || refreshed != 1 || !model.report.Deployment.Ready || model.screen != dashboardController || !strings.Contains(model.View().Content, "Applied to this controller and verified") || !strings.Contains(model.View().Content, "Enter") || !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("controller result missing: applied=%d refreshed=%d\n%s", applied, refreshed, model.View().Content)
	}
	model.controller.progress = domain.OperationProgress{
		Operation: "controller-apply", State: "completed", Phase: "complete",
		Current: 4, Total: 4, Recent: []string{"Controller revision activated and verified"},
	}
	if strings.Contains(model.View().Content, "progress details") {
		t.Fatalf("completed controller details should start collapsed:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Recent activity") || !strings.Contains(model.View().Content, "Hide details") {
		t.Fatalf("controller details did not expand:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("controller completion did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestDashboardControllerProgressShowsTypedBuildState(t *testing.T) {
	started := time.Now().UTC().Add(-2 * time.Second)
	model := dashboardModel{
		screen: dashboardController,
		busy:   "Building and activating the reviewed controller revision",
		controller: controllerModel{
			applying:   true,
			started:    started,
			progressID: 3,
		},
		width: 90,
		actions: DashboardActions{LoadControllerProgress: func() (domain.OperationProgress, error) {
			return domain.OperationProgress{}, nil
		}},
	}
	updated, command := model.Update(dashboardControllerProgressMsg{
		id: 3,
		progress: domain.OperationProgress{
			Operation: "controller-apply", State: "running", Phase: "build",
			StartedAt: started, UpdatedAt: time.Now().UTC(), Current: 1, Total: 4,
			Recent: []string{"Validated configuration and installed keys", "Building the reviewed controller system"},
		},
	})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("controller apply did not schedule another progress poll")
	}
	view := model.View().Content
	for _, expected := range []string{"Building system", "1/4", "Progress details", "Building the reviewed controller system", "Elapsed:"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("controller progress omits %q:\n%s", expected, view)
		}
	}
	if strings.Count(view, "Progress details") != 1 || strings.Contains(view, "l progress details") {
		t.Fatalf("controller progress duplicates the contextual action bar:\n%s", view)
	}
}
