package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakePXELifecycleSource struct {
	meta        domain.LabMeta
	deployment  domain.DeploymentStatus
	git         domain.GitState
	preparation domain.PXEPreparationState
	services    map[string]domain.ServiceState
	addresses   []string
	cacheErr    error
	controlErr  map[string]error
	controls    []string
}

func readyPXESource() *fakePXELifecycleSource {
	var meta domain.LabMeta
	meta.Controller.StaticIP = "10.0.0.99"
	meta.Controller.DHCPIP = "192.0.2.10"
	meta.Network.Interface = "enp1s0"
	meta.Network.PrefixLength = 8
	meta.Network.CachePort = 5000
	return &fakePXELifecycleSource{
		meta:        meta,
		deployment:  domain.DeploymentStatus{Ready: true},
		preparation: domain.PXEPreparationState{Present: true, Ready: true, Detail: "current"},
		services: map[string]domain.ServiceState{
			HarmoniaUnit:    {Name: HarmoniaUnit, Loaded: true, Active: true, State: "active"},
			PXEListenerUnit: {Name: PXEListenerUnit, Loaded: true, State: "inactive"},
			PXENetworkUnit:  {Name: PXENetworkUnit, Loaded: true, State: "inactive"},
		},
		addresses:  []string{"10.0.0.99", "192.0.2.10"},
		controlErr: map[string]error{},
	}
}

func (f *fakePXELifecycleSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}

func (f *fakePXELifecycleSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	return f.deployment, nil
}

func (f *fakePXELifecycleSource) GitState(context.Context, string) (domain.GitState, error) {
	return f.git, nil
}

func (f *fakePXELifecycleSource) PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState {
	return f.preparation
}

func (f *fakePXELifecycleSource) ServiceState(_ context.Context, name string) domain.ServiceState {
	return f.services[name]
}

func (f *fakePXELifecycleSource) InterfaceAddresses(string) ([]string, error) {
	return append([]string(nil), f.addresses...), nil
}

func (f *fakePXELifecycleSource) CacheHealth(context.Context, string, int) error {
	return f.cacheErr
}

func (f *fakePXELifecycleSource) ControlSystemUnit(_ context.Context, verb, unit string) error {
	key := verb + " " + unit
	f.controls = append(f.controls, key)
	if err := f.controlErr[key]; err != nil {
		return err
	}
	switch key {
	case "start " + PXEListenerUnit:
		listener := f.services[PXEListenerUnit]
		listener.Active = true
		listener.State = "active"
		f.services[PXEListenerUnit] = listener
		network := f.services[PXENetworkUnit]
		network.Active = true
		network.State = "active"
		f.services[PXENetworkUnit] = network
		f.addresses = []string{f.meta.Controller.DHCPIP}
	case "stop " + PXEListenerUnit:
		listener := f.services[PXEListenerUnit]
		listener.Active = false
		listener.State = "inactive"
		f.services[PXEListenerUnit] = listener
	case "stop " + PXENetworkUnit:
		network := f.services[PXENetworkUnit]
		network.Active = false
		network.State = "inactive"
		f.services[PXENetworkUnit] = network
		f.addresses = []string{f.meta.Controller.DHCPIP, f.meta.Controller.StaticIP}
	}
	return nil
}

func TestObservePXELifecycleDistinguishesRecoverableStates(t *testing.T) {
	inactiveListener := domain.ServiceState{Name: PXEListenerUnit, Loaded: true, State: "inactive"}
	inactiveNetwork := domain.ServiceState{Name: PXENetworkUnit, Loaded: true, State: "inactive"}
	activeListener := inactiveListener
	activeListener.Active = true
	activeListener.State = "active"
	activeNetwork := inactiveNetwork
	activeNetwork.Active = true
	activeNetwork.State = "active"
	failedListener := inactiveListener
	failedListener.State = "failed"

	for _, test := range []struct {
		listener    domain.ServiceState
		network     domain.ServiceState
		preparation domain.PXEPreparationState
		want        string
	}{
		{inactiveListener, inactiveNetwork, domain.PXEPreparationState{Ready: true}, "ready"},
		{activeListener, activeNetwork, domain.PXEPreparationState{}, "active"},
		{inactiveListener, activeNetwork, domain.PXEPreparationState{}, "recovery-required"},
		{failedListener, inactiveNetwork, domain.PXEPreparationState{}, "degraded"},
	} {
		if got := ObservePXELifecycle(test.listener, test.network, test.preparation).Mode; got != test.want {
			t.Fatalf("mode = %q, want %q", got, test.want)
		}
	}
}

