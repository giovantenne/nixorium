package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeRemoteInstallSource struct {
	meta          domain.LabMeta
	revision      string
	revisionErr   error
	services      map[string]domain.ServiceState
	reachable     bool
	reachableErr  error
	revalidateErr error
	dispatch      RemoteInstallDispatch
	dispatchErr   error
	dispatched    int
}

func (source *fakeRemoteInstallSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return source.meta, nil
}
func (source *fakeRemoteInstallSource) CurrentRevision(context.Context, string) (string, error) {
	return source.revision, source.revisionErr
}
func (source *fakeRemoteInstallSource) ServiceState(_ context.Context, unit string) domain.ServiceState {
	return source.services[unit]
}
func (source *fakeRemoteInstallSource) RemoteIdentityReachable(context.Context, string) (bool, error) {
	return source.reachable, source.reachableErr
}
func (source *fakeRemoteInstallSource) RevalidateRemoteInstall(context.Context, domain.RemoteInstallPreparation, domain.RemoteInstallPlan) error {
	return source.revalidateErr
}
func (source *fakeRemoteInstallSource) DispatchRemoteInstall(context.Context, domain.RemoteInstallPlan) (RemoteInstallDispatch, error) {
	source.dispatched++
	return source.dispatch, source.dispatchErr
}

func remoteTestPreparation() (domain.RemoteInstallPreparation, *fakeRemoteInstallSource) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	disk := domain.RemoteDisk{
		Path: "/dev/sda", KName: "sda", MajorMinor: "8:0", SizeBytes: 16 * 1024 * 1024 * 1024,
		Serial: "serial-1", Model: "Test disk", Transport: "sata", DiskSeq: "1", Eligible: true,
	}
	preparation := domain.RemoteInstallPreparation{
		OperationID:        "0123456789abcdef0123456789abcdef",
		Repository:         "/deployment",
		DeploymentRevision: revision,
		BundlePath:         "/nix/store/11111111111111111111111111111111-remote-installer",
		BundleClosureBytes: 256 * 1024 * 1024,
		SystemPath:         "/nix/store/00000000000000000000000000000000-nixos-system-pc01-test",
		SystemClosureBytes: 4 * 1024 * 1024 * 1024,
		Host:               domain.RemoteInstallHost{Name: "pc01", Interface: "enp0s2", LiveIP: "192.0.2.20", StaticIP: "10.0.0.1"},
		Cache:              domain.RemoteInstallCache{URL: "http://192.0.2.10:5000", PublicKey: "cache.example:YWJjZA=="},
		AdminPublicKey:     "ssh-ed25519 YWJjZA== admin@test",
		HostKeyPublic:      "ssh-ed25519 YWJjZA== root@test",
		HostFingerprint:    "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Facts: domain.RemoteMachineFacts{
			SchemaVersion: domain.RemoteInstallSchemaVersion, VariantID: "installer", VersionID: "26.05", BuildID: "26.05.test",
			Architecture: "x86_64", UEFI: true, SudoReady: true,
			BootID:               "01234567-89ab-cdef-0123-456789abcdef",
			MemoryAvailableBytes: 1024 * 1024 * 1024, StoreAvailableBytes: 1024 * 1024 * 1024,
			Interfaces: []domain.RemoteNetworkInterface{{Name: "enp0s2", Addresses: []string{"192.0.2.20"}}},
			Disks:      []domain.RemoteDisk{disk},
		},
		PreparedAt: time.Unix(1000, 0).UTC(),
		Issues:     []domain.ValidationIssue{},
	}
	meta := domain.LabMeta{DeploymentMode: "laboratory", SchemaVersion: 2}
	meta.Controller.StaticIP = "10.0.0.99"
	meta.Controller.DHCPIP = "192.0.2.10"
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1", Interface: "enp0s2"}}
	source := &fakeRemoteInstallSource{
		meta: meta, revision: revision, services: map[string]domain.ServiceState{},
		dispatch: RemoteInstallDispatch{
			Accepted: true, LogID: "usb-install-0123456789abcdef0123456789abcdef.log",
			Receipt: domain.RemoteInstallReceipt{
				SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: preparation.OperationID,
				State: "ready-to-reboot", Phase: domain.RemoteInstallPhaseReadyToReboot,
				MutationStarted: true, DiskMayBeModified: true, Installed: true,
				Message: "installation completed",
			},
		},
	}
	return preparation, source
}

