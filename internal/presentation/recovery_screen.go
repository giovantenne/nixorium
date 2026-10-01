package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type recoveryMsg struct{ report domain.RecoveryReport }

func (model dashboardModel) observeRecovery(ctx context.Context) domain.RecoveryReport {
	if model.actions.ClassroomMode || model.actions.LoadRecovery == nil {
		return domain.RecoveryReport{}
	}
	return model.actions.LoadRecovery(ctx)
}

func (model dashboardModel) openRecovery() (tea.Model, tea.Cmd) {
	model.screen = dashboardRecovery
	model.recoveryCursor = 0
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
	case domain.RecoveryPXE:
		next, command := model.openNetworkInstallation()
		return next, command, true
	case domain.RecoveryControllerChanged:
		if model.actions.PlanController != nil {
			next, command := model.openControllerReview()
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

func (model dashboardModel) updateRecovery(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	conditions := model.recovery.Conditions
	switch key.String() {
	case "esc", "left":
		model.screen = dashboardHome
		model.message = ""
	case "up", "k":
		model.recoveryCursor = max(0, model.recoveryCursor-1)
	case "down", "j":
		model.recoveryCursor = min(max(0, len(conditions)-1), model.recoveryCursor+1)
	case "r":
		return model.openRecovery()
	case "enter":
		if len(conditions) == 0 {
			return model, nil
		}
		condition := conditions[min(model.recoveryCursor, len(conditions)-1)]
		if next, command, opened := model.recoveryFlow(condition); opened {
			return next, command
		}
		model.message = condition.Next.Action
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
	lines := []string{tuiTitle("What blocks operations", model.isDark), tuiMuted("Read from local state only; nothing is changed by this screen.", model.isDark), ""}
	conditions := model.recovery.Conditions
	if len(conditions) == 0 {
		lines = append(lines, "Nothing blocks Nixorium operations.")
	}
	for index, condition := range conditions {
		lines = append(lines, tuiSelection(condition.Title, index == model.recoveryCursor, model.isDark))
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
	shell.actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "r", label: "Refresh"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
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
	if len(model.recovery.Conditions) > 0 {
		lines = append(lines, tuiSection("What blocks operations", model.isDark))
		for _, condition := range model.recovery.Conditions {
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
