package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBusyGateNamesItsHolder(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := acquireOperationGateAt(directory, false)
	if err != nil {
		t.Fatal(err)
	}
	first.describe("Update computers")
	_, err = acquireOperationGateAt(directory, false)
	var busy *OperationBusyError
	if !errors.As(err, &busy) || !busy.Known || busy.Operation != "Update computers" {
		t.Fatalf("busy error = %#v", err)
	}
	if !strings.HasPrefix(err.Error(), OperationBusyMessage+": Update computers, started") {
		t.Fatalf("message = %q", err.Error())
	}
	active, activeErr := operationActiveAt(directory, false)
	if !active || !errors.As(activeErr, &busy) || !busy.Known {
		t.Fatalf("active = %v, %v", active, activeErr)
	}
	first.Close()
	content, err := os.ReadFile(filepath.Join(directory, coordinationLockName))
	if err != nil || len(content) != 0 {
		t.Fatalf("owner not cleared after release: %q %v", content, err)
	}
}

func TestBusyGateIgnoresUnconfirmedOwners(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	for _, content := range []string{
		"",
		"not json",
		`{"operation":"Update computers","user":"admin","pid":1,"pidStart":"0","startedAt":"2026-10-01T10:00:00Z"}`,
		`{"operation":"Update computers","user":"admin","pid":999999999,"pidStart":"1","startedAt":"2026-10-01T10:00:00Z"}`,
	} {
		if err := os.WriteFile(lockPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		lock, err := os.Open(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		busy := operationBusy(lock)
		lock.Close()
		if busy.Known || !strings.Contains(busy.Error(), "open its progress") {
			t.Fatalf("unconfirmed owner %q accepted: %+v", content, busy)
		}
	}
}

func TestShellUnitOwnerUsesFriendlyLabel(t *testing.T) {
	start, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(t.TempDir(), "lock")
	content := `{"operation":"nixorium-apply-controller","user":"root","pid":` + itoa(os.Getpid()) + `,"pidStart":"` + start + `","startedAt":"2026-10-01T10:02:00Z"}`
	if err := os.WriteFile(lockPath, []byte(content+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lock, _ := os.Open(lockPath)
	defer lock.Close()
	busy := operationBusy(lock)
	if !busy.Known || busy.Operation != "Apply to controller" || strings.Contains(busy.Holder(), "root") {
		t.Fatalf("busy = %+v holder %q", busy, busy.Holder())
	}
}

func itoa(value int) string { return strconv.Itoa(value) }
