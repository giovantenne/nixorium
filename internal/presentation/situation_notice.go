package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Situation notices replace technical refusal text with the catalogue
// entry: what was not done and why, the reason Nixorium stops, and the way
// forward. The technical text stays available as a details line.

// failureNotice reports a failed attempt; prefix says what was tried.
func failureNotice(prefix, message string) tuiNotice {
	return tuiNotice{kind: tuiStatusAttention, title: message, prefix: prefix}
}

// noticeSituation recognizes a catalogued situation in a notice that does
// not already name its way forward.
func noticeSituation(notice tuiNotice) (domain.NextStep, bool) {
	if notice.kind == tuiStatusSuccess || notice.plain {
		return domain.NextStep{}, false
	}
	if notice.code != "" {
		return domain.NextStepByCode(notice.code)
	}
	text := noticeRawText(notice)
	if strings.Contains(text, "Next:") {
		return domain.NextStep{}, false
	}
	return domain.NextStepFor(text)
}

func noticeRawText(notice tuiNotice) string {
	return strings.TrimSpace(notice.title + " " + notice.detail)
}

func situationNoticeLines(notice tuiNotice, step domain.NextStep, shell tuiShell, dark bool) []string {
	headline := notice.headline
	if headline == "" {
		headline = step.Headline(notice.prefix)
	}
	lines := []string{
		tuiNoticeTitle(headline, notice.kind, dark),
		"  " + step.Why,
		"  " + situationNextLine(step, shell.jump && step.Screen != ""),
	}
	if raw := noticeRawText(notice); raw != "" && (shell.details || !shell.detailsKey) {
		lines = append(lines, "  "+tuiMuted("Details: "+raw, dark))
	}
	return lines
}

func situationNextLine(step domain.NextStep, jump bool) string {
	line := "Next: " + step.Action
	switch {
	case jump:
		line += " (Enter)"
	case step.TUI != "":
		line += " (" + step.TUI + ")"
	}
	return line
}

// noticeNextStep names the way forward for a recognized blocker, unless the
// notice already gives one.
func noticeNextStep(notice tuiNotice) string {
	step, found := noticeSituation(notice)
	if !found {
		return ""
	}
	return situationNextLine(step, false)
}

// resultSituation is the situation reported on a result screen that offers
// the direct way out: Enter opens its screen and d shows the technical text.
func (model dashboardModel) resultSituation() (domain.NextStep, bool) {
	if model.busy != "" || model.helpOpen {
		return domain.NextStep{}, false
	}
	switch {
	case model.screen == dashboardUpdate && model.updates.result.Operation != "":
	case model.screen == dashboardController && model.controller.result.Operation != "" && !model.controller.applying:
	case model.screen == dashboardDeploy && model.deployment.result.Operation != "" && model.deployment.usbRecovery == nil && !model.deployment.applying:
		notice := model.deploymentResultNotice()
		return noticeSituation(notice)
	default:
		return domain.NextStep{}, false
	}
	return noticeSituation(failureNotice("", model.message))
}

// situationScreenAvailable mirrors recoveryFlow: whether the situation's
// screen can be opened in this session.
func (model dashboardModel) situationScreenAvailable(step domain.NextStep) bool {
	switch step.Screen {
	case "":
		return false
	case domain.RecoveryControllerChanged:
		return model.actions.PlanController != nil
	case domain.RecoveryDeploymentPending:
		return model.actions.PlanDeploymentRecovery != nil
	case domain.RecoveryResetPending:
		return model.actions.PlanResetRecovery != nil
	}
	return true
}

