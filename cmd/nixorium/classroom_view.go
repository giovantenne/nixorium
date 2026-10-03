package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

// startBrowser opens the address in an app window, detached from the terminal.
func startBrowser(address string) error {
	candidates := [][]string{
		{"chromium", "--app=" + address, "--new-window"},
		{"xdg-open", address},
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
