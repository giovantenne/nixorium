package presentation

import (
	"fmt"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) deployView() string {
	path := []string{"Computers", "Update computers"}
	shell := tuiShell{path: path}
	if model.deployment.usbRecovery != nil {
		return model.deploymentUSBRecoveryView(shell)
	}
	if model.deployment.applying {
		if model.deployment.stopReview {
			return model.deploymentStopView()
		}
		lines := []string{
			tuiTitle("Distributing the prepared system", model.isDark),
			"",
			model.busyView(),
		}
		lines = append(lines, model.deploymentProgressView()...)
		shell.body = strings.Join(lines, "\n")
		shell.notices = []tuiNotice{{
			kind:   tuiStatusAttention,
			title:  "Deployment is running",
			detail: "Detailed output is saved in the private log. Closing is disabled until this foreground operation returns.",
		}}
		if model.deployment.stopRequested {
			shell.notices[0].detail = "Stopping local supervision and checking client state. Remote activation may continue; do not start another deployment."
		}
		shell.actions = nil
		if model.deploymentDetailsAvailable() {
			shell.actions = append(shell.actions, tuiAction{key: "l", label: "Progress details"})
		}
		if !model.deployment.stopRequested && model.deployment.cancel != nil {
			shell.actions = append(shell.actions, tuiAction{key: "s", label: "Stop waiting"})
		}
		shell.actions = append(shell.actions, tuiAction{key: "F1", label: "Help"})
		return model.renderShell(shell)
	}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	if model.deployment.context != "" && model.deployment.result.Operation == "" {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusNeutral, title: model.deployment.context})
	}
	if model.screen == dashboardDeployReview {
		lines := []string{
			tuiTitle("Update these computers?", model.isDark),
			"",
			fmt.Sprintf("Affects  %s · %s", strings.ReplaceAll(model.deployment.plan.ColmenaSelector, ",", ", "), countNoun(len(model.deployment.plan.Targets), "computer")),
			fmt.Sprintf("Builds   %s", deploymentConfigurationsLabel(model.deployment.plan.Targets)),
			"",
			tuiMuted("Reviewed revision  "+model.deployment.plan.Revision, model.isDark),
		}
		lines = append(lines, "", "Selected computers — latest availability check:")
		for _, target := range model.deployment.plan.Targets {
			lines = append(lines, fmt.Sprintf("%s  %s  %s", target.Name, target.IP, deploymentAvailabilityLabel(model.deployment.plan, target.Name, target.IP)))
		}
		off := deploymentOffComputers(model.deployment.plan)
		queueable := len(off) > 0 && model.actions.ApplyDeploymentQueued != nil
		if queueable {
			mark := "[ ]"
			if model.deployment.queueOff {
				mark = "[x]"
			}
			lines = append(lines, "", tuiSelection(mark+" Update "+strings.Join(off, ", ")+" when switched on", true, model.isDark),
				tuiMuted("    The controller updates each of them as soon as it answers, and its user sees a notification.", model.isDark))
		}
		shell.body = strings.Join(lines, "\n")
		shell.fixedBody = tuiSection("Enter updates these computers.", model.isDark)
		shell.notices = append(shell.notices, tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Programs on the computers may restart during the update",
			detail: deploymentUnreachableNote(queueable) + " If an update is interrupted, recovery may be needed before the next one.",
		})
		if model.message != "" {
			shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Update computers"}, {key: "Esc", label: "Selection"}, {key: "F1", label: "Help"}}
		if queueable {
			shell.actions = append([]tuiAction{{key: "F3", label: "Off computers: later / skip"}}, shell.actions...)
		}
		if model.actions.PlanReachableDeployment != nil && model.deployment.plan.ReachableRequested != "" {
			shell.actions = append([]tuiAction{{key: "F2", label: "Reachable only"}}, shell.actions...)
		}
		return model.renderShell(shell)
	}

	if model.deployment.result.Operation != "" {
		success := !model.deployment.result.HasErrors()
		summary := model.deployment.result.ResultSummary()
		lines := []string{
			tuiResult(deploymentResultHeadline(summary, success), success, model.isDark),
		}
		if line := deploymentResultCounts(summary); line != "" && !success {
			lines = append(lines, "", line)
		}
		for _, computer := range summary.Computers {
			lines = append(lines, "", computer.Name+" — "+computer.Outcome, "  "+computer.Guidance)
		}
		// A full success is already the green headline; the notice would
		// only repeat it.
		repeatsHeadline := success && len(model.deployment.result.Skipped) == 0
		if (model.message != "" && !repeatsHeadline) || model.deployment.result.RecoveryRequired {
			shell.notices = append(shell.notices, model.deploymentResultNotice())
		}
		shell.body = strings.Join(lines, "\n")
		shell.actions = []tuiAction{{key: "n", label: "New review"}, {key: "l", label: "Logs"}, {key: "Enter", label: "Computers"}, {key: "F1", label: "Help"}}
		if model.deployment.result.RecoveryRequired {
			// No new review until the interrupted one is reviewed.
			shell.actions = shell.actions[1:]
		}
		return model.renderShell(shell)
	}

	lines := []string{
		tuiTitle("Update computers", model.isDark),
		tuiMuted("Select → Review → Deploy → Verify", model.isDark),
		"",
	}
	hosts := model.report.Meta.Clients.Hosts
	selected := 0
	for _, host := range hosts {
		if model.deployment.chosen[host.Name] {
			selected++
		}
	}
	lines = append(lines, "Choose where to apply the saved configuration.", tuiMuted(observationHeading(model.computers.hosts.GeneratedAt), model.isDark), "", fmt.Sprintf("%d of %d computers selected", selected, len(hosts)), "")
	start, end := listWindow(len(hosts), model.deployment.cursor, max(3, model.height-ctaRows(model.height)-20))
	for index := start; index < end; index++ {
		host := hosts[index]
		checked := " "
		if model.deployment.chosen[host.Name] {
			checked = "x"
		}
		observed, found := model.observedHost(host.Name)
		lines = append(lines, tuiSelection(fmt.Sprintf("[%s] %-10s %s · %s", checked, host.Name, host.IP, deploymentSelectionLabel(model.deployment.plan, host, observed, found)), index == model.deployment.cursor, model.isDark))
	}
	if len(hosts) > end || start > 0 {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d", start+1, end, len(hosts)), model.isDark))
	}
	if len(hosts) == 0 {
		lines = append(lines, "No configured client computers.")
	}
	if model.message != "" {
		shell.notices = append(shell.notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	shell.body = strings.Join(lines, "\n")
	shell.actions = []tuiAction{{key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "n", label: "Those needing update"}, {key: "r", label: "Check computers"}, {key: "Enter", label: "Review"}, {key: "Esc", label: model.deployment.escLabel()}, {key: "F1", label: "Help"}}
	return model.renderShell(shell)
}

// deploymentResultHeadline leads with how many computers were updated when
// the whole update went through.
func deploymentResultHeadline(summary domain.DeploymentResultSummary, success bool) string {
	updated := 0
	for _, computer := range summary.Computers {
		if computer.Outcome == "Updated" {
			updated++
		}
	}
	if success && updated > 0 && updated == len(summary.Computers) {
		return countNoun(updated, "computer") + " updated"
	}
	return summary.Headline
}

// deploymentResultCounts summarizes the per-computer outcomes below it.
func deploymentResultCounts(summary domain.DeploymentResultSummary) string {
	counts, order := map[string]int{}, []string{}
	for _, computer := range summary.Computers {
		outcome := strings.ToLower(computer.Outcome)
		if counts[outcome] == 0 {
			order = append(order, outcome)
		}
		counts[outcome]++
	}
	parts := make([]string, 0, len(order))
	for _, outcome := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[outcome], outcome))
	}
	return strings.Join(parts, ", ")
}

