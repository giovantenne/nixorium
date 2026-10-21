package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type networkChangeFixture struct {
	status  domain.NetworkMigrationStatus
	plans   []string
	applied []domain.NetworkMigrationPlan
}

func (fixture *networkChangeFixture) actions() DashboardActions {
	return DashboardActions{
		LoadNetworkMigration: func(context.Context) domain.NetworkMigrationStatus { return fixture.status },
		PlanNetworkMigration: func(_ context.Context, action, network string, acknowledge bool) domain.NetworkMigrationPlan {
			fixture.plans = append(fixture.plans, strings.Join([]string{action, network, map[bool]string{true: "ack", false: "-"}[acknowledge]}, " "))
			plan := domain.NetworkMigrationPlan{Action: action, State: "ready", ReviewToken: "sha256:review"}
			switch action {
			case domain.NetworkMigrationStart:
				plan.From, plan.To, plan.ControllerFrom, plan.ControllerTo = "10.0.0.0/24", "10.1.0.0/24", "10.0.0.99", "10.1.0.99"
				plan.Computers = []domain.NetworkMigrationComputer{
					{Name: "pc01", From: "10.0.0.1", To: "10.1.0.1", State: domain.MigrationComputerReachable},
					{Name: "pc02", From: "10.0.0.2", To: "10.1.0.2", State: domain.MigrationComputerOff},
				}
				plan.Steps = []string{"Save 10.1.0.0/24 as the laboratory network.", "Apply to controller: it then answers on both networks.", "Update computers: each one moves."}
				if acknowledge {
					plan.Computers[1].State = domain.MigrationComputerAcknowledged
					plan.Acknowledged = []string{"pc02"}
				} else {
					plan.State, plan.ReviewToken = "blocked", ""
					plan.Issues = []domain.ValidationIssue{{Field: "computers", Message: "pc02 did not answer. Switch them on and check again, or continue without them."}}
				}
			case domain.NetworkMigrationUndo:
				plan.From, plan.To, plan.ControllerFrom, plan.ControllerTo = "10.1.0.0/24", "10.0.0.0/24", "10.1.0.99", "10.0.0.99"
				plan.Steps = []string{"Save 10.0.0.0/24 again.", "Update computers: each one returns.", "Apply to controller."}
			}
			return plan
		},
		ApplyNetworkMigration: func(plan domain.NetworkMigrationPlan) domain.NetworkMigrationResult {
			fixture.applied = append(fixture.applied, plan)
			return domain.NetworkMigrationResult{Operation: "network-migrate-" + plan.Action, Action: plan.Action, State: "saved", Revision: strings.Repeat("b", 40),
				Message: "The change to 10.1.0.0/24 is saved locally; no computer was changed yet.", Next: []string{"Apply to controller: it then answers on both networks."}}
		},
	}
}

func networkChangeModelFor(t testing.TB, fixture *networkChangeFixture) dashboardModel {
	model := newTestModel(t, withReadyLab(), withActions(fixture.actions()))
	model.width, model.height = 120, 60
	model.screen = dashboardAdministration
	return model
}

func typeText(model dashboardModel, text string) dashboardModel {
	for _, character := range text {
		updated, _ := model.Update(tea.KeyPressMsg{Code: character, Text: string(character)})
		model = updated.(dashboardModel)
	}
	return model
}

func TestNetworkChangeFormReviewsAndSavesTheStart(t *testing.T) {
	fixture := &networkChangeFixture{status: domain.NetworkMigrationStatus{State: "none", To: "10.0.0.0/24", Message: "No change of the laboratory network is in progress."}}
	model := networkChangeModelFor(t, fixture)
	next, command := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = runRead(next, command)
	view := plainView(model)
	if model.screen != dashboardNetworkChange || !contains(view, "Current laboratory network 10.0.0.0/24") || !contains(view, "New network (address/prefix): _") {
		t.Fatalf("form:\n%s", view)
	}

	// A network that does not start where its prefix starts is explained,
	// not reviewed.
	model = typeText(model, "10.1.0.5/24")
	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(dashboardModel)
	if len(fixture.plans) != 0 || !strings.Contains(model.message, "for /24 use 10.1.0.0") {
		t.Fatalf("plans = %v, message = %q", fixture.plans, model.message)
	}
	for range "5/24" {
		next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		model = next.(dashboardModel)
	}
	model = typeText(model, "0/24")

	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = runRead(next, command)
	view = plainView(model)
	if strings.Join(fixture.plans, ";") != "start 10.1.0.0/24 -" || !contains(view, "Start the change of laboratory network") || !contains(view, "What was found") ||
		!contains(view, "Laboratory network 10.0.0.0/24 → 10.1.0.0/24") || !contains(view, "pc02 10.0.0.2 → 10.1.0.2 ! off or not answering") ||
		!contains(view, "c Continue without them") || contains(view, "Enter Save") {
		t.Fatalf("blocked review:\n%s", view)
	}

	next, command = model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = runRead(next, command)
	view = plainView(model)
	if fixture.plans[1] != "start 10.1.0.0/24 ack" || !contains(view, "off; continues without it") || !contains(view, "What you can do") ||
		!contains(view, "2. Apply to controller: it then answers on both networks.") || !contains(view, "Enter Save") || !contains(view, "c Include them again") {
		t.Fatalf("acknowledged review:\n%s", view)
	}

	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = runRead(next, command)
	view = plainView(model)
	if len(fixture.applied) != 1 || fixture.applied[0].ReviewToken != "sha256:review" || !contains(view, "is saved locally") || !contains(view, "Enter Apply to this controller") {
		t.Fatalf("result:\n%s", view)
	}
}

