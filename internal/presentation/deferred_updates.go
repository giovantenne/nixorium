package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Queued updates: computers that were off when an update was reviewed. The
// controller updates each one when it answers again; this screen shows the
// queue and removes entries.

var deferredUpdatesTask = dashboardTask{id: "queued", shortcut: "u", title: "Queued updates", description: "Computers that update when they are switched on"}

type deferredModel struct {
	status domain.DeferredUpdateStatus
	cursor int
	loaded bool
}

type deferredUpdatesMsg struct{ status domain.DeferredUpdateStatus }

func (model dashboardModel) openDeferredUpdates() (tea.Model, tea.Cmd) {
	model.screen = dashboardDeferredUpdates
	model.message = ""
	model.deferred = deferredModel{}
	return model.loadDeferredUpdates()
}

func (model dashboardModel) loadDeferredUpdates() (tea.Model, tea.Cmd) {
	if model.actions.LoadDeferredUpdates == nil {
		model.message = "Queued updates are not available in this session."
		return model, nil
	}
	model.busy = "Reading the queued updates"
	load := model.actions.LoadDeferredUpdates
	return model.startRead(func(ctx context.Context) tea.Msg { return deferredUpdatesMsg{status: load(ctx)} })
}

func (model dashboardModel) updateDeferredUpdates(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	entries := model.deferred.status.Updates
	switch key.String() {
	case "esc", "left":
		model.screen = dashboardComputersArea
		model.message = ""
	case "up", "k":
		model.deferred.cursor = max(0, model.deferred.cursor-1)
	case "down", "j":
		model.deferred.cursor = min(max(0, len(entries)-1), model.deferred.cursor+1)
	case "r":
		return model.loadDeferredUpdates()
	case "x", "X":
		if len(entries) == 0 || model.actions.CancelDeferredUpdates == nil {
			return model, nil
		}
		hosts := []string{entries[min(model.deferred.cursor, len(entries)-1)].Host}
		label := hosts[0]
		if key.String() == "X" {
			hosts, label = []string{"@all"}, "every computer"
		}
		if err := model.actions.CancelDeferredUpdates(hosts); err != nil {
			model.message = "The queue was not changed: " + err.Error()
			return model, nil
		}
		model.deferred.cursor = 0
		next, command := model.loadDeferredUpdates()
		updated := next.(dashboardModel)
		updated.message = "Removed " + label + " from the queue; it keeps its current system."
		return updated, command
	}
	return model, nil
}

func (model dashboardModel) deferredUpdatesView() string {
	shell := tuiShell{path: []string{"Computers", "Queued updates"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	status := model.deferred.status
	lines := []string{tuiTitle("Computers that update when switched on", model.isDark),
		tuiMuted("The controller checks every minute and updates each computer as soon as it answers; its user then sees a notification.", model.isDark), ""}
	if status.HasErrors() {
		for _, issue := range status.Issues {
			shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusFailure, title: issue.Message})
		}
	}
	if len(status.Updates) == 0 {
		lines = append(lines, "No computer is waiting for an update.", "", tuiMuted("In Computers → Update computers, computers that are off can be queued with F3.", model.isDark))
	}
	for index, entry := range status.Updates {
		state := tuiMuted("waiting since "+entry.QueuedAt.Local().Format("02 Jan 15:04"), model.isDark)
		switch {
		case entry.Stale:
			state = tuiStatus("configuration changed since; review the update again", tuiStatusAttention, model.isDark)
		case entry.LastError != "":
			state = tuiStatus(fmt.Sprintf("last attempt failed (%d): %s", entry.Attempts, entry.LastError), tuiStatusAttention, model.isDark)
		}
		lines = append(lines, tuiSelection(fmt.Sprintf("%-8s %-15s", entry.Host, entry.IP), index == model.deferred.cursor, model.isDark)+"  "+state)
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	shell.actions = []tuiAction{{key: "r", label: "Refresh"}, {key: "Esc", label: "Computers"}, {key: "F1", label: "Help"}}
	if len(status.Updates) > 0 {
		shell.actions = append([]tuiAction{{key: "↑/↓", label: "Select"}, {key: "x", label: "Remove"}, {key: "X", label: "Remove all"}}, shell.actions...)
	}
	return model.renderShell(shell)
}
