package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
func classroomBrowserArguments(address, profile string) []string {
	return []string{"chromium", "--user-data-dir=" + profile, "--ozone-platform=x11", "--no-first-run", "--no-default-browser-check", "--app=" + address}
}

// startBrowser opens the address in an app window, detached from the terminal.
func startBrowser(address string) error {
	candidates := [][]string{{"xdg-open", address}}
	if profile, err := classroomBrowserProfile(); err == nil {
		candidates = append([][]string{classroomBrowserArguments(address, profile)}, candidates...)
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
