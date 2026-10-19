package presentation

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The `l` hint and key exist only while expanding shows something the compact
// view does not. Each case checks the hint, the key and the expanded view.

type progressDetailsCase struct {
	name        string
	empty       dashboardModel
	withDetails dashboardModel
	// marker appears only in the expanded view of withDetails.
	marker string
}

func progressDetailsCases() []progressDetailsCase {
	deployment := func(recent []string, progress domain.DeploymentProgress) dashboardModel {
		return dashboardModel{screen: dashboardDeploy, busy: "Applying", width: 100, height: 40,
			deployment: deploymentModel{applying: true, started: time.Now(), recent: recent, progress: progress}}
	}
	controller := func(recent []string) dashboardModel {
		return dashboardModel{screen: dashboardController, busy: "Activating controller", width: 100, height: 40,
			controller: controllerModel{applying: true, started: time.Now(), progress: domain.OperationProgress{
				Operation: "controller-apply", State: "running", Phase: "activate", Current: 2, Total: 4, Recent: recent}}}
	}
	pxe := func(recent []string) dashboardModel {
		return dashboardModel{screen: dashboardPXE, busy: "Preparing", width: 100, height: 40,
			installation: installationModel{pxePreparing: true, pxeStarted: time.Now(), pxeProgress: domain.OperationProgress{
				Operation: "pxe-prepare", State: "running", Phase: "clients", Current: 2, Total: 6, Recent: recent}}}
	}
	update := func(detail string) dashboardModel {
		model := dashboardModel{screen: dashboardUpdate, busy: "Checking systems", width: 100, height: 40,
			updates: updateModel{planning: true, target: "master", planStarted: time.Now(), planProgress: domain.UpdatePlanProgress{
				Phase: domain.UpdatePlanPhaseBuild, Detail: detail, Total: 5}}}
		model.beginRead(dashboardBuildTimeout)
		return model
	}
	applyProgress := domain.DeploymentProgress{Phase: domain.DeploymentPhaseApply, Completed: 2, Total: 4, TargetCurrent: 1, TargetTotal: 3}
	return []progressDetailsCase{
		{"deployment", deployment(nil, applyProgress), deployment([]string{"Activating pc01 over SSH", "Activating pc02 over SSH"}, applyProgress), "Recent activity:"},
		{"deployment output", deployment(nil, applyProgress), func() dashboardModel {
			progress := applyProgress
			progress.Output = []string{"pc01: restarting example.service"}
			return deployment(nil, progress)
		}(), "pc01: restarting example.service"},
		{"controller", controller([]string{"Building the reviewed controller system"}), controller([]string{"Validated prerequisites", "Building the reviewed controller system"}), "Validated prerequisites"},
		{"pxe", pxe([]string{"Building client systems"}), pxe([]string{"Built netboot kernel", "Built client pc02"}), "Built netboot kernel"},
		{"update check", update(""), update("Building all 5 required outputs together"), "Current check details"},
	}
}

func TestProgressDetailsHintAndKeyNeedSomethingToShow(t *testing.T) {
	for _, test := range progressDetailsCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(func() {
				if test.empty.read.cancel != nil {
					test.empty.read.cancel()
				}
				if test.withDetails.read.cancel != nil {
					test.withDetails.read.cancel()
				}
			})
			empty := test.empty
			if strings.Contains(empty.View().Content, "Progress details") {
				t.Fatalf("hint offered with nothing to show:\n%s", empty.View().Content)
			}
			updated, _ := empty.Update(keyPress("l"))
			empty = updated.(dashboardModel)
			if empty.progressDetails {
				t.Fatal("l toggled details with nothing to show")
			}
			if view := empty.View().Content; strings.Contains(view, test.marker) {
				t.Fatalf("empty expanded view rendered:\n%s", view)
			}

			shown := test.withDetails
			if !strings.Contains(shown.View().Content, "Progress details") {
				t.Fatalf("hint missing although details exist:\n%s", shown.View().Content)
			}
			if strings.Contains(shown.View().Content, test.marker) {
				t.Fatalf("details visible before l:\n%s", shown.View().Content)
			}
			updated, _ = shown.Update(keyPress("l"))
			shown = updated.(dashboardModel)
			if !shown.progressDetails || !strings.Contains(shown.View().Content, test.marker) {
				t.Fatalf("l did not expand details:\n%s", shown.View().Content)
			}
			updated, _ = shown.Update(keyPress("l"))
			shown = updated.(dashboardModel)
			if shown.progressDetails || strings.Contains(shown.View().Content, test.marker) {
				t.Fatalf("l did not collapse details:\n%s", shown.View().Content)
			}
		})
	}
}

func TestProgressDetailsAppearWhenProgressArrivesAndRenderCompactWhenItVanishes(t *testing.T) {
	model := dashboardModel{screen: dashboardDeploy, busy: "Applying", width: 100, height: 40,
		deployment: deploymentModel{applying: true, started: time.Now(), events: make(chan tea.Msg)}}
	if strings.Contains(model.View().Content, "Progress details") {
		t.Fatal("hint offered before any progress")
	}
	updated, _ := model.Update(dashboardDeploymentProgressMsg{progress: domain.DeploymentProgress{
		Phase: domain.DeploymentPhaseApply, Completed: 1, Total: 4, Activity: "Activating pc01 over SSH"}})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Progress details") {
		t.Fatalf("hint missing after progress arrived:\n%s", model.View().Content)
	}
	updated, _ = model.Update(keyPress("l"))
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Activating pc01 over SSH") {
		t.Fatalf("details missing:\n%s", model.View().Content)
	}
	// Details becoming empty again must not leave an empty expanded view.
	model.deployment.recent = nil
	view := model.View().Content
	if strings.Contains(view, "Progress details") || strings.Contains(view, "Recent activity") || !strings.Contains(view, "Updating computers") {
		t.Fatalf("expanded flag with no details did not render compact:\n%s", view)
	}
}

func TestControllerJournalIsShownAtOnceWhenProgressCannotBeRefreshed(t *testing.T) {
	model := progressDetailsCases()[2].empty
	model.controller.progressUnavailable = true
	view := model.View().Content
	if !strings.Contains(view, "journalctl") || strings.Contains(view, "Progress details") {
		t.Fatalf("the journal must be visible without the details key:\n%s", view)
	}
}
