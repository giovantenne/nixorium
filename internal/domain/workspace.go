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
	// ExtraSettings are reviewed free-form editor defaults. Typed fields,
	// managed update keys and program-launching settings are refused.
	ExtraSettings *map[string]any `json:"extraSettings,omitempty"`
	// Marketplace pins supply extensions that are not in the package set.
	Marketplace *[]WorkspaceMarketplaceExtension `json:"marketplace,omitempty"`
}

// WorkspaceMarketplaceExtension names the exact bytes of one Marketplace
// version. The download location is derived by the builder, never stored.
type WorkspaceMarketplaceExtension struct {
	Publisher *string `json:"publisher,omitempty"`
	Name      *string `json:"name,omitempty"`
	Version   *string `json:"version,omitempty"`
	Hash      *string `json:"hash,omitempty"`
	Platform  *string `json:"platform,omitempty"`
}

// ID is the lowercase extension identifier used in vscode.extensions.
func (e WorkspaceMarketplaceExtension) ID() string {
	if e.Publisher == nil || e.Name == nil {
		return ""
	}
	return strings.ToLower(*e.Publisher + "." + *e.Name)
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

var workspaceMarketplaceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
var workspaceMarketplaceVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}$`)
var workspaceMarketplaceHash = regexp.MustCompile(`^sha256-[A-Za-z0-9+/]{43}=$`)

const WorkspaceMaxMarketplace = 32

var workspaceSettingName = regexp.MustCompile(`^[\[A-Za-z0-9][\]A-Za-z0-9_.\[-]*$`)

const workspaceMaxExtraSettings = 256
const workspaceMaxSettingInteger = 1 << 53

var workspaceDeniedSettings = []string{
	"editor.fontSize", "editor.tabSize", "editor.insertSpaces", "editor.wordWrap",
	"editor.formatOnSave", "editor.minimap.enabled", "files.autoSave",
	"update.mode", "extensions.autoUpdate", "extensions.autoCheckUpdates",
	"task.allowAutomaticTasks",
}

var workspaceDeniedSettingPrefixes = []string{
	"security.workspace.trust.",
	"terminal.integrated.profiles.",
	"terminal.integrated.automationProfile.",
	"terminal.integrated.shell.",
	"terminal.integrated.shellArgs.",
	"terminal.integrated.env.",
}

// WorkspaceExtraSettingIssue explains why a name cannot be an extra setting,
// or returns an empty string. Values are checked by the profile decoder.
func WorkspaceExtraSettingIssue(name string) string {
	if len(name) > 128 || !workspaceSettingName.MatchString(name) {
		return "invalid setting name"
	}
	if slices.Contains(workspaceDeniedSettings, name) {
		return "setting is not supported as an extra setting"
	}
	for _, prefix := range workspaceDeniedSettingPrefixes {
		if strings.HasPrefix(name, prefix) {
			return "setting is not supported as an extra setting"
		}
	}
	return ""
}

// Exact names also prevent encoding/json's case-insensitive field matching.
var workspaceFields = map[string][]string{
	"$":                      {"schemaVersion", "desktop", "vscode", "browser"},
	"$.desktop":              {"favorites", "colorScheme", "enableAnimations", "dock"},
	"$.desktop.dock":         {"position", "iconSize", "autoHide", "extendHeight", "showTrash", "showMounts"},
	"$.vscode":               {"extensions", "settings", "extraSettings", "marketplace"},
	"$.vscode.marketplace[]": {"publisher", "name", "version", "hash", "platform"},
	"$.vscode.settings":      {"editor.fontSize", "editor.tabSize", "editor.insertSpaces", "editor.wordWrap", "editor.formatOnSave", "editor.minimap.enabled", "files.autoSave"},
	"$.browser":              {"defaultApplication"},
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
	if profile.VSCode != nil && profile.VSCode.Marketplace != nil {
		slices.SortFunc(*profile.VSCode.Marketplace, func(a, b WorkspaceMarketplaceExtension) int {
			return strings.Compare(a.ID(), b.ID())
		})
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
	if path == workspaceExtraSettingsPath {
		if !container || delim != '{' {
			return errors.New("expected an object")
		}
		return checkWorkspaceFreeObject(decoder, depth, true)
	}
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
		if path != "$.desktop.favorites" && path != "$.vscode.extensions" && path != "$.vscode.marketplace" {
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

const workspaceExtraSettingsPath = "$.vscode.extraSettings"

// checkWorkspaceFreeObject consumes the members of an already opened object.
// Free-form values keep the shared rules: no null, no duplicates, same depth.
func checkWorkspaceFreeObject(decoder *json.Decoder, depth int, settings bool) error {
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
		seen[key] = true
		if settings {
			if len(seen) > workspaceMaxExtraSettings {
				return errors.New("too many extra settings")
			}
			if issue := WorkspaceExtraSettingIssue(key); issue != "" {
				return errors.New(issue)
			}
		}
		if err := checkWorkspaceFreeValue(decoder, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("malformed JSON")
	}
	return nil
}

func checkWorkspaceFreeValue(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("malformed JSON")
	}
	switch value := token.(type) {
	case nil:
		return errors.New("null is not supported")
	case json.Number:
		// Integers beyond 2^53 would change when the profile is normalized.
		if integer, err := value.Int64(); err == nil {
			if integer > workspaceMaxSettingInteger || integer < -workspaceMaxSettingInteger {
				return errors.New("integer outside supported range")
			}
		} else if !strings.ContainsAny(value.String(), ".eE") {
			return errors.New("integer outside supported range")
		}
	case json.Delim:
		switch value {
		case '{':
			return checkWorkspaceFreeObject(decoder, depth, false)
		case '[':
			for decoder.More() {
				if err := checkWorkspaceFreeValue(decoder, depth+1); err != nil {
					return err
				}
			}
			if _, err := decoder.Token(); err != nil {
				return errors.New("malformed JSON")
			}
		default:
			return errors.New("unexpected delimiter")
		}
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
		if v.Marketplace != nil {
			if len(*v.Marketplace) > WorkspaceMaxMarketplace {
				add("vscode.marketplace", "too many entries")
			}
			seen := make(map[string]bool)
			for _, entry := range *v.Marketplace {
				if issue := entry.Issue(); issue != "" {
					add("vscode.marketplace", issue)
					break
				}
				if seen[entry.ID()] {
					add("vscode.marketplace", "duplicate Marketplace extension")
					break
				}
				seen[entry.ID()] = true
			}
		}
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

// Issue describes the first invalid field of a Marketplace pin, or "".
func (e WorkspaceMarketplaceExtension) Issue() string {
	switch {
	case e.Publisher == nil || e.Name == nil || e.Version == nil || e.Hash == nil:
		return "incomplete Marketplace extension"
	case len(*e.Publisher) > 128 || !workspaceMarketplaceName.MatchString(*e.Publisher),
		len(*e.Name) > 128 || !workspaceMarketplaceName.MatchString(*e.Name),
		len(e.ID()) > 128:
		return "invalid Marketplace publisher or name"
	case len(*e.Version) > 128 || !workspaceMarketplaceVersion.MatchString(*e.Version):
		return "Marketplace version must be a stable release number"
	case !workspaceMarketplaceHash.MatchString(*e.Hash):
		return "Marketplace hash must be a sha256 SRI hash"
	case e.Platform != nil && *e.Platform != "linux-x64":
		return "Marketplace platform must be linux-x64"
	}
	return ""
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