func TestRemoteInstallPlanBindsVerifiedPreparation(t *testing.T) {
	preparation, source := remoteTestPreparation()
	manager := NewRemoteInstallManager(source)
	manager.now = func() time.Time { return time.Unix(2000, 0).UTC() }
	report := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)
	if report.HasErrors() || report.Plan.Disk.Path != "/dev/sda" || report.Plan.BootID != preparation.Facts.BootID {
		t.Fatalf("plan not ready: %#v", report)
	}
	if report.ExpiresAt != manager.now().Add(domain.RemoteInstallReviewWindow) || report.ReviewToken == "" {
		t.Fatalf("invalid review lifetime: %#v", report)
	}
	if report.Confirmation != "ERASE /dev/sda FOR pc01" {
		t.Fatalf("unexpected confirmation %q", report.Confirmation)
	}
}

func TestRemoteInstallPlanAcceptsObservedCardWithoutExplicitName(t *testing.T) {
	preparation, source := remoteTestPreparation()
	notExplicit := false
	// The template fallback name differs from the card the PC booted with.
	source.meta.Clients.Hosts[0].Interface = "enp0s3"
	source.meta.Clients.Hosts[0].InterfaceExplicit = &notExplicit
	manager := NewRemoteInstallManager(source)
	manager.now = func() time.Time { return time.Unix(2000, 0).UTC() }
	if report := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false); report.HasErrors() {
		t.Fatalf("observed card rejected without an explicit name: %#v", report)
	}
	explicit := true
	source.meta.Clients.Hosts[0].InterfaceExplicit = &explicit
	if report := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false); !report.HasErrors() {
		t.Fatal("a differing explicit interface name was accepted")
	}
}

func TestRemoteInstallPlanRequiresExplicitKnownHostRotation(t *testing.T) {
	preparation, source := remoteTestPreparation()
	preparation.KnownHostConflict = true
	manager := NewRemoteInstallManager(source)
	manager.now = func() time.Time { return time.Unix(2000, 0).UTC() }

	blocked := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)
	if !blocked.HasErrors() || len(blocked.Issues) == 0 || blocked.Issues[0].Field != "hostKeyRotation" {
		t.Fatalf("known-host conflict did not require separate review: %#v", blocked)
	}
	plan := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", true)
	if plan.HasErrors() || !plan.HostKeyRotation || !plan.Plan.HostKeyRotation {
		t.Fatalf("explicitly reviewed known-host rotation was not bound to the plan: %#v", plan)
	}
	changed := plan
	changed.HostKeyRotation = false
	if domain.RemoteInstallReviewToken(changed) == plan.ReviewToken {
		t.Fatal("host-key rotation display state was not bound to the review token")
	}
}

func TestRemoteInstallPlanRejectsUnsafeStateWithoutMutation(t *testing.T) {
	tests := map[string]func(*domain.RemoteInstallPreparation, *fakeRemoteInstallSource){
		"duplicate": func(_ *domain.RemoteInstallPreparation, source *fakeRemoteInstallSource) { source.reachable = true },
		"revision": func(_ *domain.RemoteInstallPreparation, source *fakeRemoteInstallSource) {
			source.revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"nic": func(preparation *domain.RemoteInstallPreparation, _ *fakeRemoteInstallSource) {
			preparation.Host.Interface = "eno1"
		},
		"controller-ip": func(preparation *domain.RemoteInstallPreparation, _ *fakeRemoteInstallSource) {
			preparation.Host.LiveIP = "192.0.2.10"
			preparation.Facts.Interfaces[0].Addresses[0] = "192.0.2.10"
		},
		"excluded-disk": func(preparation *domain.RemoteInstallPreparation, _ *fakeRemoteInstallSource) {
			preparation.Facts.Disks[0].Eligible = false
			preparation.Facts.Disks[0].ExclusionReasons = []string{"live-media"}
		},
		"changed-disk": func(preparation *domain.RemoteInstallPreparation, _ *fakeRemoteInstallSource) {
			preparation.Facts.Disks[0].Path = "/dev/sdb"
			preparation.Facts.Disks[0].KName = "sdb"
		},
		"resources": func(preparation *domain.RemoteInstallPreparation, _ *fakeRemoteInstallSource) {
			preparation.Facts.StoreAvailableBytes = 1
		},
		"memory": func(preparation *domain.RemoteInstallPreparation, _ *fakeRemoteInstallSource) {
			preparation.Facts.MemoryAvailableBytes = 1
		},
		"pxe": func(_ *domain.RemoteInstallPreparation, source *fakeRemoteInstallSource) {
			source.services[PXEListenerUnit] = domain.ServiceState{Name: PXEListenerUnit, Loaded: true, Active: true, State: "active"}
			source.services[PXENetworkUnit] = domain.ServiceState{Name: PXENetworkUnit, Loaded: true, Active: true, State: "active"}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			preparation, source := remoteTestPreparation()
			mutate(&preparation, source)
			report := NewRemoteInstallManager(source).PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)
			if !report.HasErrors() {
				t.Fatalf("unsafe plan accepted: %#v", report)
			}
		})
	}
}

