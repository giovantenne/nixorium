package main

import (
	"testing"
	"time"
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
