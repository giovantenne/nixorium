package presentation

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateGitReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.maintenance.gitCommitResult.Operation != "" || model.maintenance.gitUndoResult.Operation != "" {
		// A saved undo continues like a settings save: Enter opens the
		// ordinary Apply to controller review (or Update computers once the
		// controller runs the saved revision); Esc returns to Maintenance.
		if key.String() == "enter" && model.maintenance.gitUndoResult.Operation != "" && hasAction(model.saveFollowupActions(""), "Enter") {
			return model.continueSavedConfiguration()
		}
		switch key.String() {
		case "enter", "esc", "left":
			if model.maintenance.gitFromHomeFiles {
				model.screen = dashboardHomeFiles
				model.message = ""
				return model, nil
			}
			if model.maintenance.gitFromWorkspace {
				model.screen = dashboardWorkspace
				model.message = ""
				return model, nil
			}
			if model.setupMode {
				model.screen = dashboardSetup
				model.message = ""
				if model.actions.LoadSetup != nil {
					model.busy = "Refreshing first-run progress"
					return model.loadSetup()
				}
			} else {
				model.screen = dashboardHome
				model.message = ""
			}
		case "r":
			model.maintenance.gitCommitResult = domain.GitCommitReport{}
			model.maintenance.gitUndoResult = domain.GitUndoReport{}
			model.busy = "Refreshing the read-only Git review"
			model.message = ""
			return model.startRead(func(ctx context.Context) tea.Msg {
				return dashboardGitReviewMsg{report: model.actions.LoadGitReview(ctx)}
			})
		}
		return model, nil
	}
	maximum := maximumGitReviewScroll(model.maintenance.gitReview, model.gitReviewHeight())
	switch key.String() {
	case "esc", "left":
		if model.maintenance.gitFromHomeFiles {
			model.screen = dashboardHomeFiles
			model.message = ""
			return model, nil
		}
		if model.maintenance.gitFromWorkspace {
			model.screen = dashboardWorkspace
			model.message = ""
			return model, nil
		}
		if model.setupMode {
			model.screen = dashboardSetup
		} else {
			model.screen = dashboardHome
		}
		model.message = ""
		if model.setupMode && model.actions.LoadSetup != nil {
			model.busy = "Refreshing first-run progress"
			return model.loadSetup()
		}
	case "up", "k":
		if model.maintenance.gitScroll > 0 {
			model.maintenance.gitScroll--
		}
	case "down", "j":
		if model.maintenance.gitScroll < maximum {
			model.maintenance.gitScroll++
		}
	case "pgup":
		model.maintenance.gitScroll -= model.gitReviewHeight()
		if model.maintenance.gitScroll < 0 {
			model.maintenance.gitScroll = 0
		}
	case "pgdown":
		model.maintenance.gitScroll += model.gitReviewHeight()
		if model.maintenance.gitScroll > maximum {
			model.maintenance.gitScroll = maximum
		}
	case "home":
		model.maintenance.gitScroll = 0
	case "end":
		model.maintenance.gitScroll = maximum
	case "r":
		model.busy = "Refreshing the read-only Git review"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardGitReviewMsg{report: model.actions.LoadGitReview(ctx)}
		})
	case "c":
		if len(model.maintenance.gitReview.Changes) == 0 || model.maintenance.gitReview.HasErrors() {
			model.message = "A clean, unblocked change review is required before selecting commit paths."
			return model, nil
		}
		model.maintenance.gitCommitChosen = map[string]bool{}
		model.maintenance.gitCommitCursor = 0
		model.maintenance.gitDiscard = false
		model.message = ""
		model.screen = dashboardGitCommitSelect
	case "u":
		if model.actions.PlanGitUndo == nil || model.actions.ApplyGitUndo == nil {
			return model, nil
		}
		return model.startGitUndoReview()
	case "x":
		if model.actions.PlanGitDiscard == nil || len(model.maintenance.gitReview.Changes) == 0 || model.maintenance.gitReview.HasErrors() {
			model.message = "A clean, unblocked change review is required before discarding changes."
			return model, nil
		}
		model.maintenance.gitCommitChosen = map[string]bool{}
		model.maintenance.gitCommitCursor = 0
		model.maintenance.gitDiscard = true
		model.message = ""
		model.screen = dashboardGitCommitSelect
	}
	return model, nil
}

