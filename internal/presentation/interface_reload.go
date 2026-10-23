package presentation

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

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
	interfaceReloadOffered
	interfaceReloadCancelled
	interfaceReloadRequested
)

type interfaceReloadModel struct {
	state   interfaceReloadState
	version string
	id      uint64
}

// line is the result-screen sentence for a successful controller activation.
// The result stays on screen until the administrator chooses: reopening on a
// timer hid the result before it could be read.
func (reload interfaceReloadModel) line() string {
	if reload.state != interfaceReloadOffered && reload.state != interfaceReloadRequested {
		return reopenSentence
	}
	subject := "The updated Nixorium"
	if reload.version != "" {
		subject = "Nixorium " + reload.version
	}
	return fmt.Sprintf("%s is active on this controller. Enter reopens the interface with it.", subject)
}

func (reload interfaceReloadModel) offered() bool { return reload.state == interfaceReloadOffered }

// reloadActions replaces the result actions while the reopening is offered.
func (model dashboardModel) reloadActions() []tuiAction {
	return []tuiAction{{key: "Enter", label: "Reopen with the update"}, {key: "Esc", label: "Keep this interface"}, {key: "F1", label: "Help"}}
}

// ReopenNoticeEnvironment carries the outcome shown before the interface
// reopened on the installed Nixorium, so the new interface shows it too.
const ReopenNoticeEnvironment = "NIXORIUM_REOPEN_NOTICE"

// reloadOutcome is the result to show after the interface reopens.
func (model dashboardModel) reloadOutcome() string {
	if model.screen == dashboardUpdate {
		return "Nixorium and this controller are updated. Next: Computers → Update computers."
	}
	return "This controller runs the updated Nixorium."
}

// offerInterfaceReload offers to reopen the interface after a successful
// controller activation when the installed Nixorium differs from the running
// one.
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
	if installed.Version == model.actions.RunningVersion {
		// A new revision of the same version (master): the number would
		// read as no change.
		installed.Version = ""
	}
	model.reload = interfaceReloadModel{state: interfaceReloadOffered, version: installed.Version, id: id}
	return model, nil
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
	if model.navigation.helpOpen || model.busy != "" {
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

// updateInterfaceReloadKey handles Enter and Esc while the reopening is offered.
func (model dashboardModel) updateInterfaceReloadKey(key tea.KeyPressMsg) (dashboardModel, tea.Cmd, bool) {
	if !model.reload.offered() || !model.reloadScreenVisible() {
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
