package presentation

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardWorkspacePlanMsg struct {
	id      uint64
	initial bool
	report  domain.WorkspacePlanReport
}
type dashboardWorkspaceSaveMsg struct {
	id     uint64
	report domain.WorkspaceApplyReport
}

func (model dashboardModel) openWorkspace() (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode || model.actions.LoadWorkspace == nil || model.actions.PlanWorkspace == nil || model.actions.SaveWorkspace == nil {
		model.message = "Student workspace editing is not available in this session."
		return model, nil
	}
	model.workspace.cancelRead()
	id := model.workspace.requestID
	model.workspace = workspaceModel{requestID: id}
	model.screen = dashboardWorkspace
	model.message = ""
	return model.startWorkspaceRead(true)
}

func (model dashboardModel) startWorkspaceRead(initial bool) (tea.Model, tea.Cmd) {
	model.workspace.cancelRead()
	ctx, activityID := model.beginRead(dashboardReadTimeout)
	model.workspace.cancel = model.read.cancel
	id := model.workspace.requestID
	candidate := model.workspace.candidate
	model.message = ""
	model.busy = "Reviewing student preferences, pinned dependencies and destinations"
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg {
		var report domain.WorkspacePlanReport
		if initial {
			report = model.actions.LoadWorkspace(ctx)
		} else {
			report = model.actions.PlanWorkspace(ctx, candidate)
		}
		return dashboardWorkspacePlanMsg{id: id, initial: initial, report: report}
	})
}

func (model dashboardModel) finishWorkspacePlan(message dashboardWorkspacePlanMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardWorkspace || message.id != model.workspace.requestID {
		return model, nil
	}
	model.workspace.cancelRead()
	model.busy = ""
	report := message.report
	if message.initial {
		model.workspace.loaded = report
		if report.HasErrors() || report.Candidate == nil || report.Inspection == nil {
			model.message = workspaceIssueText(report)
			return model, nil
		}
		model.workspace.candidate = *report.Candidate
		model.workspace.stage = workspaceOverview
		return model, nil
	}
	if report.HasErrors() || report.Inspection == nil || report.Candidate == nil {
		model.message = workspaceIssueText(report)
		return model, nil
	}
	if model.workspace.loaded.Inspection == nil || report.Inspection.Snapshot.BaseFingerprint != model.workspace.loaded.Inspection.Snapshot.BaseFingerprint || report.Inspection.Snapshot.BaseExists != model.workspace.loaded.Inspection.Snapshot.BaseExists {
		model.message = "The saved profile changed while editing. Leave and reload it before creating another proposal."
		return model, nil
	}
	model.workspace.plan = report
	model.workspace.scroll = 0
	model.workspace.confirmation = ""
	model.workspace.stage = workspaceReview
	return model, nil
}

func (model dashboardModel) finishWorkspaceSave(message dashboardWorkspaceSaveMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardWorkspace || message.id != model.workspace.requestID || !model.workspace.saving {
		return model, nil
	}
	model.busy = ""
	model.workspace.saving = false
	model.workspace.result = message.report
	if !message.report.HasErrors() && message.report.Recorded {
		model.noteControllerSave(message.report.Revision)
	}
	model.workspace.stage = workspaceResult
	model.message = ""
	return model, nil
}

