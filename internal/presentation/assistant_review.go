package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantEvaluationFailed = "Saving is not available: the proposal does not evaluate. Return to the chat and ask the assistant to fix it."

type assistantReviewModel struct {
	plan     domain.AssistantSavePlan
	result   domain.TemplateResetResult
	details  bool
	saveOnly bool
	cursor   int
	scroll   int
	saving   bool
	// changedClients holds, once saved, the client computers whose system
	// the reviewed change alters; nil when the review could not say.
	changedClients map[string]bool
}

// canSave reports whether the reviewed proposal may be saved.
func (r assistantReviewModel) canSave() bool {
	return r.plan.State == "ready" && r.plan.Evaluation.OK
}

// saveUnavailable explains, in plain words, why saving is disabled.
func (r assistantReviewModel) saveUnavailable() string {
	if !r.plan.Evaluation.OK {
		return assistantEvaluationFailed
	}
	return "Saving is not available: the proposal could not be prepared. Return to the chat and ask the assistant to try again."
}

type assistantSavePlanMsg struct{ plan domain.AssistantSavePlan }
type assistantSaveResultMsg struct{ result domain.TemplateResetResult }

func (model dashboardModel) openAssistantReview() (tea.Model, tea.Cmd) {
	actions := model.actions.AssistantService
	if model.assistant.changedFiles == 0 || model.assistant.conversation.ID == "" || actions == nil || actions.PlanSave == nil || actions.Save == nil {
		return model, nil
	}
	if model.report.Meta.Controller.Name == "" {
		return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) { return ready.openAssistantReview() })
	}
	model.assistant.review = assistantReviewModel{}
	model.message, model.navigation.pageScroll = "", 0
	model.busy = "Checking the proposal"
	ctx, id := model.beginRead(5 * time.Minute)
	model.screen = dashboardAssistantReview
	conversationID := model.assistant.conversation.ID
	return model, boundedReadCommand(ctx, id, func(ctx context.Context) tea.Msg { return assistantSavePlanMsg{actions.PlanSave(ctx, conversationID)} })
}
func (model dashboardModel) finishAssistantSave(msg assistantSaveResultMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardAssistantReview || !model.assistant.review.saving {
		return model, nil
	}
	model.assistant.review.saving = false
	model.assistant.review.result = msg.result
	model.busy, model.message = "", ""
	model.navigation.pageScroll = 0
	if msg.result.State == "saved" && !msg.result.RecoveryRequired {
		// Read before noteControllerSave forgets the reviewed inventory.
		model.assistant.review.changedClients = model.assistantChangedClients()
		model.noteControllerSave(msg.result.Revision)
		model.assistant.changedFiles = 0
		model.assistant.saved = true
	}
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.deployment.plan = domain.DeploymentPlanReport{}
	if !model.assistant.review.saveOnly && msg.result.State == "saved" && !msg.result.RecoveryRequired {
		// One Enter saved; now update the controller and then offer the
		// computers. Every stop falls back to the saved result screen.
		return model.continueSaved(true)
	}
	return model, nil
}

// assistantSavedDone reports a saved change with nothing left to do here:
// saved only, or applied to the controller and altering no client computer.
func (model dashboardModel) assistantSavedDone() bool {
	saved, revision := model.saveFollowupState()
	if !saved {
		return false
	}
	return model.assistant.review.saveOnly || (controllerVerifiedForSave(revision, model.controller.result) && model.assistantClientsUnchanged())
}

func (model dashboardModel) assistantBackToChat() (tea.Model, tea.Cmd) {
	model.screen = dashboardAssistant
	model.message = ""
	model.navigation.pageScroll = 0
	return model, model.assistant.input.Focus()
}

func (model dashboardModel) startAssistantSave(saveOnly bool) (tea.Model, tea.Cmd) {
	r := &model.assistant.review
	r.saving, r.saveOnly = true, saveOnly
	model.busy, model.message = "Creating the backup and saving the reviewed proposal", ""
	plan, action := r.plan, model.actions.AssistantService.Save
	return model, func() tea.Msg { return assistantSaveResultMsg{action(context.Background(), plan)} }
}
func (model dashboardModel) updateAssistantReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := &model.assistant.review
	if r.saving {
		return model, nil
	}
	if key.String() == "esc" {
		if r.details {
			r.details = false
			model.message, model.navigation.pageScroll = "", 0
			return model, nil
		}
		return model.assistantBackToChat()
	}
	if r.result.State != "" {
		if key.String() == "r" && r.result.RecoveryRequired {
			return model.openResetRecovery()
		}
		if key.String() == "enter" {
			if model.assistantSavedDone() {
				return model.assistantBackToChat()
			}
			return model.continueSavedConfiguration()
		}
		return model, nil
	}
	if !r.details {
		switch key.String() {
		case "up":
			r.cursor = (r.cursor + 3) % 4
		case "down":
			r.cursor = (r.cursor + 1) % 4
		case "enter":
			switch r.cursor {
			case 0, 1:
				if r.canSave() {
					return model.startAssistantSave(r.cursor == 1)
				}
				model.message = r.saveUnavailable()
			case 2:
				r.details, r.scroll = true, 0
			case 3:
				model.screen = dashboardAssistant
				return model, model.assistant.input.Focus()
			}
		}
		return model, nil
	}
	switch key.String() {
	case "up":
		r.scroll--
	case "down":
		r.scroll++
	case "pgup":
		r.scroll -= model.assistantReviewHeight()
	case "pgdown":
		r.scroll += model.assistantReviewHeight()
	case "home":
		r.scroll = 0
	case "end":
		r.scroll = len(model.assistantReviewLines())
	}
	r.scroll = max(0, min(r.scroll, len(model.assistantReviewLines())-model.assistantReviewHeight()))
	return model, nil
}
func (model dashboardModel) assistantReviewHeight() int {
	return max(3, model.height-ctaRows(model.height)-10)
}

