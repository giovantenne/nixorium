package main

import "testing"

func TestPackageBaseArguments(t *testing.T) {
	for _, args := range [][]string{
		{"package-base", "status"}, {"package-base", "plan"},
		{"package-base", "plan", "--target", "nixos-26.11", "--allow-unverified"},
		{"package-base", "apply", "--expect", "sha256:reviewed", "--yes", "--json"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"package-base"}, {"package-base", "apply"}, {"package-base", "check"},
		{"package-base", "status", "--allow-unverified"}, {"package-base", "plan", "--allow-downgrade"},
		{"update", "plan", "--target", "v2.1.0", "--allow-unverified"},
		{"package-base", "plan", "--yes"}, {"package-base", "status", "--target", "nixos-26.11"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
