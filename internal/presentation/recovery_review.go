package presentation

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// recoveryReview guides one reviewed recovery: an interrupted client update
// or an interrupted template reset. The client update review changes nothing
// on any computer and only archives its record, so Enter confirms it once
// every computer was checked (ADR 0026); the template reset review still
// asks for its typed word.
type recoveryReview struct {
	kind         string // "deploy" or "reset"
	acknowledge  bool
	deployPlan   domain.DeploymentRecoveryPlan
	deployResult domain.DeploymentRecoveryResult
	resetPlan    domain.TemplateResetRecoveryPlan
	resetResult  domain.TemplateResetRecoveryResult
	confirmation string
	done         bool
	// failure is the technical text of the apply that left the record when
	// the review opened at the point of failure; d shows it.
	failure        string
	failureDetails bool
	// usbHost is the computer of an unfinished USB installation, if any.
	usbHost string
	// returnTo is the screen Esc goes back to, with its notice.
	returnTo      dashboardScreen
	returnMessage string
	returnSet     bool
}

type recoveryPlanMsg struct {
	deploy  domain.DeploymentRecoveryPlan
	reset   domain.TemplateResetRecoveryPlan
	usbHost string
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
		reserved, remote := model.actions.RemoteReservationPresent, model.actions.LoadRemoteInstall
		if model.actions.ClassroomMode {
			reserved = nil
		}
		return model.startRead(func(ctx context.Context) tea.Msg {
			plan := action(ctx, acknowledge)
			host := ""
			// Name the computer of an unfinished USB installation: it may be
			// the one that does not answer. The worker is asked only when
			// the local reservation exists.
			if plan.Unreachable > 0 && reserved != nil && remote != nil && reserved() {
				if response, err := remote(ctx); err == nil {
					host = remoteInstallHostName(response)
				}
			}
			return recoveryPlanMsg{deploy: plan, usbHost: host}
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

// remoteInstallHostName is the computer named by a USB installation record.
func remoteInstallHostName(response domain.RemoteInstallResponse) string {
	session := response.Session
	if session == nil {
		return ""
	}
	name := session.Plan.Host.Name
	switch {
	case session.Preparation != nil && session.Preparation.Host.Name != "":
		name = session.Preparation.Host.Name
	case session.Artifacts != nil && session.Artifacts.HostName != "":
		name = session.Artifacts.HostName
	case name == "" && session.Bootstrap != nil:
		name = session.Bootstrap.Host
	}
	return sanitizeRemoteInstallText(name)
}

func (review recoveryReview) ready() (bool, string) {
	if review.kind == "deploy" {
		return review.deployPlan.State == "ready" && !review.deployPlan.HasErrors(), review.deployPlan.Confirmation
	}
	return review.resetPlan.State == "ready" && !review.resetPlan.HasErrors(), review.resetPlan.Confirmation
}

func (model dashboardModel) leaveRecoveryReview() (tea.Model, tea.Cmd) {
	review := model.recoveryReview
	if review.returnSet {
		model.screen = review.returnTo
		model.message = review.returnMessage
		return model, nil
	}
	model.screen = dashboardRecovery
	model.message = "Nothing was changed."
	return model, nil
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
	if review.kind == "deploy" {
		return model.updateDeploymentRecoveryReview(key)
	}
	ready, word := review.ready()
	switch key.String() {
	case "esc":
		return model.leaveRecoveryReview()
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

// updateDeploymentRecoveryReview: r checks again, c (or the older u)
// continues without the computers that do not answer, Enter marks the
// update as reviewed when every computer was checked, Esc goes back.
func (model dashboardModel) updateDeploymentRecoveryReview(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	review := &model.recoveryReview
	ready, _ := review.ready()
	switch key.String() {
	case "esc", "left":
		return model.leaveRecoveryReview()
	case "r":
		return model.planRecoveryReview()
	case "c", "u":
		if review.deployPlan.Unreachable > 0 {
			review.acknowledge = !review.acknowledge
			return model.planRecoveryReview()
		}
	case "d":
		if review.failure != "" {
			review.failureDetails = !review.failureDetails
		}
	case "enter":
		if !ready || model.actions.ApplyDeploymentRecovery == nil {
			return model, nil
		}
		plan, action := review.deployPlan, model.actions.ApplyDeploymentRecovery
		model.busy = "Archiving the reviewed record"
		return model, func() tea.Msg { return recoveryResultMsg{deploy: action(plan)} }
	}
	return model, nil
}

func (model dashboardModel) recoveryReviewView() string {
	review := model.recoveryReview
	title := "Review the interrupted update"
	if review.kind == "reset" {
		title = "Recover the interrupted template reset"
	}
	shell := tuiShell{path: []string{"Recovery", title}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	if review.kind == "deploy" {
		return model.deploymentRecoveryReviewView(shell, title)
	}
	var text strings.Builder
	if review.done {
		TemplateResetRecoveryResultText(&text, review.resetResult)
	} else {
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
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}

func (model dashboardModel) deploymentRecoveryReviewView(shell tuiShell, title string) string {
	review := model.recoveryReview
	lines := []string{tuiTitle(title, model.isDark), ""}
	if review.failure != "" && !review.done {
		notice := tuiNotice{kind: tuiStatusAttention, title: review.failure, code: "DEPLOY-PENDING",
			headline: "The computers were not updated: the update was interrupted."}
		step, _ := domain.NextStepByCode("DEPLOY-PENDING")
		failure := tuiShell{detailsKey: true, details: review.failureDetails}
		failureLines := situationNoticeLines(notice, step, failure, model.isDark)
		// The way forward is this screen; keep what failed and why.
		lines = append(lines, failureLines[0], failureLines[1])
		lines = append(lines, failureLines[3:]...)
		lines = append(lines, "")
	}
	if review.done {
		lines = append(lines, deploymentRecoveryResultLines(review.deployResult, model.isDark)...)
		shell.body = strings.Join(lines, "\n")
		shell.actions = []tuiAction{{key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines = append(lines, deploymentRecoveryLines(review.deployPlan, review.usbHost, model.isDark)...)
	ready, _ := review.ready()
	shell.actions = []tuiAction{{key: "r", label: "Check again"}}
	if review.deployPlan.Unreachable > 0 {
		label := "Continue without them"
		if review.acknowledge {
			label = "Include them again"
		}
		shell.actions = append(shell.actions, tuiAction{key: "c", label: label})
	}
	if ready {
		shell.actions = append([]tuiAction{{key: "Enter", label: "Mark as reviewed"}}, shell.actions...)
	}
	if review.failure != "" {
		label := "Details"
		if review.failureDetails {
			label = "Hide details"
		}
		shell.actions = append(shell.actions, tuiAction{key: "d", label: label})
	}
	shell.actions = append(shell.actions, tuiAction{key: "Esc", label: "Back"}, tuiAction{key: "F1", label: "Help"})
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
