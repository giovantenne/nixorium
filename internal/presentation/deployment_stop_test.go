package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDeploymentStopRequiresSeparateConfirmationAndPreservesRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := dashboardModel{screen: dashboardDeploy, busy: "Applying", width: 80, height: 24,
		deployment: deploymentModel{applying: true, cancel: cancel, started: time.Now()}}
	updated, _ := model.Update(keyPress("s"))
	model = updated.(dashboardModel)
	if !model.deployment.stopReview || !strings.Contains(model.View().Content, "STOP WAITING") {
		t.Fatal("stop review missing")
	}
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(dashboardModel)
	if ctx.Err() != nil || model.deployment.stopReview {
		t.Fatal("closing stop review cancelled the deployment")
	}
	updated, _ = model.Update(keyPress("s"))
	model = updated.(dashboardModel)
	updated, _ = model.Update(keyPress("enter"))
	model = updated.(dashboardModel)
	if ctx.Err() != nil {
		t.Fatal("unconfirmed stop cancelled the operation")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "STOP WAITING"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(keyPress("enter"))
	model = updated.(dashboardModel)
	if ctx.Err() == nil || !model.deployment.applying || !model.deployment.stopRequested || model.deployment.stopReview {
		t.Fatal("confirmed stop lost supervision before the result")
	}
	updated, _ = model.Update(dashboardDeploymentResultMsg{report: domain.DeploymentExecutionReport{
		Operation: "deploy-apply", State: "failed", RecoveryRequired: true,
	}})
	model = updated.(dashboardModel)
	if model.deployment.applying || strings.Contains(model.View().Content, "New review") || !strings.Contains(model.View().Content, "Recovery required") {
		t.Fatalf("unsafe recovery view:\n%s", model.View().Content)
	}
	updated, _ = model.Update(keyPress("r"))
	if !updated.(dashboardModel).deployment.result.RecoveryRequired {
		t.Fatal("retry shortcut bypassed recovery")
	}
}

func TestDeploymentDetailsShowCommandOutputWithoutLosingPhaseActivity(t *testing.T) {
	events := make(chan tea.Msg)
	model := dashboardModel{screen: dashboardDeploy, busy: "Applying", width: 80, height: 24, progressDetails: true,
		deployment: deploymentModel{applying: true, events: events, started: time.Now(), recent: []string{"Applying built configurations"}}}
	updated, _ := model.Update(dashboardDeploymentProgressMsg{progress: domain.DeploymentProgress{
		Phase: domain.DeploymentPhaseApply, Completed: 2, Total: 4, LastOutputAt: time.Now().Add(-90 * time.Second), Output: []string{"pc01: restarting example.service"},
	}})
	model = updated.(dashboardModel)
	view := model.View().Content
	for _, expected := range []string{"pc01: restarting example.service", "Last command output", "No recent output", "private log tail"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("missing %q:\n%s", expected, view)
		}
	}
	if len(model.deployment.recent) != 1 {
		t.Fatal("output-only update replaced phase history")
	}
}