func assistantProposalChanges(summary string, plan domain.AssistantSavePlan) []string {
	lines := strings.Split(assistantDisplayText(summary), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "Changes:" {
			continue
		}
		points := []string{}
		for _, next := range lines[i+1:] {
			next = strings.TrimSpace(next)
			if next == "" {
				continue
			}
			if !strings.HasPrefix(next, "- ") && !strings.HasPrefix(next, "* ") {
				break
			}
			if point := strings.TrimSpace(next[2:]); point != "" {
				points = append(points, point)
			}
			if len(points) == 4 {
				break
			}
		}
		if len(points) > 0 {
			return points
		}
	}
	points := []string{}
	for _, group := range []struct {
		paths  []string
		action string
	}{{plan.Added, "added"}, {plan.Changed, "changed"}, {plan.Removed, "removed"}} {
		for _, path := range group.paths {
			points = append(points, path+" "+group.action)
		}
	}
	return points
}

func (model dashboardModel) assistantProposalHosts() string {
	controller := model.report.Meta.Controller.Name
	clients := []string{}
	hasController := false
	for _, host := range model.assistant.review.plan.ChangedHosts {
		if controller != "" && host == controller {
			hasController = true
		} else {
			clients = append(clients, host)
		}
	}
	text := assistantCompactHosts(clients, controller)
	if hasController {
		if text != "" {
			text += ", "
		}
		text += "the controller (" + controller + ")"
	}
	if text == "" {
		return "No computer changes"
	}
	return text
}

// The last turn ended without a final answer: the files may be half changed.
const (
	assistantUnfinishedBanner = "Proposal incomplete · "
	assistantUnfinishedReview = "⚠ Assistant stopped before finishing: this proposal may be incomplete. Check what changes, or return to the chat and ask Assistant to continue."
)

const assistantCoreOverrideWarning = "⚠ Core override: this proposal forces NixOS options set by Nixorium. It is unsupported and may stop working after a Nixorium update. Update one computer first; if it does not boot or loses the network, pick the previous generation in the GRUB menu on that computer."

// assistantCoreOverrideNotice is empty unless the plan lists files that
// mention mkForce or mkOverride; it only informs and never blocks saving.
func (model dashboardModel) assistantCoreOverrideNotice() string {
	files := model.assistant.review.plan.CoreOverrideFiles
	if len(files) == 0 {
		return ""
	}
	text := assistantCoreOverrideWarning + "\nFiles with overrides: " + strings.Join(files, ", ")
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(model.isDark).attention).Render(text)
}