// deploymentUnreachableNote says what happens to computers that did not
// answer the quick check.
func deploymentUnreachableNote(queueable bool) string {
	if queueable {
		return "Computers that did not answer are left unchanged, or updated when switched on if you choose that (F3)."
	}
	return "Computers that did not answer are left unchanged."
}

func (model dashboardModel) deploymentProgressView() []string {
	progressState := model.deployment.progress
	if progressState.Phase == "" {
		return []string{"", "  Waiting for deployment progress…"}
	}
	phaseLabels := map[domain.DeploymentPhase]string{
		domain.DeploymentPhasePreflight: "Revalidating review",
		domain.DeploymentPhaseBuild:     "Building configurations",
		domain.DeploymentPhaseApply:     "Applying configurations",
		domain.DeploymentPhaseVerify:    "Verifying computers",
		domain.DeploymentPhaseComplete:  "Complete",
	}
	index := 0
	switch progressState.Phase {
	case domain.DeploymentPhasePreflight:
		index = 1
	case domain.DeploymentPhaseBuild:
		index = 0
	case domain.DeploymentPhaseApply:
		index = 2
	case domain.DeploymentPhaseVerify:
		index = 3
	case domain.DeploymentPhaseComplete:
		index = 4
	}
	lines := phaseSteps([]string{"Building configurations", "Revalidating reviewed configuration", "Updating computers", "Verifying computers"}, index, progressState.Phase == domain.DeploymentPhaseComplete, model.isDark)
	expanded := model.progressDetails && model.deploymentDetailsAvailable()
	if expanded && len(progressState.Output) > 0 {
		// Show actual activity above the fold in an 80x24 terminal instead of
		// repeating the four-step overview and hiding the log below it.
		lines = []string{"", fmt.Sprintf("Phase: %s · %d/%d steps complete", phaseLabels[progressState.Phase], progressState.Completed, progressState.Total)}
	}
	if !progressState.LastOutputAt.IsZero() {
		quiet := max(time.Duration(0), time.Since(progressState.LastOutputAt).Truncate(time.Second))
		lines = append(lines, fmt.Sprintf("Last command output: %s ago", quiet))
		if quiet >= time.Minute {
			lines = append(lines, tuiMuted("No recent output; this alone does not mean the operation has stopped.", model.isDark))
		}
	}
	if !expanded {
		if progressState.TargetTotal > 0 {
			lines = append(lines, "", fmt.Sprintf("Computers checked: %d/%d", progressState.TargetCurrent, progressState.TargetTotal))
		}
		return lines
	}
	if len(progressState.Output) > 0 {
		lines = append(lines, "Recent command output (private log tail):")
		for _, line := range progressState.Output {
			lines = append(lines, "  "+line)
		}
		return lines
	}
	lines = append(lines, "", tuiSection("Current progress", model.isDark), "  Phase: "+phaseLabels[progressState.Phase])
	if progressState.Total > 0 {
		barWidth := model.width - 8
		if barWidth < 24 {
			barWidth = 24
		}
		if barWidth > 64 {
			barWidth = 64
		}
		bar := tuiProgress(barWidth, model.isDark)
		percentage := float64(progressState.Completed) / float64(progressState.Total)
		lines = append(lines,
			fmt.Sprintf("  %d/%d", progressState.Completed, progressState.Total),
			"  "+bar.ViewAs(percentage),
		)
	}
	if progressState.TargetTotal > 0 {
		lines = append(lines, fmt.Sprintf("  Computers checked: %d/%d", progressState.TargetCurrent, progressState.TargetTotal))
	}
	if len(model.deployment.recent) > 0 {
		lines = append(lines, "  Recent activity:")
		for _, activity := range model.deployment.recent {
			lines = append(lines, "    • "+activity)
		}
	}
	return lines
}

