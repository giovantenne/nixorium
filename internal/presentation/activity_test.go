package presentation

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestReadActivityCancelsContextAndIgnoresLateReplies(t *testing.T) {
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{}, false)
	model.screen, model.busy = dashboardServices, "Reading services"
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	defer close(release)
	opened, command := model.startRead(func(ctx context.Context) tea.Msg {
		started <- ctx
		<-release // Deliberately non-cooperative, to test UI cancellation too.
		return dashboardServicesMsg{}
	})
	model = opened.(dashboardModel)
	id := model.read.id
	results := make(chan tea.Msg, 1)
	go func() { results <- command() }()
	ctx := <-started
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > dashboardReadTimeout {
		t.Fatal("read has no bounded context")
	}
	updated, next := model.Update(keyPress("esc"))
	model = updated.(dashboardModel)
	if next != nil || ctx.Err() != context.Canceled || model.busy != "" || !strings.Contains(model.message, "Loading cancelled") {
		t.Fatalf("read was not cancelled: %+v", model.read)
	}
	select {
	case result := <-results:
		updated, _ = model.Update(result)
	case <-time.After(time.Second):
		t.Fatal("cancelled callback blocked the UI")
	}
	model = updated.(dashboardModel)
	model.screen = dashboardHome
	updated, next = model.Update(activityResultMsg{id: id, message: dashboardControllerPlanMsg{report: domain.ControllerRebuildPlanReport{State: "ready"}}})
	if next != nil || updated.(dashboardModel).screen != dashboardHome || updated.(dashboardModel).busy != "" {
		t.Fatal("late read resumed a cancelled workflow")
	}
}

func TestReadTimeoutDoesNotPublishASuccessOrStartFollowUp(t *testing.T) {
	model := dashboardModel{screen: dashboardSettingsEdit, busy: "Validating settings"}
	opened, command := model.startBoundedRead(-time.Second, func(context.Context) tea.Msg {
		return dashboardSettingsPlanMsg{report: domain.ConfigPlanReport{State: "ready"}}
	})
	updated, followUp := opened.(dashboardModel).Update(command())
	model = updated.(dashboardModel)
	if followUp != nil || model.screen != dashboardSettingsEdit || model.busy != "" || !strings.Contains(model.message, "took too long") || !strings.Contains(model.message, "Reopen") {
		t.Fatalf("timeout did not fail closed: screen=%d message=%s", model.screen, model.message)
	}
}

func TestReadActivityReturnsToOriginAndShowsUnavailableKeys(t *testing.T) {
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{
		LoadServices: func(context.Context) domain.ServicesReport { return domain.ServicesReport{} },
	}, false)
	model.screen = dashboardAdministration
	model.width, model.height = 80, 24
	updated, command := model.Update(keyPress("s"))
	model = updated.(dashboardModel)
	if command == nil || model.read.cancel == nil {
		t.Fatal("services did not use a cancellable read")
	}
	updated, next := model.Update(keyPress("x"))
	model = updated.(dashboardModel)
	view := model.View().Content
	for _, want := range []string{"Elapsed:", "Read-only", "Esc", "Loading information"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	if next != nil {
		t.Fatal("unavailable key started work")
	}
	updated, _ = model.Update(keyPress("esc"))
	if updated.(dashboardModel).screen != dashboardAdministration {
		t.Fatal("cancel did not return to the originating screen")
	}
}

func TestAllBusyScreensExplainElapsedTimeAndSafety(t *testing.T) {
	for screen := dashboardHome; screen <= dashboardHostTrust; screen++ {
		for _, reading := range []bool{false, true} {
			model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{}, false)
			model.screen, model.busy = screen, "Working"
			model.width, model.height = 80, 24
			safety := "Cannot be interrupted"
			if reading {
				model.beginRead(dashboardReadTimeout)
				safety = "Esc cancels"
			}
			view := model.View().Content
			model.read.cancelRead()
			if !strings.Contains(view, "Working") || !strings.Contains(view, "Elapsed:") || !strings.Contains(view, safety) {
				t.Errorf("screen %d (read %t) omits activity safety: %s", screen, reading, view)
			}
			if lipgloss.Width(view) > 80 || lipgloss.Height(view) > 24 {
				t.Errorf("screen %d (read %t) exceeds 80x24", screen, reading)
			}
		}
	}
}

