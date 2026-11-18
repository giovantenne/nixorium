package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type recoveryMsg struct{ report domain.RecoveryReport }

// recoveryObservedMsg refreshes the overview conditions without touching the
// recovery screen or the busy indicator.
type recoveryObservedMsg struct{ report domain.RecoveryReport }

func (model dashboardModel) observeRecoveryInBackground() tea.Cmd {
	if model.actions.ClassroomMode || model.actions.LoadRecovery == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return recoveryObservedMsg{report: model.observeRecovery(ctx)}
	}
}

func (model dashboardModel) observeRecovery(ctx context.Context) domain.RecoveryReport {
	if model.actions.ClassroomMode || model.actions.LoadRecovery == nil {
		return domain.RecoveryReport{}
	}
	return model.actions.LoadRecovery(ctx)
}

func (model dashboardModel) openRecovery() (tea.Model, tea.Cmd) {
	model.screen = dashboardRecovery
	model.recovery.cursor = 0
	model.recovery.stopping = false
	model.message = ""
	if model.actions.LoadRecovery == nil {
		return model, nil
	}
	model.busy = "Checking what blocks operations"
	return model.startRead(func(ctx context.Context) tea.Msg {
		return recoveryMsg{report: model.actions.LoadRecovery(ctx)}
	})
}

// recoveryFlow opens the dedicated way out of a condition, when one exists.
func (model dashboardModel) recoveryFlow(condition domain.BlockingCondition) (tea.Model, tea.Cmd, bool) {
	switch condition.Kind {
	case domain.RecoveryUSBReserved:
		next, command := model.openUSBInstallation()
		return next, command, true
	case domain.RecoverySettingsInvalid:
		next, command := model.openMaintenanceTask("e")
		return next, command, true
	case domain.RecoveryBackupDue:
		next, command := model.openBackup()
		return next, command, true
	case domain.RecoveryDeferredUpdates:
		next, command := model.openDeferredUpdates()
		return next, command, true
	case domain.RecoveryPXE:
		next, command := model.openNetworkInstallation()
		return next, command, true
	case domain.RecoveryControllerChanged:
		if model.actions.PlanController != nil {
			next, command := model.openControllerReview()
			return next, command, true
		}
	case domain.RecoveryOperationBusy:
		if condition.Log != "" && model.actions.LoadLog != nil {
			next, command := model.openRunningLog(condition.Log)
			return next, command, true
		}
	case domain.RecoveryDeploymentPending:
		if model.actions.PlanDeploymentRecovery != nil {
			next, command := model.openDeploymentRecovery()
			return next, command, true
		}
	case domain.RecoveryResetPending:
		if model.actions.PlanResetRecovery != nil {
			next, command := model.openResetRecovery()
			return next, command, true
		}
	}
	return model, nil, false
}

// automaticUpdateStoppedMsg ends a request to stop the automatic update.
type automaticUpdateStoppedMsg struct{ err error }

// stoppableCondition is the selected condition when it is the automatic
// update of queued computers and this session can stop it.
func (model dashboardModel) stoppableCondition() bool {
	conditions := model.recovery.report.Conditions
	if len(conditions) == 0 || model.actions.StopAutomaticUpdate == nil {
		return false
	}
	selected := conditions[min(model.recovery.cursor, len(conditions)-1)]
	return selected.Kind == domain.RecoveryOperationBusy && selected.Stoppable
}

func (model dashboardModel) stopAutomaticUpdate() (tea.Model, tea.Cmd) {
	model.recovery.stopping = false
	model.busy = "Stopping the automatic update"
	model.message = ""
	stop := model.actions.StopAutomaticUpdate
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return automaticUpdateStoppedMsg{err: stop(ctx)}
	}
}

func (model dashboardModel) finishAutomaticUpdateStop(message automaticUpdateStoppedMsg) (tea.Model, tea.Cmd) {
	model.busy = ""
	next, command := model.openRecovery()
	updated := next.(dashboardModel)
	if message.err != nil {
		updated.message = "The automatic update was not stopped: " + message.err.Error()
	} else {
		updated.message = "The automatic update was stopped and automatic updates are paused; resume them in Computers → Queued updates. If it had started updating computers, review the interrupted update."
	}
	return updated, command
}

// openRunningLog shows the tail of the log the running update is writing.
func (model dashboardModel) openRunningLog(id string) (tea.Model, tea.Cmd) {
	model.maintenance.logFromRecovery = true
	model.busy = "Reading the log of the running update"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardLogMsg{report: model.actions.LoadLog(ctx, id)}
	})
}

