package presentation

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDeploymentProgressNeverBlocksAndResultIsDelivered(t *testing.T) {
	events := make(chan tea.Msg, deploymentEventBuffer)
	started := make(chan struct{})
	finished := make(chan struct{})
	action := func(_ context.Context, _ domain.DeploymentPlanReport, progress func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
		close(started)
		// Far more than the buffer, with nobody reading.
		for range 100 {
			progress(domain.DeploymentProgress{Activity: "step"})
		}
		defer close(finished)
		return domain.DeploymentExecutionReport{Operation: "deploy", State: "applied"}
	}
	command := startDeployment(context.Background(), action, domain.DeploymentPlanReport{}, events)
	first := make(chan tea.Msg, 1)
	go func() { first <- command() }()
	select {
	case <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("deployment start never produced a first message")
	}
	<-started
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("deployment goroutine blocked on an unread event channel")
	}
	var last tea.Msg
	deadline := time.After(5 * time.Second)
	for last == nil || func() bool { _, ok := last.(dashboardDeploymentResultMsg); return !ok }() {
		select {
		case message, ok := <-events:
			if !ok {
				t.Fatal("channel closed before the result was delivered")
			}
			last = message
		case <-deadline:
			t.Fatal("result was lost")
		}
	}
	if last.(dashboardDeploymentResultMsg).report.State != "applied" {
		t.Fatalf("wrong result: %+v", last)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("message queued after the result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel not closed after the result")
	}
}
