package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type disclaimerMsg struct {
	accepted bool
	err      error
}

func (model dashboardModel) loadDisclaimer() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		accepted, err := model.actions.LoadDisclaimer(ctx)
		return disclaimerMsg{accepted: accepted, err: err}
	}
}
func (model dashboardModel) finishDisclaimer(msg disclaimerMsg) (tea.Model, tea.Cmd) {
	if !model.disclaimerChecking && model.screen != dashboardDisclaimer {
		return model, nil
	}
	model.disclaimerChecking = false
	model.screen = dashboardDisclaimer
	model.busy = ""
	if msg.err != nil {
		model.message = "The acknowledgement could not be read or saved. Enter retries; q exits."
		return model, nil
	}
	if !msg.accepted {
		return model, nil
	}
	model.screen = dashboardHome
	model.message = ""
	model.busy = "Opening the laboratory and checking setup progress"
	return model, func() tea.Msg { return dashboardBeginInitialMsg{} }
}
func (model dashboardModel) updateDisclaimer(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return model, tea.Quit
	case "enter":
		if model.busy != "" || model.actions.AcceptDisclaimer == nil {
			return model, nil
		}
		model.busy = "Saving acknowledgement"
		return model, func() tea.Msg { return disclaimerMsg{accepted: true, err: model.actions.AcceptDisclaimer()} }
	case "down", "j":
		model.pageScroll++
	case "up", "k":
		model.pageScroll = max(0, model.pageScroll-1)
	}
	return model, nil
}
func (model dashboardModel) disclaimerView() string {
	lines := []string{
		tuiTitle("Welcome to Nixorium", model.isDark), "",
		tuiSection("Before you manage this laboratory", model.isDark),
		"Nixorium can erase disks, install systems, reset student homes,",
		"change configurations and interrupt active sessions.", "",
		tuiStatus("Check every computer, disk and action before confirming.", tuiStatusAttention, model.isDark),
		"Keep tested backups. Local snapshots are not backups.",
		"Use Nixorium only on computers and networks you may administer.", "",
		"Nixorium is provided under the MIT License, without warranty.",
		"You are responsible for testing changes and verifying their results.",
		tuiMuted("Full terms: DISCLAIMER.md and LICENSE in the Nixorium repository.", model.isDark), "",
		"Enter acknowledges these operational risks and continues.",
		"The next screen asks separately about optional adoption statistics.",
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Welcome", "Disclaimer"}, body: strings.Join(lines, "\n"), notices: notices,
		actions: []tuiAction{{key: "Enter", label: "Accept & continue"}, {key: "↑/↓", label: "Scroll"}, {key: "q", label: "Exit"}}})
}

// Read consent before exposing the menu; a late status reply cannot interrupt
// another task or save a choice. Esc leaves the invitation undecided.
func (model dashboardModel) beginTelemetryInvitation() (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode || model.actions.Telemetry == nil {
		return model, nil
	}
	model.screen = dashboardTelemetry
	model.telemetry.firstOffer = true
	model.telemetry.loadingOffer = true
	model.telemetry.id++
	id := model.telemetry.id
	model.busy = ""
	return model, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		report, err := model.actions.Telemetry(ctx, "status")
		return telemetryMsg{offer: true, startup: true, id: id, report: report, err: err}
	}
}
