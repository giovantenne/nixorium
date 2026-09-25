package adapters

import (
	"errors"
	"fmt"

	"github.com/giovantenne/nixorium/internal/domain"
)

// CheckControllerUSBReservation is a read-only guard, run as admin by the
// privileged controller job before stopping the worker and again under the
// shared operation lock. It never releases or rewrites the USB reservation.
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
	if !domain.RemoteInstallDiskCompleted(session) {
		return errors.New("USB disk installation is active or unresolved; controller activation requires a completed installation receipt")
	}
	return nil
}
