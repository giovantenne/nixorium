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
	if kinds(controllerOnly) != "controller-apply" {
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
