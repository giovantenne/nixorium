package presentation

import (
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestOutdatedPinMakesTheConfiguredReleaseSelectableForRepair(t *testing.T) {
	release := domain.UpdateRelease{Tag: "v3.8.0", ObjectID: strings.Repeat("1", 40), Channel: domain.UpdateChannelStable}
	report := domain.UpdateCheckReport{CurrentRef: "v3.8.0", CurrentRev: strings.Repeat("1", 40)}
	if !updateReleaseAlreadyCurrent(report, release) || updateReleaseStatus(report, release) != "  Current" {
		t.Fatal("consistent configured release must stay unselectable")
	}
	report.PinOutdated = "nixorium: flake.nix declares github:giovantenne/nixorium/v3.8.0 but flake.lock records github:giovantenne/nixorium/v3.3.1"
	if updateReleaseAlreadyCurrent(report, release) || !strings.Contains(updateReleaseStatus(report, release), "repair") {
		t.Fatalf("outdated pin: current=%t status=%q", updateReleaseAlreadyCurrent(report, release), updateReleaseStatus(report, release))
	}
}
