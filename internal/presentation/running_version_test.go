package presentation

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRunningVersionLabel(t *testing.T) {
	cases := []struct {
		version string
		build   RunningBuild
		want    string
		moving  bool
	}{
		{"3.14.0", RunningBuild{}, "3.14.0", false},
		{"development", RunningBuild{}, "development", true},
		{"", RunningBuild{}, "unknown", false},
		{"3.14.0", RunningBuild{Channel: "master", Revision: "06ae3959d1c0ffee", Date: "20261114"}, "master · 06ae395 · 14 Nov 2026", true},
		{"3.14.0", RunningBuild{Channel: "commit", Revision: "06ae395", Date: "bad"}, "commit · 06ae395", true},
	}
	for _, c := range cases {
		got, moving := runningVersionLabel(c.version, c.build)
		if got != c.want || moving != c.moving {
			t.Errorf("runningVersionLabel(%q, %+v) = %q, %v; want %q, %v", c.version, c.build, got, moving, c.want, c.moving)
		}
	}
}

func TestHeaderShowsVersionAtTheRightWhenItFits(t *testing.T) {
	shell := tuiShell{path: []string{"Overview"}, version: "master · 06ae395 · 14 Nov 2026", versionMoving: true}
	header := buildTUIShellRegions(shell, 106, true).header
	if !strings.Contains(header, "06ae395") {
		t.Fatalf("version missing from header: %q", header)
	}
	if got := lipgloss.Width(header); got != tuiContentWidth(106) {
		t.Fatalf("header width = %d, want the content width %d", got, tuiContentWidth(106))
	}
	narrow := buildTUIShellRegions(shell, 40, true).header
	if strings.Contains(narrow, "06ae395") {
		t.Fatalf("version shown although it does not fit: %q", narrow)
	}
}
