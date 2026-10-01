package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func blockedRecovery() domain.RecoveryReport {
	step, _ := domain.NextStepByCode("RESET-PENDING")
	return domain.RecoveryReport{State: "attention", Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryResetPending, Title: "A deployment template reset was interrupted", Next: step}}}
}

func TestSafeModeOffersReadOnlyTools(t *testing.T) {
	actions := DashboardActions{
		LoadRecovery:   func(context.Context) domain.RecoveryReport { return blockedRecovery() },
		LoadDoctor:     func(context.Context) (domain.DoctorReport, error) { return domain.DoctorReport{}, nil },
		PreviewSupport: func(context.Context) (domain.SupportSnapshot, error) { return domain.SupportSnapshot{}, nil },
	}
	model := dashboardModel{actions: actions, initialError: true, message: "unfinished deployment template reset", recovery: blockedRecovery(), width: 120, height: 30}
	view := model.View().Content
	for _, want := range []string{"Safe mode", "A deployment template reset was interrupted", "What blocks", "Diagnostics", "Support report", "Try again"} {
		if !strings.Contains(view, want) {
			t.Fatalf("safe mode omits %q:\n%s", want, view)
		}
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	model = updated.(dashboardModel)
	if model.screen != dashboardRecovery || command == nil {
		t.Fatalf("b opened screen %d", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "Next: Finish or undo the interrupted template reset") {
		t.Fatalf("recovery view:\n%s", model.View().Content)
	}
}

func TestOverviewListsPersistentBlockers(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), recovery: blockedRecovery(), width: 120, height: 30}
	found := false
	for _, task := range model.pendingTasks() {
		found = found || task.id == "pending-reset-recovery"
	}
	if !found {
		t.Fatalf("tasks = %+v", model.pendingTasks())
	}
}
