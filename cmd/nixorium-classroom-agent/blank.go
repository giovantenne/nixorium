package main

import (
	"time"

	"github.com/godbus/dbus/v5"
)

// GNOME blanks an idle screen after org.gnome.desktop.session idle-delay
// without locking it (Nixorium disables the lock), and refuses screen capture
// while the screen is blank. The classroom view wakes such a screen for the
// teacher, and blanks it again when the teacher stops watching if nobody used
// the computer meanwhile, so screens do not stay on all night.
const (
	screenSaverName = "org.gnome.ScreenSaver"
	idleMonitorName = "org.gnome.Mutter.IdleMonitor"
	wakeWait        = 2 * time.Second
)

// wakeScreen lifts a blank, unlocked screen and reports whether it did. A
// screen locked with a password stays locked: GNOME does not unlock it here.
func wakeScreen(connection *dbus.Conn) bool {
	saver := connection.Object(screenSaverName, "/org/gnome/ScreenSaver")
	var active bool
	if saver.Call(screenSaverName+".GetActive", 0).Store(&active) != nil || !active {
		return false
	}
	if saver.Call(screenSaverName+".SetActive", 0, false).Err != nil {
		return false
	}
	for deadline := time.Now().Add(wakeWait); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if saver.Call(screenSaverName+".GetActive", 0).Store(&active) == nil && !active {
			return true
		}
	}
	return false
}

// untouchedSince reports whether the session had no input since the wake:
// Mutter's idle time then covers the whole time since the wake.
func untouchedSince(idle time.Duration, wokeAt, now time.Time) bool {
	return idle+time.Second >= now.Sub(wokeAt)
}

// blankIfUntouched blanks the screen again when nobody used the computer
// since the classroom view woke it.
func blankIfUntouched(wokeAt time.Time) {
	connection, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer connection.Close()
	var idle uint64
	if connection.Object(idleMonitorName, "/org/gnome/Mutter/IdleMonitor/Core").Call(idleMonitorName+".GetIdletime", 0).Store(&idle) != nil {
		return
	}
	if untouchedSince(time.Duration(idle)*time.Millisecond, wokeAt, time.Now()) {
		_ = connection.Object(screenSaverName, "/org/gnome/ScreenSaver").Call(screenSaverName+".SetActive", 0, true).Err
	}
}
