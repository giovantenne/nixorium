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
	return domain.RecoveryReport{State: "attention", Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryResetPending, Title: step.Title, Next: step}}}
}

func TestSafeModeOffersReadOnlyTools(t *testing.T) {
	actions := DashboardActions{
		LoadRecovery:   func(context.Context) domain.RecoveryReport { return blockedRecovery() },
		LoadDoctor:     func(context.Context) (domain.DoctorReport, error) { return domain.DoctorReport{}, nil },
		PreviewSupport: func(context.Context) (domain.SupportSnapshot, error) { return domain.SupportSnapshot{}, nil },
	}
	model := dashboardModel{actions: actions, initialError: true, message: "unfinished deployment template reset", recovery: recoveryModel{report: blockedRecovery()}, width: 120, height: 30}
	view := model.View().Content
	for _, want := range []string{"Safe mode", "A reset of the configuration template was interrupted", "What blocks", "Diagnostics", "Support report", "Try again"} {
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
	if !strings.Contains(model.View().Content, "Next: Finish or undo the reset") {
		t.Fatalf("recovery view:\n%s", model.View().Content)
	}
}

func TestOverviewListsPersistentBlockers(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), recovery: recoveryModel{report: blockedRecovery()}, width: 120, height: 30}
	found := false
	for _, task := range model.pendingTasks() {
		found = found || task.id == "pending-reset-recovery"
	}
	if !found {
		t.Fatalf("tasks = %+v", model.pendingTasks())
	}
}

