package presentation

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func workspaceType(m dashboardModel, text string) dashboardModel {
	for _, r := range text {
		m, _ = workspaceKey(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func openOtherSettings(t *testing.T) dashboardModel {
	t.Helper()
	m := workspaceFixture(t)
	m.workspace.group = 2
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fields := workspaceGroupFields(2)
	m.workspace.cursor = len(fields) - 1
	if fields[m.workspace.cursor].kind != "settings" {
		t.Fatal("Other settings must be the last VSCode field")
	}
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.stage != workspaceFieldEdit || m.workspace.textEntry() {
		t.Fatalf("settings list not opened: %+v", m.workspace.stage)
	}
	return m
}

func TestWorkspaceOtherSettingsAddEditRemove(t *testing.T) {
	m := openOtherSettings(t)
	m = workspaceType(m, "a")
	if !m.workspace.textEntry() {
		t.Fatal("name entry must capture text, including q and ?")
	}
	m = workspaceType(m, "editor.fontSize")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.settings.stage != workspaceSettingName || !strings.Contains(m.message, "Font size") {
		t.Fatalf("guided setting accepted as extra: %q", m.message)
	}
	for range len("editor.fontSize") {
		m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m = workspaceType(m, "terminal.integrated.profiles.linux")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.settings.stage != workspaceSettingName || !strings.Contains(m.message, "cannot be preset") {
		t.Fatalf("program-launching setting accepted: %q", m.message)
	}
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = workspaceType(m, "a")
	m = workspaceType(m, "workbench.colorTheme")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = workspaceType(m, "Default Dark+")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = workspaceType(m, "a")
	m = workspaceType(m, "editor.rulers")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = workspaceType(m, "[80, 120]")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.settings.stage != workspaceSettingList || m.workspace.textEntry() {
		t.Fatal("value entry did not return to the list")
	}
	view := m.workspaceView()
	if !strings.Contains(view, `workbench.colorTheme = "Default Dark+"`) || !strings.Contains(view, "editor.rulers = [80,120]") {
		t.Fatalf("settings not listed:\n%s", view)
	}
	// Remove the first (sorted) entry, then keep the draft.
	m.workspace.settings.cursor = 0
	m = workspaceType(m, "d")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.stage != workspaceFieldList {
		t.Fatalf("draft not kept: %q", m.message)
	}
	got := *m.workspace.candidate.VSCode.ExtraSettings
	if !reflect.DeepEqual(got, map[string]any{"workbench.colorTheme": "Default Dark+"}) {
		t.Fatalf("unexpected draft: %v", got)
	}
	if !strings.Contains(m.workspaceView(), "1 setting") {
		t.Fatal("field list does not summarize the settings")
	}
	// Removing every entry omits the field instead of storing an empty object.
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = workspaceType(m, "d")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.candidate.VSCode != nil && m.workspace.candidate.VSCode.ExtraSettings != nil {
		t.Fatal("empty settings were not omitted")
	}
}

func TestWorkspaceOtherSettingsPasteAndCancel(t *testing.T) {
	m := openOtherSettings(t)
	next, _ := m.Update(tea.PasteMsg{Content: `{"a.b": 1}`})
	m = next.(dashboardModel)
	if len(m.workspace.settings.values) != 0 || !strings.Contains(m.message, "Press p first") {
		t.Fatal("paste outside paste mode changed the draft")
	}
	m = workspaceType(m, "p")
	source := `{
  // comment with "quotes" and a trailing comma below
  "workbench.colorTheme": "Default Dark+", /* block */
  "files.exclude": { "**/.git": true, },
  "editor.fontSize": 14,
  "update.mode": "default",
  "terminal.integrated.env.linux": { "A": "b" },
  "bad name": 1,
  "with.null": { "x": null },
  "url": "http://example.invalid/a//b",
}`
	next, _ = m.Update(tea.PasteMsg{Content: source})
	m = next.(dashboardModel)
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.settings.stage != workspaceSettingList {
		t.Fatalf("paste rejected: %q", m.message)
	}
	want := map[string]any{
		"workbench.colorTheme": "Default Dark+",
		"files.exclude":        map[string]any{"**/.git": true},
		"url":                  "http://example.invalid/a//b",
	}
	if !reflect.DeepEqual(m.workspace.settings.values, want) {
		t.Fatalf("imported %v", m.workspace.settings.values)
	}
	for _, name := range []string{"3 settings", "bad name", "editor.fontSize", "terminal.integrated.env.linux", "update.mode", "with.null"} {
		if !strings.Contains(m.message, name) {
			t.Fatalf("import summary lacks %q: %s", name, m.message)
		}
	}
	if strings.Contains(m.message, "default") || strings.Contains(m.message, `"b"`) {
		t.Fatal("import summary echoed a skipped value")
	}
	before, _ := domain.MarshalWorkspaceProfile(m.workspace.candidate)
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	after, _ := domain.MarshalWorkspaceProfile(m.workspace.candidate)
	if m.workspace.stage != workspaceFieldList || string(before) != string(after) {
		t.Fatal("Esc did not discard the field edit")
	}
	// Not an object: stays in paste mode with an explanation.
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = workspaceType(m, "p")
	m = workspaceType(m, "[1]")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.settings.stage != workspaceSettingPaste || !strings.Contains(m.message, "not a settings object") {
		t.Fatalf("list accepted as settings: %q", m.message)
	}
}

func TestWorkspaceSettingParse(t *testing.T) {
	for input, want := range map[string]any{
		"Default Dark+": "Default Dark+", `"true"`: "true", "true": true, "14": float64(14), "1.5": 1.5,
		`[1, "a"]`: []any{float64(1), "a"}, `{"a": {"b": false}}`: map[string]any{"a": map[string]any{"b": false}},
		"off": "off", " spaced text ": "spaced text", "12 px": "12 px",
	} {
		got, err := workspaceSettingParse(input)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: got %#v, %v", input, got, err)
		}
	}
	for _, input := range []string{"", "  ", "null", `{"a": 1`, `["a"`, `"unterminated`} {
		if value, err := workspaceSettingParse(input); err == nil {
			t.Fatalf("%q accepted as %#v", input, value)
		}
	}
}

func TestStripJSONComments(t *testing.T) {
	for input, want := range map[string]string{
		`{"a": 1, // x` + "\n}":            `{"a": 1` + " \n\n}",
		`{"a": "// not a comment, }"}`:     `{"a": "// not a comment, }"}`,
		`{"a": "\"/*", /* c */ "b": [1,]}`: `{"a": "\"/*",   "b": [1]}`,
		`{"a": 1, /* c */ }`:               `{"a": 1   }`,
	} {
		if got := stripJSONComments(input); strings.Join(strings.Fields(got), "") != strings.Join(strings.Fields(want), "") {
			t.Fatalf("%q: got %q", input, got)
		}
	}
}

func TestWorkspaceEditKeepsExistingExtensionsAndPins(t *testing.T) {
	m := workspaceFixture(t)
	profile, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"vscode":{"extensions":["a.b","c.d"],"marketplace":[{"publisher":"A","name":"B","version":"1.0.0","hash":"sha256-` + strings.Repeat("A", 43) + `="}]}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	m.workspace.candidate = profile
	for _, field := range workspaceGroupFields(2) {
		if field.kind == "extensions" || field.kind == "marketplace" {
			t.Fatalf("extension field %q is still editable", field.label)
		}
	}
	m.workspace.group = 2
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.field.path[len(m.workspace.field.path)-1] != "editor.fontSize" {
		t.Fatalf("unexpected first VSCode field: %v", m.workspace.field.path)
	}
	m = workspaceType(m, "14")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	vscode := m.workspace.candidate.VSCode
	if vscode == nil || vscode.Extensions == nil || !slices.Equal(*vscode.Extensions, []string{"a.b", "c.d"}) ||
		vscode.Marketplace == nil || len(*vscode.Marketplace) != 1 || *(*vscode.Marketplace)[0].Version != "1.0.0" {
		t.Fatalf("existing extensions or pins were changed: %+v", vscode)
	}
	if vscode.Settings == nil {
		t.Fatal("font size was not set")
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
