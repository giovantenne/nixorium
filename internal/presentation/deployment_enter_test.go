package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Update computers is confirmed with Enter on the review that shows the
// computers; no word is typed (ADR 0026, amended 2026-10-19). The reviewed
// revision and selector still travel with the plan that is applied.
func TestUpdateComputersReviewIsConfirmedWithEnter(t *testing.T) {
	plan := mixedDeploymentPlan()
	var applied []domain.DeploymentPlanReport
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		ApplyDeployment: func(_ context.Context, plan domain.DeploymentPlanReport, _ func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			applied = append(applied, plan)
			return domain.DeploymentExecutionReport{Operation: "deploy-apply", State: "completed"}
		},
	}, false)
	model.width, model.height = 100, 30
	model.screen = dashboardDeployReview
	model.deployment.plan = plan

	view := demoANSI.ReplaceAllString(model.View().Content, "")
	for _, want := range []string{"Update these computers?", "Builds   2 configurations for 2 computers", "Enter updates these computers.", "Enter", "Update computers", "Esc", "Selection"} {
		if !strings.Contains(view, want) {
			t.Fatalf("review lacks %q:\n%s", want, view)
		}
	}
	// Identical computers share one configuration (ADR 0033).
	shared := model
	shared.deployment.plan.Targets = []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1", Class: "shared-1"}, {Name: "pc02", IP: "10.0.0.2", Class: "shared-1"}}
	if view := demoANSI.ReplaceAllString(shared.View().Content, ""); !strings.Contains(view, "Builds   1 configuration for 2 computers") {
		t.Fatalf("review does not count shared configurations:\n%s", view)
	}
	if strings.Contains(view, "Type DEPLOY") || strings.Contains(view, "> _") {
		t.Fatalf("review still asks for a word:\n%s", view)
	}

	// Typed text neither starts the update nor is remembered.
	for _, key := range []tea.KeyPressMsg{{Text: "DEPLOY"}, {Code: tea.KeySpace}, {Text: "x"}} {
		updated, command := model.Update(key)
		model = updated.(dashboardModel)
		if command != nil || model.deployment.applying || len(applied) != 0 {
			t.Fatalf("key %q started the update", key.Text)
		}
	}
	// Esc cancels without building or applying anything.
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cancelled := updated.(dashboardModel)
	if command != nil || cancelled.screen != dashboardDeploy || cancelled.deployment.applying || len(applied) != 0 {
		t.Fatal("Esc did not cancel the review safely")
	}

	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.deployment.applying {
		t.Fatal("Enter did not confirm the review")
	}
	command()
	if len(applied) != 1 || applied[0].Revision != plan.Revision || applied[0].ColmenaSelector != plan.ColmenaSelector {
		t.Fatalf("the reviewed plan was not the one applied: %+v", applied)
	}
}
