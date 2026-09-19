package presentation

import (
	"strings"
	"testing"
)

func TestDemoBundleUsesRealRendererForRequiredScenarios(t *testing.T) {
	bundle := RenderDemoBundle(strings.Repeat("a", 40), "2026-09-19")
	if bundle.Terminal != "120x30" || !bundle.Synthetic || len(bundle.Scenarios) != 3 {
		t.Fatalf("unexpected bundle metadata: %+v", bundle)
	}
	for _, scenario := range bundle.Scenarios {
		if len(scenario.Frames) < 6 {
			t.Fatalf("scenario %s has too few frames", scenario.ID)
		}
		if scenario.Frames[0].Label != "Overview" {
			t.Fatalf("scenario %s does not start from the dashboard overview", scenario.ID)
		}
		for _, frame := range scenario.Frames {
			if frame.DurationMS < 1 || !strings.Contains(frame.Text, "Nixorium") || !strings.Contains(frame.ANSI, "\x1b[") {
				t.Fatalf("invalid rendered frame %s/%s", scenario.ID, frame.Label)
			}
		}
	}
	main := bundle.Scenarios[0]
	joined := ""
	for _, frame := range main.Frames {
		joined += frame.Text
	}
	for _, expected := range []string{"all clients, including future clients", "Affects  @lab · 5 computer(s)", "Reviewed revision  " + bundle.SourceCommit, "Authenticated: 5/5", "Deployment completed and verified"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("main demo omits %q", expected)
		}
	}
	if !demoFramesContain(main.Frames, "Move the cursor to Software") || !demoFramesContain(main.Frames, "Open Software directly in Search") || !demoFramesContain(main.Frames, "Type the package name") || !demoFramesContain(main.Frames, "Choose Inkscape from Search") || !demoFramesContain(main.Frames, "Review deployment to all five current clients") || !demoFramesContain(main.Frames, "Type the reviewed deployment target") || !demoFramesContain(main.Frames, "Press Enter to start deployment") {
		t.Fatal("main demo does not expose cursor movement and typing")
	}
	if demoFramesContain(main.Frames, "Open client distribution") || demoFramesContain(main.Frames, "Select all five clients for deployment") {
		t.Fatal("main demo repeats client selection after choosing the all-clients declaration scope")
	}
	search := demoFrameWithLabel(main.Frames, "Open Software directly in Search")
	if !strings.Contains(search.Text, "[Search packages]") || !strings.Contains(search.Text, "Package name  _") || strings.Contains(search.Text, "VLC") || strings.Contains(search.Text, "[Selected]") {
		t.Fatalf("main demo does not open directly in package search:\n%s", search.Text)
	}
	result := demoFrameWithLabel(main.Frames, "Find Inkscape in the pinned package set")
	if !strings.Contains(result.Text, "› Inkscape") || !strings.Contains(result.ANSI, "\x1b[") {
		t.Fatalf("main demo does not highlight the Inkscape search result:\n%s", result.Text)
	}
	shutdown := bundle.Scenarios[2]
	shutdownText := ""
	for _, frame := range shutdown.Frames {
		shutdownText += frame.Text
	}
	for _, expected := range []string{"Active user session · will shut down", "Type SHUTDOWN to confirm shutdown of active sessions", "pc02       accepted", "Accepted  2"} {
		if !strings.Contains(shutdownText, expected) {
			t.Fatalf("shutdown demo omits %q", expected)
		}
	}
	if !demoFramesContain(shutdown.Frames, "Move the cursor to pc04") || !demoFramesContain(shutdown.Frames, "Type the one-word confirmation") || !demoFramesContain(shutdown.Frames, "Press Enter to send the reviewed requests") {
		t.Fatal("shutdown demo does not expose cursor movement and typing")
	}
}

func demoFramesContain(frames []DemoFrame, label string) bool {
	for _, frame := range frames {
		if frame.Label == label {
			return true
		}
	}
	return false
}

func demoFrameWithLabel(frames []DemoFrame, label string) DemoFrame {
	for _, frame := range frames {
		if frame.Label == label {
			return frame
		}
	}
	return DemoFrame{}
}