// Enter on a running update opens the log it is writing; r reads it again
// and Esc returns to Recovery.
func TestRecoveryOpensTheLogOfTheRunningUpdate(t *testing.T) {
	step, _ := domain.NextStepByCode("OP-BUSY")
	const id = "deploy-20261118T125119.118559664Z-141912.log"
	report := domain.RecoveryReport{State: "attention", Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryOperationBusy, Title: step.Title, Next: step, Log: id}}}
	reads := 0
	actions := DashboardActions{
		LoadRecovery: func(context.Context) domain.RecoveryReport { return report },
		LoadLog: func(_ context.Context, requested string) domain.OperationLogReport {
			if requested != id {
				t.Fatalf("read log %q", requested)
			}
			reads++
			return domain.OperationLogReport{State: "available", Log: &domain.OperationLogEntry{ID: id, State: "incomplete"}, Content: "[pc04] verified at the reviewed revision\n"}
		},
	}
	model := dashboardModel{actions: actions, screen: dashboardRecovery, recovery: recoveryModel{report: report}, width: 120, height: 30}
	if !strings.Contains(model.View().Content, "View its log") {
		t.Fatalf("recovery view:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("Enter read nothing")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if model.screen != dashboardLogDetail || reads != 1 || !strings.Contains(view, "verified at the reviewed revision") || !strings.Contains(view, "Read again") {
		t.Fatalf("screen %d reads %d:\n%s", model.screen, reads, view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("r read nothing")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if reads != 2 || model.screen != dashboardLogDetail {
		t.Fatalf("after r: screen %d reads %d", model.screen, reads)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardRecovery || command == nil || model.maintenance.logFromRecovery {
		t.Fatalf("Esc went to screen %d", model.screen)
	}
}

// s on the automatic update asks to confirm; Enter stops it and refreshes
// Recovery with what to do next; Esc keeps it running.
func TestRecoveryStopsTheAutomaticUpdateAfterConfirmation(t *testing.T) {
	step, _ := domain.NextStepByCode("OP-BUSY")
	report := domain.RecoveryReport{State: "attention", Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryOperationBusy, Title: step.Title, Next: step, Stoppable: true}}}
	stops := 0
	actions := DashboardActions{
		LoadRecovery:        func(context.Context) domain.RecoveryReport { return report },
		StopAutomaticUpdate: func(context.Context) error { stops++; return nil },
	}
	model := dashboardModel{actions: actions, screen: dashboardRecovery, recovery: recoveryModel{report: report}, width: 120, height: 30}
	if !strings.Contains(model.View().Content, "Stop the automatic update") {
		t.Fatalf("recovery view:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	model = updated.(dashboardModel)
	if !model.recovery.stopping || !strings.Contains(model.View().Content, "Stop the automatic update?") {
		t.Fatalf("no confirmation:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.recovery.stopping || stops != 0 {
		t.Fatalf("Esc stopped it: stopping %v stops %d", model.recovery.stopping, stops)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("Enter did nothing")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if stops != 1 || model.screen != dashboardRecovery || !strings.Contains(model.message, "paused") || command == nil {
		t.Fatalf("stops %d screen %d message %q", stops, model.screen, model.message)
	}

	other := dashboardModel{actions: actions, screen: dashboardRecovery, recovery: recoveryModel{report: domain.RecoveryReport{Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryOperationBusy, Title: step.Title, Next: step}}}}, width: 120, height: 30}
	updated, _ = other.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if updated.(dashboardModel).recovery.stopping {
		t.Fatal("offered to stop an operation started by a person")
	}
}

// A paused queue says so and p resumes it.
func TestQueuedUpdatesResumeAfterAStop(t *testing.T) {
	status := domain.DeferredUpdateStatus{State: "ready", Paused: true, Updates: []domain.DeferredUpdateEntry{{DeferredUpdate: domain.DeferredUpdate{Host: "pc06", IP: "10.22.9.6"}}}}
	resumed := 0
	actions := DashboardActions{
		LoadDeferredUpdates:    func(context.Context) domain.DeferredUpdateStatus { return status },
		ResumeAutomaticUpdates: func() error { resumed++; status.Paused = false; return nil },
	}
	model := dashboardModel{actions: actions, screen: dashboardDeferredUpdates, deferred: deferredModel{status: status, loaded: true}, width: 120, height: 30}
	view := model.View().Content
	if !strings.Contains(view, "Automatic updates are paused") || !strings.Contains(view, "Resume automatic updates") {
		t.Fatalf("queue view:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	model = updated.(dashboardModel)
	if resumed != 1 || command == nil || !strings.Contains(model.message, "resumed") {
		t.Fatalf("resumed %d message %q", resumed, model.message)
	}
}

// x in the review asks for CLOSE; a wrong word changes nothing, CLOSE closes
// the update without checking.
func TestInterruptedUpdateClosesWithoutCheckingAfterCLOSE(t *testing.T) {
	plan := domain.DeploymentRecoveryPlan{State: "blocked", Message: "1 computer(s) are still applying the update", Targets: []domain.DeploymentRecoveryTarget{{Name: "pc25", State: domain.RecoveryTargetActivating}}}
	closes := 0
	actions := DashboardActions{
		PlanDeploymentRecovery: func(context.Context, bool) domain.DeploymentRecoveryPlan { return plan },
		ApplyDeploymentRecovery: func(domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult {
			t.Fatal("reviewed apply on a blocked plan")
			return domain.DeploymentRecoveryResult{}
		},
		CloseDeploymentRecoveryUnchecked: func() domain.DeploymentRecoveryResult {
			closes++
			return domain.DeploymentRecoveryResult{State: "completed", Unchecked: true, Archive: "/var/lib/nixorium/coordination/recovered/deployment-x-unchecked.json", Targets: []string{"pc25"}}
		},
	}
	model := dashboardModel{actions: actions, screen: dashboardRecoveryReview, recovery: recoveryModel{review: recoveryReview{kind: "deploy", deployPlan: plan}}, width: 120, height: 30}
	if !strings.Contains(model.View().Content, "Close without checking") {
		t.Fatalf("review view:\n%s", model.View().Content)
	}
	typeText := func(text string) {
		for _, r := range text {
			updated, _ := model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			model = updated.(dashboardModel)
		}
	}
	typeText("x")
	if !model.recovery.review.closing || !strings.Contains(model.View().Content, "Type CLOSE") {
		t.Fatalf("no confirmation:\n%s", model.View().Content)
	}
	// Typing is text entry: q and ? are letters here, not quit and help.
	typeText("q?")
	if model.recovery.review.confirmation != "q?" || model.screen != dashboardRecoveryReview {
		t.Fatalf("typing q? gave %q on screen %d", model.recovery.review.confirmation, model.screen)
	}
	typeText("CLOSE!")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || closes != 0 || !strings.Contains(model.message, "did not match") {
		t.Fatalf("wrong word: closes %d message %q", closes, model.message)
	}
	typeText("CLOSE")
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("CLOSE did nothing")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if closes != 1 || !model.recovery.review.done || !strings.Contains(model.View().Content, "closed without checking") {
		t.Fatalf("closes %d:\n%s", closes, model.View().Content)
	}
}