// assistantVerifiedAge renders how long ago a reused evaluation ran.
func assistantVerifiedAge(age time.Duration) string {
	switch {
	case age < time.Minute:
		return fmt.Sprintf("%d s", max(0, int(age/time.Second)))
	case age < time.Hour:
		return fmt.Sprintf("%d min", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%d h", int(age/time.Hour))
	default:
		return fmt.Sprintf("%d d", int(age/(24*time.Hour)))
	}
}
func (model dashboardModel) assistantReviewLines() []string {
	p := model.assistant.review.plan
	hosts := strings.Join(p.ChangedHosts, ", ")
	if hosts == "" {
		hosts = "None"
	}
	evaluation := "passed"
	if !p.Evaluation.OK {
		evaluation = "failed\n" + p.Evaluation.Error
	}
	override := ""
	if len(p.CoreOverrideFiles) > 0 {
		override = assistantCoreOverrideWarning + "\nFiles with overrides: " + strings.Join(p.CoreOverrideFiles, ", ") + "\n\n"
	}
	settings := ""
	if lines := p.SettingsReviewLines(); len(lines) > 0 {
		settings = strings.Join(lines, "\n") + "\n"
	}
	content := fmt.Sprintf("Summary from the assistant\n%s\n\n%sChecked by Nixorium\nFiles: %d added, %d changed, %d removed\nComputers whose system changes: %s\nNix evaluation: %s\n%s\nFull comparison\n%s", assistantDisplayText(model.assistant.summary), override, len(p.Added), len(p.Changed), len(p.Removed), hosts, evaluation, settings, p.Diff)
	return strings.Split(lipgloss.NewStyle().Width(max(20, min(106, model.width-10))).Render(assistantLinkify(assistantDisplayText(content))), "\n")
}
func (model dashboardModel) assistantReviewView() string {
	r := model.assistant.review
	body, fixed := "", ""
	actions := []tuiAction{{key: "Esc", label: "Chat"}, {key: "F1", label: "Help"}}
	switch {
	case model.busy != "":
		body = model.busyView()
		if r.saving {
			actions = []tuiAction{{key: "F1", label: "Help"}}
		}
	case r.result.State != "":
		body = r.result.Message
		if r.result.State == "saved" && !r.result.RecoveryRequired {
			switch {
			case r.saveOnly:
				body = "Saved in a local commit. Backup: " + r.result.BackupRef + ". Computers were not updated.\nYou can update them later from Computers → Update computers: try one computer first, check it, then update the others."
			case controllerVerifiedForSave(r.result.Revision, model.controller.result) && model.assistantClientsUnchanged():
				body = "Saved in a local commit. Backup: " + r.result.BackupRef + ". The controller is up to date.\nThe client computers do not change with this save: they need no update."
			case controllerVerifiedForSave(r.result.Revision, model.controller.result):
				body = "Saved in a local commit. Backup: " + r.result.BackupRef + ". The controller is up to date.\nUpdate the computers from Computers → Update computers: try one computer first, check it, then update the others."
			default:
				body = "Saved in a local commit. Backup: " + r.result.BackupRef + ". The controller was not updated.\nTry it on one computer first: Computers → Update computers, select a single computer, check it, then update the others."
			}
		} else if r.result.BackupRef != "" {
			body += "\nBackup: " + r.result.BackupRef
		}
		actions = model.saveFollowupActions("Chat")
		if model.assistantSavedDone() {
			actions = []tuiAction{{key: "Enter", label: "Chat"}, {key: "F1", label: "Help"}}
		}
		if r.result.RecoveryRequired {
			actions = append([]tuiAction{{key: "r", label: "Recover"}}, actions...)
		}
	case r.plan.State == "blocked":
		body = r.plan.Message
	case r.details:
		lines := model.assistantReviewLines()
		start := max(0, min(r.scroll, len(lines)-model.assistantReviewHeight()))
		end := min(len(lines), start+model.assistantReviewHeight())
		body = strings.Join(lines[start:end], "\n")
		fixed = fmt.Sprintf("Lines %d–%d / %d", start+1, end, len(lines))
		if !r.plan.Evaluation.OK {
			fixed += "\n" + assistantEvaluationFailed
		}
		actions = []tuiAction{{key: "↑/↓ PgUp/PgDn Home/End", label: "Scroll"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	default:
		body = "What changes\n"
		for _, point := range assistantProposalChanges(model.assistant.summary, r.plan) {
			body += "• " + point + "\n"
		}
		body += "\nComputers affected\n" + model.assistantProposalHosts()
		if lines := r.plan.SettingsReviewLines(); len(lines) > 0 {
			body += "\n\n" + strings.Join(lines, "\n")
		}
		if notice := model.assistantCoreOverrideNotice(); notice != "" {
			fixed = notice + "\n"
		}
		if model.assistant.unfinished {
			fixed += assistantUnfinishedReview + "\n"
		}
		switch {
		case !r.plan.Evaluation.OK:
			fixed += "✗ The configuration is not valid\n" + assistantEvaluationFailed + " Technical details show the error."
		case r.plan.EvaluationReused && !r.plan.EvaluationVerifiedAt.IsZero():
			fixed += "✓ Checked by Nixorium: the configuration is valid (verified " + assistantVerifiedAge(time.Since(r.plan.EvaluationVerifiedAt)) + " ago)"
		default:
			fixed += "✓ Checked by Nixorium: the configuration is valid"
		}
		fixed += "\nSaving creates a local commit and a backup. Enter on the first choice also updates this controller; you then choose the computers.\n"
		for i, label := range []string{"Save and update the controller", "Save only — update computers later", "Technical details", "Back"} {
			row := tuiSelection(label, r.cursor == i, model.isDark)
			if i < 2 && !r.canSave() {
				row = tuiMuted(row, model.isDark)
			}
			fixed += row + "\n"
		}
		fixed += tuiMuted("↑↓ choose · Enter confirm · Esc back to the chat", model.isDark)
		actions = nil
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Assistant", "Review the proposal"}, body: body, fixedBody: fixed, notices: notices, actions: actions})
}
