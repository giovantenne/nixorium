package adapters

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	deferredUpdatesName     = "deferred-updates.json"
	deferredUpdatesMaxBytes = 256 << 10
)

// DeferredUpdates stores the queue of client updates waiting for their
// computers in the controller's coordination directory. Changes are made only
// while holding the managed operation gate, so they never interleave with a
// deployment or another queue change; readers see whole files only.
type DeferredUpdates struct {
	directory string
	managed   bool
}

// ManagedDeferredUpdates is the controller's queue.
func ManagedDeferredUpdates() DeferredUpdates {
	return DeferredUpdates{directory: managedCoordinationDirectory, managed: true}
}

// Read returns the queue; a missing file is an empty queue.
func (store DeferredUpdates) Read() (domain.DeferredUpdateQueue, error) {
	directory, err := openCoordinationDirectory(store.directory, store.managed)
	if err != nil {
		return domain.DeferredUpdateQueue{}, err
	}
	defer directory.Close()
	return readDeferredUpdatesAt(directory)
}

// Mutate changes the queue under the operation gate. A busy gate is returned
// as an error and nothing changes.
func (store DeferredUpdates) Mutate(change func(*domain.DeferredUpdateQueue) error) error {
	gate, err := acquireOperationGateAt(store.directory, store.managed)
	if err != nil {
		return err
	}
	defer gate.Close()
	gate.describe("Queued client updates")
	directory, err := openCoordinationDirectory(store.directory, store.managed)
	if err != nil {
		return err
	}
	defer directory.Close()
	queue, err := readDeferredUpdatesAt(directory)
	if err != nil {
		return err
	}
	if err := change(&queue); err != nil {
		return err
	}
	if err := queue.Validate(); err != nil {
		return err
	}
	return writeDeferredUpdatesAt(directory, queue)
}

func readDeferredUpdatesAt(directory *os.File) (domain.DeferredUpdateQueue, error) {
	empty := domain.DeferredUpdateQueue{SchemaVersion: domain.DeferredUpdateSchemaVersion, Updates: []domain.DeferredUpdate{}}
	descriptor, err := syscall.Openat(int(directory.Fd()), deferredUpdatesName, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return empty, nil
	}
	if err != nil {
		return empty, fmt.Errorf("queued client updates are unreadable: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), deferredUpdatesName)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > deferredUpdatesMaxBytes {
		return empty, errors.New("queued client updates are not a private bounded regular file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Uid != uint32(os.Geteuid()) {
		return empty, errors.New("queued client updates belong to another user")
	}
	data, err := io.ReadAll(io.LimitReader(file, deferredUpdatesMaxBytes+1))
	if err != nil {
		return empty, err
	}
	var queue domain.DeferredUpdateQueue
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&queue); err != nil {
		return empty, fmt.Errorf("queued client updates are invalid: %w", err)
	}
	if queue.Updates == nil {
		queue.Updates = []domain.DeferredUpdate{}
	}
	if err := queue.Validate(); err != nil {
		return empty, fmt.Errorf("queued client updates are invalid: %w", err)
	}
	return queue, nil
}

// writeDeferredUpdatesAt replaces the file atomically; an empty queue
// removes it.
func writeDeferredUpdatesAt(directory *os.File, queue domain.DeferredUpdateQueue) error {
	if len(queue.Updates) == 0 {
		err := syscall.Unlinkat(int(directory.Fd()), deferredUpdatesName)
		if err != nil && !errors.Is(err, syscall.ENOENT) {
			return err
		}
		return directory.Sync()
	}
	content, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return err
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := ".deferred-updates." + hex.EncodeToString(suffix)
	descriptor, err := syscall.Openat(int(directory.Fd()), temporary, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), temporary)
	_, err = file.Write(append(content, '\n'))
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = syscall.Renameat(int(directory.Fd()), temporary, int(directory.Fd()), deferredUpdatesName)
	}
	if err != nil {
		_ = syscall.Unlinkat(int(directory.Fd()), temporary)
		return err
	}
	return directory.Sync()
}

// deferredUpdatesPausedName marks the queue as paused: an automatic update
// was stopped, and queued computers wait until the administrator resumes.
const deferredUpdatesPausedName = "deferred-updates-paused"

// Paused reports the pause marker. A marker that exists but cannot be
// inspected counts as present: nothing starts by itself on doubt. Without
// the coordination directory there is no queue to pause.
func (store DeferredUpdates) Paused() bool {
	directory, err := openCoordinationDirectory(store.directory, store.managed)
	if err != nil {
		return false
	}
	defer directory.Close()
	var stat unix.Stat_t
	err = unix.Fstatat(int(directory.Fd()), deferredUpdatesPausedName, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return !errors.Is(err, unix.ENOENT)
}

// SetPaused creates or removes the pause marker.
func (store DeferredUpdates) SetPaused(paused bool) error {
	directory, err := openCoordinationDirectory(store.directory, store.managed)
	if err != nil {
		return err
	}
	defer directory.Close()
	if !paused {
		if err := unix.Unlinkat(int(directory.Fd()), deferredUpdatesPausedName, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("resume queued updates: %w", err)
		}
		return directory.Sync()
	}
	descriptor, err := unix.Openat(int(directory.Fd()), deferredUpdatesPausedName, unix.O_WRONLY|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("pause queued updates: %w", err)
	}
	if err := unix.Close(descriptor); err != nil {
		return err
	}
	return directory.Sync()
}

// DeferredUpdatesPaused tells the Overview whether automatic updates of the
// controller's queue are paused.
func (Local) DeferredUpdatesPaused() bool { return ManagedDeferredUpdates().Paused() }
