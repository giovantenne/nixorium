package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestCoordinationGateIsIndependentOfUserStateRoot(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := acquireOperationGateAt(directory, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := acquireOperationGateAt(directory, false); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second state root bypassed the operation gate: %v", err)
	}
}

func TestCoordinationReservationSurvivesLockOwnerExit(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := reserveRemoteInstallAt(directory, false, validRemoteReservationPlan(), "sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.(*remoteInstallReservation).gate.Close(); err != nil {
		t.Fatal(err)
	}
	reservation.(*remoteInstallReservation).gate = nil

	active, err := operationActiveAt(directory, false)
	if err != nil || !active {
		t.Fatalf("persistent reservation active=%t error=%v", active, err)
	}
	if _, err := acquireOperationGateAt(directory, false); err == nil || !strings.Contains(err.Error(), "remains reserved") {
		t.Fatalf("new operation ignored persistent reservation: %v", err)
	}
}

func TestCoordinationFailsClosedForUnsafeReservation(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "reservation")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, remoteReservationName)); err != nil {
		t.Fatal(err)
	}
	active, err := operationActiveAt(directory, false)
	if !active || err == nil {
		t.Fatalf("unsafe reservation active=%t error=%v", active, err)
	}
}

func TestRemoteReservationRequiresMatchingOperationToRelease(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value, err := reserveRemoteInstallAt(directory, false, validRemoteReservationPlan(), "sha256:"+strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	reservation := value.(*remoteInstallReservation)
	marker := filepath.Join(directory, remoteReservationName)
	if err := os.WriteFile(marker, []byte("{\"operationId\":\"ffffffffffffffffffffffffffffffff\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reservation.ReleaseResolved(); err == nil {
		t.Fatal("mismatched reservation was removed")
	}
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("mismatched reservation was not preserved: %v", err)
	}
	_ = reservation.gate.Close()
}

func validRemoteReservationPlan() domain.RemoteInstallPlan {
	return domain.RemoteInstallPlan{
		SchemaVersion:      domain.RemoteInstallSchemaVersion,
		OperationID:        "0123456789abcdef0123456789abcdef",
		BootID:             "01234567-89ab-cdef-0123-456789abcdef",
		DeploymentRevision: "0123456789abcdef0123456789abcdef01234567",
		SystemPath:         "/nix/store/00000000000000000000000000000000-nixos-system-pc01-test",
		Host:               domain.RemoteInstallHost{Name: "pc01", Interface: "enp0s2", LiveIP: "192.0.2.20", StaticIP: "10.0.0.1"},
		Cache:              domain.RemoteInstallCache{URL: "http://192.0.2.10:5000", PublicKey: "cache.example:YWJjZA=="},
		Disk:               domain.RemoteDisk{Path: "/dev/sda", KName: "sda", MajorMinor: "8:0", SizeBytes: 16 << 30, Serial: "serial", Model: "disk", Transport: "sata", DiskSeq: "1"},
		AdminPublicKey:     "ssh-ed25519 YWJjZA== admin@test",
		HostKeyPublic:      "ssh-ed25519 YWJjZA== root@test",
	}
}
