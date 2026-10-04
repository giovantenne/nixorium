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

func TestClassroomViewArguments(t *testing.T) {
	if options, err := parseArguments([]string{"classroom-view"}); err != nil || options.command != "classroom-view" {
		t.Fatalf("classroom-view = %+v, %v", options, err)
	}
	for _, args := range [][]string{{"classroom-view", "plan"}, {"classroom-view", "--yes"}, {"classroom-view", "--on", "pc01"}} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}
