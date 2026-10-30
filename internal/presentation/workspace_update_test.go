package presentation

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWorkspaceUpdateTextDistinguishesPinsAndRuntimeEvidence(t *testing.T) {
	plan := demoWorkspaceUpdatePlan()
	var output bytes.Buffer
	UpdatePlanText(&output, plan)
	for _, want := range []string{"example.extension: 1.0 -> 2.0", "vscode: 1.0 -> 2.0", "nodejs: not selected -> 24.0", "requires packages [nodejs]", "controller (controller)", "current pin -> proposed pin (not live versions)", "not an isolated extension update", "do not certify plugin loading", "Current pin effective preferences", "Proposed pin effective preferences", "+ reviewed pin"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q:\n%s", want, &output)
		}
	}
	plan.Workspace.Proposed.Extensions[0].Version = "bad\x1b[2J\rhidden"
	output.Reset()
	workspaceUpdateText(&output, plan.Workspace)
	if strings.ContainsAny(output.String(), "\x1b\r") {
		t.Fatal("terminal controls survived")
	}
}

func TestWorkspaceUpdateScrollableReviewKeepsCancelAndVersionComparison(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{false, true} {
			m := workspaceFixture(t)
			m.width, m.height, m.isDark = size[0], size[1], dark
			m.screen = dashboardUpdateReview
			m.updates.packageBase = true
			m.updates.plan = demoWorkspaceUpdatePlan()
			m.updateDetails = true // the diff is in F4 Details
			seen := ""
			var firstView string
			for i := 0; i <= m.maximumUpdateScroll(); i++ {
				view := m.View().Content
				plain := demoANSI.ReplaceAllString(view, "")
				if i == 3 {
					firstView = plain
				}
				if lipgloss.Height(view) > size[1] || lipgloss.Width(view) > size[0] {
					t.Fatalf("overflow:\n%s", plain)
				}
				for _, want := range []string{"Esc", "Cancel", "Enter", "Apply update", "Help"} {
					if !strings.Contains(plain, want) {
						t.Fatalf("missing %q:\n%s", want, plain)
					}
				}
				seen += plain
				m, _ = workspaceKey(m, demoCode(tea.KeyDown))
			}
			for _, want := range []string{"example.extension: 1.0 -> 2.0", "vscode: 1.0 -> 2.0", "+ reviewed pin"} {
				if !strings.Contains(seen, want) {
					t.Fatalf("%dx%d cannot reach %q:\n%s", size[0], size[1], want, firstView)
				}
			}
			m, cmd := workspaceKey(m, demoCode(tea.KeyEsc))
			if cmd != nil || m.screen != dashboardUpdate {
				t.Fatal("review cancellation invoked update")
			}
		}
	}
}

func TestUpdateReviewShowsOnlyChangesUntilDetails(t *testing.T) {
	plan := demoWorkspaceUpdatePlan()
	plan.Workspace.Current.Packages = append(plan.Workspace.Current.Packages, domain.WorkspacePackage{Package: "gcc", Version: "15.2.0"})
	plan.Workspace.Proposed.Packages = append(plan.Workspace.Proposed.Packages, domain.WorkspacePackage{Package: "gcc", Version: "15.2.0"})
	summary := strings.Join(updateReviewLines(plan, false), "\n")
	for _, want := range []string{"NixOS base: nixos-26.05 (unchanged)", "Applications whose version changes:", "Package vscode: 1.0 -> 2.0", "Package nodejs: not selected -> 24.0", "F4 shows every version and the changes to the deployment files."} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary misses %q:\n%s", want, summary)
		}
	}
	for _, unwanted := range []string{"gcc", "requires packages", "effective preferences", "+ reviewed pin", "Proposed flake.nix/flake.lock changes"} {
		if strings.Contains(summary, unwanted) {
			t.Fatalf("summary shows %q:\n%s", unwanted, summary)
		}
	}
	full := strings.Join(updateReviewLines(plan, true), "\n")
	for _, want := range []string{"Package gcc: 15.2.0 -> 15.2.0", "requires packages [nodejs]", "effective preferences", "+ reviewed pin"} {
		if !strings.Contains(full, want) {
			t.Fatalf("details miss %q:\n%s", want, full)
		}
	}
	plan.Workspace.Proposed.Packages = plan.Workspace.Current.Packages
	plan.Workspace.Proposed.Extensions = plan.Workspace.Current.Extensions
	if summary := strings.Join(updateReviewLines(plan, false), "\n"); !strings.Contains(summary, "No application version changes") {
		t.Fatalf("unchanged summary:\n%s", summary)
	}
}

func TestUpdateReviewNamesMasterRevisionsAndIndentsMigrations(t *testing.T) {
	m := workspaceFixture(t)
	m.width, m.height = 100, 40
	m.screen = dashboardUpdateReview
	m.updates.plan = demoWorkspaceUpdatePlan()
	m.updates.plan.Kind = ""
	m.updates.plan.CurrentRef, m.updates.plan.Target = "master", "master"
	m.updates.plan.CurrentRev, m.updates.plan.TargetRev = strings.Repeat("3", 40), strings.Repeat("a", 40)
	m.updates.plan.Migrations = []domain.DeploymentMigrationResult{{State: domain.MigrationApplied, Files: []string{"scripts/cmd-screensaver.sh"},
		Description: "The lab screensaver draws 60 frames per second instead of 30: its effects advance once per frame, so at 30 they ran at half speed."}}
	content := strings.Join(m.updateReviewContent(), "\n")
	if !strings.Contains(content, "Nixorium: master 333333333333 → aaaaaaaaaaaa") {
		t.Fatalf("revisions missing:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "half speed") && !strings.HasPrefix(line, strings.Repeat(" ", 18)) {
			t.Fatalf("migration continuation not indented: %q\n%s", line, content)
		}
	}
}

func TestUpdateReviewLabelsTheReferenceAndSaysWhenNothingChanges(t *testing.T) {
	plan := demoWorkspaceUpdatePlan()
	summary := strings.Join(updateReviewLines(plan, false), "\n")
	if !strings.Contains(summary, "NixOS base: nixos-26.05 (unchanged)") || strings.Contains(summary, "Nixorium:") || strings.Contains(summary, "nixos-26.05 -> nixos-26.05") {
		t.Fatalf("system and package update review:\n%s", summary)
	}
	plan.Target = "nixos-26.11"
	if summary = strings.Join(updateReviewLines(plan, false), "\n"); !strings.Contains(summary, "NixOS base: nixos-26.05 → nixos-26.11") {
		t.Fatalf("channel change:\n%s", summary)
	}
	plan.Kind, plan.CurrentRef, plan.Target = "", "v3.10.0", "v3.11.0"
	if summary = strings.Join(updateReviewLines(plan, false), "\n"); !strings.Contains(summary, "Nixorium: v3.10.0 → v3.11.0") {
		t.Fatalf("release change:\n%s", summary)
	}
}
