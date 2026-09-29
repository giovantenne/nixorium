package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestClientTasksRequireInventoryOnlyWhenOpened(t *testing.T) {
	for _, action := range []string{"d", "x", "i", "h"} {
		t.Run(action, func(t *testing.T) {
			m := experienceFixture(2)
			observed := m.report
			m.report.Meta = domain.LabMeta{}
			calls := 0
			m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) { calls++; return observed, nil }
			m = press(m, "a")
			m = press(m, "esc")
			m = press(m, "c")
			if calls != 0 || m.busy != "" {
				t.Fatal("ordinary navigation loaded inventory")
			}
			next, command := m.Update(tea.KeyPressMsg{Text: action})
			loading := next.(dashboardModel)
			if command == nil || loading.inventory.cancel == nil || calls != 0 || loading.screen != dashboardComputersArea {
				t.Fatal("task bypassed inventory read")
			}
			view := loading.View().Content
			if !strings.Contains(view, "Cancel") || strings.Contains(view, "No client") {
				t.Fatal("loading presented an empty inventory")
			}
			next, _ = loading.Update(command())
			ready := next.(dashboardModel)
			if calls != 1 || ready.inventory.cancel != nil || ready.busy != "" && action != "h" || len(ready.report.Meta.Clients.Hosts) != 2 {
				t.Fatal("evaluated inventory not installed before opening task")
			}
			expected := map[string]dashboardScreen{"d": dashboardDeploy, "x": dashboardShutdown, "i": dashboardInternet, "h": dashboardHosts}[action]
			if ready.screen != expected {
				t.Fatalf("wrong destination: %v", ready.screen)
			}
		})
	}
}

func TestInventoryCancellationRejectsLateAndSupersededReads(t *testing.T) {
	m := experienceFixture(2)
	observed := m.report
	m.report.Meta = domain.LabMeta{}
	m.screen = dashboardComputersArea
	var contexts []context.Context
	m.actions.LoadInventory = func(ctx context.Context) (domain.StatusReport, error) {
		contexts = append(contexts, ctx)
		return observed, nil
	}
	next, oldCommand := m.openComputerTask("d")
	loading := next.(dashboardModel)
	// Capture a response but delay delivery until after cancellation and a new read.
	oldMessage := oldCommand()
	next, _ = loading.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	cancelled := next.(dashboardModel)
	if contexts[0].Err() != context.Canceled || cancelled.busy != "" || cancelled.screen != dashboardComputersArea {
		t.Fatal("Esc did not cancel inventory read")
	}
	next, newCommand := cancelled.openComputerTask("i")
	retry := next.(dashboardModel)
	next, _ = retry.Update(oldMessage)
	retry = next.(dashboardModel)
	if retry.inventory.cancel == nil || retry.report.Meta.Controller.Name != "" {
		t.Fatal("late inventory resumed a cancelled action")
	}
	next, _ = retry.Update(newCommand())
	if next.(dashboardModel).screen != dashboardInternet {
		t.Fatal("fresh read did not resume the requested action")
	}
}

func TestInventoryFailureDoesNotOpenClientSelection(t *testing.T) {
	for _, result := range []struct {
		report domain.StatusReport
		err    error
	}{
		{err: errors.New("invalid workspace")}, {},
	} {
		m := experienceFixture(2)
		m.report.Meta = domain.LabMeta{}
		m.screen = dashboardComputersArea
		m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) { return result.report, result.err }
		next, command := m.openComputerTask("d")
		next, _ = next.(dashboardModel).Update(command())
		failed := next.(dashboardModel)
		if failed.screen != dashboardComputersArea || failed.busy != "" || failed.message == "" || failed.inventory.cancel != nil {
			t.Fatal("failed inventory permitted selection or blocked retry")
		}
	}
}

func TestSoftwareFollowupWaitsForInventoryAndPreservesExactTargets(t *testing.T) {
	m := experienceFixture(2)
	observed := m.report
	m.report.Meta = domain.LabMeta{}
	m.screen = dashboardSoftware
	m.software.result.AffectedClients = []string{"pc02", "pc03"}
	m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) { return observed, nil }
	next, command := m.openSoftwareDeployment()
	if command == nil || next.(dashboardModel).screen != dashboardSoftware {
		t.Fatal("software followup skipped inventory")
	}
	next, _ = next.(dashboardModel).Update(command())
	ready := next.(dashboardModel)
	if ready.screen != dashboardDeploy || len(ready.deployment.chosen) != 1 || !ready.deployment.chosen["pc02"] || !strings.Contains(ready.message, "pc03") {
		t.Fatal("software followup broadened or lost reviewed targets")
	}
}

func TestFocusedDeploymentChecksFreshInventory(t *testing.T) {
	for _, name := range []string{"pc02", "pc03"} {
		m := experienceFixture(2)
		observed := m.report
		m.report.Meta = domain.LabMeta{}
		m.screen = dashboardHosts
		m.actions.LoadInventory = func(context.Context) (domain.StatusReport, error) { return observed, nil }
		next, command := m.openHostDeployment(name)
		next, _ = next.(dashboardModel).Update(command())
		ready := next.(dashboardModel)
		if name == "pc02" && (ready.screen != dashboardDeploy || len(ready.deployment.chosen) != 1 || !ready.deployment.chosen[name]) {
			t.Fatal("focused client selection lost")
		}
		if name == "pc03" && (ready.screen != dashboardHosts || ready.message == "") {
			t.Fatal("removed identity entered deployment")
		}
	}
}
