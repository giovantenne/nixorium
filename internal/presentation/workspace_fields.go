package presentation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type workspaceField struct {
	group   string
	label   string
	path    []string
	kind    string
	choices []string
}

var workspaceGroups = []string{"Desktop", "Dock", "VSCode", "Browser"}

var workspaceFields = []workspaceField{
	{"Desktop", "Favorite applications (ordered)", []string{"desktop", "favorites"}, "favorites", nil},
	{"Desktop", "Appearance", []string{"desktop", "colorScheme"}, "choice", []string{"light", "dark"}},
	{"Desktop", "Animations", []string{"desktop", "enableAnimations"}, "boolean", nil},
	{"Dock", "Position", []string{"desktop", "dock", "position"}, "choice", []string{"bottom", "left", "right", "top"}},
	{"Dock", "Icon size (16–128)", []string{"desktop", "dock", "iconSize"}, "number", nil},
	{"Dock", "Auto hide", []string{"desktop", "dock", "autoHide"}, "boolean", nil},
	{"Dock", "Extend to screen edge", []string{"desktop", "dock", "extendHeight"}, "boolean", nil},
	{"Dock", "Show trash", []string{"desktop", "dock", "showTrash"}, "boolean", nil},
	{"Dock", "Show mounts", []string{"desktop", "dock", "showMounts"}, "boolean", nil},
	{"VSCode", "VS Code extensions", []string{"vscode", "extensions"}, "extensions", nil},
	{"VSCode", "Font size (8–40)", []string{"vscode", "settings", "editor.fontSize"}, "number", nil},
	{"VSCode", "Tab size (1–8)", []string{"vscode", "settings", "editor.tabSize"}, "number", nil},
	{"VSCode", "Insert spaces", []string{"vscode", "settings", "editor.insertSpaces"}, "boolean", nil},
	{"VSCode", "Word wrap", []string{"vscode", "settings", "editor.wordWrap"}, "choice", []string{"off", "on", "wordWrapColumn", "bounded"}},
	{"VSCode", "Format on save", []string{"vscode", "settings", "editor.formatOnSave"}, "boolean", nil},
	{"VSCode", "Minimap", []string{"vscode", "settings", "editor.minimap.enabled"}, "boolean", nil},
	{"VSCode", "Auto save", []string{"vscode", "settings", "files.autoSave"}, "choice", []string{"off", "onFocusChange", "onWindowChange"}},
	{"VSCode", "Other settings", []string{"vscode", "extraSettings"}, "settings", nil},
	{"Browser", "Default browser", []string{"browser", "defaultApplication"}, "browser", nil},
}

func workspaceObject(profile domain.WorkspaceProfile) map[string]any {
	data, _ := json.Marshal(profile)
	var object map[string]any
	_ = json.Unmarshal(data, &object)
	return object
}

func workspaceValue(profile domain.WorkspaceProfile, field workspaceField) any {
	var value any = workspaceObject(profile)
	for _, key := range field.path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}

// Editing works on a fresh object, never on pointers/slices retained by a
// reviewed plan. The domain decoder owns all value and schema validation.
func workspaceSetValue(profile domain.WorkspaceProfile, field workspaceField, value any) (domain.WorkspaceProfile, error) {
	object := workspaceObject(profile)
	var set func(map[string]any, []string)
	set = func(parent map[string]any, path []string) {
		key := path[0]
		if len(path) == 1 {
			if value == nil {
				delete(parent, key)
			} else {
				parent[key] = value
			}
			return
		}
		child, ok := parent[key].(map[string]any)
		if !ok {
			child = map[string]any{}
		}
		set(child, path[1:])
		if len(child) == 0 {
			delete(parent, key)
		} else {
			parent[key] = child
		}
	}
	set(object, field.path)
	data, err := json.Marshal(object)
	if err != nil {
		return profile, err
	}
	updated, issues := domain.DecodeWorkspaceProfile(data)
	if len(issues) != 0 {
		return profile, fmt.Errorf("%s", issues[0].Message)
	}
	return updated, nil
}

func workspaceValueText(value any) string {
	if value == nil {
		return "Inherit"
	}
	if settings, ok := value.(map[string]any); ok {
		return fmt.Sprintf("%d setting(s)", len(settings))
	}
	if entries, ok := value.([]any); ok {
		if len(entries) == 0 {
			return "None (explicit empty list)"
		}
		items := make([]string, 0, len(entries))
		for _, entry := range entries {
			items = append(items, fmt.Sprint(entry))
		}
		return strings.Join(items, ", ")
	}
	return fmt.Sprint(value)
}

func workspaceGroupFields(group int) []workspaceField {
	result := []workspaceField{}
	for _, field := range workspaceFields {
		if field.group == workspaceGroups[group] {
			result = append(result, field)
		}
	}
	return result
}
