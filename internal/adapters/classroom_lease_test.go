package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func writeTestPendingDeployment(t *testing.T, directory string, names ...string) {
	t.Helper()
	targets := []domain.DeploymentTarget{}
	for index, name := range names {
		targets = append(targets, domain.DeploymentTarget{Name: name, IP: "10.0.0." + string(rune('1'+index))})
	}
	content, err := json.Marshal(map[string]any{
		"schemaVersion": 1, "repository": "/deployment", "revision": strings.Repeat("a", 40),
		"targets": targets, "startedAt": time.Unix(1_800_000_000, 0).UTC(), "controllerPid": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), content, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestClassroomLeaseIgnoresMaintenanceAndRefusesOnlyNamedComputers(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gate, err := acquireOperationGateAt(directory, false)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()

	lease, blocked, err := acquireClassroomLeaseAt(directory, false, time.Second)
	if err != nil || len(blocked) != 0 {
		t.Fatalf("a running maintenance operation blocked classroom controls: %v %v", blocked, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}

	writeTestPendingDeployment(t, directory, "pc01")
	lease, blocked, err = acquireClassroomLeaseAt(directory, false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if blocked["pc01"] != domain.ClassroomBusyReason || len(blocked) != 1 {
		t.Fatalf("blocked = %v", blocked)
	}
}

func TestClassroomLeaseFailsClosedForUnreadablePendingRecord(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := acquireClassroomLeaseAt(directory, false, time.Second); err == nil {
		t.Fatal("an unreadable record let classroom controls touch every computer")
	} else if code, _ := domain.BlockedCodeOf(err); code != "DEPLOY-PENDING" {
		t.Fatalf("error = %v", err)
	}
}

func TestPendingRecordWaitsForClassroomRequestsInFlight(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	coordination, err := openCoordinationDirectory(directory, false)
	if err != nil {
		t.Fatal(err)
	}
	defer coordination.Close()

	lease, _, err := acquireClassroomLeaseAt(directory, false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockClassroomForPending(coordination, false, 200*time.Millisecond); err == nil {
		t.Fatal("a client update wrote its record while a classroom request was being sent")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockClassroomForPending(coordination, false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// While the record is written, classroom controls wait.
	if _, _, err := acquireClassroomLeaseAt(directory, false, 100*time.Millisecond); err == nil {
		t.Fatal("a classroom control ran while a client update wrote its record")
	}
	unlock()

	// A controller without the classroom lock yet writes the record as before.
	if err := os.Remove(filepath.Join(directory, classroomLockName)); err != nil {
		t.Fatal(err)
	}
	unlock, err = lockClassroomForPending(coordination, false, time.Second)
	if err != nil {
		t.Fatalf("missing classroom lock blocked the update: %v", err)
	}
	unlock()
	if _, _, err := acquireClassroomLeaseAt(directory, false, time.Second); err == nil || !strings.Contains(err.Error(), "classroom lock") {
		t.Fatalf("classroom control without its lock did not fail closed: %v", err)
	}
}