func TestNetworkChangeInProgressShowsEveryComputerAndTheNextStep(t *testing.T) {
	fixture := &networkChangeFixture{status: domain.NetworkMigrationStatus{
		State: "in-progress", From: "10.0.0.0/24", To: "10.1.0.0/24", StartedAt: "2026-10-15T08:00:00Z", ControllerFrom: "10.0.0.99", ControllerTo: "10.1.0.99", ControllerReady: true,
		Computers: []domain.NetworkMigrationComputer{
			{Name: "pc01", From: "10.0.0.1", To: "10.1.0.1", State: domain.MigrationComputerMigrated},
			{Name: "pc02", From: "10.0.0.2", To: "10.1.0.2", State: domain.MigrationComputerPrevious, Detail: "update it so it moves to its new address"},
			{Name: "pc03", From: "10.0.0.3", To: "10.1.0.3", State: domain.MigrationComputerWaiting, Detail: "it moves when switched on"},
		},
		Migrated: 1, Previous: 1, Waiting: 1, Next: "Update computers (Computers → Update computers): 1 computer still needs the new configuration.",
	}}
	model := networkChangeModelFor(t, fixture)
	next, command := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = runRead(next, command)
	view := plainView(model)
	for _, text := range []string{"Network change in progress", "Laboratory network 10.0.0.0/24 → 10.1.0.0/24", "Controller 10.0.0.99 → 10.1.0.99 · ✓ answers on both networks",
		"1 moved · 1 queued · 1 on the previous network", "pc02 10.0.0.2 → 10.1.0.2 ! still on the previous network", "update it so it moves to its new address", "○ off; queued it moves when switched on",
		"Next: Update computers", "Enter Update computers", "u Undo"} {
		if !contains(view, text) {
			t.Fatalf("status lacks %q:\n%s", text, view)
		}
	}
	if contains(view, "f Finish") {
		t.Fatalf("finish offered before every computer moved:\n%s", view)
	}
	next, command = model.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	model = runRead(next, command)
	view = plainView(model)
	if fixture.plans[0] != "undo  -" || !contains(view, "Undo the change of laboratory network") || !contains(view, "2. Update computers: each one returns.") {
		t.Fatalf("undo review:\n%s", view)
	}
	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = runRead(next, command)
	if view := plainView(model); !contains(view, "Enter Update computers") || !contains(view, "c Apply to controller") {
		t.Fatalf("undo result:\n%s", view)
	}

	fixture.status.ControllerReady = false
	model = networkChangeModelFor(t, fixture)
	next, command = model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = runRead(next, command)
	if view := plainView(model); !contains(view, "not applied yet") || !contains(view, "Enter Apply to controller") {
		t.Fatalf("controller first:\n%s", view)
	}
	fixture.status.ControllerReady, fixture.status.CanFinish = true, true
	model = networkChangeModelFor(t, fixture)
	next, command = model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = runRead(next, command)
	if view := plainView(model); !contains(view, "Enter Finish the change") || !contains(view, "f Finish") {
		t.Fatalf("finish:\n%s", view)
	}
}

func TestOverviewShowsANetworkChangeInProgress(t *testing.T) {
	fixture := &networkChangeFixture{status: domain.NetworkMigrationStatus{State: "none", To: "10.1.0.0/24"}}
	model := networkChangeModelFor(t, fixture)
	model.screen = dashboardHome
	step, _ := domain.NextStepByCode("NETWORK-CHANGING")
	model.recovery = domain.RecoveryReport{Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryNetworkChanging, Title: step.Title, Detail: "From 10.0.0.0/24 to 10.1.0.0/24. Next: Apply to controller, so it answers on both networks.", Next: step}}}
	tasks := model.pendingTasks()
	if len(tasks) != 1 || tasks[0].id != "pending-network" || tasks[0].title != "The laboratory network is being changed" {
		t.Fatalf("tasks = %+v", tasks)
	}
	next, command := model.openPendingTask("pending-network")
	if model = runRead(next, command); model.screen != dashboardNetworkChange {
		t.Fatalf("screen = %v", model.screen)
	}
}
