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

func TestRemoteReservationRecoveryReacquiresOrphanedGate(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := createRemoteReservationAt(directory, false, "0123456789abcdef0123456789abcdef", "")
	if err != nil {
		t.Fatal(err)
	}
	orphan := created.(*remoteInstallReservation)
	if err := orphan.gate.Close(); err != nil {
		t.Fatal(err)
	}
	orphan.gate = nil

	operationID, recoveredValue, present, err := recoverRemoteReservationAt(directory, false)
	if err != nil || !present || operationID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("recover id=%q present=%t error=%v", operationID, present, err)
	}
	if _, _, _, err := recoverRemoteReservationAt(directory, false); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second worker acquired recovered reservation: %v", err)
	}
	if err := recoveredValue.ReleaseResolved(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(directory, remoteReservationName)); !os.IsNotExist(err) {
		t.Fatalf("resolved recovered marker still exists: %v", err)
	}
}

func TestRemoteReservationRecoveryRejectsMismatchedStateBinding(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, remoteReservationName)
	content := []byte(`{"schemaVersion":1,"operationId":"0123456789abcdef0123456789abcdef","statePath":"/tmp/attacker.json","tokenDigest":""}` + "\n")
	if err := os.WriteFile(marker, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, present, err := recoverRemoteReservationAt(directory, false); !present || err == nil {
		t.Fatalf("unsafe marker present=%t error=%v", present, err)
	}
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("unsafe marker was removed: %v", err)
	}
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