func TestPXEStartPreflightsAndReconcilesState(t *testing.T) {
	source := readyPXESource()
	manager := NewPXELifecycle(source)
	plan := manager.PlanStart(context.Background(), "/deployment")
	if plan.HasErrors() || plan.Mode != "ready" || plan.StaticCIDR != "10.0.0.99/8" || len(source.controls) != 0 {
		t.Fatalf("plan = %+v, controls = %v", plan, source.controls)
	}
	report := manager.Start(context.Background(), "/deployment")
	if report.HasErrors() || report.Mode != "active" || report.Operation != "pxe-start" {
		t.Fatalf("report = %+v", report)
	}
	if want := []string{"start " + PXEListenerUnit}; !reflect.DeepEqual(source.controls, want) {
		t.Fatalf("controls = %v, want %v", source.controls, want)
	}
	second := manager.Start(context.Background(), "/deployment")
	if second.HasErrors() || len(source.controls) != 1 {
		t.Fatalf("idempotent report = %+v, controls = %v", second, source.controls)
	}
}

func TestPXEStartRejectsUnsafePreflight(t *testing.T) {
	for _, mutate := range []func(*fakePXELifecycleSource){
		func(source *fakePXELifecycleSource) { source.git.Dirty = true },
		func(source *fakePXELifecycleSource) { source.preparation.Ready = false },
		func(source *fakePXELifecycleSource) { source.cacheErr = errors.New("offline") },
		func(source *fakePXELifecycleSource) { source.addresses = []string{"10.0.0.99"} },
	} {
		source := readyPXESource()
		mutate(source)
		report := NewPXELifecycle(source).Start(context.Background(), "/deployment")
		if !report.HasErrors() || len(source.controls) != 0 {
			t.Fatalf("report = %+v, controls = %v", report, source.controls)
		}
	}
}

func TestPXEStartFailureRunsSynchronousCleanup(t *testing.T) {
	source := readyPXESource()
	source.controlErr["start "+PXEListenerUnit] = errors.New("port conflict")
	report := NewPXELifecycle(source).Start(context.Background(), "/deployment")
	want := []string{"start " + PXEListenerUnit, "stop " + PXEListenerUnit, "stop " + PXENetworkUnit}
	if !report.HasErrors() || !reflect.DeepEqual(source.controls, want) {
		t.Fatalf("report = %+v, controls = %v", report, source.controls)
	}
}

func TestPXEStopAndRecoverUseOnlyFixedOrder(t *testing.T) {
	source := readyPXESource()
	manager := NewPXELifecycle(source)
	if report := manager.Stop(context.Background(), "/deployment"); report.HasErrors() || report.Mode != "stopped" {
		t.Fatalf("stop report = %+v", report)
	}
	wantStop := []string{"stop " + PXEListenerUnit, "stop " + PXENetworkUnit}
	if !reflect.DeepEqual(source.controls, wantStop) {
		t.Fatalf("stop controls = %v", source.controls)
	}
	source.controls = nil
	if report := manager.Recover(context.Background(), "/deployment"); report.HasErrors() || report.Mode != "stopped" {
		t.Fatalf("recover report = %+v", report)
	}
	wantRecover := []string{"stop " + PXEListenerUnit, "stop " + PXENetworkUnit, "start " + PXERecoverUnit}
	if !reflect.DeepEqual(source.controls, wantRecover) {
		t.Fatalf("recover controls = %v", source.controls)
	}
}
