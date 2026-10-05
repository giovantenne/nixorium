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
	report     domain.TelemetryReport
	applying   bool
	id         uint64
	firstOffer bool
	details    bool
}
type telemetryMsg struct {
	report domain.TelemetryReport
	err    error
	id     uint64
	offer  bool
	choice bool
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
	model.telemetry.firstOffer = false
	model.telemetry.details = false
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
		model.telemetry.firstOffer = true
		model.telemetry.details = false
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
	if msg.choice && model.telemetry.firstOffer {
		model.screen = dashboardHome
		model.telemetry.firstOffer = false
		model.message = "Adoption statistics are off. You can change this in Maintenance."
		if msg.report.Consent == "enabled" {
			model.message = "Daily adoption statistics enabled. You can turn them off in Maintenance."
		}
	}
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
		if model.telemetry.firstOffer {
			model.screen = dashboardHome
		}
		model.telemetry.firstOffer = false
		model.message = ""
		return model, nil
	case "r":
		firstOffer := model.telemetry.firstOffer
		next, cmd := model.openTelemetry()
		updated := next.(dashboardModel)
		updated.telemetry.firstOffer = firstOffer
		return updated, cmd
	case "p":
		model.telemetry.details = !model.telemetry.details
		model.pageScroll = 0
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
			return telemetryMsg{report: r, err: e, id: id, choice: true}
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
	body := []string{tuiTitle("Help improve Nixorium?", model.isDark),
		"Share a small daily report to help us understand adoption and lab sizes.",
		"Your choice is optional. Nixorium works fully with sharing off.", ""}
	if r.Consent != "" {
		state := "Off · nothing is sent without your explicit choice"
		if r.Consent == "enabled" {
			state = "On · at most one report per UTC day"
		}
		boot := "Unknown (not a failed installation)"
		if r.Payload.ClientBootVerified != nil && *r.Payload.ClientBootVerified {
			boot = "At least one installed client previously verified"
		}
		body = append(body, state, "",
			"Version: "+r.Payload.Version+"   Mode: "+r.Payload.DeploymentMode,
			"Configured clients: "+r.Payload.ConfiguredClients,
			"Client boot: "+boot,
			"A monthly pseudonym counts this controller within one UTC month.", "",
			"No names, files, logs or configuration. Cloudflare sees the connection IP.",
			"Sent to telemetry.nixorium.org. Change your choice anytime in Maintenance.",
			"Daily records: 90-day target; monthly totals: 24 months.",
			"Provider recovery copies expire separately. Disabling stops future sends.")
		if model.telemetry.details {
			b, _ := json.MarshalIndent(r.Payload, "", "  ")
			body = []string{tuiTitle("Exact report and privacy details", model.isDark), string(b), "", domain.TelemetryNotice,
				"", "Last attempt: " + r.LastAttempt, "Last success: " + r.LastSuccess, "Result: " + r.Result}
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	decline := "No thanks"
	if r.Consent == "enabled" {
		decline = "Disable sharing"
	}
	preview := "Exact report"
	if model.telemetry.details {
		preview = "Overview"
	}
	actions := []tuiAction{{key: "e", label: "Enable sharing"}, {key: "d", label: decline}, {key: "p", label: preview}, {key: "↑/↓", label: "Scroll"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	if model.telemetry.applying {
		actions = []tuiAction{{key: "F1", label: "Help"}}
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", "Adoption statistics"}, body: strings.Join(body, "\n"), notices: notices, actions: actions})
}
