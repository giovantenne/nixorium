package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// assistantPublicSettings is a laboratory as its public lab-settings.json
// holds it: no password hashes, every optional field present.
func assistantPublicSettings() LabSettingsFile {
	on, off := true, false
	settings := validSettings()
	settings.Lab.TeacherPassword, settings.Lab.StudentPassword, settings.Lab.AdminPassword = "", "", ""
	settings.Lab.CredentialsVersion = 3
	settings.Lab.DeploymentMode = "laboratory"
	settings.Lab.ClassroomView, settings.Lab.Assistant = &on, &off
	settings.Lab.ControllerInterfaceName, settings.Lab.ClientInterfaceName = "enp1s0", "enp2s0"
	settings.Lab.HostInterfaceNames = map[string]string{"pc01": "enp3s0"}
	return settings
}

func assistantSettingsBytes(t *testing.T, settings LabSettingsFile) []byte {
	t.Helper()
	data, err := MarshalPublicLabSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// assistantSettingsView mimics the read tool: the sensitive credentialsVersion
// is withheld, and nothing else changes in this fixture.
func assistantSettingsViewOf(t *testing.T, data []byte) []byte {
	t.Helper()
	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	tree["lab"].(map[string]any)["credentialsVersion"] = "[WITHHELD]"
	view, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func assistantSettingsEdit(t *testing.T, view []byte, edit func(lab map[string]any)) []byte {
	t.Helper()
	var tree map[string]any
	if err := json.Unmarshal(view, &tree); err != nil {
		t.Fatal(err)
	}
	edit(tree["lab"].(map[string]any))
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assistantSettingsCode(err error) string {
	var refusal *AssistantSettingsError
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

func TestAssistantWritableSettingsMatchTheSettingsFields(t *testing.T) {
	if len(assistantSettingSpecs) != len(assistantWritableSettings) {
		t.Fatal("every writable setting needs exactly one spec")
	}
	base := assistantPublicSettings()
	for _, spec := range assistantSettingSpecs {
		if !AssistantSettingWritable(spec.field) {
			t.Fatalf("%s is not writable", spec.field)
		}
		changed := base
		*spec.target(&changed.Lab) = "changed"
		diff := DiffLabSettings(base, changed)
		if len(diff) != 1 || diff[0].Field != spec.field {
			t.Fatalf("%s: DiffLabSettings reports %+v", spec.field, diff)
		}
		if !labFieldsContain(spec.key) {
			t.Fatalf("%s is not a settings key", spec.key)
		}
	}
	for _, field := range []string{"lab.pcCount", "lab.masterHostNumber", "lab.networkBase", "lab.ifaceName", "lab.teacherUser", "lab.studentUser", "lab.teacherPassword", "lab.adminPassword", "lab.credentialsVersion", "lab.classroomView", "lab.assistant", "lab.masterDhcpIp", "lab.deploymentMode", "lab.networkTransition", "lab.consoleKeyMap", "schemaVersion", "lab", ""} {
		if AssistantSettingWritable(field) {
			t.Fatalf("%q must stay read-only", field)
		}
	}
	list := AssistantWritableSettings()
	list[0] = "tampered"
	if AssistantWritableSettings()[0] == "tampered" {
		t.Fatal("the list is shared")
	}
	if !AssistantSettingsPath("lab-settings.json") || AssistantSettingsPath("modules/lab-settings.json") || AssistantSettingsPath("lab-settings.json/x") {
		t.Fatal("only the root settings file takes the settings route")
	}
}

func labFieldsContain(key string) bool { return jsonFieldNames(reflect.TypeFor[LabSettings]())[key] }

func TestMergeAssistantSettingsAcceptsWritableChanges(t *testing.T) {
	base := assistantSettingsBytes(t, assistantPublicSettings())
	view := assistantSettingsViewOf(t, base)
	proposed := assistantSettingsEdit(t, view, func(lab map[string]any) {
		lab["timeZone"] = "Europe/London"
		lab["adminGitEmail"] = "technician@school.example"
	})
	merged, err := MergeAssistantSettings(base, view, proposed)
	if err != nil {
		t.Fatal(err)
	}
	settings, issues := DecodeLabSettings(merged)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	want := assistantPublicSettings()
	want.Lab.TimeZone, want.Lab.AdminGitEmail = "Europe/London", "technician@school.example"
	if !reflect.DeepEqual(settings, want) {
		t.Fatalf("merged %+v", settings.Lab)
	}
	if !bytes.Equal(merged, assistantSettingsBytes(t, want)) {
		t.Fatal("the merged file is not the form Nixorium writes")
	}
	if !strings.Contains(string(merged), `"credentialsVersion": 3`) || strings.Contains(string(merged), "WITHHELD") {
		t.Fatal("a withheld value reached the file")
	}
}

func TestMergeAssistantSettingsReturnsTheBaseWhenNothingChanged(t *testing.T) {
	// A saved file that is not in canonical form is left alone when the model
	// changes no writable setting, so the proposal shows no change.
	base := []byte(strings.ReplaceAll(string(assistantSettingsBytes(t, assistantPublicSettings())), "  ", "\t"))
	view := assistantSettingsViewOf(t, base)
	merged, err := MergeAssistantSettings(base, view, view)
	if err != nil || !bytes.Equal(merged, base) {
		t.Fatalf("merged=%q err=%v", merged, err)
	}
}

func TestMergeAssistantSettingsRefusesEveryReadOnlyFieldByName(t *testing.T) {
	base := assistantSettingsBytes(t, assistantPublicSettings())
	view := assistantSettingsViewOf(t, base)
	fields := map[string]bool{}
	for name := range jsonFieldNames(reflect.TypeFor[LabSettings]()) {
		fields["lab."+name] = true
	}
	for _, spec := range assistantSettingSpecs {
		delete(fields, spec.field)
	}
	if len(fields) < 18 {
		t.Fatalf("only %d read-only fields found", len(fields))
	}
	for field := range fields {
		key := strings.TrimPrefix(field, "lab.")
		edits := map[string]func(map[string]any){"changed": func(lab map[string]any) { lab[key] = "changed by the model" }}
		if strings.Contains(string(view), `"`+key+`"`) {
			edits["removed"] = func(lab map[string]any) { delete(lab, key) }
		}
		for name, edit := range edits {
			_, err := MergeAssistantSettings(base, view, assistantSettingsEdit(t, view, edit))
			if assistantSettingsCode(err) != AssistantSettingsReadOnly || err.Error() != "settings_field_read_only: "+field {
				t.Fatalf("%s %s: %v", name, field, err)
			}
		}
	}
	_, err := MergeAssistantSettings(base, bytes.Replace(view, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 1`), 1), bytes.Replace(view, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 2`), 1))
	if err == nil || err.Error() != "settings_field_read_only: schemaVersion" {
		t.Fatalf("schemaVersion: %v", err)
	}
	_, err = MergeAssistantSettings(base, view, []byte(`{"schemaVersion":1,"lab":"gone"}`))
	if err == nil || err.Error() != "settings_field_read_only: lab" {
		t.Fatalf("lab replaced: %v", err)
	}
}

func TestMergeAssistantSettingsRefusesReadOnlyChangeEvenWithWritableChange(t *testing.T) {
	base := assistantSettingsBytes(t, assistantPublicSettings())
	view := assistantSettingsViewOf(t, base)
	proposed := assistantSettingsEdit(t, view, func(lab map[string]any) {
		lab["timeZone"] = "Europe/London"
		lab["pcCount"] = 25
		lab["ifaceName"] = "eth0"
	})
	_, err := MergeAssistantSettings(base, view, proposed)
	if err == nil || err.Error() != "settings_field_read_only: lab.ifaceName, lab.pcCount" {
		t.Fatalf("err=%v", err)
	}
}

func TestMergeAssistantSettingsKeepsValuesTheViewMasked(t *testing.T) {
	// The read tool masks sk-… as a secret: a faithful copy of the view must
	// not write the stand-in over the real value.
	settings := assistantPublicSettings()
	settings.Lab.HomepageURL = "https://school.example/task-list"
	settings.Lab.TeacherUser = "desk-teacher"
	base := assistantSettingsBytes(t, settings)
	view := bytes.ReplaceAll(assistantSettingsViewOf(t, base), []byte("task-list"), []byte("ta[REDACTED]"))
	view = bytes.ReplaceAll(view, []byte("desk-teacher"), []byte("de[REDACTED]"))
	proposed := assistantSettingsEdit(t, view, func(lab map[string]any) { lab["keyboardLayout"] = "us" })
	merged, err := MergeAssistantSettings(base, view, proposed)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := DecodeLabSettings(merged)
	if got.Lab.HomepageURL != "https://school.example/task-list" || got.Lab.TeacherUser != "desk-teacher" || got.Lab.KeyboardLayout != "us" {
		t.Fatalf("masked values written back: %+v", got.Lab)
	}
	// A new value that carries a stand-in is never taken.
	for _, value := range []string{"https://other.example/ta[REDACTED]", "[WITHHELD]"} {
		proposed = assistantSettingsEdit(t, view, func(lab map[string]any) { lab["homepageUrl"] = value })
		if _, err := MergeAssistantSettings(base, view, proposed); assistantSettingsCode(err) != AssistantSettingsInvalid || !strings.Contains(err.Error(), "lab.homepageUrl contains a marker") {
			t.Fatalf("%s: %v", value, err)
		}
	}
}

func TestMergeAssistantSettingsRefusesInvalidContent(t *testing.T) {
	base := assistantSettingsBytes(t, assistantPublicSettings())
	view := assistantSettingsViewOf(t, base)
	for name, test := range map[string]struct {
		edit func(map[string]any)
		want string
	}{
		"empty zone":      {func(lab map[string]any) { lab["timeZone"] = "" }, "settings_invalid: lab.timeZone: must not be empty"},
		"blank locale":    {func(lab map[string]any) { lab["defaultLocale"] = "   " }, "settings_invalid: lab.defaultLocale must not start or end with spaces"},
		"removed":         {func(lab map[string]any) { delete(lab, "keyboardLayout") }, "settings_invalid: lab.keyboardLayout: must not be empty"},
		"bad homepage":    {func(lab map[string]any) { lab["homepageUrl"] = "ftp://files.example" }, "settings_invalid: lab.homepageUrl: must be an absolute http:// or https:// URL"},
		"number":          {func(lab map[string]any) { lab["timeZone"] = 5 }, "settings_invalid: lab.timeZone must be text"},
		"null":            {func(lab map[string]any) { lab["extraLocale"] = nil }, "settings_invalid: lab.extraLocale must be text"},
		"two lines":       {func(lab map[string]any) { lab["studentGitName"] = "Student\nName" }, "settings_invalid: lab.studentGitName must be one line of text without control characters"},
		"padded":          {func(lab map[string]any) { lab["adminGitName"] = "Admin " }, "settings_invalid: lab.adminGitName must not start or end with spaces"},
		"long":            {func(lab map[string]any) { lab["studentGitEmail"] = strings.Repeat("a", 257) }, "settings_invalid: lab.studentGitEmail is longer than 256 bytes"},
		"unknown field":   {func(lab map[string]any) { lab["timezone"] = "Europe/Rome" }, "settings_invalid: unknown field lab.timezone"},
		"unknown dropped": {func(lab map[string]any) { lab["pcCnt"] = 3; lab["timeZone"] = "Europe/Paris" }, "settings_invalid: unknown field lab.pcCnt"},
	} {
		_, err := MergeAssistantSettings(base, view, assistantSettingsEdit(t, view, test.edit))
		if err == nil || err.Error() != test.want {
			t.Fatalf("%s: got %v, want %s", name, err, test.want)
		}
	}
	for name, content := range map[string]string{"empty": "", "text": "timeZone: UTC", "array": "[]", "null": "null", "trailing": string(view) + "{}", "truncated": string(view[:len(view)/2])} {
		if _, err := MergeAssistantSettings(base, view, []byte(content)); assistantSettingsCode(err) != AssistantSettingsInvalid || !strings.HasPrefix(err.Error(), "settings_invalid: not a valid settings file") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestMergeAssistantSettingsNeedsValidSavedSettings(t *testing.T) {
	settings := assistantPublicSettings()
	view := assistantSettingsViewOf(t, assistantSettingsBytes(t, settings))
	for name, base := range map[string][]byte{
		"invalid":   []byte(strings.Replace(string(assistantSettingsBytes(t, settings)), `"pcCount": 20`, `"pcCount": 0`, 1)),
		"unknown":   []byte(strings.Replace(string(assistantSettingsBytes(t, settings)), `"lab": {`, `"lab": { "obsolete": 1,`, 1)),
		"passwords": []byte(strings.Replace(string(assistantSettingsBytes(t, settings)), `"timeZone"`, `"adminPassword": "$6$salt$hash", "timeZone"`, 1)),
		"not json":  []byte("{"),
	} {
		_, err := MergeAssistantSettings(base, view, view)
		if assistantSettingsCode(err) != AssistantSettingsInvalid {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestReviewAssistantSettingsListsImpactsLikeANativeSave(t *testing.T) {
	saved := assistantPublicSettings()
	base := assistantSettingsBytes(t, saved)
	for name, test := range map[string]struct {
		edit func(*LabSettings)
		want []string
	}{
		"time zone reaches the clients":  {func(lab *LabSettings) { lab.TimeZone = "Europe/London" }, []string{"controller-apply", "client-update", "pxe-prepare"}},
		"git identity of the controller": {func(lab *LabSettings) { lab.AdminGitName = "Technician" }, []string{"controller-apply"}},
	} {
		changed := saved
		test.edit(&changed.Lab)
		review, err := ReviewAssistantSettings(base, assistantSettingsBytes(t, changed))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		kinds := []string{}
		for _, impact := range review.Impacts {
			kinds = append(kinds, impact.Kind)
		}
		if !reflect.DeepEqual(kinds, test.want) || len(review.Changes) != 1 {
			t.Fatalf("%s: impacts %v changes %+v", name, kinds, review.Changes)
		}
	}
	changed := saved
	changed.Lab.TimeZone = "Europe/London"
	review, _ := ReviewAssistantSettings(base, assistantSettingsBytes(t, changed))
	lines := AssistantSavePlan{SettingsImpacts: review.Impacts}.SettingsReviewLines()
	want := []string{
		"Checked by Nixorium: Apply to controller so this controller uses the saved settings.",
		"Checked by Nixorium: Update computers so the clients use the saved settings; offline computers keep the old ones until updated.",
		"Checked by Nixorium: Prepare network installation again before installing more computers.",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines %q", lines)
	}
	if len(AssistantSavePlan{}.SettingsReviewLines()) != 0 {
		t.Fatal("an unchanged settings file has no lines")
	}
	attention := AssistantSavePlan{SettingsImpacts: []SettingImpact{{Detail: "Careful.", Warning: true}}}.SettingsReviewLines()
	if len(attention) != 1 || attention[0] != "Checked by Nixorium: Attention: Careful." {
		t.Fatalf("warning %q", attention)
	}
}

func TestReviewAssistantSettingsRefusesWhatANativeSaveRefuses(t *testing.T) {
	saved := assistantPublicSettings()
	base := assistantSettingsBytes(t, saved)
	moved := saved
	moved.Lab.NetworkBase = "10.1.0.0"
	_, err := ReviewAssistantSettings(base, assistantSettingsBytes(t, moved))
	if assistantSettingsCode(err) != AssistantSettingsBlocked || err.Error() != "settings_blocked: "+NetworkLockedMessage(20) || !strings.Contains(err.(*AssistantSettingsError).ReviewMessage(), "Change network addressing") {
		t.Fatalf("network: %v", err)
	}
	renumbered := saved
	renumbered.Lab.MasterHostNumber = 100
	if _, err = ReviewAssistantSettings(base, assistantSettingsBytes(t, renumbered)); assistantSettingsCode(err) != AssistantSettingsBlocked || !strings.Contains(err.Error(), ControllerNumberLockedMessage(20)) {
		t.Fatalf("controller number: %v", err)
	}
	counted := saved
	counted.Lab.PCCount = 19
	_, err = ReviewAssistantSettings(base, assistantSettingsBytes(t, counted))
	if assistantSettingsCode(err) != AssistantSettingsReadOnly || err.Error() != "settings_field_read_only: lab.pcCount" || !strings.Contains(err.(*AssistantSettingsError).ReviewMessage(), "(lab.pcCount)") {
		t.Fatalf("count: %v", err)
	}
	for name, candidate := range map[string][]byte{
		"formatting": bytes.ReplaceAll(base, []byte("  "), []byte("\t")),
		"invalid":    []byte(strings.Replace(string(base), `"pcCount": 20`, `"pcCount": 0`, 1)),
		"passwords":  []byte(strings.Replace(string(base), `"timeZone"`, `"adminPassword": "$6$salt$hash", "timeZone"`, 1)),
		"duplicate":  []byte(strings.Replace(string(base), `"timeZone": "Europe/Rome"`, `"timeZone": "Europe/Rome", "timeZone": "UTC"`, 1)),
	} {
		if _, err := ReviewAssistantSettings(base, candidate); assistantSettingsCode(err) != AssistantSettingsInvalid {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := ReviewAssistantSettings([]byte("{}"), base); assistantSettingsCode(err) != AssistantSettingsInvalid {
		t.Fatalf("invalid base: %v", err)
	}
}