func TestDashboardReadCallbacksAcceptContexts(t *testing.T) {
	actions := reflect.TypeFor[DashboardActions]()
	for i := range actions.NumField() {
		field := actions.Field(i)
		if field.Type.Kind() != reflect.Func || field.Name == "LoadControllerProgress" || field.Name == "LoadPXEProgress" {
			continue // Bounded local record reads, not cancellable workflows.
		}
		read := field.Name == "Refresh"
		for _, prefix := range []string{"Load", "Plan", "Check", "Preview", "Search", "Observe"} {
			read = read || strings.HasPrefix(field.Name, prefix)
		}
		if read && (field.Type.NumIn() == 0 || field.Type.In(0) != reflect.TypeFor[context.Context]()) {
			t.Errorf("%s cannot receive the read's cancellation and deadline", field.Name)
		}
	}
}

func TestUnclassifiedMutationCannotQuitAndBackgroundPreparationCan(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c", "esc"} {
		model := dashboardModel{screen: dashboardSetupKeys, busy: "Creating controller keys"}
		updated, command := model.Update(keyPress(key))
		if command != nil || updated.(dashboardModel).busy == "" || updated.(dashboardModel).message == "" {
			t.Fatalf("%s interrupted a mutation or was silently ignored", key)
		}
	}
	model := dashboardModel{screen: dashboardPXE, busy: "Preparing", installation: installationModel{pxePreparing: true}}
	_, quit := model.Update(keyPress("q"))
	if quit == nil || !strings.Contains(model.View().Content, "continues after quit") {
		t.Fatal("background work lost its existing safe quit")
	}
}

func TestCancelledSettingsValidationPreservesEditableDraft(t *testing.T) {
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{}, false)
	model.screen, model.busy = dashboardSettingsEdit, "Validating settings"
	model.settings.editor = newSettingsEditorModel(wizardSettings(), routineSettingsGroups[0].fields, "Edit settings")
	model.settings.editor.accepted = true
	draft := model.settings.editor.settings
	input := append([]string(nil), model.settings.editor.drafts...)
	opened, _ := model.startRead(func(context.Context) tea.Msg { return dashboardSettingsPlanMsg{} })
	updated, _ := opened.(dashboardModel).Update(tea.PasteMsg{Content: "unexpected input"})
	if !reflect.DeepEqual(updated.(dashboardModel).settings.editor.drafts, input) || updated.(dashboardModel).message == "" {
		t.Fatal("paste changed a draft while its validation was running")
	}
	updated, command := updated.(dashboardModel).Update(keyPress("esc"))
	model = updated.(dashboardModel)
	if command != nil || model.settings.editor.accepted || model.screen != dashboardSettingsEdit {
		t.Fatal("cancelled validation left the editor in its accepted state")
	}
	if !reflect.DeepEqual(model.settings.editor.settings, draft) {
		t.Fatal("cancelling validation discarded the edited network")
	}
	updated, command = model.Update(keyPress("backspace"))
	if command != nil || updated.(dashboardModel).busy != "" {
		t.Fatal("editing the cancelled draft automatically restarted validation")
	}
}

func TestCancelledUpdateBuildStopsProgressAndIgnoresLateResults(t *testing.T) {
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{}, false)
	model.screen, model.busy = dashboardUpdate, "Validating candidate"
	started, finished := make(chan context.Context, 1), make(chan struct{})
	opened, command := model.startUpdatePlan(func(ctx context.Context, _ string, _, _ bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
		defer close(finished)
		started <- ctx
		progress(domain.UpdatePlanProgress{})
		<-ctx.Done()
		// More than the event buffer: publishing after cancellation cannot
		// strand the producer when the UI no longer consumes its messages.
		for range 20 {
			progress(domain.UpdatePlanProgress{})
		}
		return domain.UpdatePlanReport{State: "ready"}
	}, "master", false, false)
	model = opened.(dashboardModel)
	id := model.read.id
	first := command()
	ctx := <-started
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) < 59*time.Minute || time.Until(deadline) > dashboardBuildTimeout {
		t.Fatal("build does not have its separate generous timeout")
	}
	updated, waiter := model.Update(first)
	model = updated.(dashboardModel)
	if model.busy == "" {
		t.Fatal("empty progress hid the running activity")
	}
	if waiter == nil {
		t.Fatal("progress did not retain its event waiter")
	}
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(dashboardModel)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cancelled progress producer is blocked")
	}
	updated, next := model.Update(waiter())
	model = updated.(dashboardModel)
	if next != nil || model.updates.planning || model.read.cancel != nil {
		t.Fatal("late progress revived the activity")
	}
	updated, next = model.Update(activityResultMsg{id: id, message: dashboardUpdatePlanMsg{report: domain.UpdatePlanReport{State: "ready"}}})
	if next != nil || updated.(dashboardModel).screen != dashboardUpdate || updated.(dashboardModel).updates.plan.State != "" {
		t.Fatal("late build result opened an applicable review")
	}
}
