package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestEveryEditableSettingHasHelpAndExample(t *testing.T) {
	fields := append([]settingsField{}, settingsFields...)
	for _, group := range routineSettingsGroups {
		fields = append(fields, group.fields...)
	}
	for _, field := range fields {
		info := settingsHelp[field.id]
		if info.description == "" || info.example == "" {
			t.Fatalf("missing help: %s", field.id)
		}
		for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
			m := newSettingsEditorModel(wizardSettings(), []settingsField{field}, "Laboratory settings")
			m.width, m.height = size[0], size[1]
			m.prepareCurrentField()
			view := m.View().Content
			if strings.Contains(view, "Example:") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("field %s does not fit %v: %s", field.id, size, view)
			}
			m.helpOpen = true
			if !strings.Contains(m.View().Content, field.label) || !strings.Contains(m.View().Content, "Example:") {
				t.Fatal("help is not contextual")
			}
		}
	}
}

func TestDraftNetworkPreviewAndReadableLabels(t *testing.T) {
	m := newSettingsEditorModel(wizardSettings(), routineSettingsGroups[0].fields, "Network")
	for index, field := range m.fields {
		if field.id == "lab.networkBase" {
			m.drafts[index] = "10.42.0.0"
			m.index = index
		}
	}
	preview := strings.Join(m.networkPreview(), "\n")
	for _, address := range []string{"10.42.0.1", "10.42.0.20", "10.42.0.99"} {
		if !strings.Contains(preview, address) {
			t.Fatal(preview)
		}
	}
	for index, field := range m.fields {
		if field.id == "lab.masterDhcpIp" {
			m.drafts[index] = "10.42.0.8"
		}
	}
	if !strings.Contains(strings.Join(m.networkPreview(), "\n"), "saving is blocked") {
		t.Fatal("overlap invisible")
	}
	if settingLabel("lab.networkBase") == "lab.networkBase" {
		t.Fatal("review exposes technical field ID")
	}
}

func TestEmptyInterfaceHintUsesFallbackWithoutSavingAnOverride(t *testing.T) {
	settings := wizardSettings()
	settings.Lab.ClientInterfaceName = ""
	settings.Lab.ControllerInterfaceName = "enp8s0"
	settings.Lab.InterfaceName = "enp0s3"
	m := newSettingsEditorModel(settings, installationSettingsFields, "Install computers")
	view := demoANSI.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(view, "enp0s3 (default)") || !strings.Contains(view, "use default") || strings.Contains(view, "enp8s0") || strings.Contains(view, "Example:") {
		t.Fatalf("default is ambiguous: %s", view)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(settingsWizardModel)
	if m.index != 1 || m.settings.Lab.ClientInterfaceName != "" || m.settings.Lab.ControllerInterfaceName != "enp8s0" {
		t.Fatalf("hint became a saved override: %+v", m.settings.Lab)
	}
}

func TestExampleHintDoesNotBecomeInputAndDisappearsWhenTyping(t *testing.T) {
	m := newSettingsEditorModel(wizardSettings(), []settingsField{{id: "lab.masterDhcpIp", group: "Network", label: "Controller DHCP address"}}, "Settings")
	m.drafts[0] = ""
	if !strings.Contains(m.View().Content, "e.g. 192.168.1.20") {
		t.Fatal("missing empty-field format hint")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(settingsWizardModel)
	if m.accepted || m.drafts[0] != "" || m.err == "" {
		t.Fatal("accepted the example as input")
	}
	next, _ = m.Update(tea.PasteMsg{Content: "192.168.1.42"})
	m = next.(settingsWizardModel)
	if m.drafts[0] != "192.168.1.42" || strings.Contains(m.View().Content, "e.g.") {
		t.Fatal("placeholder interfered with real input")
	}
}
