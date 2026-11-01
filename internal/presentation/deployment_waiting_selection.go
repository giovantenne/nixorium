package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

// A computer whose queued update became obsolete (the configuration changed
// after queueing) never updates on its own. Opening Update computers selects
// those computers so the next review includes them by default; the
// administrator can still change the selection before the review.

const waitingSelectionTimeout = 5 * time.Second

type deploymentWaitingSelectionMsg struct {
	status domain.DeferredUpdateStatus
}

// loadWaitingSelection reads the queue without a busy screen: the selection
// stays usable while it loads, and a failure only leaves it unchanged.
func (model dashboardModel) loadWaitingSelection() tea.Cmd {
	load := model.actions.LoadDeferredUpdates
	if load == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), waitingSelectionTimeout)
		defer cancel()
		return deploymentWaitingSelectionMsg{status: load(ctx)}
	}
}

func (model dashboardModel) applyWaitingSelection(status domain.DeferredUpdateStatus) dashboardModel {
	// Only an untouched selection is changed, never a review in progress.
	if model.screen != dashboardDeploy || model.deployment.applying || len(model.deployment.chosen) > 0 || model.deployment.result.Operation != "" {
		return model
	}
	configured := map[string]bool{}
	for _, host := range model.report.Meta.Clients.Hosts {
		configured[host.Name] = true
	}
	chosen := map[string]bool{}
	names := []string{}
	for _, entry := range status.Updates {
		if entry.Stale && configured[entry.Host] {
			chosen[entry.Host] = true
			names = append(names, entry.Host)
		}
	}
	if len(names) == 0 {
		return model
	}
	model.deployment.chosen = chosen
	model.message = "Selected " + strings.Join(names, ", ") + ": their queued update is out of date. Space changes the selection."
	return model
}
