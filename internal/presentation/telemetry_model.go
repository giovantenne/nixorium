package presentation

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/giovantenne/nixorium/internal/domain"
	"strings"
	"time"
)

type telemetryModel struct {
	report   domain.TelemetryReport
	applying bool
	id       uint64
}
type telemetryMsg struct {
	report domain.TelemetryReport
	err    error
	id     uint64
	offer  bool
}

func (model dashboardModel) checkTelemetryOffer() tea.Cmd {
	if model.actions.ClassroomMode || model.actions.Telemetry == nil || model.screen != dashboardHome || model.managedJobConflict() != "" || model.usbReserved || len(model.recovery.Conditions) > 0 {
		return nil
	}
	action := model.actions.Telemetry
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		r, e := action(ctx, "status")
		if e != nil || r.Prompted || r.Consent != "undecided" {
			return telemetryMsg{offer: true, err: fmt.Errorf("no offer")}
		}
		r, e = action(ctx, "dismiss")
		return telemetryMsg{offer: true, report: r, err: e}
	}
}
func (model dashboardModel) openTelemetry() (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode || model.actions.Telemetry == nil {
		model.message = "Adoption statistics are unavailable on this controller."
		return model, nil
	}
	model.screen = dashboardTelemetry
	model.telemetry.id++
	model.telemetry.report = domain.TelemetryReport{}
	model.pageScroll = 0
	model.message = ""
	model.busy = "Reading local telemetry settings; no upload"
	id, action := model.telemetry.id, model.actions.Telemetry
	return model.startRead(func(ctx context.Context) tea.Msg {
		r, e := action(ctx, "status")
		return telemetryMsg{report: r, err: e, id: id}
	})
}
func (model dashboardModel) finishTelemetry(msg telemetryMsg) (tea.Model, tea.Cmd) {
	if msg.offer {
		if msg.err != nil || model.screen != dashboardHome || model.busy != "" || model.managedJobConflict() != "" || model.actions.ClassroomMode {
			return model, nil
		}
		model.screen = dashboardTelemetry
		model.pageScroll = 0
	} else if model.screen != dashboardTelemetry || msg.id != model.telemetry.id {
		return model, nil
	}
	model.busy = ""
	model.telemetry.applying = false
	if msg.err != nil {
		model.message = "Telemetry settings unavailable. Run nixorium telemetry status; if state is invalid, run nixorium telemetry disable to reset consent."
		return model, nil
	}
	model.telemetry.report = msg.report
	model.message = ""
	return model, nil
}
func (model dashboardModel) updateTelemetryKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.telemetry.applying {
		model.message = "Saving your choice locally. Please wait."
		return model, nil
	}
	switch key.String() {
	case "esc":
		model.screen = dashboardAdministration
		model.message = ""
		return model, nil
	case "r":
		return model.openTelemetry()
	case "e", "d":
		if model.actions.Telemetry == nil || (key.String() == "e" && model.telemetry.report.Consent == "") {
			return model, nil
		}
		actionName := "disable"
		if key.String() == "e" {
			actionName = "enable"
		}
		model.telemetry.applying = true
		model.busy = "Saving your telemetry choice locally"
		model.telemetry.id++
		id, action := model.telemetry.id, model.actions.Telemetry
		return model, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			r, e := action(ctx, actionName)
			return telemetryMsg{report: r, err: e, id: id}
		}
	case "down", "j":
		model.pageScroll++
	case "up", "k":
		model.pageScroll = max(0, model.pageScroll-1)
	}
	return model, nil
}
func (model dashboardModel) telemetryView() string {
	r := model.telemetry.report
	body := []string{tuiTitle("Optional adoption statistics", model.isDark), domain.TelemetryNotice, "", "Consent: " + r.Consent}
	if r.Consent != "" {
		b, _ := json.MarshalIndent(r.Payload, "", "  ")
		body = append(body, "", string(b), "", "Last attempt: "+r.LastAttempt, "Last success: "+r.LastSuccess, "Result: "+r.Result)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "e", label: "Enable sharing"}, {key: "d", label: "No thanks / disable"}, {key: "r", label: "Refresh"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	if model.telemetry.applying {
		actions = []tuiAction{{key: "F1", label: "Help"}}
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", "Adoption statistics"}, body: strings.Join(body, "\n"), notices: notices, actions: actions})
}
