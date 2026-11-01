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

func TestValidHostNameIsExactlyWhatSettingsCanProduce(t *testing.T) {
	for _, name := range []string{"pc01", "pc09", "pc10", "pc99", "pc100", "pc199", "pc200", "pc254"} {
		if !ValidHostName(name) {
			t.Fatalf("ValidHostName(%q) refused", name)
		}
	}
	for _, name := range []string{"", "pc", "pc0", "pc1", "pc00", "pc000", "pc001", "pc0100", "pc255", "pc300", "pc999", "pc1000", "PC01", "pc01\n", " pc01", "pc 01", "pc1a", "host01", "pc-1", "pc+10"} {
		if ValidHostName(name) {
			t.Fatalf("ValidHostName(%q) accepted", name)
		}
	}
	// Every number a laboratory can use gives a name the rule accepts, and
	// nothing else does.
	accepted := 0
	for number := 0; number <= 1200; number++ {
		want := number >= 1 && number <= MaxHostNumber
		if ValidHostName(HostName(number)) != want {
			t.Fatalf("ValidHostName(%q) != %v", HostName(number), want)
		}
		if want {
			accepted++
		}
	}
	if accepted != MaxHostNumber {
		t.Fatalf("accepted %d names, want %d", accepted, MaxHostNumber)
	}
}

func TestRemoteInstallAcceptsThreeDigitHostsAndKernelInterfaceNames(t *testing.T) {
	plan := validRemoteInstallPlan()
	plan.Host.Name = "pc120"
	plan.SystemPath = "/nix/store/00000000000000000000000000000000-nixos-system-pc120-test"
	plan.Host.Interface = "enp12345678901" // 14 characters
	if err := ValidateRemoteInstallPlan(plan); err != nil {
		t.Fatalf("three-digit host refused: %v", err)
	}
	plan.Host.Interface = "enp123456789012x" // 16 characters, over the kernel limit
	if err := ValidateRemoteInstallPlan(plan); err == nil {
		t.Fatal("an interface name longer than 15 characters was accepted")
	}
	plan.Host.Interface = "enp0s2"
	plan.Host.Name = "pc255"
	plan.SystemPath = "/nix/store/00000000000000000000000000000000-nixos-system-pc255-test"
	if err := ValidateRemoteInstallPlan(plan); err == nil {
		t.Fatal("pc255 was accepted")
	}
}
