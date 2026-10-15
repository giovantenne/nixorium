package domain

import (
	"bytes"
	"strings"
	"testing"
)

func transitionFrom(base string, prefix int) *NetworkTransition {
	return &NetworkTransition{PreviousBase: base, PreviousPrefixLength: prefix, StartedAt: "2026-10-15T08:00:00Z"}
}

// migratedSettings is validSettings moved from 10.0.0.0/24 to 10.1.0.0/24.
func migratedSettings() LabSettingsFile {
	settings := validSettings()
	settings.Lab.NetworkBase = "10.1.0.0"
	settings.Lab.NetworkTransition = transitionFrom("10.0.0.0", 24)
	return settings
}

func TestNetworkTransitionValidation(t *testing.T) {
	if issues := migratedSettings().Validate(); len(issues) != 0 {
		t.Fatalf("valid transition: %+v", issues)
	}
	offset := migratedSettings()
	offset.Lab.NetworkTransition.StartedAt = "2026-10-15T10:00:00.5+02:00"
	offset.Lab.NetworkTransition.PreviousBase, offset.Lab.NetworkTransition.PreviousPrefixLength = "192.168.0.0", 16
	if issues := offset.Validate(); len(issues) != 0 {
		t.Fatalf("valid transition with offset time: %+v", issues)
	}
	for _, test := range []struct {
		name   string
		change func(*LabSettings)
		field  string
		text   string
	}{
		{"same network", func(l *LabSettings) { l.NetworkTransition.PreviousBase = "10.1.0.0" }, "lab.networkTransition", "different"},
		{"same base, other prefix overlaps", func(l *LabSettings) {
			l.NetworkTransition.PreviousBase, l.NetworkTransition.PreviousPrefixLength = "10.1.0.0", 16
		}, "lab.networkTransition", "must not overlap"},
		{"contained network", func(l *LabSettings) {
			l.NetworkTransition.PreviousBase, l.NetworkTransition.PreviousPrefixLength = "10.1.0.128", 25
		}, "lab.networkTransition", "must not overlap"},
		{"host number out of range", func(l *LabSettings) { l.NetworkTransition.PreviousPrefixLength = 26 }, "lab.networkTransition.previousPrefixLength", "every configured host number"},
		{"unaligned", func(l *LabSettings) { l.NetworkTransition.PreviousBase = "10.0.0.1" }, "lab.networkTransition.previousBase", "aligned"},
		{"not an address", func(l *LabSettings) { l.NetworkTransition.PreviousBase = "10.0.0" }, "lab.networkTransition.previousBase", "IPv4"},
		{"prefix out of range", func(l *LabSettings) { l.NetworkTransition.PreviousPrefixLength = 31 }, "lab.networkTransition.previousBase", "from 1 to 30"},
		{"DHCP address on the previous network", func(l *LabSettings) { l.MasterDHCPIP = "10.0.0.42" }, "lab.networkTransition", "DHCP"},
		{"start time", func(l *LabSettings) { l.NetworkTransition.StartedAt = "yesterday" }, "lab.networkTransition.startedAt", "RFC 3339"},
		{"start time without zone", func(l *LabSettings) { l.NetworkTransition.StartedAt = "2026-10-15T08:00:00" }, "lab.networkTransition.startedAt", "RFC 3339"},
		{"impossible date", func(l *LabSettings) { l.NetworkTransition.StartedAt = "2026-02-30T08:00:00Z" }, "lab.networkTransition.startedAt", "RFC 3339"},
		{"no clients", func(l *LabSettings) { l.DeploymentMode, l.PCCount = "controller", 0 }, "lab.networkTransition", "only while client computers are configured"},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := migratedSettings()
			test.change(&settings.Lab)
			issues := settings.Validate()
			if len(issues) != 1 || issues[0].Field != test.field || !strings.Contains(issues[0].Message, test.text) {
				t.Fatalf("issues = %+v", issues)
			}
		})
	}
}

func TestNetworkTransitionRoundTripsAndIsOmittedWhenAbsent(t *testing.T) {
	data, err := MarshalLabSettings(validSettings())
	if err != nil || bytes.Contains(data, []byte("networkTransition")) {
		t.Fatalf("absent record was written: %s, %v", data, err)
	}
	data, err = MarshalLabSettings(migratedSettings())
	if err != nil || !bytes.Contains(data, []byte(`"networkTransition": {
      "previousBase": "10.0.0.0",
      "previousPrefixLength": 24,
      "startedAt": "2026-10-15T08:00:00Z"
    }`)) {
		t.Fatalf("encoded = %s, %v", data, err)
	}
	decoded, issues := DecodeLabSettings(data)
	if len(issues) != 0 || *decoded.Lab.NetworkTransition != *migratedSettings().Lab.NetworkTransition {
		t.Fatalf("decoded = %+v, issues = %+v", decoded.Lab.NetworkTransition, issues)
	}
	unknown := bytes.Replace(data, []byte(`"previousPrefixLength": 24,`), []byte(`"previousPrefixLength": 24, "previousController": "10.0.0.99",`), 1)
	if _, issues := DecodeLabSettings(unknown); len(issues) != 1 {
		t.Fatalf("unknown transition field issues = %+v", issues)
	}
}