func TestRemoteInstallApplyInstallsTheReviewedPlan(t *testing.T) {
	preparation, source := remoteTestPreparation()
	manager := NewRemoteInstallManager(source)
	now := time.Unix(2000, 0).UTC()
	manager.now = func() time.Time { return now }
	plan := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)
	report := manager.ApplyReserved(context.Background(), plan, plan.ReviewToken)
	if report.HasErrors() || !report.Installed || report.Phase != domain.RemoteInstallPhaseReadyToReboot {
		t.Fatalf("apply failed: %#v", report)
	}
	if source.dispatched != 1 {
		t.Fatalf("dispatched %d times", source.dispatched)
	}
}

func TestRemoteInstallApplyRejectsStaleOrChangedPlan(t *testing.T) {
	preparation, source := remoteTestPreparation()
	manager := NewRemoteInstallManager(source)
	now := time.Unix(2000, 0).UTC()
	manager.now = func() time.Time { return now }
	plan := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)

	if report := manager.ApplyReserved(context.Background(), plan, "sha256:stale"); !report.HasErrors() || source.dispatched != 0 {
		t.Fatal("stale token reached dispatch")
	}
	changed := plan
	changed.Plan.Disk.Serial = "replacement"
	if report := manager.ApplyReserved(context.Background(), changed, plan.ReviewToken); !report.HasErrors() || source.dispatched != 0 {
		t.Fatal("changed plan reached dispatch")
	}
	manager.now = func() time.Time { return plan.ExpiresAt }
	if report := manager.ApplyReserved(context.Background(), plan, plan.ReviewToken); !report.HasErrors() || source.dispatched != 0 {
		t.Fatal("expired review reached dispatch")
	}
}

func TestRemoteInstallApplyFailureSemantics(t *testing.T) {
	t.Run("failed revalidation does not dispatch", func(t *testing.T) {
		preparation, source := remoteTestPreparation()
		manager := NewRemoteInstallManager(source)
		manager.now = func() time.Time { return time.Unix(2000, 0).UTC() }
		plan := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)
		source.revalidateErr = errors.New("disk changed")
		report := manager.ApplyReserved(context.Background(), plan, plan.ReviewToken)
		if !report.HasErrors() || source.dispatched != 0 || report.DiskMayBeModified {
			t.Fatalf("wrong pre-dispatch semantics: %#v", report)
		}
	})

	t.Run("uncertain dispatch requires reconciliation", func(t *testing.T) {
		preparation, source := remoteTestPreparation()
		manager := NewRemoteInstallManager(source)
		manager.now = func() time.Time { return time.Unix(2000, 0).UTC() }
		plan := manager.PlanReservedWithHostKeyRotation(context.Background(), preparation, "/dev/sda", false)
		source.dispatch = RemoteInstallDispatch{Uncertain: true, LogID: "usb-install-test.log"}
		source.dispatchErr = errors.New("connection lost")
		report := manager.ApplyReserved(context.Background(), plan, plan.ReviewToken)
		if report.State != "reconciliation-required" || !report.DispatchUncertain || !report.DiskMayBeModified {
			t.Fatalf("uncertain dispatch was understated: %#v", report)
		}
	})
}
