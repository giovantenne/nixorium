package presentation

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestAssistantDemosPreserveScopeAndProposalBoundary(t *testing.T) {
	revision := strings.Repeat("a", 40)
	model := demoExploreModel(revision, 120, 30, false)
	plan := demoAssistantPlan()
	for _, host := range plan.ChangedHosts {
		found := false
		for _, observed := range model.computers.hosts.Hosts {
			found = found || observed.Name == host
		}
		if !found {
			t.Fatalf("proposal affects host outside demo inventory: %s", host)
		}
	}
	if len(plan.ChangedHosts) != 2 || !strings.Contains(plan.Diff, `"group": "4B"`) {
		t.Fatal("demo proposal lost its host scope or settings diff")
	}
	text := ""
	for _, entry := range model.assistant.entries {
		text += entry.text + "\n"
	}
	for _, expected := range []string{"Proposal ready", "separate review", "nothing is installed yet"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("graph proposal omits %q", expected)
		}
	}
	for _, invalid := range []string{"GIMP", "Inkscape", "Build check passed", "pc01–pc12"} {
		if strings.Contains(text, invalid) {
			t.Fatalf("graph proposal contains stale claim %q", invalid)
		}
	}
	scenario := RenderAssistantMenuDemo(revision, 120, 30)
	saved := demoFrameWithLabel(scenario.Frames, "A local commit and backup precede separate application")
	if !strings.Contains(saved.Text, "Computers were not updated") {
		t.Fatal(saved.Text)
	}
	diagnosis := demoFrameWithLabel(scenario.Frames, "Diagnose a device from what the computer sees")
	for _, expected := range []string{"HDMI port of pc99", "Nothing was changed"} {
		if !strings.Contains(strings.Join(strings.Fields(diagnosis.Text), " "), expected) {
			t.Fatalf("diagnosis omits evidence limit or next step %q", expected)
		}
	}
	second := demoFrameWithLabel(scenario.Frames, "A second proposal awaits review")
	if !strings.Contains(strings.Join(strings.Fields(second.Text), " "), "Proposal ready") {
		t.Fatal("second proposal omits its readiness")
	}
}

func TestAssistantCaptureFramesAndHintsFit(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		scenario := RenderAssistantMenuDemo(strings.Repeat("a", 40), size[0], size[1])
		for _, frame := range scenario.Frames {
			if lipgloss.Width(frame.ANSI) > size[0] || lipgloss.Height(frame.ANSI) > size[1] {
				t.Fatalf("%s overflows %dx%d", frame.Label, size[0], size[1])
			}
		}
		empty := demoFrameWithLabel(scenario.Frames, "The chat opens first, even before activation")
		for _, meaningful := range []string{"Jupyter", "Arduino", "projector"} {
			if !strings.Contains(strings.Join(strings.Fields(empty.Text), " "), meaningful) {
				t.Fatalf("%dx%d hint omits %q:\n%s", size[0], size[1], meaningful, empty.Text)
			}
		}
	}
	demos := RenderAssistantPricingDemos()
	if len(demos) != 5 {
		t.Fatalf("expected five locale captures, got %d", len(demos))
	}
	for _, demo := range demos {
		typed := utf8.RuneCountInString(demo.Prompt)
		if len(demo.Frames) != typed+4 {
			t.Fatalf("%s: missing typing or activity frames", demo.Locale)
		}
		for index, frame := range demo.Frames {
			if lipgloss.Width(frame.ANSI) > 84 || lipgloss.Height(frame.ANSI) > 28 {
				t.Fatalf("%s/%d overflows", demo.Locale, index)
			}
			running := strings.Contains(frame.Text, "Working… 0s")
			if running != (index >= typed+1 && index <= typed+2) {
				t.Fatalf("%s/%d: wrong running phase", demo.Locale, index)
			}
		}
		final := demo.Frames[len(demo.Frames)-1].Text
		if !strings.Contains(final, "1") || strings.Contains(final, "Build check passed") {
			t.Fatalf("invalid %s proposal:\n%s", demo.Locale, final)
		}
	}
}
