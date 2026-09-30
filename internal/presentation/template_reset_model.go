package presentation

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type templateResetModel struct {
	catalog             domain.TemplateResetCatalog
	plan                domain.TemplateResetPlan
	result              domain.TemplateResetResult
	stage, confirmation string
	cursor, scroll      int
	requestID           uint64
	cancel              context.CancelFunc
	events              chan tea.Msg
	saving              bool
}

type templateResetCatalogMsg struct {
	id      uint64
	catalog domain.TemplateResetCatalog
}
type templateResetPlanMsg struct {
	id   uint64
	plan domain.TemplateResetPlan
}
type templateResetProgressMsg struct {
	id     uint64
	detail string
}
type templateResetResultMsg struct {
	id     uint64
	result domain.TemplateResetResult
}

func (m *templateResetModel) cancelRead() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.requestID++
}

func (model dashboardModel) openTemplateReset() (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode || model.actions.LoadTemplateReset == nil || model.actions.PlanTemplateReset == nil || model.actions.ApplyTemplateReset == nil || model.templateReset.saving {
		model.message = "Deployment template reset is not available in this session."
		return model, nil
	}
	model.templateReset.cancelRead()
	id := model.templateReset.requestID
	ctx, activityID := model.beginRead(dashboardReadTimeout)
	model.templateReset = templateResetModel{requestID: id, cancel: model.read.cancel, stage: "select"}
	model.screen, model.pageScroll, model.message = dashboardTemplateReset, 0, ""
	model.busy = "Reading the exact pinned upstream template; no files changed"
	load := model.actions.LoadTemplateReset
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg { return templateResetCatalogMsg{id, load(ctx)} })
}

func (model dashboardModel) finishTemplateResetCatalog(msg templateResetCatalogMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardTemplateReset || msg.id != model.templateReset.requestID || model.templateReset.saving {
		return model, nil
	}
	model.templateReset.cancelRead()
	model.busy = ""
	model.templateReset.catalog = msg.catalog
	if msg.catalog.Error != "" {
		model.message = msg.catalog.Error
	}
	return model, nil
}

func (model dashboardModel) startTemplateResetPlan() (tea.Model, tea.Cmd) {
	m := &model.templateReset
	if m.cursor < 0 || m.cursor >= len(m.catalog.Catalog.Presets) {
		return model, nil
	}
	m.cancelRead()
	ctx, activityID := model.beginRead(dashboardBuildTimeout)
	m.cancel, m.stage, m.confirmation, m.scroll = model.read.cancel, "planning", "", 0
	m.events = make(chan tea.Msg, 8)
	id, events, preset, action := m.requestID, m.events, m.catalog.Catalog.Presets[m.cursor].ID, model.actions.PlanTemplateReset
	model.busy, model.message = "Preparing an isolated template reset candidate", ""
	return model, func() tea.Msg {
		go func() {
			defer close(events)
			plan := action(ctx, preset, func(detail string) {
				select {
				case events <- templateResetProgressMsg{id, detail}:
				case <-ctx.Done():
				}
			})
			select {
			case events <- templateResetPlanMsg{id, plan}:
			case <-ctx.Done():
			}
		}()
		return waitForActivityEvent(ctx, activityID, events)()
	}
}

func (model dashboardModel) finishTemplateResetPlan(msg templateResetPlanMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardTemplateReset || msg.id != model.templateReset.requestID || model.templateReset.saving {
		return model, nil
	}
	model.templateReset.cancelRead()
	model.busy = ""
	model.templateReset.plan = msg.plan
	if msg.plan.HasErrors() || msg.plan.UpstreamRevision != model.templateReset.catalog.UpstreamRevision || msg.plan.ReviewToken == "" || msg.plan.Confirmation != "RESET DEPLOYMENT" {
		model.templateReset.stage = "select"
		model.message = "Reset proposal unavailable or pin changed. Reload before retrying. " + msg.plan.Message
		return model, nil
	}
	model.templateReset.stage = "review"
	return model, nil
}

