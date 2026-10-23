package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateView() string {
	path := []string{"Maintenance", model.updateTitle()}
	if model.controller.applying {
		return model.controllerProgressView(path)
	}
	lines := []string{tuiTitle(model.updateTitle(), model.isDark), ""}
	if model.busy != "" {
		if model.updates.planning {
			lines = append(lines, "Target: "+model.updates.target)
			phaseLabels := updatePlanPhaseLabels()
			phaseIndex := updatePlanPhaseIndex(model.updates.planProgress.Phase)
			if model.height > 0 && model.height < 28 {
				lines = append(lines, "", fmt.Sprintf("Phase %d/%d · %s", phaseIndex+1, len(phaseLabels), phaseLabels[phaseIndex]))
			} else {
				lines = append(lines, phaseSteps(phaseLabels, phaseIndex, false, model.isDark)...)
			}
			model.busy = updatePlanProgressDescription(model.updates.planProgress)
			if model.updates.packageBase && model.updates.planProgress.Phase == domain.UpdatePlanPhaseLock {
				model.busy = "Preparing the selected system and package base"
			}
			lines = append(lines, "", model.busyView())
			if model.updates.planProgress.Total > 0 {
				if model.updates.planProgress.Current == 0 {
					lines = append(lines, fmt.Sprintf("Required outputs: %d · checking together", model.updates.planProgress.Total))
				} else {
					lines = append(lines, fmt.Sprintf("Safety check %d/%d", model.updates.planProgress.Current, model.updates.planProgress.Total))
				}
			}
			if model.progressDetails && model.updatePlanDetailsAvailable() {
				lines = append(lines, "", tuiSection("Current check details: ", model.isDark)+model.updates.planProgress.Detail)
			}
		} else {
			lines = append(lines, model.busyView())
		}
		notices := []tuiNotice{}
		if model.updates.applying {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "Update save is running", detail: "Wait for the atomic two-file result before closing Nixorium."})
		} else {
			notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: "The current deployment remains unchanged", detail: "Downloads and local builds can take several minutes."})
		}
		actions := []tuiAction{{key: "F1", label: "Help"}}
		if model.updates.planning && model.updatePlanDetailsAvailable() {
			actions = append([]tuiAction{{key: "l", label: "Progress details"}}, actions...)
		}
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
	}
	if model.updates.result.Operation != "" {
		success := !model.updates.result.HasErrors() && model.updates.result.Updated && !model.updates.result.RecoveryRequired && controllerVerifiedForSave(model.updates.result.Revision, model.controller.result)
		title := "Nixorium update needs attention"
		if success {
			title = "Nixorium and this controller are updated"
		}
		if model.updates.packageBase {
			title = "System update needs attention"
			if success {
				title = "Controller configuration activated and verified"
			}
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
		)
		saveState := model.updates.result.State
		if model.updates.result.Updated && !model.updates.result.HasErrors() {
			saveState = "saved"
		}
		lines = append(lines, saveStatusLines(saveState, model.updates.result.RecoveryRequired, true, true, success)...)
		lines = append(lines, "", "Configured target: "+model.updates.result.Target, "Running interface: "+displayRunningVersion(model.actions.RunningVersion))
		if success && model.updates.packageBase {
			lines = append(lines, "Check boot, networking, desktop and services before client distribution; a reboot may be needed.")
		} else if success {
			lines = append(lines, model.reload.line())
		} else if model.updates.result.RecoveryRequired {
			lines = append(lines, "Files were written but saving needs recovery. Complete the save before controller activation.")
		} else if model.updates.result.Updated && !model.updates.result.HasErrors() {
			lines = append(lines, "The update is saved safely. Retry controller activation after resolving the detail below.")
		}
		if model.holdsInterruptedDeployment() {
			lines = append(lines, tuiMuted(heldDeploymentLine, model.isDark))
		}
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, failureNotice(model.updateFailurePrefix(), model.message))
		}
		notices = append(notices, model.heldRecordNotices()...)
		retry := tuiAction{key: "n", label: "New update"}
		if model.updates.result.RecoveryRequired {
			retry = tuiAction{key: "s", label: "Complete save"}
		}
		actions := []tuiAction{}
		if success {
			actions = append(actions, tuiAction{key: "Enter", label: "Update computers"})
		}
		if model.updates.result.Updated && !model.updates.result.HasErrors() && !model.updates.result.RecoveryRequired && !success {
			actions = append(actions, tuiAction{key: "Enter", label: "Retry controller"}, tuiAction{key: "a", label: "Retry controller"})
		}
		actions = append(actions, retry, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
		if success && !model.updates.packageBase && model.reload.offered() {
			actions = model.reloadActions()
		}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: actions})
	}
	if model.screen == dashboardUpdateReview {
		return model.releaseReviewView()
	}
	if model.updates.packageBase {
		return model.packageBaseView()
	}
	if model.updates.check.HasErrors() {
		failureTitle, failureDetail := updateCheckFailureSummary(model.updates.check)
		lines = append(lines,
			tuiResult(failureTitle, false, model.isDark),
			"",
			failureDetail,
			"No update can be selected until this check succeeds. No files changed.",
		)
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "r", label: "Try again"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}})
	}

	releases := model.availableUpdateReleases()
	runningLine := "Running interface   " + displayRunningVersion(model.actions.RunningVersion)
	if floor := strings.TrimPrefix(model.updates.check.VersionFloor, "v"); floor != "" {
		runningLine = "Running version     " + floor + " (older releases are hidden)"
	}
	lines = append(lines,
		fmt.Sprintf("Configured target   %s", model.updates.check.CurrentRef),
		runningLine,
		tuiMuted("Source  "+model.updates.check.Upstream, model.isDark),
		"",
		tuiSection("Available updates", model.isDark),
	)
	start, end := listWindow(len(releases), model.updates.cursor, max(4, model.height-ctaRows(model.height)-15))
	latestStable := ""
	if len(model.updates.check.Stable) > 0 {
		latestStable = model.updates.check.Stable[0].Tag
	}
	for index := start; index < end; index++ {
		release := releases[index]
		note := updateReleaseStatus(model.updates.check, release)
		if note == "" && release.Tag == latestStable {
			note = "  Latest stable"
		}
		if release.Channel == domain.UpdateChannelPrerelease {
			note += "  Prerelease"
		} else if release.Channel == domain.UpdateChannelMoving {
			note += "  Development branch"
		}
		lines = append(lines, tuiSelection(release.Tag, index == model.updates.cursor, model.isDark)+tuiMuted(note, model.isDark))
	}
	if len(releases) == 0 {
		lines = append(lines, "No updates are available in the selected channel.")
	}
	if model.updates.check.Truncated {
		lines = append(lines, "", tuiMuted("The upstream result was safely limited to the newest releases.", model.isDark))
	}
	prereleaseLabel := "Show prereleases"
	if model.updates.prerelease {
		prereleaseLabel = "Hide prereleases"
	}
	lines = append(lines,
		"",
		"Select master for the latest development revision, or choose a tagged release.",
		"Selection starts validation; it does not change files.",
		"A confirmed update activates this controller; client distribution remains separate.",
	)
	notices := []tuiNotice{}
	if model.updates.check.PinOutdated != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "The saved Nixorium version is recorded inconsistently. Select the current version (or a newer one) to repair it before updating the computers."})
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if len(releases) > 0 {
		actions = append(actions, tuiAction{key: "↑/↓", label: "Select"}, tuiAction{key: "Enter", label: "Validate"})
	}
	actions = append(actions, tuiAction{key: "p", label: prereleaseLabel}, tuiAction{key: "r", label: "Fetch again"}, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func updatePlanPhaseLabels() []string {
	return []string{
		"Check current deployment",
		"Prepare selected version",
		"Check laboratory configuration",
		"Test systems before saving",
		"Prepare review",
		"Confirm files remain unchanged",
	}
}

func updatePlanProgressDescription(progress domain.UpdatePlanProgress) string {
	switch progress.Phase {
	case domain.UpdatePlanPhaseInspect:
		return "Checking this deployment and the selected version"
	case domain.UpdatePlanPhaseLock:
		return "Preparing the selected Nixorium version"
	case domain.UpdatePlanPhaseEvaluate:
		return "Checking the laboratory configuration"
	case domain.UpdatePlanPhaseBuild:
		if progress.Current == 0 && progress.Total > 1 {
			return "Testing the required systems and installation files"
		}
		detail := strings.ToLower(progress.Detail)
		switch {
		case strings.Contains(detail, "representative client"):
			return "Testing a client computer system"
		case strings.Contains(detail, "controller"):
			return "Testing the controller system"
		case strings.Contains(detail, "netboot"):
			return "Testing the network installer"
		case strings.Contains(detail, "pxe firmware"):
			return "Testing computer network boot"
		case strings.Contains(detail, "offline installer"):
			return "Testing the offline installer"
		default:
			return "Testing a required system before saving"
		}
	case domain.UpdatePlanPhaseReview:
		return "Preparing the update review"
	case domain.UpdatePlanPhaseVerify:
		return "Confirming that deployment files did not change"
	default:
		return "Starting the update safety checks"
	}
}

func updatePlanPhaseIndex(phase domain.UpdatePlanPhase) int {
	switch phase {
	case domain.UpdatePlanPhaseLock:
		return 1
	case domain.UpdatePlanPhaseEvaluate:
		return 2
	case domain.UpdatePlanPhaseBuild:
		return 3
	case domain.UpdatePlanPhaseReview:
		return 4
	case domain.UpdatePlanPhaseVerify:
		return 5
	default:
		return 0
	}
}

func displayRunningVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

func updateCheckFailureSummary(report domain.UpdateCheckReport) (string, string) {
	if len(report.Issues) > 0 {
		switch report.Issues[0].Field {
		case "input", "repository":
			return "Update setup needs attention", "Nixorium could not read a supported update source from this deployment."
		case "releases":
			return "No supported updates found", "The configured upstream responded but did not advertise a supported branch or release."
		}
	}
	return "Updates could not be fetched", "Nixorium could not obtain an update list from the configured upstream."
}

func updateReleaseAlreadyCurrent(report domain.UpdateCheckReport, release domain.UpdateRelease) bool {
	// An outdated saved record is repaired by planning the same version again.
	if release.Tag != report.CurrentRef || report.PinOutdated != "" {
		return false
	}
	if release.Channel != domain.UpdateChannelMoving {
		return true
	}
	return report.CurrentRev != "" && release.ObjectID != "" && report.CurrentRev == release.ObjectID
}

func updateReleaseStatus(report domain.UpdateCheckReport, release domain.UpdateRelease) string {
	if release.Tag != report.CurrentRef {
		return ""
	}
	if report.PinOutdated != "" {
		return "  Current · select to repair"
	}
	if release.Channel != domain.UpdateChannelMoving {
		return "  Current"
	}
	if updateReleaseAlreadyCurrent(report, release) {
		return "  Current revision"
	}
	if report.CurrentRev != "" && release.ObjectID != "" {
		return "  New revision available"
	}
	return "  Configured target"
}

func (model dashboardModel) availableUpdateReleases() []domain.UpdateRelease {
	releases := append([]domain.UpdateRelease{}, model.updates.check.Development...)
	releases = append(releases, model.updates.check.Stable...)
	if model.updates.prerelease {
		releases = append(releases, model.updates.check.Prerelease...)
	}
	available := releases[:0]
	for _, release := range releases {
		if !release.Downgrade {
			available = append(available, release)
		}
	}
	return available
}

func (model dashboardModel) checkUpdates() (tea.Model, tea.Cmd) {
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardUpdateCheckMsg{report: model.actions.CheckUpdate(ctx)}
	})
}