// updateResultSituation handles Enter and d on a result screen that shows
// a situation. Enter opens the situation's screen and Esc there returns
// here with the same notice.
func (model dashboardModel) updateResultSituation(key string) (tea.Model, tea.Cmd, bool) {
	step, found := model.resultSituation()
	if !found {
		return model, nil, false
	}
	switch key {
	case "d":
		if model.screen == dashboardController {
			// The controller result already uses d for its details.
			return model, nil, false
		}
		model.noticeDetails = !model.noticeDetails
		return model, nil, true
	case "enter":
		if !model.situationScreenAvailable(step) {
			return model, nil, false
		}
		origin, message := model.screen, model.message
		next, command, opened := model.recoveryFlow(domain.BlockingCondition{Kind: step.Screen, Next: step})
		if !opened {
			return model, nil, false
		}
		if opened, ok := next.(dashboardModel); ok && opened.screen == dashboardRecoveryReview {
			opened.recoveryReview.returnTo, opened.recoveryReview.returnMessage, opened.recoveryReview.returnSet = origin, message, true
			opened.noticeDetails = false
			return opened, command, true
		}
		return next, command, true
	}
	return model, nil, false
}

// withResultSituation lets the action bar say what Enter and d do.
func (model dashboardModel) withResultSituation(shell tuiShell) tuiShell {
	step, found := model.resultSituation()
	if !found {
		return shell
	}
	shown, raw := domain.NextStep{}, ""
	for _, notice := range shell.notices {
		if situation, found := noticeSituation(notice); found {
			shown, raw = situation, noticeRawText(notice)
			break
		}
	}
	if shown.Code != step.Code {
		return shell
	}
	if model.screen == dashboardController {
		shell.detailsKey, shell.details = true, model.controller.details
	} else {
		shell.detailsKey, shell.details = true, model.noticeDetails
		if raw != "" && !hasAction(shell.actions, "d") {
			shell.actions = insertBeforeHelp(shell.actions, tuiAction{key: "d", label: "Details"})
		}
	}
	if model.situationScreenAvailable(step) {
		shell.jump = true
		actions := []tuiAction{{key: "Enter", label: step.Action}}
		closing := ""
		for _, action := range shell.actions {
			if action.key != "Enter" {
				actions = append(actions, action)
			} else if closing == "" {
				closing = action.label
			}
		}
		// Esc still closes the screen; say where it goes when Enter did.
		if !hasAction(actions, "Esc") && closing != "" {
			actions = insertBeforeHelp(actions, tuiAction{key: "Esc", label: closing})
		}
		shell.actions = actions
	}
	return shell
}

func hasAction(actions []tuiAction, key string) bool {
	for _, action := range actions {
		if action.key == key {
			return true
		}
	}
	return false
}

func insertBeforeHelp(actions []tuiAction, action tuiAction) []tuiAction {
	for index, existing := range actions {
		if existing.key == "F1" {
			return append(append(append([]tuiAction{}, actions[:index]...), action), actions[index:]...)
		}
	}
	return append(actions, action)
}

// updateFailurePrefix says what an Update Nixorium result did not do.
func (model dashboardModel) updateFailurePrefix() string {
	saved := model.updates.result.Updated && !model.updates.result.HasErrors() && !model.updates.result.RecoveryRequired
	switch {
	case saved:
		return "The controller was not updated"
	case model.updates.packageBase:
		return "The system update was not saved"
	}
	return "Nixorium was not updated"
}

// deploymentResultNotice reports a client update result. A pending record
// left by this apply opens its review; a refusal before the build names its
// situation; other outcomes keep their own words next to the per-computer
// results.
func (model dashboardModel) deploymentResultNotice() tuiNotice {
	result := model.deployment.result
	notice := tuiNotice{kind: tuiStatusNeutral, title: model.message, prefix: "The computers were not updated"}
	switch {
	case result.RecoveryRequired:
		notice.kind, notice.code = tuiStatusAttention, "DEPLOY-PENDING"
		notice.headline = "The computers were not updated: the update was interrupted."
	case result.HasErrors() && result.Phase == domain.DeploymentPhasePreflight && !result.BuildCompleted:
		notice.kind = tuiStatusAttention
	default:
		notice.plain = true
	}
	return notice
}
