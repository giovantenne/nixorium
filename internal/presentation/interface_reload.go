package presentation

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// interfaceReloadSeconds is how long the result screen waits before it reopens
// the interface on the freshly installed Nixorium.
const interfaceReloadSeconds = 3

// reopenSentence is the guidance shown when the administrator keeps the
// running interface.
const reopenSentence = "Reopen Nixorium to use the updated interface."

// InterfaceReload describes an installed Nixorium that differs from the
// running one. Version may be empty when it cannot be read cheaply.
type InterfaceReload struct {
	Version string
}

type interfaceReloadState int

const (
	interfaceReloadNone interfaceReloadState = iota
	interfaceReloadCounting
	interfaceReloadCancelled
	interfaceReloadRequested
)

type interfaceReloadModel struct {
	state     interfaceReloadState
	version   string
	remaining int
	id        uint64
}

type interfaceReloadTickMsg struct{ id uint64 }

func scheduleInterfaceReloadTick(id uint64) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return interfaceReloadTickMsg{id: id} })
}

// line is the result-screen sentence for a successful controller activation.
func (reload interfaceReloadModel) line() string {
	if reload.state != interfaceReloadCounting && reload.state != interfaceReloadRequested {
		return reopenSentence
	}
	subject := "The updated Nixorium"
	if reload.version != "" {
		subject = "Nixorium " + reload.version
	}
	return fmt.Sprintf("%s is active. The interface reopens in %d s with it (Esc keeps this one).", subject, reload.remaining)
}

func (reload interfaceReloadModel) counting() bool { return reload.state == interfaceReloadCounting }

// reloadActions replaces the result actions while the countdown runs.
func (model dashboardModel) reloadActions() []tuiAction {
	return []tuiAction{{key: "Enter", label: "Reopen now"}, {key: "Esc", label: "Keep this interface"}, {key: "F1", label: "Help"}}
}

// offerInterfaceReload starts the countdown after a successful controller
// activation when the installed Nixorium differs from the running one.
func (model dashboardModel) offerInterfaceReload() (dashboardModel, tea.Cmd) {
	id := model.reload.id + 1
	model.reload = interfaceReloadModel{id: id}
	if model.actions.InstalledInterface == nil {
		return model, nil
	}
	installed, reload := model.actions.InstalledInterface()
	if !reload {
		return model, nil
	}
	model.reload = interfaceReloadModel{state: interfaceReloadCounting, version: installed.Version, remaining: interfaceReloadSeconds, id: id}
	return model, scheduleInterfaceReloadTick(id)
}

// offerControllerInterfaceReload applies the offer to the Maintenance and
// setup controller result screen.
func (model dashboardModel) offerControllerInterfaceReload() (dashboardModel, tea.Cmd) {
	result := model.controller.result
	if model.installation.flow || model.screen != dashboardController || result.Operation == "" || result.HasErrors() || !result.Applied || !result.Verified {
		model.reload = interfaceReloadModel{id: model.reload.id + 1}
		return model, nil
	}
	return model.offerInterfaceReload()
}

// reloadScreenVisible reports whether a controller result is still on screen.
func (model dashboardModel) reloadScreenVisible() bool {
	if model.helpOpen || model.busy != "" {
		return false
	}
	switch model.screen {
	case dashboardUpdate:
		return model.updates.result.Operation != ""
	case dashboardController:
		return model.controller.result.Operation != ""
	}
	return false
}

// updateInterfaceReloadKey handles Enter and Esc while the countdown runs.
func (model dashboardModel) updateInterfaceReloadKey(key tea.KeyPressMsg) (dashboardModel, tea.Cmd, bool) {
	if !model.reload.counting() || !model.reloadScreenVisible() {
		return model, nil, false
	}
	switch key.String() {
	case "enter":
		model.reload.state = interfaceReloadRequested
		return model, tea.Quit, true
	case "esc":
		model.reload.state = interfaceReloadCancelled
		return model, nil, true
	}
	return model, nil, false
}

func (model dashboardModel) updateInterfaceReloadTick(message interfaceReloadTickMsg) (tea.Model, tea.Cmd) {
	if !model.reload.counting() || message.id != model.reload.id {
		return model, nil
	}
	if !model.reloadScreenVisible() {
		model.reload.state = interfaceReloadCancelled
		return model, nil
	}
	model.reload.remaining--
	if model.reload.remaining <= 0 {
		model.reload.remaining = 0
		model.reload.state = interfaceReloadRequested
		return model, tea.Quit
	}
	return model, scheduleInterfaceReloadTick(message.id)
}
