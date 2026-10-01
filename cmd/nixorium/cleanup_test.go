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

func TestRecoveryArguments(t *testing.T) {
	for _, args := range [][]string{
		{"recovery", "status"}, {"recovery", "status", "--json"},
		{"deploy", "recover", "plan"}, {"deploy", "recover", "plan", "--acknowledge-unreachable"},
		{"deploy", "recover", "apply", "--expect", "sha256:x", "--yes"},
		{"template-reset", "recover", "plan", "--json"},
		{"template-reset", "recover", "apply", "--expect", "sha256:x"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"recovery", "apply"}, {"deploy", "recover"}, {"deploy", "recover", "apply"},
		{"deploy", "recover", "plan", "--on", "pc01"}, {"template-reset"},
		{"template-reset", "recover", "plan", "--acknowledge-unreachable"},
		{"deploy", "plan", "--on", "pc01", "--acknowledge-unreachable"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestGitDiscardArguments(t *testing.T) {
	for _, args := range [][]string{
		{"git", "discard", "plan", "--paths", "lab-settings.json"},
		{"git", "discard", "apply", "--paths", "lab-settings.json", "--expect", "sha256:x", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"git", "discard"}, {"git", "discard", "plan"},
		{"git", "discard", "apply", "--paths", "x"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("accepted", args)
		}
	}
}
