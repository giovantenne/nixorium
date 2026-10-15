package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWakeArguments(t *testing.T) {
	for _, args := range [][]string{
		{"wake", "--on", "@lab"},
		{"wake", "--on", "pc01,pc02", "--json"},
		{"--json", "wake", "--on", "pc03"},
	} {
		options, err := parseArguments(args)
		if err != nil || options.command != "wake" || options.on == "" {
			t.Fatal(args, options, err)
		}
	}
	// No review token, confirmation or subcommand: a packet changes no
	// system state.
	for _, args := range [][]string{
		{"wake"},
		{"wake", "--on", ""},
		{"wake", "plan", "--on", "pc01"},
		{"wake", "apply", "--on", "pc01"},
		{"wake", "--on", "pc01", "--expect", "sha256:test"},
		{"wake", "--on", "pc01", "--yes"},
		{"wake", "--on", "pc01", "--action", "lock"},
		{"wake", "lock", "--on", "pc01"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}

func TestWakeCommandPrintsOneLinePerComputer(t *testing.T) {
	plan := func(_ context.Context, requested string) domain.WakePlan {
		if requested == "pc01,pc99" {
			return domain.WakePlan{State: "blocked", Message: "Nothing was sent: pc99 is not a configured client.", Issues: []domain.ValidationIssue{{Field: "targets"}},
				Targets: []domain.WakeTarget{{HostMeta: domain.HostMeta{Name: "pc99"}, State: domain.WakeTargetInvalid, Detail: "Not a client computer of this laboratory."}}}
		}
		return domain.WakePlan{State: "ready", Targets: []domain.WakeTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, State: domain.WakeTargetSend}}}
	}
	applied := 0
	apply := func(_ context.Context, reviewed domain.WakePlan) domain.WakeReport {
		applied++
		return domain.WakeReport{State: "completed", Sent: 1, Targets: []domain.WakeOutcome{{Name: "pc01", State: domain.WakeOutcomeSent}}, Message: "Wake-on-LAN packets sent to 1 computer."}
	}
	var output bytes.Buffer
	if code := wakeCommand(context.Background(), options{on: "pc01"}, plan, apply, &output); code != 0 || output.String() != "pc01  sent\nWake-on-LAN packets sent to 1 computer.\n" {
		t.Fatalf("code %d, output:\n%s", code, output.String())
	}
	output.Reset()
	if code := wakeCommand(context.Background(), options{on: "pc01,pc99"}, plan, apply, &output); code != 1 || applied != 1 || !strings.HasPrefix(output.String(), "pc99  invalid target") {
		t.Fatalf("code %d, applied %d, output:\n%s", code, applied, output.String())
	}
	output.Reset()
	if code := wakeCommand(context.Background(), options{on: "pc01", json: true}, plan, apply, &output); code != 0 || !strings.Contains(output.String(), `"sent"`) {
		t.Fatalf("code %d, output:\n%s", code, output.String())
	}
}
