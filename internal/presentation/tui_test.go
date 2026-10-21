package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDashboardShutdownIncludesActiveSessionAndAcknowledgesUnknownSession(t *testing.T) {
	plans := 0
	applies := 0
	actions := DashboardActions{
		PlanShutdown: func(ctx context.Context, requested string, policy domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
			plans++
			if requested != "@lab" {
				t.Fatalf("shutdown requested = %q", requested)
			}
			eligible := 1
			secondEligible := false
			if policy == domain.ShutdownAcknowledgeUnknown {
				eligible = 2
				secondEligible = true
			}
			return domain.ShutdownPlanReport{
				State: "ready", Requested: requested, Policy: policy, Eligible: eligible,
				Targets: []domain.ShutdownTargetPlan{
					{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive, Eligible: true},
					{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnknown, Eligible: secondEligible},
				},
				ReviewToken: "sha256:abcdef0123456789", Confirmation: "SHUTDOWN",
			}
		},
		ApplyShutdown: func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
			applies++
			if plan.Policy != domain.ShutdownAcknowledgeUnknown || plan.Eligible != 2 {
				t.Fatalf("applied shutdown plan = %+v", plan)
			}
			return domain.ShutdownApplyReport{State: "completed", Accepted: 2, Targets: []domain.ShutdownTargetOutcome{{Name: "pc01", State: "accepted"}, {Name: "pc02", State: "accepted"}}, Message: "Requests accepted; physical state is not inferred."}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	model.width, model.height = 100, 30
	updated, _ := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	if model.screen != dashboardShutdown || strings.Contains(model.View().Content, "pc99") {
		t.Fatalf("shutdown selection includes controller or did not open:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardShutdownReview || !strings.Contains(model.View().Content, "In use · will shut down") || !strings.Contains(model.View().Content, "Session unknown · not sent") || !strings.Contains(demoANSI.ReplaceAllString(model.View().Content, ""), "Controller      excluded") || !strings.Contains(model.View().Content, "in use will be interrupted") {
		t.Fatalf("shutdown review is incomplete:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if plans != 2 || !strings.Contains(model.View().Content, "risk acknowledged") {
		t.Fatalf("unknown-session policy was not replanned:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "SHUTDOWN"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("exact shutdown confirmation did not dispatch")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applies != 1 || model.screen != dashboardShutdownResult || !strings.Contains(model.View().Content, "Shutdown requests accepted") || !strings.Contains(model.View().Content, "not evidence") {
		t.Fatalf("shutdown result is misleading:\n%s", model.View().Content)
	}
}

func TestClassroomModeExposesOnlyInventoryInternetAndPower(t *testing.T) {
	actions := DashboardActions{ClassroomMode: true}
	model := newDashboardModel(testDashboardReport("ready"), domain.SetupReport{}, actions, false)
	model.screen = dashboardComputersArea
	view := model.computersAreaView()
	for _, expected := range []string{"Computer inventory", "Power controls", "Internet access"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("classroom view omits %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "Distribute the prepared system") || strings.Contains(view, "Maintenance") {
		t.Fatalf("classroom view exposes administrative actions:\n%s", view)
	}
	updated, _ := model.updateKeyState(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if updated.(dashboardModel).screen != dashboardComputersArea {
		t.Fatal("hidden deployment shortcut opened from classroom mode")
	}
}
