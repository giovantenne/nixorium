package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// classroomViewAction returns the dashboard callback when the classroom
// service runs on this computer (the controller), or nil.
func classroomViewAction() func(context.Context) (string, error) {
	if _, err := os.Stat(adapters.ClassroomSocketPath); err != nil {
		return nil
	}
	return openClassroomView
}

// openClassroomView asks the classroom service for a one-time address and
// opens it in a browser window of the current graphical session.
func openClassroomView(ctx context.Context) (string, error) {
	// One classroom view per user: GNOME brings the open window forward
	// from the Nixorium icon (same window class), so never open another.
	if profile, err := classroomBrowserProfile(); err == nil && classroomBrowserRunning(profile) {
		return "The classroom view is already open.", nil
	}
	response, err := classroomRequest(ctx, domain.ClassroomViewOpenOperation, nil)
	if err != nil {
		return "", err
	}
	if response.State == "failed" || response.ViewURL == "" {
		if response.Message != "" {
			return "", errors.New(response.Message)
		}
		return "", errors.New("the classroom service returned no address")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		return "Open this address in a browser on this controller within a minute: " + response.ViewURL, nil
	}
	if err := startBrowser(response.ViewURL); err != nil {
		return "Open this address in a browser on this controller within a minute: " + response.ViewURL, nil
	}
	return "The classroom view opened in a browser window. Student computers show a sharing notice while it is open.", nil
}

// classroomBrowserPreferences makes Chromium leave the title bar to the
// desktop, so the classroom windows have GNOME's own minimize, maximize and
// close buttons like every other window.
const classroomBrowserPreferences = `{"browser":{"custom_chrome_frame":false}}` + "\n"

// classroomBrowserProfile prepares a private Chromium profile for the
// classroom view: its cookie stays out of the everyday browser, and the
// launch options apply even while that browser is open.
func classroomBrowserProfile() (string, error) {
	state := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	profile := filepath.Join(state, "nixorium", "classroom-browser")
	if err := os.MkdirAll(filepath.Join(profile, "Default"), 0o700); err != nil {
		return "", err
	}
	preferences := filepath.Join(profile, "Default", "Preferences")
	file, err := os.OpenFile(preferences, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return profile, nil
	}
	if err != nil {
		return "", err
	}
	_, err = file.WriteString(classroomBrowserPreferences)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return profile, err
}

// classroomBrowserArguments runs Chromium through XWayland, where GNOME draws
// the window frame; on Wayland Chromium would draw its own.
// classroomWindowClass is the X11 window class of the classroom view; the
// Nixorium launcher declares it as StartupWMClass.
const classroomWindowClass = "nixorium-classroom"

// Through XWayland Chromium does not see GNOME's dark style, so the page
// and its windows follow it only when told.
func classroomBrowserArguments(address, profile string, dark bool) []string {
	arguments := []string{"chromium", "--user-data-dir=" + profile, "--ozone-platform=x11", "--class=" + classroomWindowClass, "--no-first-run", "--no-default-browser-check"}
	if dark {
		arguments = append(arguments, "--force-dark-mode")
	}
	return append(arguments, "--app="+address)
}

// desktopPrefersDark reports GNOME's style for the current user.
func desktopPrefersDark() bool {
	output, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	return err == nil && strings.TrimSpace(string(output)) == "'prefer-dark'"
}

// startBrowser opens the address in an app window, detached from the
// terminal: Nixorium itself starts the browser and serves the user's
// desktop to the page while it is open.
func startBrowser(address string) error {
	candidates := [][]string{{"xdg-open", address}}
	if _, err := exec.LookPath("chromium"); err == nil {
		if self, err := os.Executable(); err == nil {
			candidates = append([][]string{{self, classroomBrowserCommand, address}}, candidates...)
		}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		command := exec.Command(path, candidate[1:]...)
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := command.Start(); err != nil {
			continue
		}
		_ = command.Process.Release()
		return nil
	}
	return errors.New("no browser is installed")
}