func (model dashboardModel) finishTemplateResetResult(msg templateResetResultMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardTemplateReset || msg.id != model.templateReset.requestID || !model.templateReset.saving {
		return model, nil
	}
	model.templateReset.saving, model.templateReset.stage = false, "result"
	model.templateReset.result = msg.result
	if msg.result.State == "saved" && !msg.result.RecoveryRequired {
		model.pendingRevision = msg.result.Revision
	}
	model.templateReset.scroll, model.pageScroll = 0, 0
	model.busy, model.message = "", ""
	// Invalidate any in-memory activation review of the pre-reset revision.
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.deployment.plan = domain.DeploymentPlanReport{}
	return model, nil
}

func (model dashboardModel) updateTemplateResetKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.templateReset
	if m.saving {
		model.message = "The local reset cannot be interrupted. Wait for its result; F1 opens help."
		return model, nil
	}
	if key.String() == "shift+down" || key.String() == "shift+up" {
		if key.String() == "shift+down" {
			model.pageScroll++
		} else {
			model.pageScroll = max(0, model.pageScroll-1)
		}
		return model, nil
	}
	if key.String() == "esc" {
		m.cancelRead()
		model.busy, model.message, model.pageScroll = "", "", 0
		if m.stage == "review" || m.stage == "planning" {
			m.stage, m.confirmation, m.scroll = "select", "", 0
			m.plan = domain.TemplateResetPlan{}
		} else {
			model.screen = dashboardAdministration
		}
		return model, nil
	}
	if model.busy != "" {
		return model, nil
	}
	switch m.stage {
	case "select":
		switch key.String() {
		case "r":
			return model.openTemplateReset()
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = max(0, min(len(m.catalog.Catalog.Presets)-1, m.cursor+1))
		case "enter":
			if m.catalog.Error == "" {
				return model.startTemplateResetPlan()
			}
		}
	case "review":
		switch key.String() {
		case "up":
			m.scroll--
		case "down":
			m.scroll++
		case "pgup":
			m.scroll -= model.templateResetViewport()
		case "pgdown":
			m.scroll += model.templateResetViewport()
		case "home":
			m.scroll = 0
		case "end":
			m.scroll = len(model.templateResetLines())
		case "backspace":
			if len(m.confirmation) != 0 {
				m.confirmation = m.confirmation[:len(m.confirmation)-1]
			}
		case "enter":
			if m.confirmation != "RESET DEPLOYMENT" {
				model.message = "Type RESET DEPLOYMENT exactly after reviewing all losses."
				return model, nil
			}
			m.saving = true
			model.busy, model.message = "Creating the recovery backup and saving the reviewed template", ""
			id, plan, action := m.requestID, m.plan, model.actions.ApplyTemplateReset
			return model, func() tea.Msg { return templateResetResultMsg{id, action(plan)} }
		default:
			if key.Text != "" && len(m.confirmation)+len(key.Text) <= 32 && strings.IndexFunc(key.Text, unicode.IsControl) < 0 {
				m.confirmation += key.Text
			}
		}
		m.scroll = max(0, min(m.scroll, len(model.templateResetLines())-model.templateResetViewport()))
	case "result":
		if key.String() == "enter" {
			return model.continueSavedConfiguration()
		}
	}
	return model, nil
}

func (model dashboardModel) templateResetViewport() int { return max(3, model.height-16) }

