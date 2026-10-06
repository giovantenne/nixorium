package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func testDeferredStore(t *testing.T) DeferredUpdates {
	t.Helper()
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return DeferredUpdates{directory: directory}
}

func TestDeferredUpdatesRoundTripAndRemoveWhenEmpty(t *testing.T) {
	store := testDeferredStore(t)
	queue, err := store.Read()
	if err != nil || len(queue.Updates) != 0 {
		t.Fatalf("empty queue = %+v, %v", queue, err)
	}
	update := domain.DeferredUpdate{Host: "pc01", IP: "10.0.0.1", Revision: strings.Repeat("a", 40), QueuedAt: time.Now().UTC()}
	if err := store.Mutate(func(queue *domain.DeferredUpdateQueue) error { queue.Put(update); return nil }); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.directory, deferredUpdatesName)
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("queue file = %v, %v", info, err)
	}
	queue, err = store.Read()
	if err != nil || len(queue.Updates) != 1 || queue.Updates[0].Host != "pc01" {
		t.Fatalf("read back = %+v, %v", queue, err)
	}
	if err := store.Mutate(func(queue *domain.DeferredUpdateQueue) error { queue.Remove("pc01"); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty queue left a file: %v", err)
	}
}

func TestDeferredUpdatesRefuseInvalidChangesAndFiles(t *testing.T) {
	store := testDeferredStore(t)
	bad := domain.DeferredUpdate{Host: "../x", IP: "10.0.0.1", Revision: strings.Repeat("a", 40), QueuedAt: time.Now()}
	if err := store.Mutate(func(queue *domain.DeferredUpdateQueue) error { queue.Put(bad); return nil }); err == nil {
		t.Fatal("invalid entry stored")
	}
	path := filepath.Join(store.directory, deferredUpdatesName)
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"updates":[],"extra":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); err == nil {
		t.Fatal("readable-by-others queue accepted")
	}
}

func TestDeferredUpdatesWaitForTheOperationGate(t *testing.T) {
	store := testDeferredStore(t)
	gate, err := acquireOperationGateAt(store.directory, false)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	changed := false
	if err := store.Mutate(func(*domain.DeferredUpdateQueue) error { changed = true; return nil }); err == nil || changed {
		t.Fatal("queue changed while another operation held the gate")
	}
}
