package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (model dashboardModel) installationSummaryView() string {
	lab := model.settings.current.Lab
	clientInterface := lab.ClientInterfaceName
	if clientInterface == "" {
		clientInterface = lab.InterfaceName
	}
	lines := []string{
		tuiTitle("Use the saved installation settings?", model.isDark), "",
		fmt.Sprintf("Laboratory network  %s/%d", lab.NetworkBase, lab.NetworkPrefix),
		"Controller DHCP     " + lab.MasterDHCPIP,
		"Client interface    " + clientInterface,
		fmt.Sprintf("Client computers    %d", lab.PCCount),
		"Accounts            admin / " + lab.TeacherUser + " / " + lab.StudentUser,
		"", "Continue the guided preparation using these saved values.",
		"Keys, configuration, controller and installation files are checked next.",
		"This summary is not a readiness check. Starting network installation still requires its own review.",
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
	}
	if model.message != "" {
		lines = append(lines, "", model.message)
	}
	return model.renderShell(tuiShell{path: []string{"Installation", "Network boot", "Saved settings"}, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "Enter", label: "Continue"}, {key: "e", label: "Edit settings"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}})
}

func (model dashboardModel) updateInstallationSummary(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.installation.savedSummary, model.installation.flow = false, false
		model.screen = dashboardPXE
	case "e":
		model.installation.savedSummary = false
		model.settings.editor = newSettingsEditorModel(model.settings.current, installationSettingsFields, "Nixorium — Install computers / Laboratory settings")
		model.settings.editor.width, model.settings.editor.height, model.settings.editor.isDark = model.width, model.height, model.isDark
		model.settings.editor.prepareCurrentField()
		model.screen = dashboardSettingsEdit
	case "enter":
		if model.actions.LoadSetup == nil {
			model.message = "Installation checks are unavailable in this session."
			return model, nil
		}
		model.installation.savedSummary = false
		model.screen = dashboardPXE
		model.busy = "Checking saved installation prerequisites"
		return model.loadSetup()
	}
	return model, nil
}
