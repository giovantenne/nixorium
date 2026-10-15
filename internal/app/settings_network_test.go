package app

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeNetworkObserver struct {
	controller domain.NetworkObservation
	clients    map[string]domain.NetworkObservation
	probed     *[]string
	timeout    *time.Duration
}

func (f fakeNetworkObserver) ControllerNetworkDevices(context.Context) domain.NetworkObservation {
	return f.controller
}

func (f fakeNetworkObserver) ClientNetworkDevices(_ context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.NetworkObservation {
	result := map[string]domain.NetworkObservation{}
	for _, host := range hosts {
		if f.probed != nil {
			*f.probed = append(*f.probed, host.Name+"@"+host.IP)
		}
		result[host.Name] = f.clients[host.Name]
	}
	if f.timeout != nil {
		*f.timeout = timeout
	}
	return result
}

func networkTestObserver() fakeNetworkObserver {
	wired := func(names ...string) domain.NetworkObservation {
		devices := []domain.NetworkDevice{}
		for _, name := range names {
			devices = append(devices, domain.NetworkDevice{Name: name, Kind: domain.NetworkDeviceWired})
		}
		return domain.NetworkObservation{Reachable: true, Devices: devices}
	}
	return fakeNetworkObserver{
		controller: domain.NetworkObservation{Reachable: true, Lifetimes: true, Devices: []domain.NetworkDevice{
			{Name: "enp0s3", Kind: domain.NetworkDeviceWired, Addresses: []domain.NetworkAddress{{Address: "192.168.0.198", Dynamic: true}, {Address: "10.0.0.99"}}},
			{Name: "wlp2s0", Kind: domain.NetworkDeviceWireless},
		}},
		clients: map[string]domain.NetworkObservation{
			"pc01": wired("enp3s0"),
			"pc02": wired("enp2s0"),
			"pc03": {Detail: "no answer"},
		},
	}
}

// networkSettingsFixture is a saved laboratory of three computers.
func networkSettingsFixture(t *testing.T) ([]byte, domain.LabSettingsFile) {
	t.Helper()
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	base, issues := domain.DecodeLabSettings(data)
	if len(issues) != 0 {
		t.Fatalf("template issues = %+v", issues)
	}
	base.Lab.PCCount = 3
	base.Lab.MasterDHCPIP = "192.168.0.198"
	data, err = domain.MarshalLabSettings(base)
	if err != nil {
		t.Fatal(err)
	}
	return data, base
}

func TestSettingsPlanChecksTheControllerDevice(t *testing.T) {
	data, base := networkSettingsFixture(t)
	manager := NewSettingsManager(&mutableSettingsSource{data: data}).WithNetworkObserver(networkTestObserver())
	candidate := base
	candidate.Lab.ControllerInterfaceName = "enp5s0"
	plan := manager.PlanSettings(context.Background(), ".", candidate)
	if plan.HasErrors() || plan.State != "valid" || plan.NetworkPreflight == nil || plan.NetworkPreflight.Controller == nil {
		t.Fatalf("plan = %+v", plan)
	}
	check := plan.NetworkPreflight.Controller
	want := "The controller has no network device called enp5s0. Its devices are: enp0s3 (192.168.0.198, 10.0.0.99, carries the DHCP address), wlp2s0 (wireless)."
	if check.State != domain.DeviceMissing || !check.Blocking || check.Message != want || check.Previous != "enp0s3" {
		t.Fatalf("controller check = %+v", check)
	}
	if len(plan.NetworkPreflight.Clients) != 0 {
		t.Fatalf("a controller change probed clients: %+v", plan.NetworkPreflight.Clients)
	}
	candidate.Lab.ControllerInterfaceName = "enp0s3"
	if plan := manager.PlanSettings(context.Background(), ".", candidate); plan.NetworkPreflight != nil {
		t.Fatalf("an unchanged effective device was checked: %+v", plan.NetworkPreflight)
	}
	// Without an observer (older callers, setup) the plan is unchanged.
	if plan := NewSettingsManager(&mutableSettingsSource{data: data}).PlanSettings(context.Background(), ".", candidate); plan.NetworkPreflight != nil {
		t.Fatalf("preflight without observer = %+v", plan.NetworkPreflight)
	}
}

func TestSettingsPlanProbesAffectedClientsInParallelOnce(t *testing.T) {
	data, base := networkSettingsFixture(t)
	probed := []string{}
	var timeout time.Duration
	observer := networkTestObserver()
	observer.probed, observer.timeout = &probed, &timeout
	manager := NewSettingsManager(&mutableSettingsSource{data: data}).WithNetworkObserver(observer)
	candidate := base
	candidate.Lab.ClientInterfaceName = "enp3s0"
	plan := manager.PlanSettings(context.Background(), ".", candidate)
	sort.Strings(probed)
	if strings.Join(probed, ",") != "pc01@10.0.0.1,pc02@10.0.0.2,pc03@10.0.0.3" || timeout != settingsClientProbeTimeout {
		t.Fatalf("probed = %v timeout=%s", probed, timeout)
	}
	states := []string{}
	for _, client := range plan.NetworkPreflight.Clients {
		states = append(states, client.Name+":"+client.State)
	}
	if strings.Join(states, ",") != "pc01:present,pc02:missing,pc03:unreachable" || plan.NetworkPreflight.Controller != nil {
		t.Fatalf("clients = %v", states)
	}
}

func TestSettingsSaveRefusesMissingDevicesUnlessAcknowledged(t *testing.T) {
	data, base := networkSettingsFixture(t)
	candidate := base
	candidate.Lab.ClientInterfaceName = "enp3s0"
	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		t.Fatal(err)
	}
	newManager := func() (SettingsSaveManager, *mutableSettingsSource, *fakeGitCommitSource) {
		source := &mutableSettingsSource{data: data}
		commits := settingsSaveCommitSource(candidateData)
		settings := NewSettingsManager(source).WithNetworkObserver(networkTestObserver())
		return NewSettingsSaveManager(settings, NewGitReviewManager(&fakeGitCommitSource{revision: strings.Repeat("a", 40)}), NewGitCommitManager(commits)), source, commits
	}

	manager, source, commits := newManager()
	reviewed := manager.settings.PlanSettings(context.Background(), ".", candidate)
	report := manager.Save(context.Background(), ".", candidate, reviewed)
	if report.State != "blocked" || source.writes != 0 || commits.commits != 0 || !strings.Contains(report.Message, "missing on pc02") {
		t.Fatalf("unacknowledged save = %+v writes=%d", report, source.writes)
	}

	// Acknowledging a different computer does not authorize pc02.
	reviewed.NetworkPreflight.Acknowledged = []string{"pc03"}
	if report := manager.Save(context.Background(), ".", candidate, reviewed); report.State != "blocked" || source.writes != 0 {
		t.Fatalf("wrong acknowledgement saved: %+v", report)
	}

	manager, source, commits = newManager()
	reviewed = manager.settings.PlanSettings(context.Background(), ".", candidate)
	reviewed.NetworkPreflight.Acknowledged = reviewed.NetworkPreflight.Missing()
	report = manager.Save(context.Background(), ".", candidate, reviewed)
	if report.State != "saved" || source.writes != 1 || commits.commits != 1 {
		t.Fatalf("acknowledged save = %+v writes=%d commits=%d", report, source.writes, commits.commits)
	}
}

