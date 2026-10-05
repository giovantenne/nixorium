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
	report       domain.TelemetryReport
	applying     bool
	id           uint64
	loadingOffer bool
	firstOffer   bool
	detail       string
}
type telemetryMsg struct {
	report  domain.TelemetryReport
	err     error
	id      uint64
	offer   bool
	choice  bool
	startup bool
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
		if e != nil || r.Consent != "undecided" {
			return telemetryMsg{offer: true, err: fmt.Errorf("no offer")}
		}
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
	model.telemetry.loadingOffer = false
	model.telemetry.detail = ""
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
		if msg.startup && (model.screen != dashboardTelemetry || !model.telemetry.firstOffer || msg.id != model.telemetry.id) {
			return model, nil
		}
		awaiting := model.screen == dashboardTelemetry && model.telemetry.firstOffer
		if !awaiting && (model.screen != dashboardHome || model.busy != "" || model.managedJobConflict() != "" || model.actions.ClassroomMode) {
			return model, nil
		}
		model.telemetry.loadingOffer = false
		if msg.err != nil || msg.report.Consent != "undecided" {
			if awaiting {
				model.screen = dashboardHome
				model.telemetry.firstOffer = false
			}
			return model, nil
		}
		model.screen = dashboardTelemetry
		model.telemetry.firstOffer = true
		model.telemetry.detail = ""
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
	if model.telemetry.loadingOffer && key.String() != "esc" {
		return model, nil
	}
	if model.telemetry.applying {
		model.message = "Saving your choice locally. Please wait."
		return model, nil
	}
	switch key.String() {
	case "esc":
		if model.telemetry.detail != "" {
			model.telemetry.detail = ""
			model.pageScroll = 0
			model.message = ""
			return model, nil
		}
		if model.telemetry.firstOffer {
			model.screen = dashboardHome
			model.telemetry.firstOffer = false
			model.message = "Adoption statistics remain off. Choose later in Maintenance."
			if model.telemetry.loadingOffer {
				model.message = "No statistics choice was saved. Manage it in Maintenance."
			}
			model.telemetry.loadingOffer = false
			return model, nil
		}
		model.screen = dashboardAdministration
		model.telemetry.firstOffer = false
		model.message = ""
		return model, nil
	case "r":
		firstOffer := model.telemetry.firstOffer
		next, cmd := model.openTelemetry()
		updated := next.(dashboardModel)
		updated.telemetry.firstOffer = firstOffer
		return updated, cmd
	case "p", "i":
		if model.telemetry.detail == key.String() {
			model.telemetry.detail = ""
		} else {
			model.telemetry.detail = key.String()
		}
		model.pageScroll = 0
		model.message = ""
	case "e", "enter", "n", "d":
		// Enter invokes the displayed primary action only on the overview.
		if key.String() == "enter" && model.telemetry.detail != "" {
			return model, nil
		}
		if model.actions.Telemetry == nil || (key.String() != "d" && key.String() != "n" && model.telemetry.report.Consent == "") {
			return model, nil
		}
		actionName := "disable"
		if key.String() != "d" && key.String() != "n" {
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
	if model.telemetry.loadingOffer {
		return model.renderShell(tuiShell{path: []string{"Welcome"}, body: tuiTitle("Reading your saved statistics choice", model.isDark) + "\n\nThis local check uploads no adoption report.", actions: []tuiAction{{key: "Esc", label: "Menu"}, {key: "q", label: "Exit"}}})
	}

	r := model.telemetry.report
	body := []string{tuiTitle("Help improve Nixorium", model.isDark),
		"Share basic statistics to understand adoption and lab sizes,",
		"and help prioritize development.", "",
		"Sharing is optional and off by default. Nixorium works fully without it.", "",
		"Daily: version, system mode, client count band and boot status;",
		"an identifier that changes every month.", "",
		"No names, files, logs or configuration files are included.",
		"Sent to telemetry.nixorium.org via Cloudflare, which sees your IP.", "",
		"Disable anytime in Maintenance. See Privacy & retention for details."}
	state := "Status: Off — no adoption reports are being sent."
	if r.Consent == "enabled" {
		state = "Status: On — at most one adoption report per UTC day."
	}
	if r.Consent == "" {
		state = "Status: Reading local settings…"
	}
	fixedBody := state
	if model.telemetry.detail != "" {
		fixedBody = ""
	}
	switch model.telemetry.detail {
	case "p":
		b, _ := json.MarshalIndent(r.Payload, "", "  ")
		boot := "Unknown"
		if r.Payload.ClientBootVerified != nil && *r.Payload.ClientBootVerified {
			boot = "At least one installed client previously verified"
		}
		body = []string{tuiTitle("Exact report", model.isDark),
			"This preview is generated locally; opening it sends no report.", "",
			"Version: " + r.Payload.Version + "   Mode: " + r.Payload.DeploymentMode,
			"Configured clients: " + r.Payload.ConfiguredClients,
			"Client boot status: " + boot,
			"An unknown boot status is not counted as an installation failure.",
			"Verified boot is historical evidence, not current lab health.", "", string(b), "",
			"Before consent, the monthly identifier is a placeholder.",
			"Last attempt: " + r.LastAttempt, "Last success: " + r.LastSuccess, "Result: " + r.Result}
	case "i":
		body = []string{tuiTitle("Privacy & retention", model.isDark), domain.TelemetryNotice}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	decline := "No thanks"
	if r.Consent == "enabled" {
		decline = "Disable sharing"
	}
	acceptKey := "e/Enter"
	if model.telemetry.detail != "" {
		acceptKey = "e"
	}
	actions := []tuiAction{{key: acceptKey, label: "Share statistics"}, {key: "n", label: decline},
		{key: "p", label: "Exact report"}, {key: "i", label: "Privacy & retention"}, {key: "↑/↓", label: "Scroll"}}
	back := "Back"
	if model.telemetry.firstOffer && model.telemetry.detail == "" {
		back = "Menu"
	}
	actions = append(actions, tuiAction{key: "Esc", label: back}, tuiAction{key: "q", label: "Exit"})
	actions = append(actions, tuiAction{key: "F1", label: "Help"})
	if model.telemetry.applying {
		actions = []tuiAction{{key: "F1", label: "Help"}}
	}
	path := []string{"Maintenance", "Adoption statistics"}
	if model.telemetry.firstOffer {
		path = []string{"Welcome", "Adoption statistics"}
	}
	return model.renderShell(tuiShell{path: path, body: strings.Join(body, "\n"), fixedBody: fixedBody, notices: notices, actions: actions})
}
