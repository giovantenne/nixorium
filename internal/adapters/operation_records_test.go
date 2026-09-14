package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestOperationRecordsArePrivateOrderedAndCollisionSafe(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	now := time.Date(2026, 9, 14, 18, 30, 0, 123, time.UTC)
	record := domain.OperationRecord{Operation: "pxe-start", State: "completed", Subject: "active", Summary: "PXE lifecycle transition finished"}
	if err := recordOperation(stateRoot, now, 42, record); err != nil {
		t.Fatal(err)
	}
	record.Operation = "service-restart"
	if err := recordOperation(stateRoot, now, 42, record); err != nil {
		t.Fatal(err)
	}
	records, err := (Local{}).OperationRecords(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Operation != "service-restart" || records[0].ID == records[1].ID || records[1].Operation != "pxe-start" {
		t.Fatalf("records = %+v", records)
	}
	path := filepath.Join(stateRoot, "nixorium", "operations", "records.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("record mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestOperationRecordRejectsUnsafeText(t *testing.T) {
	stateRoot := t.TempDir()
	for _, record := range []domain.OperationRecord{
		{Operation: "UPPER", State: "completed", Summary: "summary"},
		{Operation: "pxe-start", State: "completed", Summary: "line one\nline two"},
		{Operation: "pxe-start", State: "completed", Summary: strings.Repeat("x", maximumOperationFieldBytes+1)},
	} {
		if err := recordOperation(stateRoot, time.Now(), 1, record); err == nil {
			t.Fatalf("unsafe record was accepted: %+v", record)
		}
	}
}

func TestOperationRecordCorruptionAndSymlinkFailClosed(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(string, string) error
	}{
		{name: "corrupt", prepare: func(_ string, path string) error { return os.WriteFile(path, []byte("{broken"), 0600) }},
		{name: "symlink", prepare: func(directory, path string) error {
			target := filepath.Join(directory, "target")
			if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
				return err
			}
			return os.Symlink(target, path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := t.TempDir()
			directory := filepath.Join(stateRoot, "nixorium", "operations")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "records.json")
			if err := test.prepare(directory, path); err != nil {
				t.Fatal(err)
			}
			record := domain.OperationRecord{Operation: "pxe-stop", State: "completed", Summary: "PXE lifecycle transition finished"}
			if err := recordOperation(stateRoot, time.Now(), 1, record); err == nil {
				t.Fatal("unsafe operation record store was overwritten")
			}
		})
	}
}

func TestOperationRecordConcurrentWritersAreSerialized(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	const writers = 16
	errorsFound := make(chan error, writers)
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			errorsFound <- (Local{}).RecordOperation(domain.OperationRecord{Operation: "deploy-apply", State: "completed", Subject: "pc01", Summary: "phase complete; verified 1/1 target(s)"})
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	records, err := (Local{}).OperationRecords(50)
	if err != nil || len(records) != writers {
		t.Fatalf("records = %d, error = %v", len(records), err)
	}
}

func TestOperationRecordRetentionKeepsNewestBound(t *testing.T) {
	records := make([]domain.OperationRecord, maximumOperationRecords+2)
	for index := range records {
		records[index].ID = string(rune(index))
	}
	retained := retainNewestOperationRecords(records, maximumOperationRecords)
	if len(retained) != maximumOperationRecords || retained[0].ID != records[2].ID || retained[len(retained)-1].ID != records[len(records)-1].ID {
		t.Fatal("retention did not preserve the newest bounded records")
	}
}
