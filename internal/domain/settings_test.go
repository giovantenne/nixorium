package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestControllerStaticAddressMatchesNixHostOffset(t *testing.T) {
	address, err := ControllerStaticAddress(LabSettings{NetworkBase: "10.20.0.0", NetworkPrefix: 24, MasterHostNumber: 99})
	if err != nil || address != "10.20.0.99" {
		t.Fatalf("address = %q, error = %v", address, err)
	}
	for _, lab := range []LabSettings{
		{NetworkBase: "10.20.0.1", NetworkPrefix: 24, MasterHostNumber: 99},
		{NetworkBase: "10.20.0.0", NetworkPrefix: 30, MasterHostNumber: 3},
	} {
		if _, err := ControllerStaticAddress(lab); err == nil {
			t.Fatalf("invalid static address input accepted: %+v", lab)
		}
	}
}

func validSettings() LabSettingsFile {
	return LabSettingsFile{
		SchemaVersion: SettingsSchemaVersion,
		Lab: LabSettings{
			MasterDHCPIP:     "192.0.2.10",
			NetworkBase:      "10.0.0.0",
			NetworkPrefix:    24,
			PCCount:          20,
			MasterHostNumber: 99,
			InterfaceName:    "enp0s3",
			TeacherUser:      "teacher",
			StudentUser:      "student",
			TeacherPassword:  "$6$salt$teacher",
			StudentPassword:  "$6$salt$student",
			AdminPassword:    "$6$salt$admin",
			HomepageURL:      "https://example.org",
			StudentGitName:   "Student",
			StudentGitEmail:  "student@example.org",
			AdminGitName:     "Admin",
			AdminGitEmail:    "admin@example.org",
			TimeZone:         "Europe/Rome",
			DefaultLocale:    "en_US.UTF-8",
			ExtraLocale:      "it_IT.UTF-8",
			KeyboardLayout:   "it",
			ConsoleKeyMap:    "it2",
		},
	}
}

func TestLabSettingsValidation(t *testing.T) {
	settings := validSettings()
	if issues := settings.Validate(); len(issues) != 0 {
		t.Fatalf("valid settings returned issues: %#v", issues)
	}

	settings.Lab.NetworkBase = "10.0.0.1"
	settings.Lab.StudentUser = settings.Lab.TeacherUser
	issues := settings.Validate()
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2: %#v", len(issues), issues)
	}
}

func TestLabSettingsRefuseWiFiCardNames(t *testing.T) {
	for field, change := range map[string]func(*LabSettingsFile){
		"lab.ifaceName":           func(s *LabSettingsFile) { s.Lab.InterfaceName = "wlp2s0" },
		"lab.controllerIfaceName": func(s *LabSettingsFile) { s.Lab.ControllerInterfaceName = "wlan0" },
		"lab.clientIfaceName":     func(s *LabSettingsFile) { s.Lab.ClientInterfaceName = "wlo1" },
		"lab.hostIfaceNames.pc01": func(s *LabSettingsFile) { s.Lab.HostInterfaceNames = map[string]string{"pc01": "wlx001122334455"} },
	} {
		s := validSettings()
		change(&s)
		issues := s.Validate()
		if len(issues) != 1 || issues[0].Field != field || !strings.Contains(issues[0].Message, "Wi-Fi card name") {
			t.Fatalf("%s: %+v", field, issues)
		}
	}
}

func TestDHCPMustStayOutsideTheStaticLabPrefix(t *testing.T) {
	for _, address := range []string{"10.0.0.0", "10.0.0.1", "10.0.0.255"} {
		s := validSettings()
		s.Lab.MasterDHCPIP = address
		issues := s.Validate()
		if len(issues) != 1 || issues[0].Field != "lab.network" {
			t.Fatalf("overlap %s: %+v", address, issues)
		}
	}
	for _, address := range []string{"9.255.255.255", "10.0.1.0", MasterDHCPPlaceholder} {
		s := validSettings()
		s.Lab.MasterDHCPIP = address
		if len(s.Validate()) != 0 {
			t.Fatal("non-overlap rejected")
		}
	}
}

