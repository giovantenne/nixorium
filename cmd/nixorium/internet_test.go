package main

import "testing"

func TestInternetArguments(t *testing.T) {
	for _, args := range [][]string{
		{"internet", "plan", "--on", "@lab", "--action", "block"},
		{"internet", "apply", "--on", "pc01", "--action", "unblock", "--expect", "sha256:test", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"internet"}, {"internet", "plan", "--on", "pc01"},
		{"internet", "apply", "--on", "pc01", "--action", "block", "--yes"},
		{"internet", "plan", "--on", "pc01", "--action", "other"},
		{"shutdown", "plan", "--on", "pc01", "--action", "block"},
		{"internet", "plan", "--on", "pc01", "--action", "block", "--yes"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}
