package main

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestUntouchedSinceTheWake(t *testing.T) {
	woke := time.Unix(1000, 0)
	now := woke.Add(40 * time.Second)
	if !untouchedSince(30*time.Minute, woke, now) {
		t.Fatal("a session idle since before the wake was taken as used")
	}
	if untouchedSince(10*time.Second, woke, now) {
		t.Fatal("input after the wake was ignored")
	}
}

func TestSessionPathReadsLogindDisplay(t *testing.T) {
	if path, ok := sessionPath([]any{"2", dbus.ObjectPath("/org/freedesktop/login1/session/_32")}); !ok || path != "/org/freedesktop/login1/session/_32" {
		t.Fatalf("display = %q, %t", path, ok)
	}
	for _, value := range []any{nil, []any{"", dbus.ObjectPath("/")}, []any{"2"}, "session"} {
		if _, ok := sessionPath(value); ok {
			t.Fatalf("%v read as a graphical session", value)
		}
	}
}
