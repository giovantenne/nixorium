package main

import "testing"

func TestDesktopArguments(t *testing.T) {
	for _, args := range [][]string{
		{"desktop", "plan", "--on", "@lab"},
		{"desktop", "apply", "--on", "pc01", "--expect", "sha256:test", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"desktop"}, {"desktop", "plan"},
		{"desktop", "apply", "--on", "pc01", "--yes"},
		{"desktop", "plan", "--on", "pc01", "--action", "lock"},
		{"desktop", "plan", "--on", "pc01", "--yes"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}