func (model dashboardModel) updateGitCommitSelectKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	changes := model.maintenance.gitReview.Changes
	switch key.String() {
	case "esc", "left":
		model.screen = dashboardGitReview
		model.message = ""
	case "up", "k":
		if model.maintenance.gitCommitCursor > 0 {
			model.maintenance.gitCommitCursor--
		}
	case "down", "j":
		if model.maintenance.gitCommitCursor+1 < len(changes) {
			model.maintenance.gitCommitCursor++
		}
	case "space":
		if len(changes) > 0 && !changes[model.maintenance.gitCommitCursor].Private {
			path := changes[model.maintenance.gitCommitCursor].Path
			model.maintenance.gitCommitChosen[path] = !model.maintenance.gitCommitChosen[path]
		}
	case "a":
		model.maintenance.gitCommitChosen = toggleAllGitCommitPaths(changes, model.maintenance.gitCommitChosen)
	case "enter":
		paths := selectedGitCommitPaths(changes, model.maintenance.gitCommitChosen)
		if paths == "" {
			model.message = "Select at least one changed path before creating a commit plan."
			return model, nil
		}
		model.busy = "Building an isolated commit proposal from HEAD"
		model.message = ""
		plan := model.actions.PlanGitCommit
		if model.maintenance.gitDiscard {
			model.busy = "Reviewing what would be discarded"
			plan = model.actions.PlanGitDiscard
		}
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardGitCommitPlanMsg{report: plan(ctx, paths)}
		})
	}
	return model, nil
}

func (model dashboardModel) updateGitCommitReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	maximum := maximumGitCommitPlanScroll(model.maintenance.gitCommitPlan, model.gitReviewHeight())
	switch key.String() {
	case "esc":
		model.screen = dashboardGitCommitSelect
		model.confirmation = ""
		model.message = "Git commit cancelled; no repository state changed."
	case "up":
		if model.maintenance.gitScroll > 0 {
			model.maintenance.gitScroll--
		}
	case "down":
		if model.maintenance.gitScroll < maximum {
			model.maintenance.gitScroll++
		}
	case "pgup":
		model.maintenance.gitScroll -= model.gitReviewHeight()
		if model.maintenance.gitScroll < 0 {
			model.maintenance.gitScroll = 0
		}
	case "pgdown":
		model.maintenance.gitScroll += model.gitReviewHeight()
		if model.maintenance.gitScroll > maximum {
			model.maintenance.gitScroll = maximum
		}
	case "backspace":
		if model.maintenance.gitDiscard {
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		}
	case "space":
		if model.maintenance.gitDiscard {
			model.confirmation += " "
		}
	case "enter":
		// Saving a commit changes nothing that cannot be undone, so Enter
		// confirms it. Discarding unsaved changes keeps its typed word.
		if model.maintenance.gitDiscard && model.confirmation != model.maintenance.gitCommitPlan.Confirmation {
			model.confirmation = ""
			model.message = "Confirmation did not match; nothing was changed."
			return model, nil
		}
		model.busy = "Revalidating and creating the reviewed local commit"
		model.confirmation = ""
		model.message = ""
		plan := model.maintenance.gitCommitPlan
		apply := model.actions.ApplyGitCommit
		if model.maintenance.gitDiscard {
			model.busy = "Saving the current content and restoring the selected files"
			apply = model.actions.ApplyGitDiscard
		}
		return model, func() tea.Msg {
			result := apply(plan)
			ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
			defer cancel()
			return dashboardGitCommitResultMsg{report: result, review: model.actions.LoadGitReview(ctx)}
		}
	default:
		if model.maintenance.gitDiscard && key.Text != "" {
			model.confirmation += key.Text
		}
	}
	return model, nil
}

