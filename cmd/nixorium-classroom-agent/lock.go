package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// The classroom extension in the same session covers the screen while the
// computer is locked; the agent only asks for it and reads its state.
const (
	lockBusName = "org.nixorium.Classroom"
	lockPath    = dbus.ObjectPath("/org/nixorium/Classroom")
	lockTimeout = 3 * time.Second
)

var errLockUnavailable = errors.New("the classroom extension is not running in this session")

// locker locks the session and reports whether it is locked.
type locker interface {
	SetLocked(locked bool) error
	Locked() bool
}

type extensionLocker struct {
	mutex      sync.Mutex
	connection *dbus.Conn
}

func (locker *extensionLocker) object() (dbus.BusObject, error) {
	locker.mutex.Lock()
	defer locker.mutex.Unlock()
	if locker.connection == nil || !locker.connection.Connected() {
		connection, err := dbus.ConnectSessionBus()
		if err != nil {
			return nil, err
		}
		locker.connection = connection
	}
	return locker.connection.Object(lockBusName, lockPath), nil
}

func (locker *extensionLocker) SetLocked(locked bool) error {
	object, err := locker.object()
	if err != nil {
		return errLockUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	if err := object.CallWithContext(ctx, lockBusName+".SetLocked", 0, locked).Err; err != nil {
		return errLockUnavailable
	}
	return nil
}

// Locked is false when the extension does not answer: nothing covers the
// screen then.
func (locker *extensionLocker) Locked() bool {
	object, err := locker.object()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	var value dbus.Variant
	if err := object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, lockBusName, "Locked").Store(&value); err != nil {
		return false
	}
	locked, _ := value.Value().(bool)
	return locked
}
