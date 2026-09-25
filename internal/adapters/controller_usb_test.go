package adapters

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestControllerUSBGuardPreservesPendingVerificationAndRejectsUnsafeStates(t *testing.T) {
	fixture, err := os.ReadFile("../../tests/usb-completed-session.json")
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*domain.RemoteInstallSession){
		"completed": func(*domain.RemoteInstallSession) {},
		"ready-before-reboot": func(s *domain.RemoteInstallSession) {
			s.State = "ready-to-reboot"
			s.RebootRequested = false
			s.DispatchUncertain = false
		},
		"active": func(s *domain.RemoteInstallSession) {
			s.State = "running"
			s.Receipt.State = "running"
			s.Receipt.Phase = domain.RemoteInstallPhaseInstall
			s.Receipt.Installed = false
		},
		"unknown":             func(s *domain.RemoteInstallSession) { s.Receipt = nil },
		"unconsumed":          func(s *domain.RemoteInstallSession) { s.TokenConsumed = false },
		"missing-plan":        func(s *domain.RemoteInstallSession) { s.Plan = domain.RemoteInstallPlan{} },
		"missing-preparation": func(s *domain.RemoteInstallSession) { s.Preparation = nil },
		"wrong-revision": func(s *domain.RemoteInstallSession) {
			s.Preparation.DeploymentRevision = "ffffffffffffffffffffffffffffffffffffffff"
		},
		"wrong-host":    func(s *domain.RemoteInstallSession) { s.Preparation.Host.StaticIP = "10.0.0.2" },
		"wrong-receipt": func(s *domain.RemoteInstallSession) { s.Receipt.OperationID = "ffffffffffffffffffffffffffffffff" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			session, err := domain.DecodeRemoteInstallSession(fixture)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&session)
			coordination, err := ensureTestCoordinationDirectory(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			reservation, err := createRemoteReservationAt(coordination, false, session.OperationID, "")
			if err != nil {
				t.Fatal(err)
			}
			defer reservation.ReleaseResolved()
			state := t.TempDir()
			if err := os.Chmod(state, 0700); err != nil {
				t.Fatal(err)
			}
			content, err := json.Marshal(session)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(state, session.OperationID+".json")
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(coordination, remoteReservationName)
			before, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			err = checkControllerUSBReservationAt(coordination, state, false)
			allowed := name == "completed" || name == "ready-before-reboot"
			if (err == nil) != allowed {
				t.Fatalf("guard error = %v", err)
			}
			after, err := os.ReadFile(marker)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("guard changed the reservation")
			}
			after, err = os.ReadFile(path)
			if err != nil || !bytes.Equal(content, after) {
				t.Fatal("guard changed the pending verification")
			}
			if _, err := acquireOperationGateAt(coordination, false); err == nil {
				t.Fatal("guard released the worker lock")
			}
		})
	}
}

func TestControllerUSBGuardRejectsUnsafeFiles(t *testing.T) {
	coordination, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := checkControllerUSBReservationAt(coordination, "/missing", false); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(coordination, remoteReservationName)
	if err := os.Symlink("/missing", marker); err != nil {
		t.Fatal(err)
	}
	if err := checkControllerUSBReservationAt(coordination, "/missing", false); err == nil {
		t.Fatal("unsafe marker accepted")
	}
}
