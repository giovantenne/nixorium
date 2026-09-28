package presentation

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
			m := workspaceFixture()
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
