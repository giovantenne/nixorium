package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDeferredUpdatesPauseMarker(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := DeferredUpdates{directory: directory}
	if store.Paused() {
		t.Fatal("a new queue is paused")
	}
	for range 2 {
		if err := store.SetPaused(true); err != nil {
			t.Fatal(err)
		}
		if !store.Paused() {
			t.Fatal("pause not recorded")
		}
	}
	for range 2 {
		if err := store.SetPaused(false); err != nil {
			t.Fatal(err)
		}
		if store.Paused() {
			t.Fatal("pause not cleared")
		}
	}
}

// Closing without checking archives even an unreadable record, but never
// while an operation holds the lock.
func TestUncheckedArchiveMovesAnyRecordUnderTheLock(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archivePendingDeploymentUncheckedAt(directory, false); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("archive without a record = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	// An update holding the lock, as while it runs.
	holder, err := os.OpenFile(filepath.Join(directory, coordinationLockName), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	var busy *OperationBusyError
	if _, err := archivePendingDeploymentUncheckedAt(directory, false); !errors.As(err, &busy) {
		t.Fatalf("archive while the lock is held = %v", err)
	}
	holder.Close()
	archive, err := archivePendingDeploymentUncheckedAt(directory, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(archive, "-unchecked.json") {
		t.Fatalf("archive = %q", archive)
	}
	if _, err := os.Lstat(filepath.Join(directory, deploymentPendingName)); !os.IsNotExist(err) {
		t.Fatalf("record still present: %v", err)
	}
	if content, err := os.ReadFile(archive); err != nil || string(content) != "{" {
		t.Fatalf("archived content %q, %v", content, err)
	}
}
