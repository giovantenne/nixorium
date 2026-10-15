package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func TestNetworkMigrateArguments(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		valid     bool
	}{
		{[]string{"network", "migrate", "plan", "--to", "10.1.0.0/24"}, true},
		{[]string{"network", "migrate", "plan", "--to", "10.1.0.0/24", "--acknowledge-unreachable", "--json"}, true},
		{[]string{"network", "migrate", "start", "--to", "10.1.0.0/24", "--expect", "sha256:review", "--yes"}, true},
		{[]string{"network", "migrate", "status", "--json", "--repo", "/srv/lab"}, true},
		{[]string{"network", "migrate", "finish"}, true},
		{[]string{"network", "migrate", "finish", "--expect", "sha256:review", "--yes", "--json"}, true},
		{[]string{"network", "migrate", "undo", "--expect", "sha256:review"}, true},
		{[]string{"network"}, false},
		{[]string{"network", "migrate"}, false},
		{[]string{"network", "change", "plan"}, false},
		{[]string{"network", "migrate", "apply"}, false},
		{[]string{"network", "migrate", "plan"}, false},
		{[]string{"network", "migrate", "plan", "--to", "10.1.0.5/24"}, false},
		{[]string{"network", "migrate", "plan", "--to", "10.1.0.0"}, false},
		{[]string{"network", "migrate", "plan", "--to", "10.1.0.0/24", "--expect", "x"}, false},
		{[]string{"network", "migrate", "start", "--to", "10.1.0.0/24"}, false},
		{[]string{"network", "migrate", "status", "--yes"}, false},
		{[]string{"network", "migrate", "finish", "--to", "10.1.0.0/24"}, false},
		{[]string{"network", "migrate", "undo", "--acknowledge-unreachable"}, false},
		{[]string{"network", "migrate", "finish", "--yes"}, false},
		{[]string{"network", "migrate", "status", "extra"}, false},
	} {
		parsed, err := parseArguments(test.arguments)
		if (err == nil) != test.valid {
			t.Fatalf("%q: valid=%t, err=%v", test.arguments, test.valid, err)
		}
		if test.valid && (parsed.command != "network" || parsed.subcommand != test.arguments[2]) {
			t.Fatalf("%q parsed as %+v", test.arguments, parsed)
		}
	}
	parsed, _ := parseArguments([]string{"network", "migrate", "start", "--to", "10.1.0.0/24", "--expect", "sha256:x", "--acknowledge-unreachable"})
	if parsed.network != "10.1.0.0/24" || parsed.expect != "sha256:x" || !parsed.acknowledgeUnreachable {
		t.Fatalf("parsed = %+v", parsed)
	}
	if command := networkMigrationApplyCommand(parsed); command != "nixorium network migrate start --to 10.1.0.0/24 --acknowledge-unreachable" {
		t.Fatalf("command = %q", command)
	}
}

func TestNetworkMigrateUsageAndRepositoryRequirement(t *testing.T) {
	var output bytes.Buffer
	usage(&output)
	for _, line := range []string{
		"network migrate plan --to <network/prefix>",
		"network migrate start --to <network/prefix> --expect <review-token>",
		"network migrate status [--json]",
		"network migrate finish|undo [--expect <review-token> [--yes]]",
	} {
		if !strings.Contains(output.String(), line) {
			t.Fatalf("usage lacks %q", line)
		}
	}
	parsed, _ := parseArguments([]string{"network", "migrate", "status"})
	if !commandRequiresRepository(parsed) {
		t.Fatal("network migrate must require the deployment repository")
	}
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"network", "migrate", "plan", "--to", "10.1.0.0/33"}, &bytes.Buffer{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "prefix length") {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
}

func TestNetworkMigrationTextNamesEveryAddressAndStep(t *testing.T) {
	plan := domain.NetworkMigrationPlan{Action: domain.NetworkMigrationStart, State: "ready", From: "10.0.0.0/24", To: "10.1.0.0/24", ControllerFrom: "10.0.0.99", ControllerTo: "10.1.0.99",
		Computers: []domain.NetworkMigrationComputer{{Name: "pc01", From: "10.0.0.1", To: "10.1.0.1", State: domain.MigrationComputerReachable},
			{Name: "pc02", From: "10.0.0.2", To: "10.1.0.2", State: domain.MigrationComputerAcknowledged, Detail: "moves when it is next updated; it is queued if still off"}},
		Steps: []string{"Save.", "Apply to controller."}, ReviewToken: "sha256:token", Message: "Ready."}
	var output bytes.Buffer
	presentation.NetworkMigrationPlanText(&output, plan)
	for _, text := range []string{"Start the change of laboratory network: READY", "Laboratory network:  10.0.0.0/24 → 10.1.0.0/24", "Controller address:  10.0.0.99 → 10.1.0.99",
		"pc02   10.0.0.2        → 10.1.0.2        off; continues without it: moves when it is next updated", "  2. Apply to controller.", "Review token: sha256:token"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("plan text lacks %q:\n%s", text, output.String())
		}
	}
	approved, err := presentation.ConfirmNetworkMigration(strings.NewReader("\n"), &bytes.Buffer{}, plan)
	if err != nil || !approved {
		t.Fatalf("Enter did not confirm: %t, %v", approved, err)
	}
	if approved, _ := presentation.ConfirmNetworkMigration(strings.NewReader("n\n"), &bytes.Buffer{}, plan); approved {
		t.Fatal("n confirmed")
	}
	plan.State = "blocked"
	if approved, _ := presentation.ConfirmNetworkMigration(strings.NewReader("\n"), &bytes.Buffer{}, plan); approved {
		t.Fatal("a blocked review was confirmed")
	}
}
