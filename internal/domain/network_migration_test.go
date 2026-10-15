package domain

import (
	"strings"
	"testing"
	"time"
)

func TestParseLabNetwork(t *testing.T) {
	if base, prefix, err := ParseLabNetwork(" 10.1.0.0/24 "); err != nil || base != "10.1.0.0" || prefix != 24 {
		t.Fatalf("parsed %s/%d, %v", base, prefix, err)
	}
	for value, text := range map[string]string{
		"10.1.0.0":     "prefix length",
		"10.1.0.0/x":   "number",
		"10.1.0.5/24":  "for /24 use 10.1.0.0",
		"10.1.0.0/31":  "from 1 to 30",
		"fd00::/64":    "IPv4",
		"10.1.0.0/024": "",
	} {
		_, _, err := ParseLabNetwork(value)
		if text == "" {
			continue
		}
		if err == nil || !strings.Contains(err.Error(), text) {
			t.Fatalf("%s: %v", value, err)
		}
	}
}

func TestMigrationAddressesKeepHostNumbers(t *testing.T) {
	lab := validSettings().Lab
	lab.PCCount = 2
	controllerFrom, controllerTo, clients, err := MigrationAddresses(lab, "172.16.4.0", 22)
	if err != nil || controllerFrom != "10.0.0.99" || controllerTo != "172.16.4.99" || len(clients) != 2 ||
		clients[1] != (NetworkMigrationComputer{Name: "pc02", From: "10.0.0.2", To: "172.16.4.2"}) {
		t.Fatalf("%s → %s, %+v, %v", controllerFrom, controllerTo, clients, err)
	}
	if _, _, _, err := MigrationAddresses(lab, "10.1.0.0", 26); err == nil {
		t.Fatal("a network too small for the controller number was accepted")
	}
}

func TestNetworkMigrationCandidates(t *testing.T) {
	saved := validSettings()
	started := time.Date(2026, 10, 15, 8, 30, 15, 999, time.FixedZone("CEST", 7200))
	candidate, err := StartNetworkCandidate(saved, "10.1.0.0", 24, started)
	if err != nil || candidate.Lab.NetworkBase != "10.1.0.0" || *candidate.Lab.NetworkTransition != (NetworkTransition{PreviousBase: "10.0.0.0", PreviousPrefixLength: 24, StartedAt: "2026-10-15T06:30:15Z"}) {
		t.Fatalf("candidate = %+v, %v", candidate.Lab.NetworkTransition, err)
	}
	if issues := candidate.Validate(); len(issues) != 0 {
		t.Fatalf("start candidate issues = %+v", issues)
	}
	if issues := SettingsTransitionIssues(saved, candidate); len(issues) != 0 {
		t.Fatalf("start refused: %+v", issues)
	}
	if _, err := StartNetworkCandidate(candidate, "10.2.0.0", 24, started); err == nil {
		t.Fatal("a second change started during the first")
	}
	if _, err := StartNetworkCandidate(saved, "10.0.0.0", 24, started); err == nil {
		t.Fatal("moving to the same network was accepted")
	}

	finished, err := FinishNetworkCandidate(candidate)
	if err != nil || finished.Lab.NetworkTransition != nil || finished.Lab.NetworkBase != "10.1.0.0" {
		t.Fatalf("finish = %+v, %v", finished.Lab, err)
	}
	if issues := SettingsTransitionIssues(candidate, finished); len(issues) != 0 {
		t.Fatalf("finish refused: %+v", issues)
	}

	undone, err := UndoNetworkCandidate(candidate)
	if err != nil || undone.Lab.NetworkTransition != nil || undone.Lab.NetworkBase != "10.0.0.0" || undone.Lab.NetworkPrefix != 24 {
		t.Fatalf("undo = %+v, %v", undone.Lab, err)
	}
	if issues := SettingsTransitionIssues(candidate, undone); len(issues) != 0 {
		t.Fatalf("undo refused: %+v", issues)
	}
	if _, err := FinishNetworkCandidate(saved); err == nil {
		t.Fatal("finish without a change in progress")
	}
	if _, err := UndoNetworkCandidate(saved); err == nil {
		t.Fatal("undo without a change in progress")
	}
}

func TestOnlyTheUndoMayDropTheRecordWithANetworkChange(t *testing.T) {
	saved := migratedSettings()
	// Removing the record while moving to a third network is not an undo.
	third := saved
	third.Lab.NetworkBase, third.Lab.NetworkTransition = "10.2.0.0", nil
	issues := SettingsTransitionIssues(saved, third)
	if len(issues) == 0 || issues[0].Next == nil || issues[0].Next.Code != "NETWORK-LOCKED" {
		t.Fatalf("issues = %+v", issues)
	}
	// Without a saved record nothing can be "undone".
	plain := validSettings()
	moved := plain
	moved.Lab.NetworkBase = "10.1.0.0"
	if issues := SettingsTransitionIssues(plain, moved); len(issues) == 0 {
		t.Fatal("a plain network change was accepted")
	}
}

func TestNetworkChangingIsCatalogued(t *testing.T) {
	step, found := NextStepByCode("NETWORK-CHANGING")
	if !found || step.Screen != RecoveryNetworkChanging || step.TUI != "Maintenance → Change network addressing" || step.Command != "nixorium network migrate status" {
		t.Fatalf("step = %+v", step)
	}
	detail := NetworkChangingDetail(migratedSettings().Lab, false)
	if !strings.Contains(detail, "From 10.0.0.0/24 to 10.1.0.0/24") || !strings.Contains(detail, "Apply to controller") {
		t.Fatalf("detail = %q", detail)
	}
	if detail := NetworkChangingDetail(migratedSettings().Lab, true); !strings.Contains(detail, "update the computers") {
		t.Fatalf("detail = %q", detail)
	}
}

func TestInventoryNamesComputersStillOnThePreviousNetwork(t *testing.T) {
	host := HostStatus{Name: "pc02", IP: "10.1.0.2", PreviousIP: "10.0.0.2", Reachability: ReachabilityReachable, SSH: SSHAvailable, Network: HostNetworkPrevious, Deployment: DeploymentCurrent}
	level, label, guidance := ComputerCondition(host)
	if level != LevelWarning || label != "Still on the previous network" || !strings.Contains(guidance, "10.0.0.2") {
		t.Fatalf("%s %q %q", level, label, guidance)
	}
	host.Network = HostNetworkCurrent
	if _, label, _ := ComputerCondition(host); label != "Up to date" {
		t.Fatalf("moved computer = %q", label)
	}
}
