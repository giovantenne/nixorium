package domain

import (
	"bytes"
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
			VeyonNativeHosts: []string{"pc01"},
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
	settings.Lab.VeyonNativeHosts = []string{"pc98"}
	issues := settings.Validate()
	if len(issues) != 3 {
		t.Fatalf("got %d issues, want 3: %#v", len(issues), issues)
	}
}

func TestControllerOnlySettingsAreExplicitAndRoundTrip(t *testing.T) {
	settings := validSettings()
	settings.Lab.PCCount = 0
	settings.Lab.VeyonNativeHosts = nil
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
