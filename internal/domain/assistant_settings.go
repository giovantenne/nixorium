package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

// AssistantSettingsFile is the only path through which Assistant may change
// laboratory settings, and then only the fields in AssistantWritableSettings.
const AssistantSettingsFile = "lab-settings.json"

// Refusal codes of the settings route. The code and the field or reason are
// shown to the model as "<code>: <detail>".
const (
	AssistantSettingsReadOnly = "settings_field_read_only"
	AssistantSettingsInvalid  = "settings_invalid"
	// AssistantSettingsBlocked is raised only when a save is reviewed: a
	// transition rule of the laboratory settings forbids the saved change.
	AssistantSettingsBlocked = "settings_blocked"
)

// assistantWritableSettings are the settings the model may propose changes to.
// Names are the field names DiffLabSettings reports. Everything else in
// lab-settings.json (network, counts, interfaces, accounts, passwords,
// credentialsVersion, assistant, schemaVersion) stays read-only.
var assistantWritableSettings = []string{
	"lab.homepageUrl", "lab.timeZone", "lab.defaultLocale", "lab.extraLocale", "lab.keyboardLayout",
	"lab.studentGitName", "lab.studentGitEmail", "lab.adminGitName", "lab.adminGitEmail",
	"lab.classroomView", "lab.pcCount", "lab.teacherUser", "lab.studentUser",
}

// AssistantWritableSettings lists the settings Assistant may change.
func AssistantWritableSettings() []string {
	return append([]string(nil), assistantWritableSettings...)
}

// AssistantSettingWritable tells whether field (for example "lab.timeZone")
// is one of the settings Assistant may change.
func AssistantSettingWritable(field string) bool {
	for _, writable := range assistantWritableSettings {
		if field == writable {
			return true
		}
	}
	return false
}

// AssistantSettingsPath tells whether path names the laboratory settings file
// at the root of the deployment. A file of the same name elsewhere stays
// read-only like any other protected path.
func AssistantSettingsPath(path string) bool { return path == AssistantSettingsFile }

// AssistantSettingsError is a refusal of the settings route.
type AssistantSettingsError struct {
	Code   string
	Detail string
}

func (err *AssistantSettingsError) Error() string { return err.Code + ": " + err.Detail }

// ReviewMessage words the refusal for the administrator, who sees it in the
// save review.
func (err *AssistantSettingsError) ReviewMessage() string {
	switch err.Code {
	case AssistantSettingsReadOnly:
		return "The proposal changes a setting the assistant may not change (" + err.Detail + "). Return to the chat and discard it."
	case AssistantSettingsBlocked:
		return err.Detail
	}
	return "The proposed settings are not valid: " + err.Detail + ". Return to the chat and ask the assistant to fix them."
}

func assistantSettingsInvalid(format string, args ...any) error {
	return &AssistantSettingsError{Code: AssistantSettingsInvalid, Detail: fmt.Sprintf(format, args...)}
}

func assistantSettingsReadOnly(fields []string) error {
	return &AssistantSettingsError{Code: AssistantSettingsReadOnly, Detail: strings.Join(fields, ", ")}
}

func settingsIssueText(issues []ValidationIssue) string {
	texts := make([]string, 0, len(issues))
	for _, issue := range issues {
		texts = append(texts, issue.Field+": "+issue.Message)
	}
	return strings.Join(texts, "; ")
}

// assistantSettingsBaseProblem is non-empty when the saved settings cannot be
// the base of a proposal: they do not validate, or still hold password hashes
// that a rewrite of the public file would drop.
func assistantSettingsBaseProblem(settings LabSettingsFile, issues []ValidationIssue) string {
	if len(issues) > 0 {
		return "the saved settings do not validate (" + settingsIssueText(issues) + "); repair them in Maintenance → Change settings first"
	}
	return assistantSettingsPasswordProblem(settings)
}

func assistantSettingsPasswordProblem(settings LabSettingsFile) string {
	if settings.Lab.AdminPassword != "" || settings.Lab.TeacherPassword != "" || settings.Lab.StudentPassword != "" {
		return "the settings file holds password hashes; update the deployment with Maintenance → Change settings first"
	}
	return ""
}

