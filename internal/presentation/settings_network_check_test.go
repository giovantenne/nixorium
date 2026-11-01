package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func networkCheckDevices() domain.NetworkObservation {
	return domain.NetworkObservation{Reachable: true, Lifetimes: true, Devices: []domain.NetworkDevice{
		{Name: "enp0s3", Kind: domain.NetworkDeviceWired, Addresses: []domain.NetworkAddress{{Address: "192.168.0.198", Dynamic: true}}},
		{Name: "wlp2s0", Kind: domain.NetworkDeviceWireless},
	}}
}

// networkReviewModel is a review of a changed interface with the given
// network check, as the application plan returns it.
func networkReviewModel(t *testing.T, change func(*domain.LabSettings), preflight *domain.NetworkPreflight, saved *[]domain.ConfigPlanReport) dashboardModel {
	t.Helper()
	base := wizardSettings()
	base.Lab.PCCount = 3
	candidate := base
	change(&candidate.Lab)
	changes := domain.DiffLabSettings(base, candidate)
	return dashboardModel{screen: dashboardSettingsReview, width: 240, height: 70,
		settings: settingsModel{current: base, candidate: candidate, plan: domain.ConfigPlanReport{
			State: "valid", Changes: changes, Impacts: domain.SettingsImpacts(base, candidate, changes), NetworkPreflight: preflight,
		}},
		actions: DashboardActions{SaveSettings: func(_ domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			*saved = append(*saved, plan)
			return domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved"}
		}},
	}
}

func pressReviewKey(t *testing.T, model dashboardModel, key tea.KeyPressMsg) (dashboardModel, tea.Cmd) {
	t.Helper()
	updated, command := model.Update(key)
	return updated.(dashboardModel), command
}

