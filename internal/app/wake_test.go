package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeWakeSource struct {
	meta      domain.LabMeta
	on        map[string]bool
	addresses map[string]domain.WakeAddress
	failures  map[string]error
	learned   []string
	probed    int
	sent      []string
	network   domain.WakeNetwork
}

func (source *fakeWakeSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return source.meta, nil
}

func (source *fakeWakeSource) SSHStatus(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.SSHProbe {
	source.probed++
	probes := map[string]domain.SSHProbe{}
	for _, host := range hosts {
		probes[host.Name] = domain.SSHProbe{Reachability: domain.ReachabilityUnreachable}
		if source.on[host.Name] {
			probes[host.Name] = domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
		}
	}
	return probes
}

func (source *fakeWakeSource) LearnWakeAddresses(hosts []domain.HostMeta) {
	for _, host := range hosts {
		source.learned = append(source.learned, host.Name)
	}
}

func (source *fakeWakeSource) WakeAddresses() map[string]domain.WakeAddress {
	return source.addresses
}

func (source *fakeWakeSource) SendWakePackets(_ context.Context, network domain.WakeNetwork, macs []string) map[string]error {
	source.network = network
	source.sent = append(source.sent, macs...)
	results := map[string]error{}
	for _, mac := range macs {
		results[mac] = source.failures[mac]
	}
	return results
}

func wakeLab() *fakeWakeSource {
	meta := domain.LabMeta{}
	meta.Controller.Name = "controller"
	meta.Controller.StaticIP = "10.42.0.1"
	meta.Controller.Interface = "enp3s0"
	meta.Network.Base = "10.42.0.0"
	meta.Network.PrefixLength = 24
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}, {Name: "pc02", IP: "10.42.0.12"}, {Name: "pc03", IP: "10.42.0.13"}}
	return &fakeWakeSource{
		meta: meta,
		on:   map[string]bool{"pc01": true},
		addresses: map[string]domain.WakeAddress{
			"pc01": {MAC: "52:54:00:00:00:01"},
			"pc02": {MAC: "52:54:00:00:00:02"},
		},
		failures: map[string]error{},
	}
}

func TestWakePlanSendsOnlyToComputersOffWithAKnownAddress(t *testing.T) {
	source := wakeLab()
	manager := NewWakeManager(source)
	plan := manager.Plan(context.Background(), "/lab", "@lab")
	if plan.HasErrors() || plan.Requested != "@lab" || len(plan.Targets) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	states := map[string]string{}
	for _, target := range plan.Targets {
		states[target.Name] = target.State
	}
	if states["pc01"] != domain.WakeTargetOn || states["pc02"] != domain.WakeTargetSend || states["pc03"] != domain.WakeTargetUnknown {
		t.Fatalf("states = %v", states)
	}
	if !strings.Contains(plan.Targets[2].Detail, "it is learned while the computer is on and the classroom view is open") {
		t.Fatalf("unknown detail = %q", plan.Targets[2].Detail)
	}
	// The computer that answered teaches its address at once.
	if len(source.learned) != 1 || source.learned[0] != "pc01" {
		t.Fatalf("learned = %v", source.learned)
	}
	if plan.Message != "1 computer will receive a Wake-on-LAN packet; 1 already on, 1 with no known address yet." {
		t.Fatalf("message = %q", plan.Message)
	}
}

func TestWakePlanRefusesNamesThatAreNotClients(t *testing.T) {
	source := wakeLab()
	manager := NewWakeManager(source)
	for requested, invalid := range map[string][]string{
		"pc01,pc99":       {"pc99"},
		"pc01,pc01":       {"pc01"},
		"controller,pc02": {"controller"},
	} {
		plan := manager.Plan(context.Background(), "/lab", requested)
		if !plan.HasErrors() || len(plan.Targets) != len(invalid) || !strings.HasPrefix(plan.Message, "Nothing was sent") {
			t.Fatalf("%s: plan = %+v", requested, plan)
		}
		for index, name := range invalid {
			if plan.Targets[index].Name != name || plan.Targets[index].State != domain.WakeTargetInvalid {
				t.Fatalf("%s: targets = %+v", requested, plan.Targets)
			}
		}
		if report := manager.Apply(context.Background(), plan); report.State != "blocked" || len(source.sent) != 0 {
			t.Fatalf("%s: an invalid plan was applied: %+v", requested, report)
		}
	}
	if source.probed != 0 {
		t.Fatal("computers were probed for an invalid request")
	}
}

