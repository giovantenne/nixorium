package main

import "testing"

func TestCleanupArguments(t *testing.T) {
	for _, args := range [][]string{
		{"cleanup", "plan", "--on", "controller,@lab"},
		{"cleanup", "apply", "--on", "pc01", "--expect", "sha256:test", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"cleanup"}, {"cleanup", "plan"},
		{"cleanup", "apply", "--on", "pc01", "--yes"},
		{"cleanup", "plan", "--on", "pc01", "--yes"},
		{"cleanup", "plan", "--on", "pc01", "--action", "block"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}
