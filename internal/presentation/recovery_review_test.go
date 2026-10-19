package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

const pendingRefusal = "controller build or activation failed: an unfinished client deployment blocks new operations; inspect deployment-pending.json and follow the deployment recovery guide"

func runRead(next tea.Model, command tea.Cmd) dashboardModel {
	model := next.(dashboardModel)
	if command != nil {
		updated, _ := model.Update(command())
		model = updated.(dashboardModel)
	}
	return model
}

func plainView(model dashboardModel) string {
	return ansi.Strip(model.View().Content)
}

// contains ignores wrapping and alignment.
func contains(view, want string) bool {
	normalize := func(text string) string { return strings.Join(strings.Fields(text), " ") }
	return strings.Contains(normalize(view), normalize(want))
}

// recoveryFixture plans pc01 finished and pc02 not answering, the way the
// application layer does.
type recoveryFixture struct {
	acknowledged []bool
	applied      int
	pxeMode      string
}

func (fixture *recoveryFixture) actions() DashboardActions {
	started := time.Date(2026, 10, 15, 15, 4, 0, 0, time.Local)
	return DashboardActions{
		PlanDeploymentRecovery: func(_ context.Context, acknowledge bool) domain.DeploymentRecoveryPlan {
			fixture.acknowledged = append(fixture.acknowledged, acknowledge)
			plan := domain.DeploymentRecoveryPlan{State: "blocked", Unreachable: 1, AcknowledgeUnreachable: acknowledge, PXEMode: fixture.pxeMode,
				Message: "1 computer(s) could not be checked. Check them at the console; then acknowledge them explicitly to continue.",
				Pending: domain.PendingDeployment{StartedAt: started, Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}}},
				Targets: []domain.DeploymentRecoveryTarget{
					{Name: "pc01", IP: "10.0.0.1", State: domain.RecoveryTargetReviewed, Detail: "finished on the reviewed revision"},
					{Name: "pc02", IP: "10.0.0.2", State: domain.RecoveryTargetUnreachable, Detail: "off or not reachable; check it at the console"},
				}}
			if acknowledge {
				plan.State, plan.Confirmation, plan.ReviewToken, plan.Message = "ready", "RECOVERED", "sha256:x", "Every reachable computer has finished."
			} else {
				plan.Issues = []domain.ValidationIssue{{Field: "unreachable", Message: plan.Message}}
			}
			return plan
		},
		ApplyDeploymentRecovery: func(domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult {
			fixture.applied++
			return domain.DeploymentRecoveryResult{State: "completed", Archive: "/var/lib/nixorium/coordination/recovered/deployment-1.json", Message: "The interrupted update is recorded as reviewed and operations are unblocked."}
		},
	}
}

func TestDeploymentRecoveryReviewConfirmsWithEnter(t *testing.T) {
	fixture := &recoveryFixture{}
	model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 160, height: 40}
	model = runRead(model.openDeploymentRecovery())
	view := plainView(model)
	for _, want := range []string{
		"Review the interrupted update",
		"The update of 2026-10-15 15:04 to 2 computer(s) stopped before Nixorium could see how it ended. Each computer was checked:",
		"pc01  10.0.0.1  finished, running the new version",
		"pc02  10.0.0.2  not answering (switched off, or not reachable at this address)",
		`connected with a second network card; see "Computer not reachable" in the troubleshooting guide.`,
		"1 computer(s) do not answer. Switch them on and press r, or press c to continue without them; you can update them later from Computers → Update computers.",
		"c Continue without them", "r Check again", "Esc Back",
	} {
		if !contains(view, want) {
			t.Fatalf("blocked review omits %q:\n%s", want, view)
		}
	}
	if contains(view, "RECOVERED") || contains(view, "Mark as reviewed") || contains(view, "network installation is still open") {
		t.Fatalf("blocked review offers confirmation:\n%s", view)
	}
	model = cleanupPress(t, model, keyPress("enter"))
	if fixture.applied != 0 {
		t.Fatal("Enter applied an incomplete review")
	}
	model = runRead(model.Update(keyPress("c")))
	view = plainView(model)
	if len(fixture.acknowledged) != 2 || !fixture.acknowledged[1] || !contains(view, "c Include them again") ||
		!contains(view, "Press Enter to mark this update as reviewed. Nothing changes on the computers; other operations are unblocked. Then run a new update so every computer is current.") ||
		!contains(view, "Enter Mark as reviewed") {
		t.Fatalf("continued review (%v):\n%s", fixture.acknowledged, view)
	}
	// u is the older key for the same toggle.
	model = runRead(model.Update(keyPress("u")))
	if len(fixture.acknowledged) != 3 || fixture.acknowledged[2] || !contains(plainView(model), "c Continue without them") {
		t.Fatalf("u did not toggle back (%v)", fixture.acknowledged)
	}
	model = runRead(model.Update(keyPress("u")))
	// Esc never changes anything.
	left := cleanupPress(t, model, keyPress("esc"))
	if fixture.applied != 0 || left.screen != dashboardRecovery || !strings.Contains(left.message, "Nothing was changed") {
		t.Fatalf("Esc changed something: applied %d, screen %d", fixture.applied, left.screen)
	}
	model = runRead(model.Update(keyPress("enter")))
	view = plainView(model)
	if fixture.applied != 1 || !contains(view, "The interrupted update is marked as reviewed.") ||
		!contains(view, "Other operations are unblocked. Use Computers → Update computers to bring every computer to the current version.") ||
		!contains(view, "Enter Overview") {
		t.Fatalf("result (applied %d):\n%s", fixture.applied, view)
	}
}

