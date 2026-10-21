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
	for _, want := range []string{"Nixorium: nixos-26.05 -> nixos-26.05", "Applications whose version changes:", "Package vscode: 1.0 -> 2.0", "Package nodejs: not selected -> 24.0", "+ reviewed pin"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary misses %q:\n%s", want, summary)
		}
	}
	for _, unwanted := range []string{"gcc", "requires packages", "effective preferences"} {
		if strings.Contains(summary, unwanted) {
			t.Fatalf("summary shows %q:\n%s", unwanted, summary)
		}
	}
	full := strings.Join(updateReviewLines(plan, true), "\n")
	for _, want := range []string{"Package gcc: 15.2.0 -> 15.2.0", "requires packages [nodejs]", "effective preferences"} {
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