func jsonFieldNames(structure reflect.Type) map[string]bool {
	names := map[string]bool{}
	for index := 0; index < structure.NumField(); index++ {
		name, _, _ := strings.Cut(structure.Field(index).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names[name] = true
		}
	}
	return names
}

func settingsKeys(first, second map[string]any) []string {
	seen := map[string]bool{}
	for key := range first {
		seen[key] = true
	}
	for key := range second {
		seen[key] = true
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// changedSettingFields names, sorted, the settings in which two parsed copies
// of lab-settings.json differ. Names follow DiffLabSettings: "lab.<key>" for
// a lab setting, the key itself for a top-level one.
func changedSettingFields(shown, wrote map[string]any) []string {
	changed := []string{}
	for _, key := range settingsKeys(shown, wrote) {
		before, hadBefore := shown[key]
		after, hasAfter := wrote[key]
		if hadBefore == hasAfter && reflect.DeepEqual(before, after) {
			continue
		}
		beforeLab, beforeIsObject := before.(map[string]any)
		afterLab, afterIsObject := after.(map[string]any)
		if key != "lab" || !beforeIsObject || !afterIsObject {
			changed = append(changed, key)
			continue
		}
		for _, inner := range settingsKeys(beforeLab, afterLab) {
			x, hadX := beforeLab[inner]
			y, hasY := afterLab[inner]
			if hadX != hasY || !reflect.DeepEqual(x, y) {
				changed = append(changed, "lab."+inner)
			}
		}
	}
	sort.Strings(changed)
	return changed
}

// assistantSettingSpec ties a writable field to its place in LabSettings.
// set applies the value the model wrote (present is false when the key is
// missing from the model's copy) and returns what is wrong with it, if
// anything, in words that follow the field name.
type assistantSettingSpec struct {
	field string
	key   string
	set   func(lab *LabSettings, present bool, raw any) string
}

func textSetting(field, key string, target func(*LabSettings) *string, limit int) assistantSettingSpec {
	return assistantSettingSpec{field, key, func(lab *LabSettings, present bool, raw any) string {
		text := ""
		if present {
			var isText bool
			if text, isText = raw.(string); !isText {
				return "must be text"
			}
			if problem := assistantSettingValueProblem(text, limit); problem != "" {
				return problem
			}
		}
		*target(lab) = text
		return ""
	}}
}

var assistantSettingSpecs = []assistantSettingSpec{
	textSetting("lab.homepageUrl", "homepageUrl", func(lab *LabSettings) *string { return &lab.HomepageURL }, 2048),
	textSetting("lab.timeZone", "timeZone", func(lab *LabSettings) *string { return &lab.TimeZone }, 128),
	textSetting("lab.defaultLocale", "defaultLocale", func(lab *LabSettings) *string { return &lab.DefaultLocale }, 128),
	textSetting("lab.extraLocale", "extraLocale", func(lab *LabSettings) *string { return &lab.ExtraLocale }, 128),
	textSetting("lab.keyboardLayout", "keyboardLayout", func(lab *LabSettings) *string { return &lab.KeyboardLayout }, 128),
	textSetting("lab.studentGitName", "studentGitName", func(lab *LabSettings) *string { return &lab.StudentGitName }, 256),
	textSetting("lab.studentGitEmail", "studentGitEmail", func(lab *LabSettings) *string { return &lab.StudentGitEmail }, 256),
	textSetting("lab.adminGitName", "adminGitName", func(lab *LabSettings) *string { return &lab.AdminGitName }, 256),
	textSetting("lab.adminGitEmail", "adminGitEmail", func(lab *LabSettings) *string { return &lab.AdminGitEmail }, 256),
	textSetting("lab.teacherUser", "teacherUser", func(lab *LabSettings) *string { return &lab.TeacherUser }, 64),
	textSetting("lab.studentUser", "studentUser", func(lab *LabSettings) *string { return &lab.StudentUser }, 64),
	{"lab.pcCount", "pcCount", func(lab *LabSettings, present bool, raw any) string {
		number, isNumber := raw.(float64)
		if !present || !isNumber || number != math.Trunc(number) || math.Abs(number) > 1e9 {
			return "must be an integer"
		}
		lab.PCCount = int(number)
		return ""
	}},
	// Same as the menu's save: on leaves the key out (an explicit true stays),
	// off writes false. A missing key reads as on.
	{"lab.classroomView", "classroomView", func(lab *LabSettings, present bool, raw any) string {
		on := true
		if present {
			var isBool bool
			if on, isBool = raw.(bool); !isBool {
				return "must be true or false"
			}
		}
		switch {
		case on && !lab.ClassroomViewOn():
			lab.ClassroomView = nil
		case !on:
			off := false
			lab.ClassroomView = &off
		}
		return ""
	}},
}

// assistantSettingValueProblem checks a value the model wrote beyond what the
// settings validation requires: one line of text, no padding and no stand-in
// for a value the model was never shown.
func assistantSettingValueProblem(value string, limit int) string {
	switch {
	case strings.Contains(value, "[REDACTED]") || strings.Contains(value, "[WITHHELD]"):
		return "contains a marker that replaces a value you could not read; do not copy it"
	case len(value) > limit:
		return fmt.Sprintf("is longer than %d bytes", limit)
	case strings.TrimSpace(value) != value:
		return "must not start or end with spaces"
	case strings.IndexFunc(value, unicode.IsControl) >= 0:
		return "must be one line of text without control characters"
	}
	return ""
}

// MergeAssistantSettings returns the lab-settings.json the candidate holds
// after the model wrote proposed.
//
// base is the saved file. view is the text of base as the read tool showed it
// to the model, with sensitive fields withheld. The model only ever sees that
// copy, so every setting outside AssistantWritableSettings keeps the value of
// base, and a model copy that differs from the view in any such setting is
// refused by name. Writable settings take the model's value where it differs
// from the view; where it does not, base keeps its real value, so a value the
// view masked is never written back. The result is the file Nixorium itself
// writes (MarshalPublicLabSettings), or base unchanged when the model changed
// nothing.
func MergeAssistantSettings(base, view, proposed []byte) ([]byte, error) {
	baseSettings, issues := DecodeLabSettings(base)
	if problem := assistantSettingsBaseProblem(baseSettings, issues); problem != "" {
		return nil, assistantSettingsInvalid("%s", problem)
	}
	var shown, wrote map[string]any
	if err := json.Unmarshal(view, &shown); err != nil || shown == nil {
		return nil, assistantSettingsInvalid("the saved settings cannot be shown; repair them in Maintenance → Change settings first")
	}
	if err := json.Unmarshal(proposed, &wrote); err != nil || wrote == nil {
		if err == nil {
			err = fmt.Errorf("the content is not a JSON object")
		}
		return nil, assistantSettingsInvalid("not a valid settings file: %v", err)
	}
	topLevel, labFields := jsonFieldNames(reflect.TypeFor[LabSettingsFile]()), jsonFieldNames(reflect.TypeFor[LabSettings]())
	readOnly, unknown := []string{}, []string{}
	for _, field := range changedSettingFields(shown, wrote) {
		inner, isLab := strings.CutPrefix(field, "lab.")
		known := topLevel[field] || (isLab && labFields[inner])
		switch {
		case known && !AssistantSettingWritable(field):
			readOnly = append(readOnly, field)
		case !known:
			unknown = append(unknown, field)
		}
	}
	if len(readOnly) > 0 {
		return nil, assistantSettingsReadOnly(readOnly)
	}
	if len(unknown) > 0 {
		return nil, assistantSettingsInvalid("unknown field %s", strings.Join(unknown, ", "))
	}
	shownLab, _ := shown["lab"].(map[string]any)
	wroteLab, _ := wrote["lab"].(map[string]any)
	merged := baseSettings
	changed := false
	for _, spec := range assistantSettingSpecs {
		newValue, hasNew := wroteLab[spec.key]
		oldValue, hadOld := shownLab[spec.key]
		if hasNew == hadOld && reflect.DeepEqual(newValue, oldValue) {
			continue
		}
		if problem := spec.set(&merged.Lab, hasNew, newValue); problem != "" {
			return nil, assistantSettingsInvalid("%s %s", spec.field, problem)
		}
		changed = true
	}
	// A write that changes nothing (classroomView true where the key is
	// absent, say) leaves the saved file as it is, whatever its formatting.
	if !changed || reflect.DeepEqual(merged, baseSettings) {
		return base, nil
	}
	if issues := merged.Validate(); len(issues) > 0 {
		return nil, assistantSettingsInvalid("%s", settingsIssueText(issues))
	}
	data, err := MarshalPublicLabSettings(merged)
	if err != nil {
		return nil, assistantSettingsInvalid("%v", err)
	}
	// The diff of the real files, not of the model's copy, is the last word.
	result, issues := DecodeLabSettings(data)
	if len(issues) > 0 {
		return nil, assistantSettingsInvalid("%s", settingsIssueText(issues))
	}
	if fields := assistantReadOnlyChanges(DiffLabSettings(baseSettings, result)); len(fields) > 0 {
		return nil, assistantSettingsReadOnly(fields)
	}
	return data, nil
}

func assistantReadOnlyChanges(changes []SettingChange) []string {
	fields := []string{}
	for _, change := range changes {
		if !AssistantSettingWritable(change.Field) {
			fields = append(fields, change.Field)
		}
	}
	return fields
}

// AssistantSettingsReview is what a proposed change of lab-settings.json
// means once saved, computed from the saved and the proposed files alone.
type AssistantSettingsReview struct {
	Changes []SettingChange
	Impacts []SettingImpact
}

// ReviewAssistantSettings checks a proposed lab-settings.json against the
// saved one when a save is reviewed, whatever wrote it: both must validate,
// the proposal must be in the exact form Nixorium writes, the transition rules
// of a native settings save must allow it and only writable settings may
// differ. It returns the changed settings and the impacts a native settings
// review shows for them.
func ReviewAssistantSettings(base, candidate []byte) (AssistantSettingsReview, error) {
	var review AssistantSettingsReview
	saved, issues := DecodeLabSettings(base)
	if problem := assistantSettingsBaseProblem(saved, issues); problem != "" {
		return review, assistantSettingsInvalid("%s", problem)
	}
	proposed, issues := DecodeLabSettings(candidate)
	if len(issues) > 0 {
		return review, assistantSettingsInvalid("%s", settingsIssueText(issues))
	}
	if problem := assistantSettingsPasswordProblem(proposed); problem != "" {
		return review, assistantSettingsInvalid("%s", problem)
	}
	canonical, err := MarshalPublicLabSettings(proposed)
	if err != nil || !bytes.Equal(canonical, candidate) {
		return review, assistantSettingsInvalid("the settings file is not in the form Nixorium writes")
	}
	if transition := SettingsTransitionIssues(saved, proposed); len(transition) > 0 {
		messages := make([]string, 0, len(transition))
		for _, issue := range transition {
			messages = append(messages, issue.Message)
		}
		return review, &AssistantSettingsError{Code: AssistantSettingsBlocked, Detail: strings.Join(messages, " ")}
	}
	review.Changes = DiffLabSettings(saved, proposed)
	if fields := assistantReadOnlyChanges(review.Changes); len(fields) > 0 {
		return review, assistantSettingsReadOnly(fields)
	}
	review.Impacts = SettingsImpacts(saved, proposed, review.Changes)
	return review, nil
}

// SettingsReviewLines words the impacts of a proposed settings change for the
// save review, one "Checked by Nixorium" line each, as a native settings
// review words them under "What happens next".
func (plan AssistantSavePlan) SettingsReviewLines() []string {
	lines := make([]string, 0, len(plan.SettingsImpacts))
	for _, impact := range plan.SettingsImpacts {
		prefix := "Checked by Nixorium: "
		if impact.Warning {
			prefix += "Attention: "
		}
		lines = append(lines, prefix+impact.Detail)
	}
	return lines
}