func TestDeploymentRecoveryReviewNamesTheUnfinishedUSBInstallation(t *testing.T) {
	fixture := &recoveryFixture{}
	actions := fixture.actions()
	actions.RemoteReservationPresent = func() bool { return true }
	actions.LoadRemoteInstall = func(context.Context) (domain.RemoteInstallResponse, error) {
		return domain.RemoteInstallResponse{State: "reconciliation-required", OperationID: "op-1", Session: &domain.RemoteInstallSession{
			OperationID: "op-1", Preparation: &domain.RemoteInstallPreparation{Host: domain.RemoteInstallHost{Name: "pc02"}},
		}}, nil
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 200, height: 40}
	model = runRead(model.openDeploymentRecovery())
	view := plainView(model)
	for _, want := range []string{
		"pc02  10.0.0.2  not answering: its USB installation was not finished, so it was never reachable",
		"you can update them later from Computers → Update computers. Close or resume that installation from Installation → USB over SSH after this review.",
	} {
		if !contains(view, want) {
			t.Fatalf("review omits %q:\n%s", want, view)
		}
	}
	if contains(view, "pc01  10.0.0.1  not answering") {
		t.Fatalf("the answering computer was blamed:\n%s", view)
	}
	// Without the local reservation the worker is not asked.
	actions.RemoteReservationPresent = func() bool { return false }
	actions.LoadRemoteInstall = func(context.Context) (domain.RemoteInstallResponse, error) {
		t.Fatal("worker asked without a reservation")
		return domain.RemoteInstallResponse{}, nil
	}
	model = dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 200, height: 40}
	model = runRead(model.openDeploymentRecovery())
	if contains(plainView(model), "USB installation was not finished") {
		t.Fatal("USB note without a reservation")
	}
}

func TestDeploymentRecoveryReviewNamesAnOpenNetworkInstallation(t *testing.T) {
	fixture := &recoveryFixture{pxeMode: "recovery-required"}
	model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 260, height: 40}
	model = runRead(model.openDeploymentRecovery())
	if want := "A network installation is still open on this controller. A computer installed through it may not have finished; finish or stop the session from Installation → Network boot (PXE) after this review, then install it again."; !contains(plainView(model), want) {
		t.Fatalf("review omits the network installation note:\n%s", plainView(model))
	}
}

func TestDeploymentRecoveryReviewMessages(t *testing.T) {
	started := time.Date(2026, 10, 15, 15, 4, 0, 0, time.Local)
	pending := domain.PendingDeployment{StartedAt: started, ControllerPID: 99, Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}}}
	for name, test := range map[string]struct {
		plan domain.DeploymentRecoveryPlan
		want string
	}{
		"updating": {domain.DeploymentRecoveryPlan{State: "blocked", Pending: pending, Targets: []domain.DeploymentRecoveryTarget{{Name: "pc01", IP: "10.0.0.1", State: domain.RecoveryTargetActivating}}, Issues: []domain.ValidationIssue{{Field: "targets", Message: "1 computer(s) are still applying"}}},
			"pc01  10.0.0.1  still updating"},
		"updating message": {domain.DeploymentRecoveryPlan{State: "blocked", Pending: pending, Targets: []domain.DeploymentRecoveryTarget{{Name: "pc01", IP: "10.0.0.1", State: domain.RecoveryTargetActivating}}, Issues: []domain.ValidationIssue{{Field: "targets", Message: "1 computer(s) are still applying"}}},
			"1 computer(s) are still updating. Wait a minute, then press r to check again."},
		"other version": {domain.DeploymentRecoveryPlan{State: "ready", Pending: pending, ReviewToken: "sha256:x", Targets: []domain.DeploymentRecoveryTarget{{Name: "pc01", IP: "10.0.0.1", State: domain.RecoveryTargetOther}}},
			"pc01  10.0.0.1  finished, but running another version"},
		"process": {domain.DeploymentRecoveryPlan{State: "blocked", Pending: pending, Issues: []domain.ValidationIssue{{Field: "process", Message: "the Nixorium process that started the update (PID 99) is still running"}}},
			"Nixorium is still open in another window or terminal (started 15:04). Close it, then press r."},
		"record": {domain.DeploymentRecoveryPlan{State: "blocked", Issues: []domain.ValidationIssue{{Field: "record", Message: "not private"}}},
			"The record of the interrupted update cannot be read. Keep it as it is and collect a support report from Maintenance → Diagnostics."},
	} {
		plan := test.plan
		actions := DashboardActions{PlanDeploymentRecovery: func(context.Context, bool) domain.DeploymentRecoveryPlan { return plan }}
		model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 200, height: 40}
		model = runRead(model.openDeploymentRecovery())
		view := plainView(model)
		if !contains(view, test.want) || contains(view, "PID") {
			t.Fatalf("%s: review omits %q:\n%s", name, test.want, view)
		}
	}
}

