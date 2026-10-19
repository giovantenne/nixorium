package adapters

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Classroom controls (Internet access, power) run beside maintenance. They
// never wait for the operation gate: they refuse only the computers named in
// an unfinished client update. classroom.lock orders them against the one
// moment that matters, the creation of deployment-pending.json: classroom
// controls hold it shared while they check and send their requests, and a
// client update holds it exclusively while it writes the record. A record
// therefore never appears between a classroom check and its request.

const (
	classroomLockName = "classroom.lock"
	// classroomLeaseWait bounds how long a classroom control waits for a
	// client update that is writing its record (a few milliseconds).
	classroomLeaseWait = 10 * time.Second
	// deploymentPendingLockWait bounds how long a client update waits for
	// classroom requests in flight before it writes its record.
	deploymentPendingLockWait = 60 * time.Second
)

type classroomLease struct {
	file *os.File
}

func (lease *classroomLease) Close() error {
	if lease == nil || lease.file == nil {
		return nil
	}
	_ = syscall.Flock(int(lease.file.Fd()), syscall.LOCK_UN)
	err := lease.file.Close()
	lease.file = nil
	return err
}

// ClassroomBlockedHosts names the computers a classroom control must leave
// alone now, without taking any lock. It is used while reviewing.
func (Local) ClassroomBlockedHosts() (map[string]string, error) {
	return classroomBlockedHostsAt(managedCoordinationDirectory, true)
}

// AcquireClassroomLease holds the shared classroom lock for the duration of
// one classroom control and returns the computers it must not touch.
func (Local) AcquireClassroomLease(_ string) (io.Closer, map[string]string, error) {
	lease, blocked, err := acquireClassroomLeaseAt(managedCoordinationDirectory, true, classroomLeaseWait)
	if err != nil {
		return nil, nil, err
	}
	return lease, blocked, nil
}

func classroomBlockedHostsAt(directoryPath string, managed bool) (map[string]string, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	// A USB installation is attended by the administrator and binds a
	// computer that is not verified yet: classroom controls wait for it.
	if present, markerErr := inspectReservationAt(directory); present {
		if markerErr != nil {
			return nil, domain.Blocked("USB-RESERVED", fmt.Errorf("USB installation reservation blocks classroom controls: %w", markerErr))
		}
		return nil, domain.Blocked("USB-RESERVED", errors.New(domain.MessageUSBReserved+"; reconcile or close that operation first"))
	}
	pending, present, err := pendingDeploymentAt(directoryPath)
	if !present {
		return map[string]string{}, nil
	}
	if err != nil {
		// An unreadable record cannot say which computers it names.
		return nil, domain.Blocked("DEPLOY-PENDING", fmt.Errorf("%s: %w", domain.MessageDeploymentPending, err))
	}
	blocked := map[string]string{}
	for _, target := range pending.Targets {
		blocked[target.Name] = domain.ClassroomBusyReason
	}
	return blocked, nil
}

func acquireClassroomLeaseAt(directoryPath string, managed bool, wait time.Duration) (*classroomLease, map[string]string, error) {
	lock, err := openClassroomLock(directoryPath, managed)
	if err != nil {
		return nil, nil, err
	}
	if err := flockWithin(lock, syscall.LOCK_SH, wait); err != nil {
		lock.Close()
		return nil, nil, fmt.Errorf("a client update is starting; try again in a moment: %w", err)
	}
	lease := &classroomLease{file: lock}
	blocked, err := classroomBlockedHostsAt(directoryPath, managed)
	if err != nil {
		_ = lease.Close()
		return nil, nil, err
	}
	return lease, blocked, nil
}

func openClassroomLock(directoryPath string, managed bool) (*os.File, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), classroomLockName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open classroom lock: %w; activate the Nixorium controller configuration", err)
	}
	lock := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, classroomLockName))
	if err := validateCoordinationLock(lock, managed); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

// flockWithin takes a lock without blocking forever: it retries a
// non-blocking attempt until the deadline.
func flockWithin(lock *os.File, how int, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(lock.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		if !time.Now().Before(deadline) {
			return errors.New("timed out waiting for the classroom lock")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// lockClassroomForPending holds the classroom lock exclusively while a client
// update writes its pending record. A controller whose configuration has no
// classroom lock yet has no classroom control that could race: the record is
// written without it.
func lockClassroomForPending(directory *os.File, managed bool, wait time.Duration) (func(), error) {
	lock, err := openClassroomLock(directory.Name(), managed)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return func() {}, nil
		}
		return nil, err
	}
	if err := flockWithin(lock, syscall.LOCK_EX, wait); err != nil {
		lock.Close()
		return nil, fmt.Errorf("classroom controls are sending requests; try the update again in a minute: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}, nil
}
