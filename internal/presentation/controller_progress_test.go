package presentation

import (
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
				PlanController: func() domain.ControllerRebuildPlanReport { return domain.ControllerRebuildPlanReport{State: "ready"} },
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
	view := model.View().Content
	if !strings.Contains(view, "could not be refreshed") || strings.Contains(view, "private filesystem error") || !strings.Contains(view, "journalctl") {
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
			for _, expanded := range []bool{true, false, true} {
				updated, command := model.Update(keyPress("l"))
				model = updated.(dashboardModel)
				view := demoANSI.ReplaceAllString(model.View().Content, "")
				if command != nil || model.screen != dashboardUpdate || !model.updates.planning || model.controller.applying || model.updates.applying {
					t.Fatal("toggling details changed the running operation")
				}
				for _, expected := range []string{model.updateTitle(), "Target: master", "Test systems before saving", "Testing the controller system", "Safety check 2/5", "elapsed", "current deployment remains unchanged", "Progress details", "Help"} {
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