func TestUpdateResultNoticeOpensTheInterruptedUpdateReview(t *testing.T) {
	fixture := &recoveryFixture{}
	actions := fixture.actions()
	actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport { return domain.ControllerRebuildPlanReport{} }
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 160, height: 40, screen: dashboardUpdate,
		updates: updateModel{result: domain.UpdateApplyReport{Operation: "update-apply", State: "saved", Updated: true, Target: "v3.7.0"}},
		message: pendingRefusal}
	view := plainView(model)
	for _, want := range []string{
		"! The controller was not updated: a previous update of the student computers was interrupted.",
		"  Nixorium does not know how it ended on every computer, so maintenance waits until you review it; classroom controls leave only those computers alone.",
		"  Next: Review the interrupted update (Enter)",
		"Enter Review the interrupted update", "a Retry controller", "d Details", "Esc Maintenance",
	} {
		if !contains(view, want) {
			t.Fatalf("update result omits %q:\n%s", want, view)
		}
	}
	if contains(view, "deployment-pending.json") || contains(view, "Enter Retry controller") {
		t.Fatalf("raw text or old Enter shown by default:\n%s", view)
	}
	model = cleanupPress(t, model, keyPress("d"))
	if view := plainView(model); !contains(view, "Details: "+pendingRefusal) {
		t.Fatalf("d did not reveal the raw text:\n%s", view)
	}
	model = cleanupPress(t, model, keyPress("d"))
	if contains(plainView(model), "deployment-pending.json") {
		t.Fatal("d did not hide the raw text again")
	}
	review := runRead(model.Update(keyPress("enter")))
	if review.screen != dashboardRecoveryReview || len(fixture.acknowledged) != 1 || !contains(plainView(review), "Review the interrupted update") {
		t.Fatalf("Enter did not open the review (screen %d):\n%s", review.screen, plainView(review))
	}
	back := cleanupPress(t, review, keyPress("esc"))
	if back.screen != dashboardUpdate || back.message != pendingRefusal || fixture.applied != 0 {
		t.Fatalf("Esc did not return to the update result: screen %d, message %q", back.screen, back.message)
	}
}

func TestDeploymentApplyNeedingRecoveryLandsOnTheReview(t *testing.T) {
	fixture := &recoveryFixture{}
	model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 160, height: 40, screen: dashboardDeploy, busy: "Applying",
		deployment: deploymentModel{applying: true, started: time.Now()}}
	message := "remote activation completion is unconfirmed; 1/2 targets expose the reviewed revision, but this is not completion evidence. New operations are blocked by the pending deployment marker; review the private log and deployment recovery guide: colmena apply: exit status 1"
	model = runRead(model.Update(dashboardDeploymentResultMsg{report: domain.DeploymentExecutionReport{
		Operation: "deploy-apply", State: "failed", Phase: domain.DeploymentPhaseApply, RecoveryRequired: true, Message: message,
	}}))
	view := plainView(model)
	for _, want := range []string{
		"! The computers were not updated: the update was interrupted.",
		"Nixorium does not know how it ended on every computer",
		"Review the interrupted update", "Each computer was checked:", "d Details",
	} {
		if !contains(view, want) {
			t.Fatalf("review after the failed apply omits %q:\n%s", want, view)
		}
	}
	if model.screen != dashboardRecoveryReview || contains(view, "colmena apply") {
		t.Fatalf("screen %d or raw text shown:\n%s", model.screen, view)
	}
	model = cleanupPress(t, model, keyPress("d"))
	if !contains(plainView(model), "colmena apply: exit status 1") {
		t.Fatal("d did not reveal the apply failure")
	}
	back := cleanupPress(t, model, keyPress("esc"))
	if back.screen != dashboardDeploy || !back.deployment.result.RecoveryRequired || fixture.applied != 0 {
		t.Fatalf("Esc did not return to the result: screen %d", back.screen)
	}
	if view := plainView(back); !contains(view, "Enter Review the interrupted update") || contains(view, "New review") {
		t.Fatalf("result screen does not offer the review:\n%s", view)
	}
}

