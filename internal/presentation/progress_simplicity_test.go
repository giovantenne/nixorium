package presentation

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

// A running screen says once what runs: the spinner on the current step, one
// elapsed line, the footer. No "Running"/"Waiting" words, no second spinner
// line, no timeout, no "Esc" outside the footer, and no ○ on a notice.
func TestUpdateProgressSaysOnceWhatRuns(t *testing.T) {
	model := dashboardModel{screen: dashboardUpdate, busy: "Checking the laboratory configuration", width: 100, height: 40,
		updates: updateModel{planning: true, target: "master", planStarted: time.Now(), planProgress: domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseEvaluate}}}
	model.beginRead(dashboardBuildTimeout)
	t.Cleanup(model.read.cancel)
	view := ansi.Strip(model.View().Content)
	for _, unwanted := range []string{"· Running", "· Waiting", "Checking the laboratory configuration", "Limit:", "Read-only", "Esc cancels", "○ Nothing changes", "\n   Update Nixorium\n"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("update progress still says %q:\n%s", unwanted, view)
		}
	}
	for _, wanted := range []string{"Target: master", ansi.Strip(model.activityMarker()) + " Check laboratory configuration", "○ Test systems before saving", "Elapsed:", "Nothing changes until you confirm the review.", "Esc Cancel"} {
		if !strings.Contains(view, wanted) {
			t.Errorf("update progress lacks %q:\n%s", wanted, view)
		}
	}
	if strings.Count(view, ansi.Strip(model.activityMarker())) != 1 {
		t.Errorf("update progress shows the spinner more than once:\n%s", view)
	}
}

// Every recorded demo frame shows at most one spinner: the running step
// carries it, and no separate spinner line repeats it.
func TestDemoFramesShowAtMostOneSpinner(t *testing.T) {
	bundle := RenderDemoBundle(strings.Repeat("a", 40), "2026-09-19")
	for _, scenario := range bundle.Scenarios {
		for index, frame := range scenario.Frames {
			count := 0
			for _, glyph := range tuiSpinnerFrames.Frames {
				count += strings.Count(frame.Text, strings.TrimSpace(glyph))
			}
			if count > 1 {
				t.Errorf("%s frame %d shows %d spinners:\n%s", scenario.ID, index, count, frame.Text)
			}
		}
	}
}