func hostAvailability(hosts []domain.HostStatus) (int, int) {
	available := 0
	for _, host := range hosts {
		if host.SSH == domain.SSHAvailable {
			available++
		}
	}
	return available, len(hosts)
}

func selectedDeploymentTargets(hosts []domain.HostMeta, chosen map[string]bool) string {
	selected := selectedDeploymentTargetNames(hosts, chosen)
	if len(selected) == len(hosts) && len(hosts) > 0 {
		return "@lab"
	}
	return strings.Join(selected, ",")
}

func selectedDeploymentTargetNames(hosts []domain.HostMeta, chosen map[string]bool) []string {
	selected := []string{}
	for _, host := range hosts {
		if chosen[host.Name] {
			selected = append(selected, host.Name)
		}
	}
	return selected
}

func toggleAllDeploymentTargets(hosts []domain.HostMeta, chosen map[string]bool) map[string]bool {
	allSelected := len(hosts) > 0
	for _, host := range hosts {
		if !chosen[host.Name] {
			allSelected = false
			break
		}
	}
	result := map[string]bool{}
	if !allSelected {
		for _, host := range hosts {
			result[host.Name] = true
		}
	}
	return result
}

func deploymentPlanIssues(report domain.DeploymentPlanReport) string {
	issues := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		issues = append(issues, issue.Field+": "+issue.Message)
	}
	return strings.Join(issues, "; ")
}