func TestControllerRefusalOffersTheInterruptedUpdateReview(t *testing.T) {
	fixture := &recoveryFixture{}
	model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 160, height: 40, screen: dashboardController, message: pendingRefusal}
	view := plainView(model)
	if !contains(view, "! The controller cannot be updated now: a previous update of the student computers was interrupted.") || !contains(view, "Enter Review the interrupted update") || contains(view, "deployment-pending.json") {
		t.Fatalf("controller refusal:\n%s", view)
	}
	review := runRead(model.Update(keyPress("enter")))
	if review.screen != dashboardRecoveryReview {
		t.Fatalf("Enter opened screen %d", review.screen)
	}
	if back := cleanupPress(t, review, keyPress("esc")); back.screen != dashboardController || back.message != pendingRefusal {
		t.Fatalf("Esc returned to screen %d", back.screen)
	}
}

// Controller activation is allowed past an interrupted client update (the
// way to the fix is never blocked); its result must still show the record.
func TestUpdateResultAfterActivationPastInterruptedUpdateKeepsItsReview(t *testing.T) {
	fixture := &recoveryFixture{}
	revision := "0123456789abcdef0123456789abcdef01234567"
	started := time.Date(2026, 10, 15, 15, 4, 0, 0, time.Local)
	held := domain.RecoveryReport{Conditions: []domain.BlockingCondition{
		{Kind: domain.RecoveryDeploymentPending, Title: situationTitle("DEPLOY-PENDING"), Detail: "Update of 2026-10-15 15:04 to pc01; how it ended is unknown.", Since: &started},
	}}
	model := dashboardModel{report: testDashboardReport("ready"), actions: fixture.actions(), width: 160, height: 44, screen: dashboardUpdate,
		updates: updateModel{applying: true, result: domain.UpdateApplyReport{Operation: "update-save", State: "saved", Updated: true, Revision: revision, Target: "v3.7.0"}}}
	updated, _ := model.Update(dashboardUpdateControllerMsg{
		plan:     domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "ready", Revision: revision},
		report:   domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Revision: revision, Applied: true, Verified: true},
		recovery: &held,
	})
	model = updated.(dashboardModel)
	view := plainView(model)
	for _, want := range []string{
		"Nixorium and this controller are updated",
		heldDeploymentLine,
		"! A previous update of the student computers was interrupted.",
		"  Next: Review the interrupted update (Enter)",
		"Enter Review the interrupted update",
	} {
		if !contains(view, want) {
			t.Fatalf("update result omits %q:\n%s", want, view)
		}
	}
	if contains(view, "Enter Update computers") {
		t.Fatalf("update result offers updating the computers past the record:\n%s", view)
	}
	review := runRead(model.Update(keyPress("enter")))
	if review.screen != dashboardRecoveryReview || fixture.applied != 0 {
		t.Fatalf("Enter did not open the review (screen %d):\n%s", review.screen, plainView(review))
	}
	home := cleanupPress(t, cleanupPress(t, review, keyPress("esc")), keyPress("esc"))
	found := false
	for _, task := range home.pendingTasks() {
		found = found || task.id == "pending-deploy-recovery"
	}
	if home.screen != dashboardHome || !found {
		t.Fatalf("the Overview lost the interrupted update (screen %d): %+v", home.screen, home.pendingTasks())
	}
}

func TestControllerResultPastUSBReservationShowsItWithoutTheClientLine(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	held := domain.RecoveryReport{Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryUSBReserved, Title: situationTitle("USB-RESERVED")}}}
	model := dashboardModel{report: testDashboardReport("ready"), width: 160, height: 40, screen: dashboardController, controller: controllerModel{applying: true}}
	updated, _ := model.Update(dashboardControllerResultMsg{
		report:   domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Revision: revision, Applied: true, Verified: true},
		recovery: &held,
	})
	model = updated.(dashboardModel)
	view := plainView(model)
	if !contains(view, "Applied to this controller and verified") || !contains(view, "! A USB installation was not finished.") ||
		!contains(view, "Next: Resume or close the USB installation") || contains(view, heldDeploymentLine) {
		t.Fatalf("controller result does not show the USB reservation:\n%s", view)
	}
	// Without a record the result stays as before.
	model.recovery = domain.RecoveryReport{}
	if view := plainView(model); contains(view, "USB installation was not finished") || contains(view, heldDeploymentLine) {
		t.Fatalf("controller result shows a record that is not there:\n%s", view)
	}
}
