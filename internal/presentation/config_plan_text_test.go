package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestConfigPlanJSONAndTextIncludeTheNetworkCheck(t *testing.T) {
	devices := domain.NetworkObservation{Reachable: true, Devices: []domain.NetworkDevice{{Name: "enp0s3", Kind: domain.NetworkDeviceWired, Addresses: []domain.NetworkAddress{{Address: "192.168.0.198", Dynamic: true}}}}}
	controller := domain.CheckControllerInterface("enp0s3", "enp5s0", devices, "192.168.0.198")
	client := domain.CheckClientInterface(domain.HostMeta{Name: "pc02", IP: "10.0.0.2", Interface: "enp3s0"}, "", domain.NetworkObservation{Reachable: true, Devices: []domain.NetworkDevice{{Name: "enp2s0"}}})
	report := domain.ConfigPlanReport{Operation: "config-plan", State: "valid", Changes: []domain.SettingChange{}, Issues: []domain.ValidationIssue{},
		NetworkPreflight: &domain.NetworkPreflight{Controller: &controller, Clients: []domain.ClientNetworkCheck{client}}}
	var output bytes.Buffer
	if err := JSON(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"networkPreflight": {`, `"controller": {`, `"state": "missing"`, `"blocking": true`, `"dhcpDevice": "enp0s3"`, `"dynamic": true`, `"clients": [`, `"name": "pc02"`, `"devices": [`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %s in JSON:\n%s", want, output.String())
		}
	}
	output.Reset()
	report.NetworkPreflight = nil
	if err := JSON(&output, report); err != nil || strings.Contains(output.String(), "networkPreflight") {
		t.Fatalf("plans without a network change gained the field: %s", output.String())
	}
	output.Reset()
	report.NetworkPreflight = &domain.NetworkPreflight{Controller: &controller, Clients: []domain.ClientNetworkCheck{client}}
	ConfigPlanText(&output, report)
	for _, want := range []string{"Network check:", "  ! The controller has no network device called enp5s0.", "  ! pc02: enp3s0 missing (devices: enp2s0)"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q in text:\n%s", want, output.String())
		}
	}
}
