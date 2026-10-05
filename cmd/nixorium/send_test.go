package main

import "testing"

func TestSendArguments(t *testing.T) {
	for _, args := range [][]string{
		{"send", "plan", "--file", "/home/teacher/lesson", "--on", "@lab"},
		{"send", "apply", "--file", "notes.txt", "--on", "pc01", "--expect", "sha256:test", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"send"}, {"send", "plan"},
		{"send", "plan", "--on", "@lab"},
		{"send", "apply", "--file", "notes.txt", "--on", "pc01", "--yes"},
		{"send", "plan", "--file", "notes.txt", "--on", "pc01", "--action", "lock"},
		{"send", "plan", "--file", "notes.txt", "--on", "pc01", "--yes"},
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