func (model dashboardModel) templateResetLines() []string {
	p := model.templateReset.plan
	lines := []string{
		"Pinned upstream: " + p.UpstreamRevision,
		"Replace software with: " + p.Preset.Label + " (shared: controller and clients)",
		"Packages: " + strings.Join(p.Preset.Packages, ", "),
		"Discard current custom software, home preferences, assets and modules listed below.",
		"Guided home: enabled with the template's initial desktop/dock/browser profile; VS Code favorite when included.",
		"Effect on student homes requires separate system application and reboot.",
		"Backup: a durable local Git ref to the current committed deployment, before replacement.",
		"Only tracked, committed files enter the backup. Untracked/ignored files remain in place.",
		"No input update, activation, client deployment, live home reset, reboot or push.", "",
	}
	for _, check := range p.Checks {
		lines = append(lines, check.ID+": "+check.Message)
	}
	if len(p.PreviousSoftware) != 0 {
		lines = append(lines, "", "Current software declarations to replace (including their scopes):")
		for _, declaration := range p.PreviousSoftware {
			lines = append(lines, "  "+declaration.Package+" · "+softwareScopeLabel(declaration.Scope))
		}
	}
	lines = append(lines, "", "Preserved tracked files:")
	for _, name := range p.Preserved {
		lines = append(lines, "  "+name)
	}
	lines = append(lines, "", "Changes (local modules and assets may contain policies that will be lost):")
	for _, change := range p.Changes {
		lines = append(lines, fmt.Sprintf("  %-7s %s", change.Action, change.Path))
	}
	width := min(116, max(20, model.width-6))
	return strings.Split(lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n")), "\n")
}

func (model dashboardModel) templateResetView() string {
	m := model.templateReset
	lines := []string{tuiTitle("Reset deployment template", model.isDark), "Replace local customizations. Keep settings, keys and exact input pins.", ""}
	actions := []tuiAction{{key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	switch {
	case model.busy != "":
		lines = append(lines, model.busyView())
		if m.saving {
			actions = []tuiAction{{key: "F1", label: "Help"}}
		}
	case m.stage == "review":
		content := model.templateResetLines()
		start := max(0, min(m.scroll, len(content)-model.templateResetViewport()))
		end := min(len(content), start+model.templateResetViewport())
		lines = append(lines, content[start:end]...)
		lines = append(lines, fmt.Sprintf("Lines %d–%d / %d", start+1, end, len(content)), "Type RESET DEPLOYMENT: "+m.confirmation)
		actions = append([]tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "Enter", label: "Back up and reset"}}, actions...)
	case m.stage == "result":
		kind := tuiStatusAttention
		if m.result.State == "saved" {
			kind = tuiStatusSuccess
		}
		lines = append(lines, tuiStatus(strings.ToUpper(m.result.State), kind, model.isDark), m.result.Message)
		lines = append(lines, "")
		lines = append(lines, saveStatusLines(m.result.State, m.result.RecoveryRequired, true, true, controllerVerifiedForSave(m.result.Revision, model.controller.result))...)
		lines = append(lines, "", "Student-home changes take effect at the next computer start after system application.", "Applying the controller is optional here; no client deployment or reboot starts automatically.")
		if m.result.BackupRef != "" {
			lines = append(lines, "", "Backup: "+m.result.BackupRef)
		}
		if m.result.Revision != "" {
			lines = append(lines, "Local commit: "+m.result.Revision)
		}
		actions = model.saveFollowupActions("Maintenance")
	case m.catalog.Error != "" || len(m.catalog.Catalog.Presets) == 0:
		lines = append(lines, "The pinned template catalog could not be loaded. No file was changed.")
		actions = append([]tuiAction{{key: "r", label: "Retry"}}, actions...)
	default:
		lines = append(lines, "Upstream: "+m.catalog.UpstreamRevision, "", "Choose the replacement software preset:")
		start := max(0, m.cursor-max(2, model.height-17)+1)
		end := min(len(m.catalog.Catalog.Presets), start+max(2, model.height-17))
		for i := start; i < end; i++ {
			entry := m.catalog.Catalog.Presets[i]
			lines = append(lines, tuiSelection(entry.Label, i == m.cursor, model.isDark))
		}
		if m.cursor < len(m.catalog.Catalog.Presets) {
			lines = append(lines, "", m.catalog.Catalog.Presets[m.cursor].Description)
		}
		actions = append([]tuiAction{{key: "↑/↓", label: "Choose"}, {key: "Enter", label: "Review reset"}, {key: "r", label: "Reload"}}, actions...)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", "Reset template"}, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}