func TestWakeApplySendsFromTheControllersOwnRecord(t *testing.T) {
	source := wakeLab()
	manager := NewWakeManager(source)
	plan := manager.Plan(context.Background(), "/lab", "@lab")
	// A plan cannot choose the address: the record is read again.
	source.addresses["pc02"] = domain.WakeAddress{MAC: "52:54:00:00:00:22"}
	report := manager.Apply(context.Background(), plan)
	if report.State != "partial" || report.Sent != 1 || report.AlreadyOn != 1 || report.Unknown != 1 || report.Failed != 0 {
		t.Fatalf("report = %+v", report)
	}
	if len(source.sent) != 1 || source.sent[0] != "52:54:00:00:00:22" {
		t.Fatalf("sent = %v", source.sent)
	}
	if source.network != (domain.WakeNetwork{Broadcast: "10.42.0.255", Source: "10.42.0.1", Interface: "enp3s0"}) {
		t.Fatalf("network = %+v", source.network)
	}
	outcomes := map[string]string{}
	for _, outcome := range report.Targets {
		outcomes[outcome.Name] = outcome.State
	}
	if outcomes["pc01"] != domain.WakeOutcomeOn || outcomes["pc02"] != domain.WakeOutcomeSent || outcomes["pc03"] != domain.WakeOutcomeUnknown {
		t.Fatalf("outcomes = %v", outcomes)
	}
	// Sending is never reported as a computer that started.
	for _, word := range []string{"switched on", "started", "is on"} {
		if strings.Contains(strings.ToLower(report.Message), word) {
			t.Fatalf("message claims %q: %s", word, report.Message)
		}
	}
	if !strings.HasPrefix(report.Message, "Wake-on-LAN packets sent to 1 computer; 1 already on; 1 with no known address yet.") {
		t.Fatalf("message = %q", report.Message)
	}
}

func TestWakeApplyReportsPacketsThatDidNotLeave(t *testing.T) {
	source := wakeLab()
	source.on = map[string]bool{}
	source.failures["52:54:00:00:00:01"] = errors.New("network is unreachable")
	manager := NewWakeManager(source)
	report := manager.Apply(context.Background(), manager.Plan(context.Background(), "/lab", "pc01,pc02"))
	if report.State != "partial" || report.Sent != 1 || report.Failed != 1 || !strings.Contains(report.Targets[0].Detail, "network is unreachable") {
		t.Fatalf("report = %+v", report)
	}
}

func TestWakeApplyRefusesStaleOrChangedReviews(t *testing.T) {
	source := wakeLab()
	manager := NewWakeManager(source)
	plan := manager.Plan(context.Background(), "/lab", "pc02")
	changed := plan
	changed.Targets = append([]domain.WakeTarget(nil), plan.Targets...)
	changed.Targets[0].IP = "10.42.0.200"
	if report := manager.Apply(context.Background(), changed); report.State != "blocked" || !strings.Contains(report.Message, "inventory changed") {
		t.Fatalf("changed address = %+v", report)
	}
	controller := plan
	controller.Targets = []domain.WakeTarget{{HostMeta: domain.HostMeta{Name: "controller", IP: "10.42.0.1"}, State: domain.WakeTargetSend}}
	if report := manager.Apply(context.Background(), controller); report.State != "blocked" {
		t.Fatalf("controller = %+v", report)
	}
	manager.now = func() time.Time { return time.Now().Add(wakeReviewWindow + time.Minute) }
	if report := manager.Apply(context.Background(), plan); report.State != "blocked" || !strings.Contains(report.Message, "expired") {
		t.Fatalf("expired = %+v", report)
	}
	if len(source.sent) != 0 {
		t.Fatalf("sent = %v", source.sent)
	}
}

func TestWakeWithNothingToSendSaysWhy(t *testing.T) {
	source := wakeLab()
	manager := NewWakeManager(source)
	plan := manager.Plan(context.Background(), "/lab", "pc01,pc03")
	if plan.HasErrors() || plan.Count(domain.WakeTargetSend) != 0 || plan.Message != "No packet will be sent: 1 already on, 1 with no known address yet." {
		t.Fatalf("plan = %+v", plan)
	}
	report := manager.Apply(context.Background(), plan)
	if report.Sent != 0 || len(source.sent) != 0 || report.State != "partial" {
		t.Fatalf("report = %+v", report)
	}
}
