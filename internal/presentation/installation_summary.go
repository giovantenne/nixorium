package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) installationSummaryView() string {
	lab := model.settings.current.Lab
	clientInterface := lab.ClientInterfaceName
	if clientInterface == "" {
		clientInterface = "automatic (wired network card)"
	}
	lines := []string{
		tuiTitle("Use the saved installation settings?", model.isDark), "",
		fmt.Sprintf("Laboratory network    %s/%d", lab.NetworkBase, lab.NetworkPrefix),
		"Controller interface  " + lab.ControllerInterface(),
		"Controller DHCP       " + summaryDHCPAddress(lab.MasterDHCPIP, lab.ControllerInterface(), model.settings.addresses),
		"Client interface      " + clientInterface,
		fmt.Sprintf("Client computers      %d", lab.PCCount),
		"Accounts              admin / " + lab.TeacherUser + " / " + lab.StudentUser,
		"", "Continue the guided preparation using these saved values.",
		"Keys, configuration, controller and installation files are checked next.",
		"This summary is not a readiness check. The installation still requires its own review.",
	}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	method := "Network boot"
	if model.installation.method == domain.RemoteInstallUSBSSH {
		method = "USB over SSH"
	}
	return model.renderShell(tuiShell{path: []string{"Installation", method, "Saved settings"}, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Enter", label: "Continue"}, {key: "e", label: "Edit settings"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}})
}

func (model dashboardModel) updateInstallationSummary(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		model.installation.savedSummary, model.installation.flow = false, false
		model.screen = dashboardPXE
		if model.installation.method == domain.RemoteInstallUSBSSH {
			model.screen = dashboardInstallationArea
		}
	case "e":
		model.installation.savedSummary = false
		model.settings.editor = model.newSettingsEditor(model.settings.current, installationSettingsFields, "Installation / Laboratory settings")
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

// summaryDHCPAddress shows the address the controller has now when the saved
// one is gone (a changed lease, a restored lab): preparation uses the
// current address, so the saved one would only mislead.
func summaryDHCPAddress(saved, device string, addresses domain.ControllerAddresses) string {
	if !addresses.Detected || len(addresses.Choices) == 0 {
		return saved
	}
	for _, choice := range addresses.Choices {
		if choice.Address == saved {
			return saved
		}
	}
	current := addresses.Choices[0].Address
	for _, choice := range addresses.Choices {
		if choice.Device == device {
			current = choice.Address
			break
		}
	}
	if saved == "" || saved == domain.MasterDHCPPlaceholder {
		return current + " (detected now)"
	}
	return current + " (detected now; the saved " + saved + " is no longer on this controller)"
}
