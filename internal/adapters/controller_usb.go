package adapters

import (
	"errors"
	"fmt"

	"github.com/giovantenne/nixorium/internal/domain"
)

// CheckControllerUSBReservation is a read-only guard, run as admin by the
// privileged controller job before stopping the worker and again under the
// shared operation lock. It never releases or rewrites the USB reservation.
// It accepts a completed disk installation awaiting reboot or verification,
// and a reservation whose installation was never dispatched.
func CheckControllerUSBReservation() error {
	return checkControllerUSBReservationAt(managedCoordinationDirectory, "/var/lib/nixorium/remote-install", true)
}

func checkControllerUSBReservationAt(coordination, statePath string, managed bool) error {
	directory, err := openCoordinationDirectory(coordination, managed)
	if err != nil {
		return err
	}
	defer directory.Close()
	record, present, err := readRemoteReservationAt(directory)
	if err != nil || !present {
		return err
	}
	store, err := NewRemoteInstallStateStore(statePath)
	if err != nil {
		return err
	}
	session, err := store.Load(record.OperationID)
	if err != nil {
		return fmt.Errorf("inspect reserved USB installation: %w", err)
	}
	// A completed disk no longer depends on the controller, and a session
	// that never dispatched its installation has not touched the disk.
	if !domain.RemoteInstallDiskCompleted(session) && !domain.RemoteInstallIdleForControllerRebuild(session) {
		return errors.New("USB disk installation is in progress, awaiting its confirmation or unresolved; controller activation requires a completed installation receipt or an installation that never started")
	}
	return nil
}