func (model dashboardModel) updateRecovery(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	conditions := model.recovery.report.Conditions
	if model.recovery.stopping {
		switch key.String() {
		case "enter":
			if !model.stoppableCondition() {
				// A refresh found that the automatic update had ended.
				model.recovery.stopping = false
				model.message = "The automatic update has already ended; nothing was stopped."
				return model, nil
			}
			return model.stopAutomaticUpdate()
		case "esc", "left":
			model.recovery.stopping = false
			model.message = "The automatic update goes on; nothing was changed."
		}
		return model, nil
	}
	switch key.String() {
	case "s":
		if model.stoppableCondition() {
			model.recovery.stopping = true
			model.message = ""
		}
	case "esc", "left":
		model.screen = dashboardHome
		model.message = ""
	case "up", "k":
		model.recovery.cursor = max(0, model.recovery.cursor-1)
	case "down", "j":
		model.recovery.cursor = min(max(0, len(conditions)-1), model.recovery.cursor+1)
	case "r":
		return model.openRecovery()
	case "enter":
		if len(conditions) == 0 {
			return model, nil
		}
		condition := conditions[min(model.recovery.cursor, len(conditions)-1)]
		if next, command, opened := model.recoveryFlow(condition); opened {
			return next, command
		}
		model.message = condition.Next.Action + "."
	}
	return model, nil
}

func (model dashboardModel) recoveryView() string {
	shell := tuiShell{path: []string{"Recovery"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	if model.recovery.stopping {
		shell.path = []string{"Recovery", "Stop the automatic update"}
		shell.body = strings.Join([]string{
			tuiTitle("Stop the automatic update?", model.isDark), "",
			"The controller stops updating the queued computers that answered again.",
			"Computers it already started finish on their own; Nixorium then asks you to review the interrupted update.",
			"Automatic updates stay paused until you resume them in Computers → Queued updates.",
		}, "\n")
		shell.actions = []tuiAction{{key: "Enter", label: "Stop it"}, {key: "Esc", label: "Keep it running"}, {key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{tuiTitle("What blocks operations", model.isDark), tuiMuted("Read from local state only; nothing is changed by this screen.", model.isDark), ""}
	conditions := model.recovery.report.Conditions
	if len(conditions) == 0 {
		lines = append(lines, "Nothing blocks Nixorium operations.")
	}
	for index, condition := range conditions {
		lines = append(lines, tuiSelection(condition.Title, index == model.recovery.cursor, model.isDark))
		if condition.Detail != "" {
			lines = append(lines, "  "+condition.Detail)
		}
		if condition.Blocks != "" {
			lines = append(lines, tuiMuted("  Blocks: "+condition.Blocks, model.isDark))
		}
		next := "  Next: " + condition.Next.Action
		if condition.Next.TUI != "" {
			next += " (" + condition.Next.TUI + ")"
		}
		if condition.Next.Command != "" {
			next += fmt.Sprintf(" · `%s`", condition.Next.Command)
		}
		lines = append(lines, next, "")
	}
	shell.body = strings.TrimSpace(strings.Join(lines, "\n"))
	open := "Open"
	if len(conditions) > 0 {
		if selected := conditions[min(model.recovery.cursor, len(conditions)-1)]; selected.Kind == domain.RecoveryOperationBusy && selected.Log != "" && model.actions.LoadLog != nil {
			open = "View its log"
		}
	}
	shell.actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: open}}
	if model.stoppableCondition() {
		shell.actions = append(shell.actions, tuiAction{key: "s", label: "Stop the automatic update"})
	}
	shell.actions = append(shell.actions, tuiAction{key: "r", label: "Refresh"}, tuiAction{key: "Esc", label: "Overview"}, tuiAction{key: "F1", label: "Help"})
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}

// safeModeView replaces a bare retry when the laboratory cannot be opened:
// it explains the cause, lists persistent blockers and offers only reads.
func (model dashboardModel) safeModeView() string {
	notices := append(model.managedJobNotices(), tuiNotice{
		kind: tuiStatusFailure, title: "The laboratory could not be opened",
		detail: model.message + " No configuration or computer was changed.",
	})
	lines := []string{tuiTitle("Safe mode", model.isDark), "Nixorium cannot read the saved laboratory state. These tools still work and change nothing.", ""}
	if len(model.recovery.report.Conditions) > 0 {
		lines = append(lines, tuiSection("What blocks operations", model.isDark))
		for _, condition := range model.recovery.report.Conditions {
			lines = append(lines, "• "+condition.Title, tuiMuted("  Next: "+condition.Next.Action, model.isDark))
		}
		lines = append(lines, "")
	}
	actions := []tuiAction{{key: "Enter", label: "Try again"}}
	if !model.actions.ClassroomMode {
		if model.actions.LoadRecovery != nil {
			actions = append(actions, tuiAction{key: "b", label: "What blocks"})
		}
		if model.actions.LoadDoctor != nil {
			actions = append(actions, tuiAction{key: "d", label: "Diagnostics"})
		}
		if model.actions.LoadGitReview != nil {
			actions = append(actions, tuiAction{key: "g", label: "Git changes"})
		}
		if model.actions.PreviewSupport != nil {
			actions = append(actions, tuiAction{key: "s", label: "Support report"})
		}
		if model.actions.LoadManagedJobs != nil {
			actions = append(actions, tuiAction{key: "v", label: "View progress"})
		}
	}
	actions = append(actions, tuiAction{key: "q", label: "Quit"}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{
		path:    []string{"Overview"},
		body:    strings.TrimSpace(strings.Join(lines, "\n")),
		notices: notices,
		actions: actions,
	})
}