func TestInstallationSettingsCompleteIsNotDeploymentReadiness(t *testing.T) {
	if !validSettings().InstallationSettingsComplete() {
		t.Fatal("complete form rejected")
	}
	for _, change := range []func(*LabSettingsFile){
		func(s *LabSettingsFile) { s.Lab.DeploymentMode = "controller"; s.Lab.PCCount = 0 },
		func(s *LabSettingsFile) { s.Lab.MasterDHCPIP = MasterDHCPPlaceholder },
		func(s *LabSettingsFile) { s.Lab.AdminPassword = DefaultPasswordHash },
		func(s *LabSettingsFile) { s.Lab.StudentPassword = DefaultPasswordHash },
		func(s *LabSettingsFile) { s.Lab.TeacherPassword = DefaultPasswordHash },
		func(s *LabSettingsFile) { s.Lab.NetworkBase = "invalid" },
	} {
		s := validSettings()
		change(&s)
		if s.InstallationSettingsComplete() {
			t.Fatal("incomplete settings skipped form")
		}
	}
}

func TestSharedLabSettingsValidationCases(t *testing.T) {
	type validationCase struct {
		Name      string         `json:"name"`
		Overrides map[string]any `json:"overrides"`
	}
	type validationCases struct {
		Invalid []validationCase `json:"invalid"`
	}

	data, err := os.ReadFile("../../tests/lab-settings-validation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases validationCases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Invalid) == 0 {
		t.Fatal("shared validation corpus has no invalid cases")
	}

	for _, testCase := range cases.Invalid {
		t.Run(testCase.Name, func(t *testing.T) {
			base, err := json.Marshal(validSettings())
			if err != nil {
				t.Fatal(err)
			}
			var candidate map[string]any
			if err := json.Unmarshal(base, &candidate); err != nil {
				t.Fatal(err)
			}
			lab, ok := candidate["lab"].(map[string]any)
			if !ok {
				t.Fatal("valid settings did not encode a lab object")
			}
			for field, value := range testCase.Overrides {
				lab[field] = value
			}
			candidateData, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, issues := DecodeLabSettings(candidateData); len(issues) == 0 {
				t.Fatalf("shared invalid case was accepted: %s", candidateData)
			}
		})
	}
}

func TestControllerOnlySettingsAreExplicitAndRoundTrip(t *testing.T) {
	settings := validSettings()
	settings.Lab.PCCount = 0
	if issues := settings.Validate(); len(issues) == 0 {
		t.Fatal("legacy laboratory accepted zero clients")
	}
	settings.Lab.DeploymentMode = "controller"
	settings.Lab.MasterDHCPIP = MasterDHCPPlaceholder
	data, err := MarshalLabSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	decoded, issues := DecodeLabSettings(data)
	if len(issues) != 0 || decoded.Lab.DeploymentMode != "controller" || decoded.Lab.PCCount != 0 {
		t.Fatalf("round trip = %+v, issues = %+v", decoded, issues)
	}
	settings.Lab.PCCount = 1
	if issues := settings.Validate(); len(issues) == 0 {
		t.Fatal("controller-only mode accepted a client inventory")
	}
	settings.Lab.DeploymentMode = "unknown"
	if issues := settings.Validate(); len(issues) == 0 {
		t.Fatal("unknown deployment mode accepted")
	}
}

