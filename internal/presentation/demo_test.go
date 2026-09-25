package presentation

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestDemoRendererFitsSupportedLayouts(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		bundle := RenderDemoBundleAtSize(strings.Repeat("a", 40), "2026-09-19", size[0], size[1])
		if bundle.Terminal != fmt.Sprintf("%dx%d", size[0], size[1]) {
			t.Fatalf("terminal metadata = %q", bundle.Terminal)
		}
		for _, scenario := range bundle.Scenarios {
			for _, frame := range scenario.Frames {
				if lipgloss.Width(frame.ANSI) > size[0] || lipgloss.Height(frame.ANSI) > size[1] {
					t.Fatalf("%s/%s overflows %dx%d", scenario.ID, frame.Label, size[0], size[1])
				}
			}
		}
	}
}

func TestDemoBundleUsesRealRendererForRequiredScenarios(t *testing.T) {
	bundle := RenderDemoBundle(strings.Repeat("a", 40), "2026-09-19")
	if bundle.Terminal != "120x30" || !bundle.Synthetic || len(bundle.Scenarios) != 5 {
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
	for _, expected := range []string{"all clients, including future clients", "Affects  pc01,pc02,pc03,pc04,pc05 · 5 computer(s)", "Reviewed revision  " + bundle.SourceCommit, "Authenticated: 5/5", "Deployment completed and verified"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("main demo omits %q", expected)
		}
	}
	if !demoFramesContain(main.Frames, "Move the cursor to Software") || !demoFramesContain(main.Frames, "Open Software directly in Search") || !demoFramesContain(main.Frames, "Type the package name") || !demoFramesContain(main.Frames, "Choose Inkscape from Search") || !demoFramesContain(main.Frames, "Review deployment to all five current clients") || !demoFramesContain(main.Frames, "Type the one-word deployment confirmation") || !demoFramesContain(main.Frames, "Press Enter to start deployment") {
		t.Fatal("main demo does not expose cursor movement and typing")
	}
	if demoFramesContain(main.Frames, "Open client distribution") || demoFramesContain(main.Frames, "Select all five clients for deployment") {
		t.Fatal("main demo repeats client selection after choosing the all-clients declaration scope")
	}
	if demoFramesContain(main.Frames, "Save the declaration; no client changed") {
		t.Fatal("main demo pauses on the intermediate declaration result")
	}
	search := demoFrameWithLabel(main.Frames, "Open Software directly in Search")
	if !strings.Contains(search.Text, "[Search packages]") || !strings.Contains(search.Text, "Package name  _") || strings.Contains(search.Text, "VLC") || strings.Contains(search.Text, "[Selected]") {
		t.Fatalf("main demo does not open directly in package search:\n%s", search.Text)
	}
	result := demoFrameWithLabel(main.Frames, "Find Inkscape in the pinned package set")
	if !strings.Contains(result.Text, "› Inkscape") || !strings.Contains(result.ANSI, "\x1b[") {
		t.Fatalf("main demo does not highlight the Inkscape search result:\n%s", result.Text)
	}
	installation := bundle.Scenarios[1]
	installationText := ""
	for _, frame := range installation.Frames {
		installationText += frame.Text
	}
	for _, expected := range []string{"Service address:    " + demoServiceAddress, "Building system · Running", "Activating system · Running", "Verifying activation · Running", "Controller operation completed", "Type START to continue", "Next: install computers", "Boot one configured computer using UEFI network boot", "run /installer/setup.sh"} {
		if !strings.Contains(installationText, expected) {
			t.Fatalf("installation demo omits %q", expected)
		}
	}
	if strings.Contains(installationText, "192.0.2.44") {
		t.Fatal("installation demo still contains the obsolete service address")
	}
	for _, label := range []string{"Build the controller configuration", "Activate the controller configuration", "Verify the active controller", "Complete controller activation and verification", "Complete every preparation step", "Type the one-word network-impact confirmation", "Press Enter to start network installation", "Complete every controller-side installation step"} {
		if !demoFramesContain(installation.Frames, label) {
			t.Fatalf("installation demo omits animation frame %q", label)
		}
	}
	completed := demoFrameWithLabel(installation.Frames, "Complete every controller-side installation step")
	for _, step := range []string{"✓ Laboratory settings", "✓ Save configuration", "✓ Controller keys", "✓ Activate controller", "✓ Prepare clients", "✓ Start PXE"} {
		if !strings.Contains(completed.Text, step) {
			t.Fatalf("completed installation frame omits green step %q:\n%s", step, completed.Text)
		}
	}
	controllerCompleted := demoFrameWithLabel(installation.Frames, "Complete controller activation and verification")
	if !strings.Contains(controllerCompleted.Text, "Controller activation and verification completed") || strings.Contains(controllerCompleted.Text, "Controller activation is running") {
		t.Fatalf("completed controller animation still looks active:\n%s", controllerCompleted.Text)
	}
	lastInstallationFrame := installation.Frames[len(installation.Frames)-1]
	if lastInstallationFrame.Label != "Follow the installation steps on each client" || !strings.Contains(lastInstallationFrame.Text, "Only the disk confirmed locally in the installer is erased") {
		t.Fatalf("installation demo does not finish on client instructions:\n%s", lastInstallationFrame.Text)
	}
	shutdown := bundle.Scenarios[2]
	shutdownText := ""
	for _, frame := range shutdown.Frames {
		shutdownText += frame.Text
	}

	profile := bundle.Scenarios[3]
	if profile.ID != "software-profile" || len(profile.Frames) < 6 {
		t.Fatalf("profile scenario = %+v", profile)
	}
	profileText := ""
	for _, frame := range profile.Frames {
		profileText += frame.Text
	}
	for _, expected := range []string{"Add a software profile", "Essential packages", "excluded", "Validated together", "ready on this controller"} {
		if !strings.Contains(profileText, expected) {
			t.Fatalf("profile demo omits %q", expected)
		}
	}
	for _, expected := range []string{"Active user session · will shut down", "Type SHUTDOWN to confirm shutdown of active sessions", "pc02       accepted", "Accepted  2"} {
		if !strings.Contains(shutdownText, expected) {
			t.Fatalf("shutdown demo omits %q", expected)
		}
	}
	if !demoFramesContain(shutdown.Frames, "Move the cursor to pc04") || !demoFramesContain(shutdown.Frames, "Type the one-word confirmation") || !demoFramesContain(shutdown.Frames, "Press Enter to send the reviewed requests") {
		t.Fatal("shutdown demo does not expose cursor movement and typing")
	}
	usb := bundle.Scenarios[4]
	if usb.ID != "installation-usb" || len(usb.Frames) < 10 {
		t.Fatalf("USB installation scenario = %+v", usb)
	}
	usbText := ""
	for _, frame := range usb.Frames {
		usbText += frame.Text
	}
	for _, expected := range []string{"USB over SSH", "official NixOS Minimal 26.05 ISO", "SHA256:AAAAAAAA", "/dev/sda", "boot-media", "/dev/nvme0n1", "ERASE /dev/nvme0n1 FOR pc01", "Remove or deprioritize the USB medium", "Boot verified:       true"} {
		if !strings.Contains(usbText, expected) {
			t.Fatalf("USB installation demo omits %q", expected)
		}
	}
	if strings.Contains(usbText, strings.Repeat("x", 12)) || !demoFramesContain(usb.Frames, "Compare the automatically observed fingerprint") || !demoFramesContain(usb.Frames, "Confirm the physical fingerprint match") || !demoFramesContain(usb.Frames, "Enter only the temporary password") || !demoFramesContain(usb.Frames, "Authorize reboot separately") || !demoFramesContain(usb.Frames, "Verify the installed identity after reboot") {
		t.Fatal("USB installation demo exposes a secret or omits the separate reboot/verification boundary")
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
