package presentation

import (
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestUnescapeUdevShowsReadableDiskNames(t *testing.T) {
	for input, want := range map[string]string{
		`VBOX\x20HARDDISK`: "VBOX HARDDISK",
		`Samsung\x20SSD `:  "Samsung SSD",
		`plain`:            "plain",
		`bad\xZZ`:          `bad\xZZ`,
		`end\x2`:           `end\x2`,
	} {
		if got := unescapeUdev(input); got != want {
			t.Errorf("unescapeUdev(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestJoinNonEmptyLeavesNoDanglingSeparator(t *testing.T) {
	if got := joinNonEmpty(" / ", "VB123", ""); got != "VB123" {
		t.Errorf("serial only = %q", got)
	}
	if got := joinNonEmpty(" / ", "", "eui.1"); got != "eui.1" {
		t.Errorf("wwn only = %q", got)
	}
	if got := joinNonEmpty(" / ", "VB123", "eui.1"); got != "VB123 / eui.1" {
		t.Errorf("both = %q", got)
	}
}

func TestChangedHostKeyHasTheSameWordsInUpdateComputersAndInventory(t *testing.T) {
	host := domain.HostStatus{HostKeyCondition: domain.HostKeyChanged, Reachability: domain.ReachabilityReachable}
	_, inventory, _ := domain.ComputerCondition(host)
	if got := deploymentStateLabel(host, true); got != inventory || got != "SSH key changed" {
		t.Errorf("update computers = %q, inventory = %q", got, inventory)
	}
}
