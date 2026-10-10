package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantSavePrompt = "Type SAVE to record this proposal in a local commit with a backup. Nothing is built or activated."
const assistantEvaluationFailed = "Saving is not available: the proposal does not evaluate. Return to the chat and ask the assistant to fix it."

type assistantReviewModel struct {
	plan         domain.AssistantSavePlan
	result       domain.TemplateResetResult
	confirmation string
	scroll       int
	saving       bool
}
type assistantSavePlanMsg struct{ plan domain.AssistantSavePlan }
type assistantSaveResultMsg struct{ result domain.TemplateResetResult }

func (model dashboardModel) openAssistantReview() (tea.Model, tea.Cmd) {
	actions := model.actions.AssistantService
	if model.assistant.changedFiles == 0 || model.assistant.conversation.ID == "" || actions == nil || actions.PlanSave == nil || actions.Save == nil {
		return model, nil
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
	case "backspace":
		if len(r.confirmation) > 0 {
			r.confirmation = r.confirmation[:len(r.confirmation)-1]
		}
	case "enter":
		if r.plan.State != "ready" || !r.plan.Evaluation.OK {
			return model, nil
		}
		if r.confirmation != "SAVE" {
			model.message = "Type SAVE exactly after reviewing the proposal."
			return model, nil
		}
		r.saving = true
		model.busy, model.message = "Creating the backup and saving the reviewed proposal", ""
		plan, action := r.plan, model.actions.AssistantService.Save
		return model, func() tea.Msg { return assistantSaveResultMsg{action(context.Background(), plan)} }
	default:
		if r.plan.State == "ready" && r.plan.Evaluation.OK && key.Text != "" && len(r.confirmation)+len(key.Text) <= 32 && strings.IndexFunc(key.Text, unicode.IsControl) < 0 {
			r.confirmation += key.Text
		}
	}
	r.scroll = max(0, min(r.scroll, len(model.assistantReviewLines())-model.assistantReviewHeight()))
	return model, nil
}
func (model dashboardModel) assistantReviewHeight() int { return max(3, model.height-14) }
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
	default:
		lines := model.assistantReviewLines()
		start := max(0, min(r.scroll, len(lines)-model.assistantReviewHeight()))
		end := min(len(lines), start+model.assistantReviewHeight())
		body = strings.Join(lines[start:end], "\n")
		fixed = fmt.Sprintf("Lines %d–%d / %d\n", start+1, end, len(lines))
		if r.plan.Evaluation.OK {
			fixed += assistantSavePrompt + "\n› " + r.confirmation
			actions = append([]tuiAction{{key: "Enter", label: "Save"}}, actions...)
		} else {
			fixed += assistantEvaluationFailed
		}
		actions = append([]tuiAction{{key: "↑/↓ PgUp/PgDn Home/End", label: "Scroll"}}, actions...)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Assistant", "Proposal"}, body: body, fixedBody: fixed, notices: notices, actions: actions})
}
