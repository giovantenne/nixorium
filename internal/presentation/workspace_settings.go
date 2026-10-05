package presentation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type workspaceSettingStage int

const (
	workspaceSettingList workspaceSettingStage = iota
	workspaceSettingName
	workspaceSettingValue
	workspaceSettingPaste
)

// The profile file itself is limited to 64 KiB; a pasted source with comments
// may be somewhat larger before it is reduced to the accepted settings.
const workspaceSettingInputLimit = 256 * 1024

// workspaceSettingsEditor edits a private copy of vscode.extraSettings. The
// domain decoder remains the authority when the draft is kept.
type workspaceSettingsEditor struct {
	stage  workspaceSettingStage
	values map[string]any
	cursor int
	name   string
	input  string
}

func newWorkspaceSettingsEditor(value any) workspaceSettingsEditor {
	editor := workspaceSettingsEditor{values: map[string]any{}}
	if current, ok := value.(map[string]any); ok {
		for name, item := range current {
			editor.values[name] = item
		}
	}
	return editor
}

func (editor workspaceSettingsEditor) names() []string {
	names := make([]string, 0, len(editor.values))
	for name := range editor.values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (editor workspaceSettingsEditor) textEntry() bool {
	return editor.stage != workspaceSettingList
}

// result returns nil for an empty set: omitting the field inherits the
// deployment baseline, and an empty object would not clear it anyway.
func (editor workspaceSettingsEditor) result() any {
	if len(editor.values) == 0 {
		return nil
	}
	return editor.values
}

func workspaceSettingText(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

// workspaceSettingParse reads what an administrator typed. JSON is used when
// it parses; anything else is kept as plain text, so a theme name needs no
// quotes while true, 14, lists and objects keep their JSON meaning.
func workspaceSettingParse(input string) (any, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, errors.New("enter a value, or press Esc to cancel")
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	if err := decoder.Decode(&value); err == nil {
		if _, err := decoder.Token(); errors.Is(err, io.EOF) {
			if value == nil {
				return nil, errors.New("null is not supported; remove the setting instead")
			}
			return value, nil
		}
	}
	if strings.ContainsAny(trimmed[:1], `{["`) {
		return nil, errors.New("this looks like JSON but is not valid; check quotes, commas and brackets")
	}
	return trimmed, nil
}

// stripJSONComments accepts the comments and trailing commas that the editor
// allows in its own settings file, leaving string contents untouched.
func stripJSONComments(source string) string {
	var out strings.Builder
	inString, escaped := false, false
	for index := 0; index < len(source); index++ {
		char := source[index]
		if inString {
			out.WriteByte(char)
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			}
			continue
		}
		switch {
		case char == '"':
			inString = true
			out.WriteByte(char)
		case char == '/' && index+1 < len(source) && source[index+1] == '/':
			for index < len(source) && source[index] != '\n' {
				index++
			}
			out.WriteByte('\n')
		case char == '/' && index+1 < len(source) && source[index+1] == '*':
			end := strings.Index(source[index+2:], "*/")
			if end < 0 {
				return out.String()
			}
			index += end + 3
			out.WriteByte(' ')
		case char == ',':
			rest := strings.TrimLeft(stripLeadingComments(source[index+1:]), " \t\r\n")
			if rest == "" || (rest[0] != '}' && rest[0] != ']') {
				out.WriteByte(char)
			}
		default:
			out.WriteByte(char)
		}
	}
	return out.String()
}

func stripLeadingComments(source string) string {
	for {
		trimmed := strings.TrimLeft(source, " \t\r\n")
		switch {
		case strings.HasPrefix(trimmed, "//"):
			end := strings.IndexByte(trimmed, '\n')
			if end < 0 {
				return ""
			}
			source = trimmed[end+1:]
		case strings.HasPrefix(trimmed, "/*"):
			end := strings.Index(trimmed[2:], "*/")
			if end < 0 {
				return ""
			}
			source = trimmed[end+4:]
		default:
			return trimmed
		}
	}
}

func workspaceHasNull(value any) bool {
	switch item := value.(type) {
	case nil:
		return true
	case map[string]any:
		for _, child := range item {
			if workspaceHasNull(child) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(item, workspaceHasNull)
	}
	return false
}

// importSettings merges a pasted settings object. Names the profile cannot
// carry are skipped and named, never their values.
func (editor *workspaceSettingsEditor) importSettings(source string) (message string, ok bool) {
	var object map[string]any
	decoder := json.NewDecoder(strings.NewReader(stripJSONComments(source)))
	if err := decoder.Decode(&object); err != nil || object == nil {
		return "The pasted text is not a settings object. Paste the whole file, from { to }.", false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return "The pasted text has content after the settings object.", false
	}
	added, skipped := 0, []string{}
	for name, value := range object {
		if domain.WorkspaceExtraSettingIssue(name) != "" || workspaceHasNull(value) {
			skipped = append(skipped, name)
			continue
		}
		editor.values[name] = value
		added++
	}
	sort.Strings(skipped)
	message = countNoun(added, "setting") + " taken from the pasted text."
	if len(skipped) != 0 {
		shown := skipped
		if len(shown) > 8 {
			shown = append(append([]string{}, shown[:8]...), fmt.Sprintf("and %d more", len(skipped)-8))
		}
		message += " Skipped (guided field, managed or unsupported): " + strings.Join(shown, ", ") + "."
	}
	return message, true
}

func (editor *workspaceSettingsEditor) paste(content string) string {
	if editor.stage == workspaceSettingList {
		return "Press p first, then paste the settings."
	}
	if editor.stage == workspaceSettingName {
		content = strings.Join(strings.Fields(content), "")
	}
	if len(editor.input)+len(content) > workspaceSettingInputLimit {
		return "The text is too long for a settings profile."
	}
	editor.input += content
	return ""
}

// update handles one key and reports whether the field edit is finished:
// "keep" stores the draft, "cancel" abandons it, "" stays in the editor.
func (editor *workspaceSettingsEditor) update(key tea.KeyPressMsg) (action, message string) {
	if editor.stage == workspaceSettingList {
		names := editor.names()
		switch key.String() {
		case "esc":
			return "cancel", ""
		case "enter":
			return "keep", ""
		case "up", "k":
			editor.cursor = max(0, editor.cursor-1)
		case "down", "j":
			editor.cursor = min(max(0, len(names)-1), editor.cursor+1)
		case "a":
			editor.stage, editor.name, editor.input = workspaceSettingName, "", ""
		case "p":
			editor.stage, editor.input = workspaceSettingPaste, ""
		case "e":
			if len(names) != 0 {
				editor.name = names[editor.cursor]
				editor.stage, editor.input = workspaceSettingValue, workspaceSettingText(editor.values[editor.name])
			}
		case "d":
			if len(names) != 0 {
				delete(editor.values, names[editor.cursor])
				editor.cursor = min(editor.cursor, max(0, len(names)-2))
				return "", "Setting removed from the draft."
			}
		}
		return "", ""
	}
	switch key.String() {
	case "esc":
		editor.stage, editor.input = workspaceSettingList, ""
		return "", "Nothing was added."
	case "backspace":
		if editor.input != "" {
			_, size := utf8.DecodeLastRuneInString(editor.input)
			editor.input = editor.input[:len(editor.input)-size]
		}
		return "", ""
	case "enter":
		switch editor.stage {
		case workspaceSettingName:
			name := strings.TrimSpace(editor.input)
			if issue := domain.WorkspaceExtraSettingIssue(name); issue != "" {
				return "", workspaceSettingNameHelp(name, issue)
			}
			editor.name, editor.input = name, ""
			if current, exists := editor.values[name]; exists {
				editor.input = workspaceSettingText(current)
			}
			editor.stage = workspaceSettingValue
		case workspaceSettingValue:
			value, err := workspaceSettingParse(editor.input)
			if err != nil {
				return "", err.Error()
			}
			if workspaceHasNull(value) {
				return "", "null is not supported; remove the setting instead"
			}
			editor.values[editor.name] = value
			editor.cursor = max(0, slices.Index(editor.names(), editor.name))
			editor.stage, editor.input = workspaceSettingList, ""
			return "", "Setting kept in the draft; nothing has been saved."
		case workspaceSettingPaste:
			message, ok := editor.importSettings(editor.input)
			if ok {
				editor.stage, editor.input = workspaceSettingList, ""
			}
			return "", message
		}
		return "", ""
	}
	return "", editor.paste(key.Text)
}

func workspaceSettingNameHelp(name, issue string) string {
	if issue == "invalid setting name" {
		return "Use the setting's exact name, for example workbench.colorTheme (letters, digits, dots, dashes, underscores or [language])."
	}
	for _, field := range workspaceFields {
		if field.path[len(field.path)-1] == name {
			return "This setting has its own guided field: " + field.label + "."
		}
	}
	return "This setting cannot be preset: it is managed by Nixorium or can start programs."
}

func (editor workspaceSettingsEditor) view(width, capacity int, dark bool) (lines []string, fixed string, actions []tuiAction) {
	switch editor.stage {
	case workspaceSettingName:
		return []string{"Setting name: " + workspaceShort(editor.input, width-16) + "_", "", "Type the exact VS Code setting name, for example workbench.colorTheme."},
			"", []tuiAction{{key: "Enter", label: "Next"}, {key: "Esc", label: "Back"}}
	case workspaceSettingValue:
		return []string{"Setting: " + workspaceShort(editor.name, width-10), "Value: " + workspaceInputTail(editor.input, width-8) + "_", "",
				"Plain text is saved as text. true, false, numbers, [lists] and {objects} keep their meaning."},
			"", []tuiAction{{key: "Enter", label: "Keep value"}, {key: "Esc", label: "Back"}}
	case workspaceSettingPaste:
		return []string{"Paste the contents of a VS Code settings file, then press Enter.", "",
				fmt.Sprintf("Received: %d characters", utf8.RuneCountInString(editor.input)),
				workspaceInputTail(editor.input, width-2)},
			"Guided fields, managed update keys and settings that can start programs are skipped.",
			[]tuiAction{{key: "Enter", label: "Take settings"}, {key: "Esc", label: "Back"}}
	}
	names := editor.names()
	if len(names) == 0 {
		lines = append(lines, "No other settings; VS Code defaults apply.")
	}
	start, end := listWindow(len(names), editor.cursor, capacity)
	for index := start; index < end; index++ {
		lines = append(lines, tuiSelection(workspaceShort(names[index]+" = "+workspaceSettingText(editor.values[names[index]]), width-2), index == editor.cursor, dark))
	}
	if len(names) > capacity {
		lines = append(lines, fmt.Sprintf("%d–%d of %d settings", start+1, end, len(names)))
	}
	return lines, "These are starting values; students can change them until the next reset.",
		[]tuiAction{{key: "a", label: "Add"}, {key: "e", label: "Edit value"}, {key: "d", label: "Remove"}, {key: "p", label: "Paste file"}, {key: "Enter", label: "Keep draft"}, {key: "Esc", label: "Cancel field"}}
}

// workspaceInputTail shows the end of a long single-line input, where typing
// happens, without terminal control characters.
func workspaceInputTail(value string, width int) string {
	runes := []rune(safeWorkspaceText(value))
	if len(runes) > max(8, width) {
		runes = append([]rune("…"), runes[len(runes)-max(8, width)+1:]...)
	}
	return string(runes)
}
