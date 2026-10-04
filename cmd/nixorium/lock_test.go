package main

import "testing"

func TestLockArguments(t *testing.T) {
	for _, args := range [][]string{
		{"lock", "plan", "--on", "@lab", "--action", "lock"},
		{"lock", "apply", "--on", "pc01", "--action", "unlock", "--expect", "sha256:test", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"lock"}, {"lock", "plan", "--on", "pc01"},
		{"lock", "apply", "--on", "pc01", "--action", "lock", "--yes"},
		{"lock", "plan", "--on", "pc01", "--action", "block"},
		{"internet", "plan", "--on", "pc01", "--action", "lock"},
		{"lock", "plan", "--on", "pc01", "--action", "lock", "--yes"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}
