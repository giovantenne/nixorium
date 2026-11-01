package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func offComputerPlan() domain.DeploymentPlanReport {
	return domain.DeploymentPlanReport{Operation: "deploy-plan", State: "ready", Requested: "pc01,pc02", ColmenaSelector: "pc01,pc02", Revision: strings.Repeat("a", 40),
		Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}},
		Availability: []domain.DeploymentTargetAvailability{
			{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
			{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown},
		}}
}

func TestDeploymentReviewQueuesComputersThatAreOffByDefault(t *testing.T) {
	queued, direct := 0, 0
	actions := DashboardActions{
		ApplyDeployment: func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			direct++
			return domain.DeploymentExecutionReport{Operation: "deploy-apply", State: "completed"}
		},
		ApplyDeploymentQueued: func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			queued++
			return domain.DeploymentExecutionReport{Operation: "deploy-apply", State: "completed", Queued: []string{"pc02"}}
		},
	}
	model := newTestModel(t, withReadyLab(), withActions(actions))
	model.width, model.height = 120, 40
	updated, _ := model.Update(dashboardDeploymentPlanMsg{report: offComputerPlan()})
	model = updated.(dashboardModel)
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	if model.screen != dashboardDeployReview || !model.deployment.queueOff || !strings.Contains(view, "[x] Update pc02 when switched on") || !strings.Contains(view, "F3") {
		t.Fatalf("review does not offer queueing:\n%s", view)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	model = updated.(dashboardModel)
	if model.deployment.queueOff || !strings.Contains(demoANSI.ReplaceAllString(model.View().Content, ""), "[ ] Update pc02") {
		t.Fatal("F3 did not turn queueing off")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("apply did not start")
	}
	deadline := time.After(5 * time.Second)
	for message := command(); queued == 0; {
		select {
		case <-deadline:
			t.Fatal("queued apply did not run")
		default:
		}
		if message == nil {
			break
		}
		updated, command = model.Update(message)
		model = updated.(dashboardModel)
		if command == nil {
			break
		}
		message = command()
	}
	if queued != 1 || direct != 0 {
		t.Fatalf("queued=%d direct=%d", queued, direct)
	}
}

