package main

import (
	"os"
	"os/exec"
	"syscall"
)

// desktopOpener returns a non-blocking xdg-open launcher, or nil when the TUI
// has no graphical session (for example over SSH).
func desktopOpener() func(string) error {
	return desktopOpenerFor(os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != "")
}

func desktopOpenerFor(graphical bool) func(string) error {
	if !graphical {
		return nil
	}
	return func(address string) error {
		path, err := exec.LookPath("xdg-open")
		if err != nil {
			return err
		}
		command := exec.Command(path, address)
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := command.Start(); err != nil {
			return err
		}
		_ = command.Process.Release()
		return nil
	}
}
