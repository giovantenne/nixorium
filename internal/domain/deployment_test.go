package domain

import "testing"

func TestClientNumberAndSharedSystem(t *testing.T) {
	for name, want := range map[string]int{"pc01": 1, "pc7": 7, "pc0120": 120} {
		if number, ok := ClientNumber(name); !ok || number != want {
			t.Fatalf("ClientNumber(%q) = %d, %v", name, number, ok)
		}
	}
	for _, name := range []string{"", "pc", "pc00", "pc1a", "host01", "pc12345"} {
		if _, ok := ClientNumber(name); ok {
			t.Fatalf("ClientNumber(%q) accepted", name)
		}
	}
	if !(DeploymentTarget{Name: "pc01", Class: "shared-1"}).SharedSystem() {
		t.Fatal("a member of a shared class")
	}
	for _, target := range []DeploymentTarget{{Name: "pc03", Class: "pc03"}, {Name: "pc03"}} {
		if target.SharedSystem() {
			t.Fatalf("%+v has its own system", target)
		}
	}
}

func TestDeploymentConfigurationsCountsOneSystemPerSharedClass(t *testing.T) {
	targets := []DeploymentTarget{
		{Name: "pc01", Class: "shared-1"}, {Name: "pc02", Class: "shared-1"},
		{Name: "pc03", Class: "pc03"}, {Name: "pc04"}, {Name: "pc05", Class: "shared-2"},
	}
	if got := DeploymentConfigurations(targets); got != 4 {
		t.Fatalf("configurations = %d, want 4", got)
	}
	if ClientSystemName("pc01", "shared-1") != "nixorium-client-shared-1" || ClientSystemName("pc03", "pc03") != "pc03" || ClientSystemName("pc04", "") != "pc04" {
		t.Fatal("system names")
	}
	for _, class := range []string{"shared-", "shared-x", "pc02", "other"} {
		if ValidClientClass("pc01", class) {
			t.Fatalf("class %q accepted for pc01", class)
		}
	}
}
