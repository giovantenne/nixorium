package presentation

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// VS Code may not be installed, so the TUI does not edit its settings or
// extensions; saving another field keeps whatever the profile already has.
func TestWorkspaceEditKeepsExistingVSCodePreferences(t *testing.T) {
	if slices.Contains(workspaceGroups, "VSCode") {
		t.Fatal("VS Code preferences are still offered in the TUI")
	}
	for _, field := range workspaceFields {
		if field.path[0] == "vscode" {
			t.Fatalf("VS Code field %q is still editable", field.label)
		}
	}
	m := workspaceFixture(t)
	profile, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"vscode":{"extensions":["a.b","c.d"],"settings":{"editor.fontSize":16},"extraSettings":{"workbench.colorTheme":"Default Dark+"},"marketplace":[{"publisher":"A","name":"B","version":"1.0.0","hash":"sha256-` + strings.Repeat("A", 43) + `="}]}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	m.workspace.candidate = profile
	m.workspace.group, m.workspace.cursor = 1, 1
	m.workspace.startField()
	if m.workspace.field.path[len(m.workspace.field.path)-1] != "iconSize" {
		t.Fatalf("unexpected Dock field: %v", m.workspace.field.path)
	}
	m.workspace.number = "48"
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	vscode := m.workspace.candidate.VSCode
	if vscode == nil || vscode.Extensions == nil || !slices.Equal(*vscode.Extensions, []string{"a.b", "c.d"}) ||
		vscode.Marketplace == nil || len(*vscode.Marketplace) != 1 || *(*vscode.Marketplace)[0].Version != "1.0.0" ||
		vscode.Settings == nil || *vscode.Settings.FontSize != 16 ||
		vscode.ExtraSettings == nil || (*vscode.ExtraSettings)["workbench.colorTheme"] != "Default Dark+" {
		t.Fatalf("existing VS Code preferences were changed: %+v", vscode)
	}
	if dock := m.workspace.candidate.Desktop; dock == nil || dock.Dock == nil || *dock.Dock.IconSize != 48 {
		t.Fatal("icon size was not set")
	}
}

func TestWorkspaceReviewNamesSettingAndPinChanges(t *testing.T) {
	before, _ := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"vscode":{"extensions":["a.b"],"extraSettings":{"workbench.colorTheme":"Default Dark+","editor.rulers":[80]},"marketplace":[{"publisher":"A","name":"B","version":"1.0.0","hash":"sha256-` + strings.Repeat("A", 43) + `="}]}}`))
	after, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"vscode":{"extensions":["a.b"],"extraSettings":{"workbench.colorTheme":"Default Light+","files.trimTrailingWhitespace":true},"marketplace":[{"publisher":"A","name":"B","version":"1.1.0","hash":"sha256-` + strings.Repeat("B", 43) + `="}]}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var output strings.Builder
	workspaceChangesText(&output, domain.WorkspaceInspection{Base: &before, Resolution: domain.WorkspaceResolution{Declared: after}})
	for _, want := range []string{
		`VSCode / Other settings / editor.rulers: [80] → removed`,
		`VSCode / Other settings / files.trimTrailingWhitespace: (none) → true`,
		`VSCode / Other settings / workbench.colorTheme: "Default Dark+" → "Default Light+"`,
		`VSCode / Marketplace pins / a.b: 1.0.0 → 1.1.0`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("review lacks %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "setting(s) →") {
		t.Fatal("settings were summarized by count")
	}
}
