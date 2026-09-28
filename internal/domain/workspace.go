package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const WorkspaceSchemaVersion = 1
const WorkspaceMaxBytes = 64 * 1024

// WorkspaceProfile describes initial student preferences, not account policy.
// Absent fields inherit the deployment baseline; explicit empty lists clear it.
// Absence of the entire file is handled separately by callers (legacy mode).
type WorkspaceProfile struct {
	SchemaVersion int               `json:"schemaVersion"`
	Desktop       *WorkspaceDesktop `json:"desktop,omitempty"`
	VSCode        *WorkspaceVSCode  `json:"vscode,omitempty"`
	Browser       *WorkspaceBrowser `json:"browser,omitempty"`
}

type WorkspaceDesktop struct {
	Favorites        *[]string      `json:"favorites,omitempty"`
	ColorScheme      *string        `json:"colorScheme,omitempty"`
	EnableAnimations *bool          `json:"enableAnimations,omitempty"`
	Dock             *WorkspaceDock `json:"dock,omitempty"`
}

type WorkspaceDock struct {
	Position     *string `json:"position,omitempty"`
	IconSize     *int    `json:"iconSize,omitempty"`
	AutoHide     *bool   `json:"autoHide,omitempty"`
	ExtendHeight *bool   `json:"extendHeight,omitempty"`
	ShowTrash    *bool   `json:"showTrash,omitempty"`
	ShowMounts   *bool   `json:"showMounts,omitempty"`
}

type WorkspaceVSCode struct {
	Extensions *[]string                `json:"extensions,omitempty"`
	Settings   *WorkspaceVSCodeSettings `json:"settings,omitempty"`
}

type WorkspaceVSCodeSettings struct {
	FontSize     *int    `json:"editor.fontSize,omitempty"`
	TabSize      *int    `json:"editor.tabSize,omitempty"`
	InsertSpaces *bool   `json:"editor.insertSpaces,omitempty"`
	WordWrap     *string `json:"editor.wordWrap,omitempty"`
	FormatOnSave *bool   `json:"editor.formatOnSave,omitempty"`
	Minimap      *bool   `json:"editor.minimap.enabled,omitempty"`
	AutoSave     *string `json:"files.autoSave,omitempty"`
}

type WorkspaceBrowser struct {
	DefaultApplication *string `json:"defaultApplication,omitempty"`
}

var workspaceDesktopID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*\.desktop$`)
var workspaceExtensionID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9][a-z0-9-]*$`)

// Exact names also prevent encoding/json's case-insensitive field matching.
var workspaceFields = map[string][]string{
	"$":                 {"schemaVersion", "desktop", "vscode", "browser"},
	"$.desktop":         {"favorites", "colorScheme", "enableAnimations", "dock"},
	"$.desktop.dock":    {"position", "iconSize", "autoHide", "extendHeight", "showTrash", "showMounts"},
	"$.vscode":          {"extensions", "settings"},
	"$.vscode.settings": {"editor.fontSize", "editor.tabSize", "editor.insertSpaces", "editor.wordWrap", "editor.formatOnSave", "editor.minimap.enabled", "files.autoSave"},
	"$.browser":         {"defaultApplication"},
}

func DecodeWorkspaceProfile(data []byte) (WorkspaceProfile, []ValidationIssue) {
	invalid := func(field, message string) (WorkspaceProfile, []ValidationIssue) {
		return WorkspaceProfile{}, []ValidationIssue{{Field: field, Message: message}}
	}
	if len(data) > WorkspaceMaxBytes || !utf8.Valid(data) {
		return invalid("$", "must be UTF-8 JSON of at most 65536 bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := checkWorkspaceJSON(decoder, "$", 0); err != nil {
		// Do not echo unknown names or arbitrary supplied values (possibly secrets).
		return invalid("$", "invalid workspace JSON: "+err.Error())
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return invalid("$", "workspace file contains trailing data")
	}
	var profile WorkspaceProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return invalid("$", "workspace fields have invalid types")
	}
	issues := profile.validateValues()
	if len(issues) != 0 {
		return WorkspaceProfile{}, issues
	}
	if profile.VSCode != nil && profile.VSCode.Extensions != nil {
		slices.Sort(*profile.VSCode.Extensions)
	}
	return profile, nil
}

