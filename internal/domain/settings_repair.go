package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
)

// RepairLabSettings reads settings that no longer validate, for example after
// an update removed a field or added a rule. Unknown fields are dropped and
// listed; value problems stay for the operator to fix in the editor. Only a
// file that is not a JSON object cannot be repaired this way.
func RepairLabSettings(data []byte) (LabSettingsFile, []string, error) {
	var settings LabSettingsFile
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return settings, nil, errors.New("the settings file is not valid JSON; restore it from Git or a backup")
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, nil, errors.New("the settings file has values of the wrong type; restore it from Git or a backup")
	}
	removed := unknownJSONFields(raw, reflect.TypeOf(settings), "")
	if lab, found := raw["lab"]; found {
		var labFields map[string]json.RawMessage
		if json.Unmarshal(lab, &labFields) == nil {
			removed = append(removed, unknownJSONFields(labFields, reflect.TypeOf(settings.Lab), "lab.")...)
		}
	}
	sort.Strings(removed)
	return settings, removed, nil
}

func unknownJSONFields(raw map[string]json.RawMessage, structure reflect.Type, prefix string) []string {
	known := map[string]bool{}
	for index := 0; index < structure.NumField(); index++ {
		name, _, _ := strings.Cut(structure.Field(index).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			known[name] = true
		}
	}
	unknown := []string{}
	for name := range raw {
		if !known[name] {
			unknown = append(unknown, prefix+name)
		}
	}
	return unknown
}

// SettingsRepairIssues lists what must change before the settings validate:
// obsolete fields (removed on the next save) and invalid values.
func SettingsRepairIssues(settings LabSettingsFile, removed []string) []ValidationIssue {
	issues := []ValidationIssue{}
	for _, field := range removed {
		issues = append(issues, ValidationIssue{Field: field, Message: "this field is no longer used; it is removed when the settings are saved"})
	}
	return append(issues, settings.Validate()...)
}