func TestQueuedUpdatesScreenListsAndRemoves(t *testing.T) {
	removed := []string{}
	status := domain.DeferredUpdateStatus{State: "ready", Updates: []domain.DeferredUpdateEntry{
		{DeferredUpdate: domain.DeferredUpdate{Host: "pc02", IP: "10.0.0.2", Revision: strings.Repeat("a", 40), QueuedAt: time.Now()}},
		{DeferredUpdate: domain.DeferredUpdate{Host: "pc03", IP: "10.0.0.3", Revision: strings.Repeat("b", 40), QueuedAt: time.Now()}, Stale: true},
	}}
	actions := DashboardActions{
		LoadDeferredUpdates: func(context.Context) domain.DeferredUpdateStatus { return status },
		CancelDeferredUpdates: func(hosts []string) error {
			removed = append(removed, hosts...)
			return nil
		},
	}
	model := newTestModel(t, withReadyLab(), withActions(actions))
	model.width, model.height = 100, 30
	found := false
	for _, task := range model.availableComputerTasks() {
		found = found || task.id == "queued"
	}
	if !found {
		t.Fatal("Queued updates is not in the Computers menu")
	}
	updated, command := model.openDeferredUpdates()
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	if !strings.Contains(view, "pc02") || !strings.Contains(view, "review the update again") || !strings.Contains(view, "Remove all") {
		t.Fatalf("queue view:\n%s", view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model = updated.(dashboardModel)
	if strings.Join(removed, ",") != "pc02" || command == nil {
		t.Fatalf("removed %v", removed)
	}
}

func queuedEntryModel(t testing.TB) dashboardModel {
	actions := DashboardActions{
		LoadDeferredUpdates: func(context.Context) domain.DeferredUpdateStatus {
			return domain.DeferredUpdateStatus{State: "ready"}
		},
	}
	model := newTestModel(t, withReadyLab(), withActions(actions))
	model.width, model.height = 100, 30
	model.screen = dashboardComputersArea
	return model
}

func TestComputersAreaEnterOpensQueuedUpdates(t *testing.T) {
	model := queuedEntryModel(t)
	for index, task := range model.availableComputerTasks() {
		if task.id == "queued" {
			model.computers.areaCursor = index
		}
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeferredUpdates || command == nil || model.navigation.areaReturn != dashboardComputersArea {
		t.Fatalf("Enter did not open Queued updates: screen=%d command=%v", model.screen, command != nil)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.(dashboardModel).screen != dashboardComputersArea {
		t.Fatalf("Esc did not return to Computers: screen=%d", updated.(dashboardModel).screen)
	}
}

func TestComputersAreaShortcutOpensQueuedUpdates(t *testing.T) {
	model := queuedEntryModel(t)
	updated, command := model.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeferredUpdates || command == nil {
		t.Fatalf("u did not open Queued updates: screen=%d command=%v", model.screen, command != nil)
	}
}

func TestComputersAreaIgnoresQueuedShortcutInClassroomMode(t *testing.T) {
	model := queuedEntryModel(t)
	model.actions.ClassroomMode = true
	updated, command := model.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if updated.(dashboardModel).screen != dashboardComputersArea || command != nil {
		t.Fatal("u must stay ignored in classroom mode")
	}
}

// Every entry an area lists must do something: its shortcut and Enter must
// leave the area or start a read.
func TestEveryListedAreaTaskIsHandled(t *testing.T) {
	noop := func(context.Context) domain.DeferredUpdateStatus { return domain.DeferredUpdateStatus{} }
	base := func() dashboardModel {
		model := experienceFixture(t, 2)
		model.actions.LoadDeferredUpdates = noop
		model.actions.PlanLock = func(context.Context, string, domain.LockAction) domain.LockPlan { return domain.LockPlan{} }
		model.actions.PlanShare = func(context.Context, string, string) domain.SharePlan { return domain.SharePlan{} }
		model.actions.OpenClassroomView = func(context.Context) (string, error) { return "", nil }
		model.actions.LoadRemoteInstall = func(context.Context) (domain.RemoteInstallResponse, error) {
			return domain.RemoteInstallResponse{}, nil
		}
		model.actions.LoadTemplateReset = func(context.Context) domain.TemplateResetCatalog { return domain.TemplateResetCatalog{} }
		model.actions.PlanTemplateReset = func(context.Context, string, func(string)) domain.TemplateResetPlan {
			return domain.TemplateResetPlan{}
		}
		model.actions.ApplyTemplateReset = func(domain.TemplateResetPlan) domain.TemplateResetResult {
			return domain.TemplateResetResult{}
		}
		model.actions.Telemetry = func(context.Context, string) (domain.TelemetryReport, error) {
			return domain.TelemetryReport{}, nil
		}
		model.width, model.height = 120, 40
		return model
	}
	areas := []struct {
		name   string
		screen dashboardScreen
		tasks  func(dashboardModel) []dashboardTask
	}{
		{"home", dashboardHome, func(m dashboardModel) []dashboardTask { return m.homeAreas() }},
		{"computers", dashboardComputersArea, func(m dashboardModel) []dashboardTask { return m.availableComputerTasks() }},
		{"installation", dashboardInstallationArea, func(dashboardModel) []dashboardTask { return installationAreaTasks }},
		{"maintenance", dashboardAdministration, func(dashboardModel) []dashboardTask { return administrationTasks }},
	}
	for _, area := range areas {
		for index, task := range area.tasks(base()) {
			for _, viaEnter := range []bool{false, true} {
				model := base()
				model.screen = area.screen
				model.homeMenu.list.Select(index)
				model.computers.areaCursor = index
				model.navigation.installationAreaCursor = index
				model.navigation.adminCursor = index
				key := tea.KeyPressMsg{Code: []rune(task.shortcut)[0], Text: task.shortcut}
				if viaEnter {
					key = tea.KeyPressMsg{Code: tea.KeyEnter}
				}
				updated, command := model.Update(key)
				if updated.(dashboardModel).screen == area.screen && command == nil {
					t.Errorf("%s: entry %q (%s, enter=%v) does nothing", area.name, task.title, task.shortcut, viaEnter)
				}
			}
		}
	}
}
