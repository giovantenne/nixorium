package presentation

import (
	"strings"
	"testing"

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
			if !strings.Contains(view, "Example:") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("field %s does not fit %v: %s", field.id, size, view)
			}
			m.helpOpen = true
			if !strings.Contains(m.View().Content, field.label) {
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