func TestNetworkTransitionIsReviewed(t *testing.T) {
	base, candidate := validSettings(), migratedSettings()
	var found *SettingChange
	for _, change := range DiffLabSettings(base, candidate) {
		if change.Field == "lab.networkTransition" {
			found = &change
		}
	}
	if found == nil {
		t.Fatal("transition record is not part of the review")
	}
	before, after := found.Before.(*NetworkTransition), found.After.(*NetworkTransition)
	if before.String() != "none" || after.String() != "from 10.0.0.0/24 since 2026-10-15T08:00:00Z" {
		t.Fatalf("review values = %q → %q", before, after)
	}
	same := DiffLabSettings(candidate, migratedSettings())
	if len(same) != 0 {
		t.Fatalf("equal records reported as changed: %+v", same)
	}
}

func TestSettingsTransitionIssuesAcceptOnlyAGuidedNetworkChange(t *testing.T) {
	codes := func(issues []ValidationIssue) string {
		fields := []string{}
		for _, issue := range issues {
			if issue.Next == nil || issue.Next.Code != "NETWORK-LOCKED" {
				t.Fatalf("issue without NETWORK-LOCKED: %+v", issue)
			}
			fields = append(fields, issue.Field)
		}
		return strings.Join(fields, ",")
	}
	saved := validSettings()
	moved := migratedSettings()
	for _, test := range []struct {
		name            string
		base, candidate LabSettingsFile
		want            string
	}{
		{"plain base change", saved, func() LabSettingsFile { c := saved; c.Lab.NetworkBase = "10.1.0.0"; return c }(), "lab.networkBase"},
		{"start from the saved network", saved, moved, ""},
		{"start with a new prefix", saved, func() LabSettingsFile { c := moved; c.Lab.NetworkBase, c.Lab.NetworkPrefix = "10.1.0.0", 23; return c }(), ""},
		{"record names another previous network", saved, func() LabSettingsFile { c := moved; c.Lab.NetworkTransition = transitionFrom("10.2.0.0", 24); return c }(), "lab.networkBase,lab.networkTransition"},
		{"record names another previous prefix", saved, func() LabSettingsFile {
			c := moved
			c.Lab.NetworkTransition = transitionFrom("10.0.0.0", 23)
			return c
		}(), "lab.networkBase,lab.networkTransition"},
		{"record without a network change", saved, func() LabSettingsFile { c := saved; c.Lab.NetworkTransition = transitionFrom("10.2.0.0", 24); return c }(), "lab.networkTransition"},
		{"controller number stays locked", saved, func() LabSettingsFile { c := moved; c.Lab.MasterHostNumber = 98; return c }(), "lab.masterHostNumber"},
		{"finish removes the record", moved, func() LabSettingsFile { c := moved; c.Lab.NetworkTransition = nil; return c }(), ""},
		{"undo goes back to the previous network", moved, func() LabSettingsFile {
			c := saved
			c.Lab.NetworkTransition = transitionFrom("10.1.0.0", 24)
			return c
		}(), ""},
		{"no second change while one is in progress", moved, func() LabSettingsFile {
			c := moved
			c.Lab.NetworkBase = "10.2.0.0"
			c.Lab.NetworkTransition = transitionFrom("10.1.0.0", 24)
			return c
		}(), "lab.networkBase,lab.networkTransition"},
		{"rewriting the record in progress", moved, func() LabSettingsFile { c := moved; c.Lab.NetworkTransition = transitionFrom("10.3.0.0", 24); return c }(), "lab.networkTransition"},
		{"other settings during a change", moved, func() LabSettingsFile { c := moved; c.Lab.HomepageURL = "https://changed.example.org"; return c }(), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := codes(SettingsTransitionIssues(test.base, test.candidate)); got != test.want {
				t.Fatalf("blocked fields = %q, want %q", got, test.want)
			}
		})
	}
	// No clients: initial configuration is free and needs no record.
	empty := saved
	empty.Lab.DeploymentMode, empty.Lab.PCCount = "controller", 0
	candidate := empty
	candidate.Lab.NetworkBase, candidate.Lab.MasterHostNumber = "10.1.0.0", 42
	if issues := SettingsTransitionIssues(empty, candidate); issues != nil {
		t.Fatalf("controller-only change refused: %+v", issues)
	}
	if got := SettingsTransitionIssues(saved, func() LabSettingsFile { c := saved; c.Lab.MasterHostNumber = 98; return c }()); len(got) != 1 || !strings.HasPrefix(got[0].Message, "The controller number cannot be changed while 20 computers are configured:") {
		t.Fatalf("controller number refusal = %+v", got)
	}
}
