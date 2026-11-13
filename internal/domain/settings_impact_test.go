package domain

import (
	"strings"
	"testing"
)

func TestSettingsImpacts(t *testing.T) {
	base := LabSettingsFile{Lab: LabSettings{PCCount: 20, NetworkBase: "10.0.0.0", NetworkPrefix: 24}}
	kinds := func(impacts []SettingImpact) string {
		names := []string{}
		for _, impact := range impacts {
			name := impact.Kind
			if impact.Warning {
				name += "!"
			}
			names = append(names, name)
		}
		return strings.Join(names, ",")
	}
	if impacts := SettingsImpacts(base, base, nil); impacts != nil {
		t.Fatalf("no changes = %+v", impacts)
	}
	controllerOnly := SettingsImpacts(base, base, []SettingChange{{Field: "lab.masterDhcpIp"}})
	if kinds(controllerOnly) != "controller-apply,pxe-address" {
		t.Fatalf("controller-only = %s", kinds(controllerOnly))
	}
	smaller := base
	smaller.Lab.PCCount, smaller.Lab.NetworkBase = 18, "10.1.0.0"
	impacts := SettingsImpacts(base, smaller, []SettingChange{{Field: "lab.pcCount"}, {Field: "lab.networkBase"}, {Field: "lab.studentPassword"}})
	if got := kinds(impacts); got != "controller-apply,client-update,client-addresses!,computers-removed!,passwords,pxe-prepare" {
		t.Fatalf("impacts = %s", got)
	}
	if !strings.Contains(impacts[3].Detail, "pc19–pc20") {
		t.Fatalf("removed range = %q", impacts[3].Detail)
	}
	larger := base
	larger.Lab.PCCount = 21
	if got := kinds(SettingsImpacts(base, larger, []SettingChange{{Field: "lab.pcCount"}})); !strings.Contains(got, "computers-added") {
		t.Fatalf("added = %s", got)
	}
}

func TestSettingsInterfaceChangesExplainConnectionRisk(t *testing.T) {
	base := LabSettingsFile{Lab: LabSettings{PCCount: 20, InterfaceName: "enp0s3"}}
	for _, field := range []string{"lab.controllerIfaceName", "lab.clientIfaceName", "lab.ifaceName", "lab.hostIfaceNames"} {
		t.Run(field, func(t *testing.T) {
			impacts := SettingsImpacts(base, base, []SettingChange{{Field: field}})
			for _, impact := range impacts {
				if impact.Kind == "network-interface" && impact.Warning && strings.Contains(impact.Detail, "local console") {
					return
				}
			}
			t.Fatalf("missing interface warning: %+v", impacts)
		})
	}
}

func TestStudentSnapshotsImpact(t *testing.T) {
	base := validSettings()
	for _, test := range []struct {
		count   int
		warning bool
		detail  string
	}{
		{0, true, "keeps none from then on"},
		{3, true, "deletes the older saved sessions"},
		{12, false, "keeps up to its last 12 sessions"},
	} {
		candidate := base
		count := test.count
		candidate.Lab.StudentSnapshots = &count
		impacts := SettingsImpacts(base, candidate, DiffLabSettings(base, candidate))
		found := false
		for _, impact := range impacts {
			if impact.Kind == "student-snapshots" {
				found = true
				if impact.Warning != test.warning || !strings.Contains(impact.Detail, test.detail) || !strings.Contains(impact.Detail, "Update computers") {
					t.Fatalf("%d: unexpected impact %+v", test.count, impact)
				}
			}
		}
		if !found {
			t.Fatalf("%d: no student snapshots impact in %+v", test.count, impacts)
		}
	}
	unchanged := base
	five := DefaultStudentSnapshots
	unchanged.Lab.StudentSnapshots = &five
	if changes := DiffLabSettings(base, unchanged); len(changes) != 0 {
		t.Fatalf("an explicit default is reported as a change: %+v", changes)
	}
}
