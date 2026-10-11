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

const assistantSavePrompt = "Save? A local commit and a backup are created. Nothing is installed."
const assistantEvaluationFailed = "Saving is not available: the proposal does not evaluate. Return to the chat and ask the assistant to fix it."

type assistantReviewModel struct {
	plan       domain.AssistantSavePlan
	result     domain.TemplateResetResult
	confirming bool
	details    bool
	cursor     int
	scroll     int
	saving     bool
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
	model.message, model.pageScroll = "", 0
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
	model.pageScroll = 0
	if msg.result.State == "saved" && !msg.result.RecoveryRequired {
		model.noteControllerSave(msg.result.Revision)
		model.assistant.changedFiles = 0
	}
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.deployment.plan = domain.DeploymentPlanReport{}
	return model, nil
}
func (model dashboardModel) updateAssistantReviewKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := &model.assistant.review
	if r.saving {
		return model, nil
	}
	if key.String() == "esc" {
		if r.details || r.confirming {
			r.details, r.confirming = false, false
			model.message, model.pageScroll = "", 0
			return model, nil
		}
		model.screen = dashboardAssistant
		model.message = ""
		model.pageScroll = 0
		return model, model.assistant.input.Focus()
	}
	if r.result.State != "" {
		if key.String() == "r" && r.result.RecoveryRequired {
			return model.openResetRecovery()
		}
		if key.String() == "enter" {
			return model.continueSavedConfiguration()
		}
		return model, nil
	}
	if r.confirming {
		if key.String() != "enter" || r.plan.State != "ready" || !r.plan.Evaluation.OK {
			return model, nil
		}
		r.saving, r.confirming = true, false
		model.busy, model.message = "Creating the backup and saving the reviewed proposal", ""
		plan, action := r.plan, model.actions.AssistantService.Save
		return model, func() tea.Msg { return assistantSaveResultMsg{action(context.Background(), plan)} }
	}
	if !r.details {
		switch key.String() {
		case "up":
			r.cursor = (r.cursor + 2) % 3
		case "down":
			r.cursor = (r.cursor + 1) % 3
		case "enter":
			switch r.cursor {
			case 0:
				if r.plan.State == "ready" && r.plan.Evaluation.OK {
					r.confirming = true
				}
			case 1:
				r.details, r.scroll = true, 0
			case 2:
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
func (model dashboardModel) assistantReviewHeight() int { return max(3, model.height-10) }

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
	content := fmt.Sprintf("Summary from the assistant\n%s\n\nChecked by Nixorium\nFiles: %d added, %d changed, %d removed\nComputers whose system changes: %s\nNix evaluation: %s\n\nFull comparison\n%s", assistantDisplayText(model.assistant.summary), len(p.Added), len(p.Changed), len(p.Removed), hosts, evaluation, p.Diff)
	return strings.Split(lipgloss.NewStyle().Width(max(20, min(106, model.width-10))).Render(assistantDisplayText(content)), "\n")
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
			body = "Saved in a local commit. Backup: " + r.result.BackupRef + ". Nothing was built or activated.\nTry it on one computer first: Computers → Update computers, select a single computer, check it, then update the others."
		} else if r.result.BackupRef != "" {
			body += "\nBackup: " + r.result.BackupRef
		}
		actions = model.saveFollowupActions("Chat")
		if r.result.RecoveryRequired {
			actions = append([]tuiAction{{key: "r", label: "Recover"}}, actions...)
		}
	case r.plan.State == "blocked":
		body = r.plan.Message
	case r.confirming:
		body = assistantSavePrompt
		fixed = tuiMuted("Enter Save · Esc Cancel", model.isDark)
		actions = nil
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
		fixed = "✓ Checked by Nixorium: the configuration is valid"
		if !r.plan.Evaluation.OK {
			fixed = "✗ The configuration is not valid"
		}
		fixed += "\nNothing is installed yet. After saving, update one computer first to try it.\n"
		for i, label := range []string{"Save this proposal", "Show technical details", "Back to the chat"} {
			row := tuiSelection(label, r.cursor == i, model.isDark)
			if i == 0 && !r.plan.Evaluation.OK {
				row = tuiMuted(row, model.isDark)
			}
			fixed += row + "\n"
		}
		fixed += tuiMuted("↑↓ choose · Enter confirm · Esc back", model.isDark)
		actions = nil
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Assistant", "Review the proposal"}, body: body, fixedBody: fixed, notices: notices, actions: actions})
}
