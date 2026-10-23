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

// wakeScreen lifts a blank, unlocked screen and reports whether it did.
// GNOME's SetActive(false) also lifts a lock screen that asks for a password,
// for any program of the session, so a session that logind marks as locked
// (or whose state cannot be read) is never woken: it stays locked and the
// classroom view reports it as locked.
func wakeScreen(connection *dbus.Conn) bool {
	if sessionLocked() {
		return false
	}
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

// sessionLocked reports whether logind marks this user's graphical session as
// locked: GNOME sets LockedHint while its lock screen asks for a password. It
// fails closed: an unreadable state counts as locked.
func sessionLocked() bool {
	system, err := dbus.ConnectSystemBus()
	if err != nil {
		return true
	}
	defer system.Close()
	display, err := system.Object(login1Name, "/org/freedesktop/login1/user/self").GetProperty(login1Name + ".User.Display")
	if err != nil {
		return true
	}
	session, ok := sessionPath(display.Value())
	if !ok {
		return true
	}
	hint, err := system.Object(login1Name, session).GetProperty(login1Name + ".Session.LockedHint")
	if err != nil {
		return true
	}
	locked, ok := hint.Value().(bool)
	return !ok || locked
}

const login1Name = "org.freedesktop.login1"

// sessionPath reads logind's User.Display property, a (session id, object
// path) pair; an empty path means the user has no graphical session.
func sessionPath(value any) (dbus.ObjectPath, bool) {
	pair, ok := value.([]any)
	if !ok || len(pair) != 2 {
		return "", false
	}
	path, ok := pair[1].(dbus.ObjectPath)
	return path, ok && path.IsValid() && path != "/"
}