func TestLegacySettingsDoNotAcquireDeploymentModeOnSave(t *testing.T) {
	data, err := MarshalLabSettings(validSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range [][]byte{[]byte(`"deploymentMode"`), []byte(`"controllerIfaceName"`), []byte(`"clientIfaceName"`), []byte(`"hostIfaceNames"`)} {
		if bytes.Contains(data, field) {
			t.Fatalf("legacy settings silently acquired %s", field)
		}
	}
}

func TestRoleAndHostInterfaceOverridesRoundTrip(t *testing.T) {
	settings := validSettings()
	settings.Lab.ControllerInterfaceName = "eno1"
	settings.Lab.ClientInterfaceName = "enp2s0"
	settings.Lab.HostInterfaceNames = map[string]string{"pc01": "enp3s0", "pc99": "eno2"}
	data, err := MarshalLabSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	decoded, issues := DecodeLabSettings(data)
	if len(issues) != 0 || decoded.Lab.ControllerInterfaceName != "eno1" || decoded.Lab.ClientInterfaceName != "enp2s0" || decoded.Lab.HostInterfaceNames["pc01"] != "enp3s0" {
		t.Fatalf("round trip = %+v, issues = %+v", decoded, issues)
	}
	settings.Lab.HostInterfaceNames["pc00"] = "eth0"
	settings.Lab.ClientInterfaceName = "interface-name-is-too-long"
	if issues := settings.Validate(); len(issues) != 2 {
		t.Fatalf("invalid interface overrides issues = %+v", issues)
	}
}

func TestDecodeRejectsUnknownAndTrailingValues(t *testing.T) {
	data, err := MarshalLabSettings(validSettings())
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(data, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 1, "unknown": true`), 1)
	if _, issues := DecodeLabSettings(unknown); len(issues) != 1 {
		t.Fatalf("unknown field issues = %#v", issues)
	}
	if _, issues := DecodeLabSettings(append(data, []byte("{}")...)); len(issues) != 1 {
		t.Fatalf("trailing value issues = %#v", issues)
	}
}

func TestMarshalIsDeterministicAndEndsWithNewline(t *testing.T) {
	first, err := MarshalLabSettings(validSettings())
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalLabSettings(validSettings())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || len(first) == 0 || first[len(first)-1] != '\n' {
		t.Fatalf("non-deterministic or unterminated output:\n%s", first)
	}
}

func TestClassroomViewIsOnUnlessTurnedOff(t *testing.T) {
	settings := validSettings()
	data, _ := MarshalLabSettings(settings)
	if strings.Contains(string(data), "classroomView") || !settings.Lab.ClassroomViewOn() {
		t.Fatalf("an absent setting is not on by default: %s", data)
	}
	off := false
	settings.Lab.ClassroomView = &off
	data, err := MarshalLabSettings(settings)
	if err != nil || !strings.Contains(string(data), `"classroomView": false`) || settings.Lab.ClassroomViewOn() {
		t.Fatalf("encoded = %s, %v", data, err)
	}
	decoded, issues := DecodeLabSettings(data)
	if len(issues) > 0 || decoded.Lab.ClassroomViewOn() {
		t.Fatalf("decoded = %+v, %v", decoded.Lab.ClassroomView, issues)
	}
	changes := DiffLabSettings(LabSettingsFile{}, LabSettingsFile{Lab: LabSettings{ClassroomView: &off}})
	found := false
	for _, change := range changes {
		found = found || (change.Field == "lab.classroomView" && change.Before == true && change.After == false)
	}
	if !found {
		t.Fatalf("turning the classroom view off is not reviewed: %+v", changes)
	}
}

func TestClassroomViewImpactNamesWhatStopsAndWhatStays(t *testing.T) {
	base := validSettings()
	off := false
	candidate := base
	candidate.Lab.ClassroomView = &off
	found := ""
	for _, impact := range SettingsImpacts(base, candidate, DiffLabSettings(base, candidate)) {
		if impact.Kind == "classroom-view" {
			found = impact.Detail
		}
	}
	for _, want := range []string{"locking screens", "messages", "sending files", "collecting files", "dock icon", "Internet pause", "schedule", "Apply to controller", "Update computers"} {
		if !strings.Contains(found, want) {
			t.Fatalf("impact lacks %q: %q", want, found)
		}
	}
	found = ""
	for _, impact := range SettingsImpacts(candidate, base, DiffLabSettings(candidate, base)) {
		if impact.Kind == "classroom-view" {
			found = impact.Detail
		}
	}
	if !strings.HasPrefix(found, "Classroom view on") {
		t.Fatalf("turning it back on: %q", found)
	}
}

func TestAssistantDefaultsOnAndSurvivesSettingsSave(t *testing.T) {
	settings := validSettings()
	data, err := MarshalLabSettings(settings)
	if err != nil || strings.Contains(string(data), `"assistant"`) || !settings.Lab.AssistantOn() {
		t.Fatalf("absent Assistant setting: %s, %v", data, err)
	}
	loaded, issues := DecodeLabSettings(data)
	if len(issues) != 0 || loaded.Lab.Assistant != nil || !loaded.Lab.AssistantOn() {
		t.Fatalf("decoded absent Assistant setting: %+v, %v", loaded.Lab.Assistant, issues)
	}
	loaded.Lab.HomepageURL = "https://changed.example.org"
	saved, err := MarshalLabSettings(loaded)
	if err != nil || strings.Contains(string(saved), `"assistant"`) {
		t.Fatalf("absent Assistant setting changed on save: %s, %v", saved, err)
	}
	for _, enabled := range []bool{false, true} {
		settings.Lab.Assistant = &enabled
		data, err = MarshalLabSettings(settings)
		if err != nil {
			t.Fatal(err)
		}
		loaded, issues := DecodeLabSettings(data)
		if len(issues) != 0 || loaded.Lab.Assistant == nil || loaded.Lab.AssistantOn() != enabled {
			t.Fatalf("decoded Assistant setting: %+v, %v", loaded.Lab.Assistant, issues)
		}
		loaded.Lab.HomepageURL = "https://changed.example.org"
		saved, err := MarshalLabSettings(loaded)
		if err != nil || !strings.Contains(string(saved), fmt.Sprintf(`"assistant": %t`, enabled)) {
			t.Fatalf("Assistant lost on save: %s, %v", saved, err)
		}
	}
	off := false
	changes := DiffLabSettings(LabSettingsFile{}, LabSettingsFile{Lab: LabSettings{Assistant: &off}})
	found := false
	for _, change := range changes {
		found = found || (change.Field == "lab.assistant" && change.Before == true && change.After == false)
	}
	if !found {
		t.Fatalf("disabling Assistant is not reviewed: %+v", changes)
	}
}

func TestGitIdentityAndKeyboardLayoutValidation(t *testing.T) {
	accepted := []struct{ name, field, value string }{
		{"name with accents quotes and spaces", "name", "José O'Brien \"Jo\" Núñez"},
		{"plain email", "email", "ada.lovelace+lab@example.org"},
		// Labs may already have emails of any shape Git accepts.
		{"email without at", "email", "admin"},
		{"email with name", "email", "Mario Rossi <m@x.it>"},
		{"simple layout", "layout", "us"},
		{"layout with variant", "layout", "us(intl)"},
		{"layout list", "layout", "us,it"},
		{"layout with underscore", "layout", "de_CH-legacy+1"},
	}
	rejected := []struct{ name, field, value string }{
		{"name with shell injection and newline", "name", "x\"; touch /tmp/x #\nname"},
		{"name with tab", "name", "Ada\tLovelace"},
		{"name with DEL", "name", "Ada\x7f"},
		{"name too long", "name", strings.Repeat("a", 257)},
		{"email with newline", "email", "ada@example.org\n[core]"},
		{"email too long", "email", "a@" + strings.Repeat("b", 255)},
		{"layout with quote", "layout", "us')]; touch /tmp/x #"},
		{"layout with space", "layout", "us intl"},
		{"layout with newline", "layout", "us\n"},
		{"layout too long", "layout", strings.Repeat("u", 129)},
	}
	apply := func(field, value string) []ValidationIssue {
		settings := validSettings()
		switch field {
		case "name":
			settings.Lab.StudentGitName, settings.Lab.AdminGitName = value, value
		case "email":
			settings.Lab.StudentGitEmail, settings.Lab.AdminGitEmail = value, value
		default:
			settings.Lab.KeyboardLayout = value
		}
		return settings.Validate()
	}
	for _, c := range accepted {
		t.Run("accepts "+c.name, func(t *testing.T) {
			if issues := apply(c.field, c.value); len(issues) != 0 {
				t.Fatalf("%q rejected: %v", c.value, issues)
			}
		})
	}
	for _, c := range rejected {
		t.Run("rejects "+c.name, func(t *testing.T) {
			if issues := apply(c.field, c.value); len(issues) == 0 {
				t.Fatalf("%q accepted", c.value)
			}
		})
	}
}