func (model dashboardModel) updateWorkspaceKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.workspace.saving {
		model.message = "The local save cannot be interrupted. Wait for its result; F1 opens help."
		return model, nil
	}
	if (key.String() == "shift+up" || key.String() == "shift+down") && !(model.workspace.stage == workspaceFieldEdit && model.workspace.field.kind == "favorites") {
		if key.String() == "shift+up" {
			model.pageScroll = max(0, model.pageScroll-1)
		} else {
			model.pageScroll++
		}
		return model, nil
	}
	if model.busy != "" {
		if key.String() == "esc" {
			model.workspace.cancelRead()
			model.busy = ""
			model.message = "Cancelled; no file changed."
			if model.workspace.loaded.Inspection == nil {
				model.screen = dashboardSettings
			}
		}
		return model, nil
	}
	if model.workspace.loaded.HasErrors() || model.workspace.loaded.Inspection == nil {
		switch key.String() {
		case "esc":
			model.screen = dashboardSettings
			model.message = ""
		case "r":
			return model.startWorkspaceRead(true)
		}
		return model, nil
	}
	switch model.workspace.stage {
	case workspaceOverview:
		switch key.String() {
		case "esc":
			model.workspace.cancelRead()
			model.screen = dashboardSettings
			model.message = "Workspace editor closed; unsaved draft discarded."
		case "up", "k":
			model.workspace.group = max(0, model.workspace.group-1)
		case "down", "j":
			model.workspace.group = min(len(workspaceGroups)-1, model.workspace.group+1)
		case "enter":
			model.workspace.cursor = 0
			model.workspace.stage = workspaceFieldList
			model.message = ""
		case "v":
			return model.startWorkspaceRead(false)
		}
	case workspaceFieldList:
		switch key.String() {
		case "esc":
			model.workspace.stage = workspaceOverview
			model.message = ""
		case "up", "k":
			model.workspace.cursor = max(0, model.workspace.cursor-1)
		case "down", "j":
			model.workspace.cursor = min(len(workspaceGroupFields(model.workspace.group))-1, model.workspace.cursor+1)
		case "enter":
			model.workspace.startField()
			model.message = ""
		case "v":
			return model.startWorkspaceRead(false)
		}
	case workspaceFieldEdit:
		if model.workspace.searching && key.String() == "enter" {
			return model.startWorkspaceSearch()
		}
		if model.workspace.field.kind == "extensions" && !model.workspace.searching && model.workspace.pending == nil && key.String() == "u" {
			ids := []string{}
			for id := range model.workspace.marketplace {
				ids = append(ids, id)
			}
			if len(ids) == 0 {
				model.message = "No Marketplace extensions are pinned in this draft."
				return model, nil
			}
			slices.Sort(ids)
			return model.startWorkspaceMarketplace(ids, true)
		}
		model.message = model.workspace.editField(key)
	case workspaceReview:
		switch key.String() {
		case "esc":
			model.workspace.stage = workspaceOverview
			model.workspace.confirmation = ""
			model.message = "Review cancelled; no file changed."
		case "up":
			model.workspace.scroll = max(0, model.workspace.scroll-1)
		case "down":
			model.workspace.scroll++
		case "pgup":
			model.workspace.scroll = max(0, model.workspace.scroll-max(1, model.height-17))
		case "pgdown":
			model.workspace.scroll += max(1, model.height-17)
		case "backspace":
			if len(model.workspace.confirmation) > 0 {
				model.workspace.confirmation = model.workspace.confirmation[:len(model.workspace.confirmation)-1]
			}
		case "enter":
			if model.workspace.plan.State == "unchanged" {
				model.workspace.stage = workspaceOverview
				model.message = "The declaration is unchanged; no save was requested."
				return model, nil
			}
			if model.workspace.plan.HasErrors() || model.workspace.plan.State != "ready" || model.workspace.plan.Confirmation != "SAVE" || model.workspace.plan.ReviewToken == "" || model.workspace.confirmation != "SAVE" {
				model.message = "Type SAVE to confirm this declaration-only change."
				return model, nil
			}
			model.workspace.saving = true
			model.workspace.requestID++
			id, plan := model.workspace.requestID, model.workspace.plan
			model.busy = "Rechecking, saving and recording workspace-profile.json"
			model.message = ""
			return model, func() tea.Msg { return dashboardWorkspaceSaveMsg{id: id, report: model.actions.SaveWorkspace(plan)} }
		default:
			if key.Text != "" && len(model.workspace.confirmation)+len(key.Text) <= 4 {
				model.workspace.confirmation += key.Text
			}
		}
	case workspaceResult:
		switch key.String() {
		case "enter":
			return model.continueSavedConfiguration()
		case "esc":
			model.screen = dashboardSettings
			model.message = ""
		case "r":
			if !model.workspace.result.RecoveryRequired && model.workspace.result.State != "saved" {
				return model.openWorkspace()
			}
		case "g":
			if workspaceRecordNeedsInspection(model.workspace.result) && model.actions.LoadGitReview != nil {
				return model.openMaintenanceTask("g")
			}
		}
	}
	return model, nil
}