func (model dashboardModel) updateUpdateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.updates.result.Operation != "" {
		switch key.String() {
		case "enter":
			if model.updates.result.Updated && !model.updates.result.HasErrors() && !model.updates.result.RecoveryRequired {
				if controllerVerifiedForSave(model.updates.result.Revision, model.controller.result) {
					return model.openSavedDeployment(false)
				}
				return model.startUpdateControllerApply()
			}
		case "esc":
			model.screen = dashboardHome
			model.message = ""
		case "s", "n":
			if model.updates.result.RecoveryRequired != (key.String() == "s") {
				return model, nil
			}
			if model.updates.result.RecoveryRequired {
				model.busy = "Completing the local update save"
				model.updates.applying = true
				model.message = ""
				plan := model.updates.plan
				return model, func() tea.Msg {
					return dashboardUpdateResultMsg{report: model.saveReviewedUpdate(plan)}
				}
			}
			model.updates.result = domain.UpdateApplyReport{}
			model.updates.check = domain.UpdateCheckReport{}
			model.updates.target = ""
			model.message = ""
			if model.updates.packageBase {
				return model.openPackageBase()
			}
			if model.actions.CheckUpdate != nil {
				model.busy = "Fetching available Nixorium updates"
				return model.checkUpdates()
			}
		case "a":
			if model.updates.result.Updated && !model.updates.result.HasErrors() && !model.updates.result.RecoveryRequired {
				return model.startUpdateControllerApply()
			}
		}
		return model, nil
	}
	if model.updates.packageBase {
		return model.updatePackageBaseKey(key)
	}
	switch key.String() {
	case "esc":
		model.screen = dashboardHome
		model.message = ""
	case "p", "f2":
		model.updates.prerelease = !model.updates.prerelease
		model.updates.cursor = 0
		model.message = ""
	case "r":
		if model.actions.CheckUpdate == nil {
			model.message = "Update discovery is not available in this session."
			return model, nil
		}
		model.updates.check = domain.UpdateCheckReport{}
		model.updates.target = ""
		model.message = ""
		model.busy = "Fetching available Nixorium updates"
		return model.checkUpdates()
	case "up", "k":
		model.updates.cursor = max(0, model.updates.cursor-1)
	case "down", "j":
		releases := model.availableUpdateReleases()
		if model.updates.cursor+1 < len(releases) {
			model.updates.cursor++
		}
	case "enter":
		releases := model.availableUpdateReleases()
		if model.updates.check.HasErrors() || len(releases) == 0 {
			model.message = "Fetch available updates before selecting a target."
			return model, nil
		}
		selected := releases[min(model.updates.cursor, len(releases)-1)]
		target := selected.Tag
		if updateReleaseAlreadyCurrent(model.updates.check, selected) {
			if selected.Channel == domain.UpdateChannelMoving {
				model.message = target + " already points to the current upstream revision."
			} else {
				model.message = target + " is already the configured Nixorium release."
			}
			return model, nil
		}
		model.updates.target = target
		model.busy = "Checking the proposed update"
		model.message = ""
		allowPrerelease := selected.Channel == domain.UpdateChannelPrerelease
		if model.actions.PlanUpdateWithProgress != nil {
			model.updates.planning = true
			model.updates.planProgress = domain.UpdatePlanProgress{}
			model.updates.planStarted = time.Now().UTC()
			return model.startUpdatePlan(model.actions.PlanUpdateWithProgress, target, allowPrerelease, false)
		}
		return model.startBoundedRead(dashboardBuildTimeout, func(ctx context.Context) tea.Msg {
			return dashboardUpdatePlanMsg{report: model.actions.PlanUpdate(ctx, target, allowPrerelease, false)}
		})
	}
	return model, nil
}

func (model dashboardModel) updateUpdateReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	maximum := model.maximumUpdateScroll()
	switch key.String() {
	case "f4":
		model.updateDetails = !model.updateDetails
		if model.updates.plan.Workspace != nil {
			model.updates.scroll = 0
		}
	case "esc":
		model.screen = dashboardUpdate
		model.message = "Update cancelled; flake.nix and flake.lock were not changed."
	case "up":
		if model.updates.scroll > 0 {
			model.updates.scroll--
		}
	case "down":
		if model.updates.scroll < maximum {
			model.updates.scroll++
		}
	case "pgup":
		model.updates.scroll -= model.updateReviewHeight()
		if model.updates.scroll < 0 {
			model.updates.scroll = 0
		}
	case "pgdown":
		model.updates.scroll += model.updateReviewHeight()
		if model.updates.scroll > maximum {
			model.updates.scroll = maximum
		}
	case "enter":
		model.busy = "Saving the validated update"
		model.updates.applying = true
		model.message = ""
		plan := model.updates.plan
		return model, func() tea.Msg {
			return dashboardUpdateResultMsg{report: model.saveReviewedUpdate(plan)}
		}
	}
	return model, nil
}
