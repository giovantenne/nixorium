package main

import (
	"strings"
	"testing"
)

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

func TestDeployQueueArguments(t *testing.T) {
	for args, want := range map[string]string{
		"deploy queue status":           "queue-status",
		"deploy queue run --json":       "queue-run",
		"deploy queue cancel --on pc01": "queue-cancel",
		"deploy queue cancel --on @all": "queue-cancel",
	} {
		options, err := parseArguments(strings.Fields(args))
		if err != nil || options.subcommand != want {
			t.Fatalf("%s = %+v, %v", args, options.subcommand, err)
		}
	}
	options, err := parseArguments(strings.Fields("deploy apply --on @lab --expect " + strings.Repeat("a", 40) + " --queue-unreachable"))
	if err != nil || !options.queueUnreachable {
		t.Fatalf("queue flag = %+v, %v", options, err)
	}
	for _, args := range []string{
		"deploy queue", "deploy queue cancel", "deploy queue status --on pc01",
		"deploy plan --on @lab --queue-unreachable", "shutdown queue status", "deploy queue reboot",
	} {
		if _, err := parseArguments(strings.Fields(args)); err == nil {
			t.Fatal("accepted", args)
		}
	}
}
