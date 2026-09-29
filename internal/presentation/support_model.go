package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type supportModel struct {
	snapshot  domain.SupportSnapshot
	result    domain.SupportExportResult
	requestID uint64
	cancel    context.CancelFunc
	saving    bool
	scroll    int
}

type dashboardSupportPreviewMsg struct {
	id       uint64
	snapshot domain.SupportSnapshot
	err      error
}

type dashboardSupportExportMsg struct {
	id     uint64
	result domain.SupportExportResult
}

func (s *supportModel) cancelRead() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.requestID++
}

func (model dashboardModel) openSupportPreview() (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode || model.actions.PreviewSupport == nil || model.support.saving {
		return model, nil
	}
	model.support.cancelRead()
	model.support.snapshot = domain.SupportSnapshot{}
	model.support.result = domain.SupportExportResult{}
	model.support.scroll = 0
	model.pageScroll = 0
	model.message = ""
	model.screen = dashboardSupport
	model.busy = "Collecting local diagnostics; no build or upload"
	ctx, cancel := context.WithCancel(context.Background())
	model.support.cancel = cancel
	id, action := model.support.requestID, model.actions.PreviewSupport
	return model, func() tea.Msg {
		snapshot, err := action(ctx)
		return dashboardSupportPreviewMsg{id: id, snapshot: snapshot, err: err}
	}
}

func (model dashboardModel) finishSupportPreview(message dashboardSupportPreviewMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardSupport || message.id != model.support.requestID || model.support.saving {
		return model, nil
	}
	if model.support.cancel != nil {
		model.support.cancel()
		model.support.cancel = nil
	}
	model.busy = ""
	if message.err != nil || !message.snapshot.Valid() {
		model.support.snapshot = domain.SupportSnapshot{}
		model.message = "No support preview is available. Retry collection; nothing was saved."
	} else {
		model.support.snapshot = message.snapshot
		model.message = ""
	}
	return model, nil
}

func (model dashboardModel) finishSupportExport(message dashboardSupportExportMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardSupport || message.id != model.support.requestID || !model.support.saving {
		return model, nil
	}
	model.support.saving = false
	model.busy = ""
	model.support.result = message.result
	model.support.scroll = 0
	model.message = ""
	return model, nil
}

func (model dashboardModel) updateSupportKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.support.saving {
		return model, nil
	}
	switch key.String() {
	case "esc":
		model.support.cancelRead()
		model.support.snapshot = domain.SupportSnapshot{}
		model.busy, model.message = "", ""
		model.screen = dashboardDiagnostics
		return model, nil
	case "r":
		return model.openSupportPreview()
	}
	if model.busy != "" {
		return model, nil
	}
	switch key.String() {
	case "enter":
		if model.support.result.State != "" {
			model.screen = dashboardDiagnostics
			return model, nil
		}
		if !model.support.snapshot.Valid() || model.actions.ExportSupport == nil {
			return model, nil
		}
		model.support.saving = true
		model.busy = "Saving the reviewed report locally"
		action, snapshot, id := model.actions.ExportSupport, model.support.snapshot, model.support.requestID
		return model, func() tea.Msg { return dashboardSupportExportMsg{id: id, result: action(snapshot)} }
	case "up", "k":
		model.support.scroll--
	case "down", "j":
		model.support.scroll++
	case "pgup":
		model.support.scroll -= model.supportViewportHeight()
	case "pgdown":
		model.support.scroll += model.supportViewportHeight()
	case "home":
		model.support.scroll = 0
	case "end":
		model.support.scroll = len(model.supportLines())
	}
	model.support.scroll = max(0, min(model.support.scroll, len(model.supportLines())-model.supportViewportHeight()))
	return model, nil
}

func (model dashboardModel) supportViewportHeight() int { return max(3, model.height-16) }

func (model dashboardModel) supportLines() []string {
	width := min(116, max(20, model.width-6))
	return strings.Split(strings.TrimRight(lipgloss.NewStyle().Width(width).Render(model.support.snapshot.JSON()), "\n"), "\n")
}

func (model dashboardModel) supportView() string {
	lines := []string{tuiTitle("Local support report", model.isDark), "Minimized, not anonymous. No upload or remediation.", ""}
	actions := []tuiAction{{key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		if model.support.saving {
			actions = []tuiAction{{key: "F1", label: "Help"}}
		}
	} else if result := model.support.result; result.State != "" {
		kind := tuiStatusAttention
		if result.State == "saved" {
			kind = tuiStatusSuccess
		}
		lines = append(lines, tuiStatus(strings.ToUpper(result.State), kind, model.isDark), result.Message)
		if result.Path != "" {
			lines = append(lines, fmt.Sprintf("Local file: %q", result.Path), "SHA-256: "+result.SHA256)
		}
		actions = []tuiAction{{key: "Enter", label: "Diagnostics"}, {key: "r", label: "New preview"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	} else if model.support.snapshot.Valid() {
		content := model.supportLines()
		start := max(0, min(model.support.scroll, len(content)-model.supportViewportHeight()))
		end := min(len(content), start+model.supportViewportHeight())
		lines = append(lines, content[start:end]...)
		lines = append(lines, "", fmt.Sprintf("Lines %d–%d / %d · Versions, revision, time and counts remain visible.", start+1, end, len(content)))
		actions = []tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "Enter", label: "Save locally"}, {key: "r", label: "Refresh"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else {
		lines = append(lines, "No report collected. Nothing has been saved.")
		actions = append([]tuiAction{{key: "r", label: "Retry"}}, actions...)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", "Diagnostics", "Support report"}, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}
