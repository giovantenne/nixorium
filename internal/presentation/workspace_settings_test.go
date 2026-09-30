package presentation

import (
	"context"
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
	m := workspaceFixture()
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
	if !strings.Contains(m.workspaceView(), "1 setting(s)") {
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
	for _, name := range []string{"3 setting(s)", "bad name", "editor.fontSize", "terminal.integrated.env.linux", "update.mode", "with.null"} {
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

func TestWorkspaceExtensionSearchAddsPackagedChoices(t *testing.T) {
	m := workspaceFixture()
	var queries []string
	m.actions.SearchSoftware = func(_ context.Context, query string) domain.SoftwareSearchReport {
		queries = append(queries, query)
		return domain.SoftwareSearchReport{State: "ready", Results: []domain.SoftwareCatalogItem{
			{ID: "vscode-extensions.ms-python.python", Label: "python", Summary: "Python support", Version: "2026.4.0", Availability: "available"},
			{ID: "vscode-extensions.ms-python.vscode-pylance", Label: "pylance", Summary: "Pylance", Version: "2026.2.1", Availability: "available"},
			{ID: "vscode-extensions.other.blocked", Label: "blocked", Summary: "Broken", Version: "1", Availability: "blocked-broken"},
			{ID: "python3", Label: "python3", Summary: "Interpreter", Version: "3", Availability: "available"},
		}}
	}
	m.workspace.group = 2
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.field.kind != "extensions" {
		t.Fatal("first VSCode field must be the extension list")
	}
	m = workspaceType(m, "/")
	if !m.workspace.searching || !m.workspace.textEntry() {
		t.Fatal("search did not capture text")
	}
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.workspace.searching || m.workspace.stage != workspaceFieldEdit {
		t.Fatal("Esc must close only the search")
	}
	m = workspaceType(m, "/")
	m = workspaceType(m, "q")
	m, cmd := workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(m.message, "two characters") {
		t.Fatal("a one-letter query must not search")
	}
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = workspaceType(m, "Python")
	m, cmd = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = workspaceComplete(t, m, cmd)
	if len(queries) != 1 || queries[0] != "vscode-extensions.python" {
		t.Fatalf("unexpected search %v", queries)
	}
	if m.workspace.searching || !strings.Contains(m.message, "2 packaged extension(s)") {
		t.Fatalf("results not reported: %q", m.message)
	}
	for _, id := range []string{"other.blocked", "python3"} {
		if slices.Contains(m.workspace.choices, id) {
			t.Fatalf("offered unusable result %s", id)
		}
	}
	if m.workspace.choices[m.workspace.choice] != "ms-python.python" || !strings.Contains(m.workspaceView(), "2026.4.0 · Python support") {
		t.Fatal("search result is not focused with its version")
	}
	m = workspaceType(m, " ")
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !slices.Contains(*m.workspace.candidate.VSCode.Extensions, "ms-python.python") {
		t.Fatal("searched extension was not kept in the draft")
	}
	// Reopening keeps a selected non-catalog extension visible.
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !slices.Contains(m.workspace.choices, "ms-python.python") {
		t.Fatal("selected extension disappeared from the choices")
	}
}

func TestWorkspaceReviewNamesSettingChanges(t *testing.T) {
	before, _ := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"vscode":{"extensions":["a.b"],"extraSettings":{"workbench.colorTheme":"Default Dark+","editor.rulers":[80]}}}`))
	after, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"vscode":{"extensions":["a.b"],"extraSettings":{"workbench.colorTheme":"Default Light+","files.trimTrailingWhitespace":true}}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var output strings.Builder
	workspaceChangesText(&output, domain.WorkspaceInspection{Base: &before, Resolution: domain.WorkspaceResolution{Declared: after}})
	for _, want := range []string{
		`VSCode / Other settings / editor.rulers: [80] → removed`,
		`VSCode / Other settings / files.trimTrailingWhitespace: (none) → true`,
		`VSCode / Other settings / workbench.colorTheme: "Default Dark+" → "Default Light+"`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("review lacks %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "setting(s) →") {
		t.Fatal("settings were summarized by count")
	}
}
