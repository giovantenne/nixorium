package presentation

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// recoveryReview guides one reviewed recovery: an interrupted client update
// or an interrupted template reset. It applies nothing without the typed word.
type recoveryReview struct {
	kind         string // "deploy" or "reset"
	acknowledge  bool
	deployPlan   domain.DeploymentRecoveryPlan
	deployResult domain.DeploymentRecoveryResult
	resetPlan    domain.TemplateResetRecoveryPlan
	resetResult  domain.TemplateResetRecoveryResult
	confirmation string
	done         bool
}

type recoveryPlanMsg struct {
	deploy domain.DeploymentRecoveryPlan
	reset  domain.TemplateResetRecoveryPlan
}

type recoveryResultMsg struct {
	deploy domain.DeploymentRecoveryResult
	reset  domain.TemplateResetRecoveryResult
}

func (model dashboardModel) openDeploymentRecovery() (tea.Model, tea.Cmd) {
	model.recoveryReview = recoveryReview{kind: "deploy"}
	return model.planRecoveryReview()
}

func (model dashboardModel) openResetRecovery() (tea.Model, tea.Cmd) {
	model.recoveryReview = recoveryReview{kind: "reset"}
	return model.planRecoveryReview()
}

func (model dashboardModel) planRecoveryReview() (tea.Model, tea.Cmd) {
	review := model.recoveryReview
	model.screen = dashboardRecoveryReview
	model.message = ""
	model.recoveryReview.confirmation = ""
	if review.kind == "deploy" {
		if model.actions.PlanDeploymentRecovery == nil {
			model.message = "Recovery is not available in this session."
			return model, nil
		}
		model.busy = "Checking every computer of the interrupted update"
		action, acknowledge := model.actions.PlanDeploymentRecovery, review.acknowledge
		return model.startRead(func(ctx context.Context) tea.Msg {
			return recoveryPlanMsg{deploy: action(ctx, acknowledge)}
		})
	}
	if model.actions.PlanResetRecovery == nil {
		model.message = "Recovery is not available in this session."
		return model, nil
	}
	model.busy = "Comparing the repository with the interrupted reset"
	action := model.actions.PlanResetRecovery
	return model.startRead(func(ctx context.Context) tea.Msg {
		return recoveryPlanMsg{reset: action(ctx)}
	})
}

func (review recoveryReview) ready() (bool, string) {
	if review.kind == "deploy" {
		return review.deployPlan.State == "ready" && !review.deployPlan.HasErrors(), review.deployPlan.Confirmation
	}
	return review.resetPlan.State == "ready" && !review.resetPlan.HasErrors(), review.resetPlan.Confirmation
}

func (model dashboardModel) updateRecoveryReview(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	review := &model.recoveryReview
	if review.done {
		switch key.String() {
		case "enter", "esc":
			if model.initialError && model.actions.LoadInitial != nil {
				model.initialError = false
				model.initializing = true
				model.screen = dashboardHome
				model.busy = "Opening the laboratory and checking setup progress"
				return model.loadInitial()
			}
			model.screen = dashboardHome
			return model.refreshOverview()
		}
		return model, nil
	}
	ready, word := review.ready()
	switch key.String() {
	case "esc":
		model.screen = dashboardRecovery
		model.message = "Nothing was changed."
		return model, nil
	case "u":
		if review.kind == "deploy" && review.deployPlan.Unreachable > 0 {
			review.acknowledge = !review.acknowledge
			return model.planRecoveryReview()
		}
	case "r":
		if review.confirmation == "" {
			return model.planRecoveryReview()
		}
		review.confirmation += "r"
	case "backspace":
		value := []rune(review.confirmation)
		if len(value) > 0 {
			review.confirmation = string(value[:len(value)-1])
		}
	case "enter":
		if !ready {
			return model, nil
		}
		if review.confirmation != word {
			review.confirmation = ""
			model.message = "Confirmation did not match; nothing was changed."
			return model, nil
		}
		review.confirmation = ""
		if review.kind == "deploy" {
			plan, action := review.deployPlan, model.actions.ApplyDeploymentRecovery
			model.busy = "Archiving the reviewed record"
			return model, func() tea.Msg { return recoveryResultMsg{deploy: action(plan)} }
		}
		plan, action := review.resetPlan, model.actions.ApplyResetRecovery
		model.busy = "Resolving the interrupted template reset"
		return model, func() tea.Msg { return recoveryResultMsg{reset: action(plan)} }
	default:
		if ready && key.Text != "" && key.Text != " " {
			review.confirmation += key.Text
		}
	}
	return model, nil
}

func (model dashboardModel) recoveryReviewView() string {
	review := model.recoveryReview
	title := "Recover the interrupted client update"
	if review.kind == "reset" {
		title = "Recover the interrupted template reset"
	}
	shell := tuiShell{path: []string{"Recovery", title}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	var text strings.Builder
	switch {
	case review.done && review.kind == "deploy":
		DeploymentRecoveryResultText(&text, review.deployResult)
	case review.done:
		TemplateResetRecoveryResultText(&text, review.resetResult)
	case review.kind == "deploy":
		DeploymentRecoveryPlanText(&text, review.deployPlan)
	default:
		TemplateResetRecoveryPlanText(&text, review.resetPlan)
	}
	lines := []string{tuiTitle(title, model.isDark), ""}
	for _, line := range strings.Split(strings.TrimRight(text.String(), "\n"), "\n") {
		if !strings.HasPrefix(line, "Review token:") {
			lines = append(lines, line)
		}
	}
	ready, word := review.ready()
	switch {
	case review.done:
		shell.actions = []tuiAction{{key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
	case ready:
		lines = append(lines, "", tuiSection("Type "+word+" to continue:", model.isDark), "> "+review.confirmation+"_")
		shell.actions = []tuiAction{{key: "Enter", label: "Confirm"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	default:
		shell.actions = []tuiAction{{key: "r", label: "Check again"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	}
	if !review.done && review.kind == "deploy" && review.deployPlan.Unreachable > 0 {
		label := "Acknowledge unreachable"
		if review.acknowledge {
			label = "Withdraw acknowledgement"
		}
		shell.actions = append([]tuiAction{{key: "u", label: label}}, shell.actions...)
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
