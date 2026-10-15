package domain

import (
	"strings"
	"testing"
)

func preflightSettings() LabSettingsFile {
	settings := validSettings()
	settings.Lab.PCCount = 3
	return settings
}

func clientNames(hosts []HostMeta) string {
	names := []string{}
	for _, host := range hosts {
		names = append(names, host.Name+"="+host.Interface+"@"+host.IP)
	}
	return strings.Join(names, ",")
}

func TestNetworkPreflightNeedsFollowsInterfaceResolution(t *testing.T) {
	base := preflightSettings()
	for name, test := range map[string]struct {
		change     func(*LabSettings)
		controller bool
		clients    string
	}{
		"controller name":                      {change: func(lab *LabSettings) { lab.ControllerInterfaceName = "enp5s0" }, controller: true},
		"fallback reaches only the controller": {change: func(lab *LabSettings) { lab.InterfaceName = "enp9s0" }, controller: true},
		"controller override": {change: func(lab *LabSettings) {
			lab.HostInterfaceNames = map[string]string{"pc99": "enp7s0"}
		}, controller: true},
		"client name": {change: func(lab *LabSettings) { lab.ClientInterfaceName = "enp3s0" },
			clients: "pc01=enp3s0@10.0.0.1,pc02=enp3s0@10.0.0.2,pc03=enp3s0@10.0.0.3"},
		"one override": {change: func(lab *LabSettings) { lab.HostInterfaceNames = map[string]string{"pc02": "enp4s0"} },
			clients: "pc02=enp4s0@10.0.0.2"},
		"unrelated": {change: func(lab *LabSettings) { lab.HomepageURL = "https://example.net" }},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			test.change(&candidate.Lab)
			controller, clients := NetworkPreflightNeeds(base, candidate)
			if controller != test.controller || clientNames(clients) != test.clients {
				t.Fatalf("needs = %v %q", controller, clientNames(clients))
			}
		})
	}

	// A fallback change does not move a client with its own name, and a
	// client returning to the wired card by type needs no name check.
	named := base
	named.Lab.ClientInterfaceName = "enp3s0"
	named.Lab.ControllerInterfaceName = "enp0s3"
	candidate := named
	candidate.Lab.InterfaceName = "eth0"
	if controller, clients := NetworkPreflightNeeds(named, candidate); controller || len(clients) != 0 {
		t.Fatalf("fallback change = %v %+v", controller, clients)
	}
	candidate = named
	candidate.Lab.ClientInterfaceName = ""
	if _, clients := NetworkPreflightNeeds(named, candidate); len(clients) != 0 {
		t.Fatalf("by-type clients were checked: %+v", clients)
	}
	// New computers are not installed yet; controller-only mode has no lab.
	candidate = base
	candidate.Lab.PCCount = 5
	candidate.Lab.ClientInterfaceName = "enp3s0"
	if _, clients := NetworkPreflightNeeds(base, candidate); len(clients) != 3 {
		t.Fatalf("new computers were probed: %+v", clients)
	}
	candidate = base
	candidate.Lab.DeploymentMode, candidate.Lab.PCCount = "controller", 0
	candidate.Lab.ControllerInterfaceName = "enp5s0"
	if controller, clients := NetworkPreflightNeeds(base, candidate); controller || clients != nil {
		t.Fatalf("controller mode = %v %+v", controller, clients)
	}
}

func controllerDevices() NetworkObservation {
	return NetworkObservation{Reachable: true, Lifetimes: true, Devices: []NetworkDevice{
		{Name: "enp0s3", Kind: NetworkDeviceWired, Addresses: []NetworkAddress{{Address: "192.168.0.198", Dynamic: true}, {Address: "10.0.0.99"}}},
		{Name: "enp0s8", Kind: NetworkDeviceWired},
		{Name: "wlp2s0", Kind: NetworkDeviceWireless},
	}}
}

func TestCheckControllerInterfaceClassifiesDevices(t *testing.T) {
	missing := CheckControllerInterface("enp0s3", "enp5s0", controllerDevices(), "192.168.0.198")
	want := "The controller has no network device called enp5s0. Its devices are: enp0s3 (192.168.0.198, 10.0.0.99, carries the DHCP address), enp0s8, wlp2s0 (wireless)."
	if missing.State != DeviceMissing || !missing.Blocking || missing.Message != want || missing.DHCPDevice != "enp0s3" {
		t.Fatalf("missing = %+v\nwant %q", missing, want)
	}
	wireless := CheckControllerInterface("enp0s3", "wlp2s0", controllerDevices(), "192.168.0.198")
	if wireless.State != DeviceNotWired || !wireless.Blocking || !strings.Contains(wireless.Message, "wlp2s0 is a wireless device") {
		t.Fatalf("wireless = %+v", wireless)
	}
	present := CheckControllerInterface("enp0s8", "enp0s3", controllerDevices(), "192.168.0.198")
	if present.State != DevicePresent || present.Blocking || !strings.HasPrefix(present.Message, "The controller has enp0s3.") {
		t.Fatalf("present = %+v", present)
	}
	elsewhere := CheckControllerInterface("enp0s3", "enp0s8", controllerDevices(), "192.168.0.198")
	if elsewhere.State != DevicePresent || elsewhere.Blocking || !strings.Contains(elsewhere.Message, "its DHCP address is on enp0s3") {
		t.Fatalf("elsewhere = %+v", elsewhere)
	}
	// The saved address is gone: the leased address still marks its device.
	leased := CheckControllerInterface("enp0s3", "enp0s3", controllerDevices(), "192.0.2.10")
	if leased.DHCPDevice != "enp0s3" {
		t.Fatalf("leased = %+v", leased)
	}
	unknown := CheckControllerInterface("enp0s3", "enp5s0", NetworkObservation{Detail: "network devices are unreadable"}, "")
	if unknown.State != DeviceUnknown || unknown.Blocking || !strings.Contains(unknown.Message, "could not be read") {
		t.Fatalf("unknown = %+v", unknown)
	}
}