func TestSettingsReviewRefusesAMissingControllerDevice(t *testing.T) {
	check := domain.CheckControllerInterface("eth0", "enp5s0", networkCheckDevices(), "192.168.0.198")
	saved := []domain.ConfigPlanReport{}
	model := networkReviewModel(t, func(lab *domain.LabSettings) { lab.ControllerInterfaceName = "enp5s0" }, &domain.NetworkPreflight{Controller: &check}, &saved)
	view := plainView(model)
	for _, want := range []string{
		"Network check",
		"× The controller has no network device called enp5s0. Its devices are: enp0s3 (192.168.0.198, carries the DHCP address), wlp2s0 (wireless).",
		"Saving is blocked by the network check",
		"Attention: Applying to the controller can move its laboratory connection",
	} {
		if !contains(view, want) {
			t.Fatalf("missing %q in review:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Enter Save") || strings.Contains(view, "Save anyway") {
		t.Fatalf("a blocked review offers saving:\n%s", view)
	}
	model, command := pressReviewKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil || len(saved) != 0 || !strings.Contains(model.message, "Not saved: The controller has no network device called enp5s0") {
		t.Fatalf("blocked save: command=%v saved=%d message=%q", command != nil, len(saved), model.message)
	}
}

func clientNetworkPreflight() *domain.NetworkPreflight {
	host := func(name, ip string) domain.HostMeta { return domain.HostMeta{Name: name, IP: ip, Interface: "enp3s0"} }
	wired := func(names ...string) domain.NetworkObservation {
		devices := []domain.NetworkDevice{}
		for _, name := range names {
			devices = append(devices, domain.NetworkDevice{Name: name, Kind: domain.NetworkDeviceWired})
		}
		return domain.NetworkObservation{Reachable: true, Devices: devices}
	}
	return &domain.NetworkPreflight{Clients: []domain.ClientNetworkCheck{
		domain.CheckClientInterface(host("pc01", "10.0.0.1"), "", wired("enp3s0")),
		domain.CheckClientInterface(host("pc02", "10.0.0.2"), "", wired("enp2s0", "wlp3s0")),
		domain.CheckClientInterface(host("pc03", "10.0.0.3"), "", domain.NetworkObservation{Detail: "no answer"}),
	}}
}

func TestSettingsReviewBlocksMissingClientDevicesUntilAcknowledged(t *testing.T) {
	saved := []domain.ConfigPlanReport{}
	model := networkReviewModel(t, func(lab *domain.LabSettings) { lab.ClientInterfaceName = "enp3s0" }, clientNetworkPreflight(), &saved)
	view := plainView(model)
	for _, want := range []string{
		"• Update one computer first and check it answers before updating the others.",
		"Attention: Updating clients can move their laboratory connection",
		"✓ pc01  device present (enp3s0)",
		"× pc02  device missing: no enp3s0 (its devices: enp2s0, wlp3s0)",
		"○ pc03  not reachable: check it at its screen before updating it",
		"Correct the name, or press c to save anyway for the computers listed.",
		"Saving is blocked by the network check",
		"c Save anyway for the computers listed",
	} {
		if !contains(view, want) {
			t.Fatalf("missing %q in review:\n%s", want, view)
		}
	}
	model, command := pressReviewKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil || len(saved) != 0 || !strings.Contains(model.message, "missing on pc02") {
		t.Fatalf("unacknowledged save: message=%q", model.message)
	}

	model, _ = pressReviewKey(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	view = plainView(model)
	for _, want := range []string{"! pc02  device missing", "Saving anyway for pc02", "Save these settings?", "Enter Save", "c Do not save anyway"} {
		if !contains(view, want) {
			t.Fatalf("missing %q after acknowledging:\n%s", want, view)
		}
	}
	model, command = pressReviewKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("acknowledged review did not save")
	}
	command()
	if len(saved) != 1 || strings.Join(saved[0].NetworkPreflight.Acknowledged, ",") != "pc02" {
		t.Fatalf("saved plans = %+v", saved)
	}
	if model.settings.plan.NetworkPreflight.Acknowledged == nil {
		t.Fatal("a retried save would lose the acknowledgement")
	}
}

func TestSettingsReviewSavesWhenEveryReachableComputerHasTheDevice(t *testing.T) {
	preflight := clientNetworkPreflight()
	preflight.Clients = append(preflight.Clients[:1], preflight.Clients[2])
	saved := []domain.ConfigPlanReport{}
	model := networkReviewModel(t, func(lab *domain.LabSettings) { lab.ClientInterfaceName = "enp3s0" }, preflight, &saved)
	view := plainView(model)
	if !strings.Contains(view, "Save these settings?") || strings.Contains(view, "Save anyway") || !strings.Contains(view, "pc03  not reachable") {
		t.Fatalf("review:\n%s", view)
	}
	model, _ = pressReviewKey(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if model.settings.saveAnyway {
		t.Fatal("c acknowledged although no device is missing")
	}
	_, command := pressReviewKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("review did not save")
	}
	command()
	if len(saved) != 1 || saved[0].NetworkPreflight.Acknowledged != nil {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestInstallationReturnsToTheFormWhenTheNetworkCheckRefuses(t *testing.T) {
	check := domain.CheckControllerInterface("eth0", "enp5s0", networkCheckDevices(), "")
	model := dashboardModel{width: 120, height: 40, installation: installationModel{flow: true}, settings: settingsModel{candidate: wizardSettings()},
		actions: DashboardActions{SaveSettings: func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			t.Fatal("installation saved a refused network change")
			return domain.ConfigurationSaveReport{}
		}}}
	updated, command := model.Update(dashboardSettingsPlanMsg{report: domain.ConfigPlanReport{State: "valid", Changes: []domain.SettingChange{{Field: "lab.clientIfaceName"}}, NetworkPreflight: &domain.NetworkPreflight{Controller: &check}}})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardSettingsEdit || !strings.Contains(model.settings.editor.err, "no network device called enp5s0") {
		t.Fatalf("screen=%d err=%q", model.screen, model.settings.editor.err)
	}
}

func networkGroupFields() []settingsField {
	for _, group := range routineSettingsGroups {
		if group.id == "network" {
			return group.fields
		}
	}
	return nil
}

func dhcpFieldEditor(t *testing.T, saved string, addresses domain.ControllerAddresses) settingsWizardModel {
	t.Helper()
	current := wizardSettings()
	current.Lab.MasterDHCPIP = saved
	model := dashboardModel{width: 120, height: 40, settings: settingsModel{current: current, addresses: addresses}}
	model = model.openSettingsEditor(networkGroupFields(), "Maintenance / Settings / Network")
	editor := model.settings.editor
	for index, field := range editor.fields {
		if field.id == "lab.masterDhcpIp" {
			return editor.moveToField(index)
		}
	}
	t.Fatal("no DHCP field")
	return editor
}

func editorKey(model settingsWizardModel, key tea.KeyPressMsg) settingsWizardModel {
	updated, _ := model.Update(key)
	return updated.(settingsWizardModel)
}

func TestControllerDHCPAddressIsPickedFromDetectedAddresses(t *testing.T) {
	addresses := domain.ControllerAddresses{Detected: true, Choices: []domain.DHCPAddressChoice{
		{Address: "198.51.100.7", Device: "enp0s8"}, {Address: "192.0.2.10", Device: "enp0s3"},
	}}
	editor := dhcpFieldEditor(t, "192.0.2.10", addresses)
	view := ansi.Strip(renderTUIShell(editor.shell(), 120, false))
	for _, want := range []string{"Choose one of the controller's current addresses, or Other to type one.", "198.51.100.7", "Current address on enp0s8", "192.0.2.10", "Other…"} {
		if !contains(view, want) {
			t.Fatalf("missing %q in picker:\n%s", want, view)
		}
	}
	if selected := editor.selector.SelectedItem().(settingsChoiceItem); selected.value != "192.0.2.10" {
		t.Fatalf("current value is not preselected: %+v", selected)
	}
	// Picking a detected address needs no warning.
	picked := editorKey(editorKey(editor, tea.KeyPressMsg{Code: tea.KeyUp}), tea.KeyPressMsg{Code: tea.KeyEnter})
	if picked.settings.Lab.MasterDHCPIP != "198.51.100.7" || picked.warning != "" || picked.fields[picked.index].id != "lab.networkBase" {
		t.Fatalf("picked = %q warning=%q field=%s", picked.settings.Lab.MasterDHCPIP, picked.warning, picked.fields[picked.index].id)
	}

	// Other… accepts a typed address with a warning.
	other := editorKey(editorKey(editor, tea.KeyPressMsg{Code: tea.KeyDown}), tea.KeyPressMsg{Code: tea.KeyEnter})
	if !other.custom {
		t.Fatal("Other… did not open typing")
	}
	for _, character := range "203.0.113.4" {
		other = editorKey(other, tea.KeyPressMsg{Code: character, Text: string(character)})
	}
	other = editorKey(other, tea.KeyPressMsg{Code: tea.KeyEnter})
	if other.settings.Lab.MasterDHCPIP != "203.0.113.4" || !strings.Contains(other.warning, "203.0.113.4 is not one of the controller's current addresses (found: 198.51.100.7, 192.0.2.10)") {
		t.Fatalf("typed = %q warning=%q", other.settings.Lab.MasterDHCPIP, other.warning)
	}
	if view := ansi.Strip(renderTUIShell(other.shell(), 160, false)); !strings.Contains(view, "is not one of the controller's current addresses") {
		t.Fatalf("warning not shown:\n%s", view)
	}
}

func TestControllerDHCPPickerKeepsAnUndetectedSavedValue(t *testing.T) {
	editor := dhcpFieldEditor(t, "192.0.2.10", domain.ControllerAddresses{Detected: true, Choices: []domain.DHCPAddressChoice{{Address: "198.51.100.7", Device: "enp0s8"}}})
	view := ansi.Strip(renderTUIShell(editor.shell(), 120, false))
	if !strings.Contains(view, "Saved value; the controller does not have it now") {
		t.Fatalf("saved value not explained:\n%s", view)
	}
	if selected := editor.selector.SelectedItem().(settingsChoiceItem); selected.value != "192.0.2.10" {
		t.Fatalf("saved value is not preselected: %+v", selected)
	}
	// Without detection the field stays a typed value, as before.
	plain := dhcpFieldEditor(t, "192.0.2.10", domain.ControllerAddresses{})
	if len(plain.fields[plain.index].choices) != 0 {
		t.Fatalf("undetected addresses offered choices: %+v", plain.fields[plain.index])
	}
}

func TestSettingsLoadReadsControllerAddresses(t *testing.T) {
	read := 0
	model := dashboardModel{actions: DashboardActions{
		LoadSettings: func(context.Context) (domain.LabSettingsFile, error) { return wizardSettings(), nil },
		LoadControllerAddresses: func(_ context.Context, settings domain.LabSettingsFile) domain.ControllerAddresses {
			read++
			return domain.ControllerAddresses{Detected: true, Choices: []domain.DHCPAddressChoice{{Address: "198.51.100.7", Device: "enp0s8"}}}
		},
	}}
	message := model.loadSettingsMsg(context.Background()).(dashboardSettingsMsg)
	updated, _ := model.Update(message)
	if read != 1 || !updated.(dashboardModel).settings.addresses.Detected {
		t.Fatalf("addresses not loaded: read=%d", read)
	}
}

func TestInstallationFormMovesTheControllerToTheChosenDHCPDevice(t *testing.T) {
	current := wizardSettings()
	current.Lab.ControllerInterfaceName = "enp0s8"
	current.Lab.MasterDHCPIP = "198.51.100.7"
	addresses := domain.ControllerAddresses{Detected: true, Choices: []domain.DHCPAddressChoice{
		{Address: "198.51.100.7", Device: "enp0s8"}, {Address: "192.0.2.10", Device: "enp0s3"},
	}}
	model := dashboardModel{width: 120, height: 40, settings: settingsModel{current: current, addresses: addresses}}
	editor := model.newSettingsEditor(current, installationSettingsFields, "Installation / Laboratory settings")
	for editor.fields[editor.index].id != "lab.masterDhcpIp" {
		editor = editor.moveToField(editor.index + 1)
	}
	picked := editorKey(editorKey(editor, tea.KeyPressMsg{Code: tea.KeyDown}), tea.KeyPressMsg{Code: tea.KeyEnter})
	if picked.settings.Lab.MasterDHCPIP != "192.0.2.10" || picked.settings.Lab.ControllerInterface() != "enp0s3" {
		t.Fatalf("dhcp=%q controller=%q", picked.settings.Lab.MasterDHCPIP, picked.settings.Lab.ControllerInterface())
	}
	if view := ansi.Strip(renderTUIShell(picked.shell(), 160, false)); !strings.Contains(view, "The controller will use enp0s3 for the laboratory network: it carries 192.0.2.10.") {
		t.Fatalf("change not reported:\n%s", view)
	}
	// Picking the address on the controller's own device changes nothing.
	same := editorKey(editor, tea.KeyPressMsg{Code: tea.KeyEnter})
	if same.settings.Lab.ControllerInterface() != "enp0s8" || same.note != "" {
		t.Fatalf("controller=%q note=%q", same.settings.Lab.ControllerInterface(), same.note)
	}
	// The network settings group asks for the device, so it is never moved.
	group := dhcpFieldEditor(t, "198.51.100.7", addresses)
	group.settings.Lab.ControllerInterfaceName = "enp0s8"
	kept := editorKey(editorKey(group, tea.KeyPressMsg{Code: tea.KeyDown}), tea.KeyPressMsg{Code: tea.KeyEnter})
	if kept.settings.Lab.ControllerInterface() != "enp0s8" {
		t.Fatalf("network group moved the controller to %q", kept.settings.Lab.ControllerInterface())
	}
}

func TestInstallationSummaryNamesTheControllerInterface(t *testing.T) {
	current := wizardSettings()
	current.Lab.ControllerInterfaceName = "enp0s3"
	current.Lab.ClientInterfaceName = ""
	model := dashboardModel{width: 120, height: 40, settings: settingsModel{current: current}}
	view := ansi.Strip(model.installationSummaryView())
	for _, want := range []string{"Controller interface  enp0s3", "Client interface      automatic (wired network card)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestControllerInterfaceIsChosenAmongItsWiredCards(t *testing.T) {
	current := wizardSettings()
	current.Lab.InterfaceName, current.Lab.ControllerInterfaceName = "enp0s3", ""
	addresses := domain.ControllerAddresses{Detected: true, Wired: []domain.NetworkDevice{
		{Name: "enp0s3", Kind: domain.NetworkDeviceWired, Addresses: []domain.NetworkAddress{{Address: "192.168.57.99"}, {Address: "10.0.0.99"}}},
		{Name: "enp0s8", Kind: domain.NetworkDeviceWired},
	}}
	model := dashboardModel{width: 120, height: 40, settings: settingsModel{current: current, addresses: addresses}}
	model = model.openSettingsEditor(networkGroupFields(), "Maintenance / Settings / Network")
	editor := model.settings.editor
	if editor.fields[editor.index].id != "lab.controllerIfaceName" {
		t.Fatalf("first field = %s", editor.fields[editor.index].id)
	}
	for _, field := range editor.fields {
		if field.id == "lab.ifaceName" {
			t.Fatal("the shared fallback name is still asked")
		}
	}
	view := ansi.Strip(renderTUIShell(editor.shell(), 140, false))
	for _, want := range []string{"Choose the controller's card on the laboratory network", "Wired card · 192.168.57.99, 10.0.0.99", "Wired card without an address now", "Type a device name that is not listed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	if selected := editor.selector.SelectedItem().(settingsChoiceItem); selected.value != "enp0s3" {
		t.Fatalf("the card in use is not preselected: %+v", selected)
	}
	// Confirming the card already in use changes nothing.
	kept := editorKey(editor, tea.KeyPressMsg{Code: tea.KeyEnter})
	if kept.settings.Lab.ControllerInterfaceName != "" {
		t.Fatalf("confirming the inherited card saved %q", kept.settings.Lab.ControllerInterfaceName)
	}
	moved := editorKey(editorKey(editor, tea.KeyPressMsg{Code: tea.KeyDown}), tea.KeyPressMsg{Code: tea.KeyEnter})
	if moved.settings.Lab.ControllerInterfaceName != "enp0s8" {
		t.Fatalf("choosing another card saved %q", moved.settings.Lab.ControllerInterfaceName)
	}
}

func TestSettingsReviewRefusesAClientWiFiCard(t *testing.T) {
	preflight := &domain.NetworkPreflight{Clients: []domain.ClientNetworkCheck{
		domain.CheckClientInterface(domain.HostMeta{Name: "pc01", IP: "10.0.0.1", Interface: "wlp2s0"}, "", networkCheckDevices()),
		// A missing device elsewhere must not offer "save anyway" past the Wi-Fi card.
		domain.CheckClientInterface(domain.HostMeta{Name: "pc02", IP: "10.0.0.2", Interface: "wlp2s0"}, "", domain.NetworkObservation{Reachable: true}),
	}}
	saved := []domain.ConfigPlanReport{}
	model := networkReviewModel(t, func(lab *domain.LabSettings) { lab.ClientInterfaceName = "wlp2s0" }, preflight, &saved)
	view := plainView(model)
	for _, want := range []string{"× pc01  wlp2s0 is a Wi-Fi card: the laboratory needs a wired one", "Saving is blocked by the network check"} {
		if !contains(view, want) {
			t.Fatalf("missing %q in review:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Save anyway") {
		t.Fatalf("a Wi-Fi card can be saved anyway:\n%s", view)
	}
	model, command := pressReviewKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if command != nil || len(saved) != 0 || !strings.Contains(model.message, "Wi-Fi card on pc01") {
		t.Fatalf("Wi-Fi card saved: message=%q", model.message)
	}
	if model, _ = pressReviewKey(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"}); model.settings.saveAnyway {
		t.Fatal("c acknowledged past a Wi-Fi card")
	}
}