func TestSettingsSaveRefusesAMissingControllerDevice(t *testing.T) {
	data, base := networkSettingsFixture(t)
	candidate := base
	candidate.Lab.ControllerInterfaceName = "wlp2s0"
	source := &mutableSettingsSource{data: data}
	manager := NewSettingsSaveManager(NewSettingsManager(source).WithNetworkObserver(networkTestObserver()), NewGitReviewManager(&fakeGitCommitSource{revision: strings.Repeat("a", 40)}), NewGitCommitManager(&fakeGitCommitSource{}))
	reviewed := manager.settings.PlanSettings(context.Background(), ".", candidate)
	// Acknowledgement covers clients only, never the controller.
	reviewed.NetworkPreflight.Acknowledged = []string{"controller", "pc99"}
	report := manager.Save(context.Background(), ".", candidate, reviewed)
	if report.State != "blocked" || source.writes != 0 || !strings.Contains(report.Message, "wlp2s0 is a wireless device") {
		t.Fatalf("controller save = %+v", report)
	}
}

func TestSettingsPlanWarnsAboutAnUndetectedDHCPAddress(t *testing.T) {
	data, base := networkSettingsFixture(t)
	manager := NewSettingsManager(&mutableSettingsSource{data: data}).WithNetworkObserver(networkTestObserver())
	candidate := base
	candidate.Lab.MasterDHCPIP = "192.168.0.50"
	plan := manager.PlanSettings(context.Background(), ".", candidate)
	if plan.HasErrors() || plan.NetworkPreflight == nil || plan.NetworkPreflight.DHCPAddress == nil || plan.NetworkPreflight.DHCPAddress.Found || plan.NetworkPreflight.SaveRefusal() != "" {
		t.Fatalf("plan = %+v", plan.NetworkPreflight)
	}
	addresses := manager.ControllerAddresses(context.Background(), base)
	if !addresses.Detected || len(addresses.Choices) != 1 || addresses.Choices[0].Address != "192.168.0.198" || addresses.Choices[0].Device != "enp0s3" {
		t.Fatalf("addresses = %+v", addresses)
	}
}