func TestCheckClientInterfaceAndSaveRefusal(t *testing.T) {
	host := func(name string) HostMeta { return HostMeta{Name: name, IP: "10.0.0.1", Interface: "enp3s0"} }
	present := CheckClientInterface(host("pc01"), "", NetworkObservation{Reachable: true, Devices: []NetworkDevice{{Name: "enp3s0", Kind: NetworkDeviceWired}}})
	missing := CheckClientInterface(host("pc02"), "", NetworkObservation{Reachable: true, Devices: []NetworkDevice{{Name: "enp2s0"}, {Name: "wlp3s0"}}})
	unreachable := CheckClientInterface(host("pc03"), "", NetworkObservation{Detail: "no answer"})
	if present.State != DevicePresent || missing.State != DeviceMissing || strings.Join(missing.Devices, ",") != "enp2s0,wlp3s0" || unreachable.State != DeviceUnreachable {
		t.Fatalf("checks = %+v %+v %+v", present, missing, unreachable)
	}
	preflight := &NetworkPreflight{Clients: []ClientNetworkCheck{present, missing, unreachable}}
	if strings.Join(preflight.Missing(), ",") != "pc02" || strings.Join(preflight.Unreachable(), ",") != "pc03" {
		t.Fatalf("missing/unreachable = %v %v", preflight.Missing(), preflight.Unreachable())
	}
	if refusal := preflight.SaveRefusal(); !strings.Contains(refusal, "missing on pc02") {
		t.Fatalf("refusal = %q", refusal)
	}
	preflight.Acknowledged = []string{"pc01"}
	if preflight.SaveRefusal() == "" {
		t.Fatal("acknowledging another computer allowed the save")
	}
	preflight.Acknowledged = []string{"pc02"}
	if refusal := preflight.SaveRefusal(); refusal != "" {
		t.Fatalf("acknowledged refusal = %q", refusal)
	}
	controller := CheckControllerInterface("enp0s3", "enp5s0", controllerDevices(), "")
	preflight.Controller = &controller
	if refusal := preflight.SaveRefusal(); !strings.Contains(refusal, "no network device called enp5s0") {
		t.Fatalf("controller refusal = %q", refusal)
	}
	var none *NetworkPreflight
	if none.SaveRefusal() != "" || none.Missing() != nil {
		t.Fatal("an absent preflight refused")
	}
}

func TestControllerDHCPCandidatesAndCheck(t *testing.T) {
	observation := controllerDevices()
	observation.Devices = append(observation.Devices, NetworkDevice{Name: "enp0s9", Kind: NetworkDeviceWired, Addresses: []NetworkAddress{{Address: "169.254.3.4", Dynamic: true}, {Address: "172.16.0.5"}, {Address: "10.0.0.7", Dynamic: true}}})
	lab := validSettings().Lab
	choices := ControllerDHCPCandidates(observation, lab)
	if len(choices) != 1 || choices[0] != (DHCPAddressChoice{Address: "192.168.0.198", Device: "enp0s3"}) {
		t.Fatalf("choices = %+v", choices)
	}
	observation.Lifetimes = false
	if choices := ControllerDHCPCandidates(observation, lab); len(choices) != 2 {
		t.Fatalf("choices without lifetimes = %+v", choices)
	}
	if check := CheckDHCPAddress("192.168.0.198", choices); !check.Found || check.Message != "" {
		t.Fatalf("found = %+v", check)
	}
	if check := CheckDHCPAddress("192.168.0.50", choices); check.Found || !strings.Contains(check.Message, "192.168.0.50 is not one of the controller's current DHCP addresses (found: 192.168.0.198)") {
		t.Fatalf("not found = %+v", check)
	}
}

func TestNetworkLockedRefusalUsesTheCatalogue(t *testing.T) {
	base := validSettings()
	candidate := base
	candidate.Lab.NetworkBase = "10.1.0.0"
	issues := SettingsTransitionIssues(base, candidate)
	want := "The laboratory network cannot be changed while 20 computers are configured: they would keep their old addresses and lose the controller. A guided change of the laboratory network is planned; until then keep the saved value."
	if len(issues) != 1 || issues[0].Message != want || issues[0].Next == nil || issues[0].Next.Code != "NETWORK-LOCKED" {
		t.Fatalf("issues = %+v", issues)
	}
	if got := NetworkLockedMessage(1); !strings.HasPrefix(got, "The laboratory network cannot be changed while 1 computer is configured:") {
		t.Fatalf("singular = %q", got)
	}
	step, found := NextStepByCode("NETWORK-LOCKED")
	if !found || step.Title == "" || step.Why == "" || step.Action == "" {
		t.Fatalf("catalogue = %+v", step)
	}
}

func TestClientInterfaceChangeAdvisesOneComputerFirst(t *testing.T) {
	base := validSettings()
	for _, field := range []string{"lab.clientIfaceName", "lab.hostIfaceNames", "lab.ifaceName"} {
		found := false
		for _, impact := range SettingsImpacts(base, base, []SettingChange{{Field: field}}) {
			if impact.Kind == "network-interface-order" && impact.Detail == "Update one computer first and check it answers before updating the others." {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s lacks the one-computer-first advice", field)
		}
	}
	for _, impact := range SettingsImpacts(base, base, []SettingChange{{Field: "lab.controllerIfaceName"}}) {
		if impact.Kind == "network-interface-order" {
			t.Fatal("a controller-only change advised updating computers")
		}
	}
}