func (model dashboardModel) startUpdateControllerApply() (tea.Model, tea.Cmd) {
	if model.actions.PlanController == nil || model.actions.ApplyController == nil {
		model.message = "Controller activation is not available. The validated update remains saved."
		return model, nil
	}
	model.busy = "Building, activating, and verifying the updated controller"
	model.updates.applying = true
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
	operation := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer cancel()
		plan := model.actions.PlanController(ctx)
		if plan.HasErrors() {
			return dashboardUpdateControllerMsg{plan: plan}
		}
		report := model.actions.ApplyController(plan)
		return dashboardUpdateControllerMsg{plan: plan, report: report, recovery: model.recoveryAfterControllerApply()}
	}
	command := model.trackControllerApply(operation)
	return model, command
}

func (model dashboardModel) updateReviewHeight() int {
	if model.height <= 0 {
		return 10
	}
	if model.updates.plan.Workspace != nil {
		height := model.height - ctaRows(model.height) - 16
		if model.message != "" {
			height -= 4
		}
		return max(1, height)
	}
	height := model.height - ctaRows(model.height) - 21
	if model.updates.packageBase {
		height -= 4
	}
	if model.updateDetails {
		height -= len(model.updates.plan.Checks) + 2
	}
	return max(1, height)
}

func (model dashboardModel) maximumUpdateScroll() int {
	maximum := len(model.updateReviewContent()) - model.updateReviewHeight()
	if maximum < 0 {
		return 0
	}
	return maximum
}
