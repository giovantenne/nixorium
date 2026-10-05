package presentation

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	dashboardReadTimeout  = 2 * time.Minute
	dashboardBuildTimeout = 60 * time.Minute
)

// One request owns its context and replies. Cancelling a read never authorizes
// a mutation, and a reply from an older request cannot resume a workflow.
type readActivity struct {
	id           uint64
	ctx          context.Context
	cancel       context.CancelFunc
	returnScreen dashboardScreen
	limit        time.Duration
}

type activityResultMsg struct {
	id      uint64
	message tea.Msg
	err     error
	more    bool
}

func (model dashboardModel) startRead(read func(context.Context) tea.Msg) (tea.Model, tea.Cmd) {
	return model.startBoundedRead(dashboardReadTimeout, read)
}

func (model dashboardModel) startBoundedRead(limit time.Duration, read func(context.Context) tea.Msg) (tea.Model, tea.Cmd) {
	ctx, id := model.beginRead(limit)
	return model, boundedReadCommand(ctx, id, read)
}

func boundedReadCommand(ctx context.Context, id uint64, read func(context.Context) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return activityResultMsg{id: id, err: err}
		}
		// The buffered result also lets a non-cooperative callback finish after
		// cancellation without blocking the UI or publishing a stale reply.
		result := make(chan tea.Msg, 1)
		go func() { result <- read(ctx) }()
		select {
		case message := <-result:
			return activityResultMsg{id: id, message: message, err: ctx.Err()}
		case <-ctx.Done():
			return activityResultMsg{id: id, err: ctx.Err()}
		}
	}
}

func (model *dashboardModel) beginRead(limit time.Duration) (context.Context, uint64) {
	model.read.cancelRead()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	model.read.ctx, model.read.cancel = ctx, cancel
	model.read.limit = limit
	model.read.returnScreen = model.screen
	model.busyStarted = time.Now()
	return ctx, model.read.id
}

func (read *readActivity) cancelRead() {
	if read.cancel != nil {
		read.cancel()
	}
	read.cancel, read.ctx = nil, nil
	read.id++
}

func (model dashboardModel) finishRead(message activityResultMsg) (tea.Model, tea.Cmd) {
	if model.read.cancel == nil || model.read.id != message.id {
		return model, nil
	}
	if message.err == nil {
		message.err = model.read.ctx.Err()
	}
	if message.err != nil {
		model = model.cancelActivity()
		if errors.Is(message.err, context.DeadlineExceeded) {
			model.message = "Loading took too long. Nothing was changed. Reopen this view, or use Maintenance → Diagnostics if it happens again."
		}
		return model, nil
	}
	if !message.more {
		model.read.cancelRead()
	}
	return model.updateState(message.message)
}

func (model dashboardModel) cancelActivity() dashboardModel {
	model.controller.fromSave = false
	model.screen = model.read.returnScreen
	model.read.cancelRead()
	model.inventory.cancelRead()
	model.workspace.cancelRead()
	model.support.cancelRead()
	model.templateReset.cancelRead()
	if model.hostTrust.cancel != nil {
		model.hostTrust.cancel()
		model.hostTrust.cancel = nil
		model.hostTrust.id++
	}
	if model.templateReset.stage == "planning" {
		model.templateReset.stage = "select"
		model.templateReset.confirmation = ""
	}
	if model.screen == dashboardUSBInstall && model.installation.remote.fingerprint == "" && model.installation.remote.stage == remoteInstallFingerprint {
		model.installation.remote.stage = remoteInstallConsole
	}
	model.busy = ""
	model.updates.planning = false
	model.updates.planEvents = nil
	model.initializing = false
	// The accepted flag is sticky in the editor. A cancelled validation must
	// leave the draft editable, not resubmit it on the very next keystroke.
	model.settings.editor.accepted = false
	model.deployment.usbRecovery = nil
	if model.screen != dashboardSettingsEdit && model.screen != dashboardSettingsPasswords {
		model.installation.flow, model.installation.failed = false, false
	}
	model.message = "Loading cancelled. Nothing was changed."
	return model
}

func (model dashboardModel) activityStatus() string {
	started := model.busyStarted
	if model.read.cancel == nil {
		switch {
		case model.controller.applying && !model.controller.started.IsZero():
			started = model.controller.started
		case model.installation.pxePreparing && !model.installation.pxeStarted.IsZero():
			started = model.installation.pxeStarted
		case model.deployment.applying && !model.deployment.started.IsZero():
			started = model.deployment.started
		}
	}
	elapsed := time.Duration(0)
	if !started.IsZero() {
		elapsed = max(time.Duration(0), time.Since(started).Truncate(time.Second))
	}
	status := fmt.Sprintf("Elapsed: %s", elapsed)
	switch {
	case model.read.cancel != nil || model.inventory.cancel != nil || model.workspace.cancel != nil || model.support.cancel != nil || model.templateReset.cancel != nil || model.hostTrust.cancel != nil:
		status += " · Read-only; Esc cancels."
		if model.read.limit > 0 {
			status += fmt.Sprintf(" Limit: %s.", model.read.limit)
		}
	case model.installation.pxePreparing:
		status += " · Background work continues after quit."
	default:
		status += " · Cannot be interrupted; wait for the result."
	}
	return status
}

func (model dashboardModel) hasCancellableRead() bool {
	return model.read.cancel != nil || model.inventory.cancel != nil || model.workspace.cancel != nil || model.support.cancel != nil || model.templateReset.cancel != nil || model.hostTrust.cancel != nil
}