// Check tokens before decoding: duplicate keys and null must never silently
// replace a value. Only the small, fixed object graph is accepted.
func checkWorkspaceJSON(decoder *json.Decoder, path string, depth int) error {
	if depth > 8 {
		return errors.New("nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("malformed JSON")
	}
	if token == nil {
		return errors.New("null is not supported")
	}
	delim, container := token.(json.Delim)
	if !container {
		if _, object := workspaceFields[path]; object {
			return errors.New("expected an object")
		}
		return nil
	}
	switch delim {
	case '{':
		allowed, ok := workspaceFields[path]
		if !ok {
			return errors.New("unexpected object")
		}
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return errors.New("malformed object")
			}
			if seen[key] {
				return errors.New("duplicate field")
			}
			if !slices.Contains(allowed, key) {
				return errors.New("unsupported field")
			}
			seen[key] = true
			if err := checkWorkspaceJSON(decoder, path+"."+key, depth+1); err != nil {
				return err
			}
		}
	case '[':
		if path != "$.desktop.favorites" && path != "$.vscode.extensions" {
			return errors.New("unexpected array")
		}
		for decoder.More() {
			if err := checkWorkspaceJSON(decoder, path+"[]", depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected delimiter")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("malformed JSON")
	}
	return nil
}

func (p WorkspaceProfile) validateValues() []ValidationIssue {
	var issues []ValidationIssue
	add := func(field, message string) {
		issues = append(issues, ValidationIssue{Field: field, Message: message})
	}
	enum := func(field string, value *string, allowed ...string) {
		if value != nil && !slices.Contains(allowed, *value) {
			add(field, "must be one of: "+strings.Join(allowed, ", "))
		}
	}
	integer := func(field string, value *int, min, max int) {
		if value != nil && (*value < min || *value > max) {
			add(field, "integer outside supported range")
		}
	}
	identifier := func(value string, pattern *regexp.Regexp) bool {
		return len(value) <= 128 && pattern.MatchString(value)
	}
	list := func(field string, values *[]string, max int, pattern *regexp.Regexp) {
		if values == nil {
			return
		}
		if len(*values) > max {
			add(field, "too many entries")
		}
		seen := make(map[string]bool)
		for _, value := range *values {
			if !identifier(value, pattern) || seen[value] {
				add(field, "entries must be unique supported identifiers of at most 128 ASCII bytes")
				break
			}
			seen[value] = true
		}
	}
	if p.SchemaVersion != WorkspaceSchemaVersion {
		add("schemaVersion", "must be 1")
	}
	if d := p.Desktop; d != nil {
		list("desktop.favorites", d.Favorites, 32, workspaceDesktopID)
		enum("desktop.colorScheme", d.ColorScheme, "light", "dark")
		if dock := d.Dock; dock != nil {
			enum("desktop.dock.position", dock.Position, "top", "bottom", "left", "right")
			integer("desktop.dock.iconSize", dock.IconSize, 16, 128)
		}
	}
	if v := p.VSCode; v != nil {
		list("vscode.extensions", v.Extensions, 64, workspaceExtensionID)
		if s := v.Settings; s != nil {
			integer("vscode.settings.editor.fontSize", s.FontSize, 8, 40)
			integer("vscode.settings.editor.tabSize", s.TabSize, 1, 8)
			enum("vscode.settings.editor.wordWrap", s.WordWrap, "off", "on", "wordWrapColumn", "bounded")
			enum("vscode.settings.files.autoSave", s.AutoSave, "off", "onFocusChange", "onWindowChange")
		}
	}
	if p.Browser != nil && p.Browser.DefaultApplication != nil && !identifier(*p.Browser.DefaultApplication, workspaceDesktopID) {
		add("browser.defaultApplication", "must be a desktop identifier of at most 128 ASCII bytes")
	}
	return issues
}

func (p WorkspaceProfile) Validate() []ValidationIssue {
	data, err := json.Marshal(p)
	if err != nil {
		return []ValidationIssue{{Field: "$", Message: "workspace cannot be encoded"}}
	}
	_, issues := DecodeWorkspaceProfile(data)
	return issues
}

// MarshalWorkspaceProfile validates and sorts the extension set without
// mutating caller-owned slices. Favorites retain their meaningful dock order.
func MarshalWorkspaceProfile(profile WorkspaceProfile) ([]byte, error) {
	data, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	normalized, issues := DecodeWorkspaceProfile(data)
	if len(issues) != 0 {
		return nil, errors.New("workspace validation failed: " + issues[0].Field + ": " + issues[0].Message)
	}
	data, err = json.MarshalIndent(normalized, "", "  ")
	return append(data, '\n'), err
}
