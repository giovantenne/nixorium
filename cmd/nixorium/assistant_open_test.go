package main

import "testing"

func TestDesktopOpenerRequiresAGraphicalSession(t *testing.T) {
	if desktopOpenerFor(false) != nil || desktopOpenerFor(true) == nil {
		t.Fatal("opener availability does not follow the display")
	}
}
