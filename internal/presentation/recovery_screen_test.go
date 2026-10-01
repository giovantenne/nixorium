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

func TestDeploymentRecoveryReviewNeedsTheWordAndAcknowledgement(t *testing.T) {
	acknowledged := []bool{}
	applied := 0
	actions := DashboardActions{
		PlanDeploymentRecovery: func(_ context.Context, acknowledge bool) domain.DeploymentRecoveryPlan {
			acknowledged = append(acknowledged, acknowledge)
			plan := domain.DeploymentRecoveryPlan{State: "blocked", Unreachable: 1, AcknowledgeUnreachable: acknowledge, Message: "1 computer(s) could not be checked.",
				Targets: []domain.DeploymentRecoveryTarget{{Name: "pc01", State: domain.RecoveryTargetReviewed, Detail: "finished on the reviewed revision"}, {Name: "pc02", State: domain.RecoveryTargetUnreachable, Detail: "off or not reachable"}}}
			if acknowledge {
				plan.State, plan.Confirmation, plan.ReviewToken, plan.Message = "ready", "RECOVERED", "sha256:x", "Every reachable computer has finished."
			} else {
				plan.Issues = []domain.ValidationIssue{{Field: "unreachable", Message: plan.Message}}
			}
			return plan
		},
		ApplyDeploymentRecovery: func(domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult {
			applied++
			return domain.DeploymentRecoveryResult{State: "completed", Message: "The interrupted update is recorded as reviewed and operations are unblocked."}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 120, height: 30}
	run := func(next tea.Model, command tea.Cmd) dashboardModel {
		model := next.(dashboardModel)
		if command != nil {
			updated, _ := model.Update(command())
			model = updated.(dashboardModel)
		}
		return model
	}
	model = run(model.openDeploymentRecovery())
	if !strings.Contains(model.View().Content, "Acknowledge unreachable") || strings.Contains(model.View().Content, "Type RECOVERED") {
		t.Fatalf("blocked review:\n%s", model.View().Content)
	}
	model = run(model.Update(tea.KeyPressMsg{Code: 'u', Text: "u"}))
	if len(acknowledged) != 2 || !acknowledged[1] || !strings.Contains(model.View().Content, "Type RECOVERED to continue") {
		t.Fatalf("acknowledged review (%v):\n%s", acknowledged, model.View().Content)
	}
	for _, key := range "RECOVER" {
		model = cleanupPress(t, model, tea.KeyPressMsg{Code: key, Text: string(key)})
	}
	model = cleanupPress(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if applied != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatal("an incomplete word applied the recovery")
	}
	for _, key := range "RECOVERED" {
		model = cleanupPress(t, model, tea.KeyPressMsg{Code: key, Text: string(key)})
	}
	model = run(model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if applied != 1 || !strings.Contains(model.View().Content, "operations are unblocked") {
		t.Fatalf("result (applied %d):\n%s", applied, model.View().Content)
	}
}