func (model dashboardModel) workspaceView() string {
	w := model.workspace
	width := min(110, max(24, model.width-10))
	capacity := max(2, model.height-17)
	if model.height == 0 {
		capacity = 8
	}
	lines := []string{tuiTitle("Student workspace", model.isDark)}
	actions := []tuiAction{{key: "Esc", label: "Settings"}, {key: "F1", label: "Help"}}
	fixed := ""
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		if w.saving {
			actions = []tuiAction{{key: "F1", label: "Help"}}
		}
	} else if w.loaded.HasErrors() || w.loaded.Inspection == nil {
		lines = append(lines, "", "Workspace metadata could not be loaded.", "Check the deployment pin, catalog and prerequisites.")
		actions = append([]tuiAction{{key: "r", label: "Retry"}}, actions...)
	} else {
		resolved := w.loaded.Inspection.Resolution
		switch w.stage {
		case workspaceOverview:
			fixed = "Packaged extensions update with Maintenance → Update system and packages;\nMarketplace pins with u in VSCode → extensions."
			lines = append(lines, fmt.Sprintf("Student: %s · Controller + %d client(s)", resolved.StudentUser, len(resolved.Targets)-1), workspaceApplicationText(), "")
			if w.loaded.Inspection.Base == nil {
				lines = append(lines, "No saved profile; this is a new draft.")
			}
			for index, group := range workspaceGroups {
				lines = append(lines, tuiSelection(group, index == w.group, model.isDark))
			}
			actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Edit"}, {key: "v", label: "Review"}, {key: "Esc", label: "Settings"}, {key: "F1", label: "Help"}}
		case workspaceFieldList:
			lines = append(lines, workspaceGroups[w.group]+" · Draft only", "")
			fields := workspaceGroupFields(w.group)
			start, end := listWindow(len(fields), w.cursor, max(1, capacity/2))
			for index := start; index < end; index++ {
				field := fields[index]
				lines = append(lines, tuiSelection(field.label, index == w.cursor, model.isDark), tuiMuted("  "+workspaceShort(workspaceValueText(workspaceValue(w.candidate, field)), width-4), model.isDark))
			}
			lines = append(lines, fmt.Sprintf("%d–%d of %d fields", start+1, end, len(fields)))
			actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Change"}, {key: "v", label: "Review"}, {key: "Esc", label: "Sections"}, {key: "F1", label: "Help"}}
		case workspaceFieldEdit:
			lines = append(lines, w.field.label, "")
			actions = []tuiAction{{key: "Enter", label: "Keep draft"}, {key: "Esc", label: "Cancel field"}, {key: "F1", label: "Help"}}
			if w.field.kind == "settings" {
				settingLines, settingFixed, settingActions := w.settings.view(width, capacity, model.isDark)
				lines = append(lines, settingLines...)
				fixed = settingFixed
				actions = append(settingActions, tuiAction{key: "F1", label: "Help"})
			} else if w.field.kind == "number" {
				lines = append(lines, "Value: "+w.number+"_", "Leave empty to inherit the deployment baseline.")
			} else {
				multiple := w.field.kind == "favorites" || w.field.kind == "extensions"
				if w.pending != nil {
					lines = append(lines, w.marketplaceView(width)...)
					actions = []tuiAction{{key: "Enter", label: "Add to draft"}, {key: "Esc", label: "Do not add"}, {key: "F1", label: "Help"}}
					break
				}
				if w.searching && w.searchMode == "marketplace" {
					lines = append(lines, "Marketplace identifier: "+w.query+"_", "Type publisher.name exactly as on the Marketplace page, for example platformio.platformio-ide.", "The controller needs Internet access; the package is downloaded into its Nix store.", "")
					actions = []tuiAction{{key: "Enter", label: "Download"}, {key: "Esc", label: "Close"}, {key: "F1", label: "Help"}}
				} else if w.searching {
					lines = append(lines, "Search packaged extensions: "+w.query+"_", "Type part of a name, for example python or java, then press Enter.", "")
					actions = []tuiAction{{key: "Enter", label: "Search"}, {key: "Esc", label: "Close search"}, {key: "F1", label: "Help"}}
				} else if w.field.kind == "extensions" {
					actions = append([]tuiAction{{key: "/", label: "Search"}, {key: "m", label: "Marketplace"}, {key: "u", label: "Check updates"}}, actions...)
				}
				if multiple {
					mode := "Explicit selection"
					if w.inherit {
						mode = "Inherit baseline (preview below)"
					}
					lines = append(lines, mode)
					actions = append([]tuiAction{{key: "Space", label: "Toggle"}, {key: "i", label: "Inherit"}, {key: "c", label: "Clear"}}, actions...)
				}
				start, end := listWindow(len(w.choices), w.choice, capacity)
				for index := start; index < end; index++ {
					label := w.choices[index]
					if multiple {
						position := slices.Index(w.selected, label)
						marker := "[ ] "
						if position >= 0 {
							marker = "[x] "
							if w.field.kind == "favorites" {
								marker = fmt.Sprintf("[%d] ", position+1)
							}
						}
						label = marker + label
						if note := w.choiceNotes[w.choices[index]]; note != "" {
							label += "  " + note
						}
					}
					lines = append(lines, tuiSelection(workspaceShort(label, width-2), index == w.choice, model.isDark))
				}
				if len(w.choices) == 0 {
					lines = append(lines, "The deployment catalog has no entries.")
				}
				if len(w.choices) > capacity {
					lines = append(lines, fmt.Sprintf("%d–%d of %d choices", start+1, end, len(w.choices)))
				}
				if w.field.kind == "favorites" {
					fixed = "Shift ↑/↓ changes the order of the selected favorite."
				}
			}
		case workspaceReview:
			var review bytes.Buffer
			displayed := w.plan
			if displayed.State == "ready" {
				displayed.Message = "Save and record only workspace-profile.json locally. No system application, deployment or home reset is included."
			}
			WorkspacePlanText(&review, displayed)
			wrapped := strings.Split(lipgloss.NewStyle().Width(width).Render(strings.TrimSpace(review.String())), "\n")
			start := min(w.scroll, max(0, len(wrapped)-capacity))
			lines = append(lines, wrapped[start:min(len(wrapped), start+capacity)]...)
			lines = append(lines, fmt.Sprintf("Review lines %d–%d of %d", start+1, min(len(wrapped), start+capacity), len(wrapped)))
			fixed = "Save and record only the profile JSON; no system apply or reset.\nType SAVE: " + w.confirmation + "_"
			label := "Save JSON"
			if w.plan.State == "unchanged" {
				fixed = "Declaration unchanged; live home state is not inferred."
				label = "Back"
			}
			actions = []tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "Enter", label: label}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		case workspaceResult:
			level := tuiStatusSuccess
			if w.result.HasErrors() {
				level = tuiStatusAttention
			}
			lines = append(lines, tuiStatus(strings.ToUpper(w.result.State), level, model.isDark), "", safeWorkspaceText(w.result.Message))
			lines = append(lines, saveStatusLines(w.result.State, w.result.RecoveryRequired, true, true, controllerVerifiedForSave(w.result.Revision, model.controller.result))...)
			actions = model.saveFollowupActions("Settings")
			for _, issue := range w.result.Issues {
				lines = append(lines, safeWorkspaceText(issue.Message))
			}
			if w.result.State == "saved" {
				lines = append(lines, "", "Apply to this controller, then review the computers to update.", workspaceApplicationText())
			} else if !w.result.RecoveryRequired {
				actions = append([]tuiAction{{key: "r", label: "Reload"}}, actions...)
			}
			if workspaceRecordNeedsInspection(w.result) && model.actions.LoadGitReview != nil {
				lines = append(lines, "Inspect and finish the local record in Git review before applying systems.")
				actions = append([]tuiAction{{key: "g", label: "Inspect Git state"}}, actions...)
			}
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: safeWorkspaceText(model.message)})
	}
	return model.renderShell(tuiShell{path: []string{"Settings", "Student workspace"}, body: strings.Join(lines, "\n"), fixedBody: fixed, notices: notices, actions: actions})
}

func workspaceShort(value string, width int) string {
	value = safeWorkspaceText(value)
	if lipgloss.Width(value) <= width {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(value)
}
