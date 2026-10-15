package presentation

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func wakeDashboard(t *testing.T, plan domain.WakePlan, applied *int) dashboardModel {
	t.Helper()
	actions := DashboardActions{
		PlanShutdown: func(context.Context, string, domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
			t.Fatal("Switch on planned a shutdown")
			return domain.ShutdownPlanReport{}
		},
		PlanWake: func(_ context.Context, requested string) domain.WakePlan {
			if requested != "@lab" {
				t.Fatalf("requested = %q", requested)
			}
			return plan
		},
		ApplyWake: func(reviewed domain.WakePlan) domain.WakeReport {
			*applied++
			return domain.WakeReport{State: "partial", Sent: 1, AlreadyOn: 1, Targets: []domain.WakeOutcome{
				{Name: "pc01", State: domain.WakeOutcomeSent, Detail: "Packet sent."},
				{Name: "pc02", State: domain.WakeOutcomeOn, Detail: "Already on; nothing was sent."},
			}, Message: "Wake-on-LAN packets sent to 1 computer; 1 already on."}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	model.width, model.height = 110, 40
	return model
}

func wakePress(model dashboardModel, key tea.Msg) (dashboardModel, tea.Cmd) {
	updated, command := model.Update(key)
	return updated.(dashboardModel), command
}

func TestPowerControlsSwitchOnReviewsThenSendsPackets(t *testing.T) {
	plan := domain.WakePlan{State: "ready", Requested: "@lab", Message: "1 computer will receive a Wake-on-LAN packet; 1 already on.", Targets: []domain.WakeTarget{
		{HostMeta: domain.HostMeta{Name: "pc01", IP: "192.0.2.11"}, State: domain.WakeTargetSend},
		{HostMeta: domain.HostMeta{Name: "pc02", IP: "192.0.2.12"}, State: domain.WakeTargetOn, Detail: "Already on."},
	}}
	applied := 0
	model := wakeDashboard(t, plan, &applied)
	model, _ = wakePress(model, tea.KeyPressMsg{Text: "c"})
	model, _ = wakePress(model, tea.KeyPressMsg{Text: "x"})
	if model.screen != dashboardShutdown || !strings.Contains(model.View().Content, "Shut down / restart / switch on") {
		t.Fatalf("power controls do not offer Switch on:\n%s", model.View().Content)
	}
	model, _ = wakePress(model, tea.KeyPressMsg{Code: tea.KeyTab})
	model, _ = wakePress(model, tea.KeyPressMsg{Code: tea.KeyTab})
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	if !model.shutdown.wake || !strings.Contains(view, "Action  Switch on") || !strings.Contains(view, "Those that are off") {
		t.Fatalf("Switch on is not chosen:\n%s", view)
	}
	model, _ = wakePress(model, tea.KeyPressMsg{Text: "a"})
	model, command := wakePress(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = wakePress(model, command())
	view = demoANSI.ReplaceAllString(model.View().Content, "")
	for _, want := range []string{"Switch on 1 computer?", "pc01 · Off · receives the packet", "pc02 · Already on · skipped", "A packet is not proof that a computer starts", "Press Enter to send the packet to 1 computer."} {
		if model.screen != dashboardShutdownReview || !strings.Contains(view, want) {
			t.Fatalf("review lacks %q:\n%s", want, view)
		}
	}
	model, command = wakePress(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter did not send the reviewed packets")
	}
	model, _ = wakePress(model, command())
	view = demoANSI.ReplaceAllString(model.View().Content, "")
	if applied != 1 || model.screen != dashboardShutdownResult {
		t.Fatalf("result screen = %d, applied %d", model.screen, applied)
	}
	for _, want := range []string{"pc01       sent", "pc02       already on", "Sent  1    Already on  1", "does not prove that a computer started", "h Check computers"} {
		if !strings.Contains(view, want) {
			t.Fatalf("result lacks %q:\n%s", want, view)
		}
	}
	// A new review starts from the selection again, still on Switch on.
	model, _ = wakePress(model, tea.KeyPressMsg{Text: "n"})
	if model.screen != dashboardShutdown || !model.shutdown.wake || len(model.shutdown.wakeResult.Targets) != 0 {
		t.Fatalf("new review = %d, wake %t", model.screen, model.shutdown.wake)
	}
}

func TestPowerControlsSwitchOnWithNothingToSendDoesNotApply(t *testing.T) {
	plan := domain.WakePlan{State: "ready", Message: "No packet will be sent: 1 already on, 1 with no known address yet.", Targets: []domain.WakeTarget{
		{HostMeta: domain.HostMeta{Name: "pc01"}, State: domain.WakeTargetOn, Detail: "Already on."},
		{HostMeta: domain.HostMeta{Name: "pc02"}, State: domain.WakeTargetUnknown, Detail: domain.WakeUnknownDetail},
	}}
	applied := 0
	model := wakeDashboard(t, plan, &applied)
	model.screen = dashboardShutdown
	model.shutdown.wake = true
	model, _ = wakePress(model, tea.KeyPressMsg{Text: "a"})
	model, command := wakePress(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = wakePress(model, command())
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	for _, want := range []string{"No computer to switch on", "pc02 · Address not known yet · skipped", "it is learned while the computer is on and the classroom view is open", "No packet will be sent"} {
		if !strings.Contains(view, want) {
			t.Fatalf("review lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Enter Send packets") {
		t.Fatalf("a review without packets offers sending:\n%s", view)
	}
	model, command = wakePress(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil || applied != 0 || model.screen != dashboardShutdownReview {
		t.Fatalf("Enter applied an empty review: applied %d", applied)
	}
	model, _ = wakePress(model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.screen != dashboardShutdown || !strings.Contains(model.message, "no packet was sent") {
		t.Fatalf("Esc = %d, %q", model.screen, model.message)
	}
}

func TestPowerControlsWithoutSwitchOnKeepShutdownAndRestart(t *testing.T) {
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
	model.width, model.height = 110, 40
	model.screen = dashboardShutdown
	for range 3 {
		model, _ = wakePress(model, tea.KeyPressMsg{Code: tea.KeyTab})
		if model.shutdown.wake {
			t.Fatal("Switch on was offered without its actions")
		}
	}
	if strings.Contains(model.View().Content, "switch on") {
		t.Fatalf("the action bar offers Switch on:\n%s", model.View().Content)
	}
}

func TestWakeCommandTextHasOneLinePerComputer(t *testing.T) {
	var output bytes.Buffer
	WakeReportText(&output, domain.WakeReport{Targets: []domain.WakeOutcome{
		{Name: "pc01", State: domain.WakeOutcomeSent, Detail: "Packet sent."},
		{Name: "pc02", State: domain.WakeOutcomeOn, Detail: "Already on; nothing was sent."},
		{Name: "pc03", State: domain.WakeOutcomeUnknown, Detail: domain.WakeUnknownDetail},
		{Name: "pc04", State: domain.WakeOutcomeFailed, Detail: "The packet could not be sent: network is unreachable."},
	}, Message: "Wake-on-LAN packets sent to 1 computer."})
	want := "pc01  sent\npc02  already on\npc03  address unknown: it is learned while the computer is on and the classroom view is open or the computer inventory runs.\npc04  not sent: network is unreachable.\nWake-on-LAN packets sent to 1 computer.\n"
	if output.String() != want {
		t.Fatalf("report text:\n%s", output.String())
	}
	output.Reset()
	WakePlanText(&output, domain.WakePlan{State: "blocked", Message: "Nothing was sent: pc99 is not a configured client.", Targets: []domain.WakeTarget{
		{HostMeta: domain.HostMeta{Name: "pc99"}, State: domain.WakeTargetInvalid, Detail: "Not a client computer of this laboratory."},
	}})
	if output.String() != "pc99  invalid target: not a client computer of this laboratory.\nNothing was sent: pc99 is not a configured client.\n" {
		t.Fatalf("plan text:\n%s", output.String())
	}
}
