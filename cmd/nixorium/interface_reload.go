package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/presentation"
)

// installedInterfacePath is the controller's system-wide Nixorium command,
// installed by modules/management.nix through environment.systemPackages. It
// follows the active system generation, so it names the new binary as soon as
// a controller activation succeeds.
const installedInterfacePath = "/run/current-system/sw/bin/nixorium"

const reopenInterfaceHint = "Reopen Nixorium to use the updated interface."

var installedStorePackage = regexp.MustCompile(`^/nix/store/[0-9a-z]{32}-nixorium-([^/]+)/`)

// detectInterfaceReload reports whether the installed command is a different
// binary from the running one. The version comes from the installed store
// path and is empty when the path does not name one.
func detectInterfaceReload(executable func() (string, error), installedPath string) (presentation.InterfaceReload, bool) {
	installed, err := filepath.EvalSymlinks(installedPath)
	if err != nil {
		return presentation.InterfaceReload{}, false
	}
	if info, err := os.Stat(installed); err != nil || !info.Mode().IsRegular() {
		return presentation.InterfaceReload{}, false
	}
	running, err := executable()
	if err != nil || running == "" {
		return presentation.InterfaceReload{}, false
	}
	if resolved, err := filepath.EvalSymlinks(running); err == nil {
		running = resolved
	}
	if normalizeInterfacePath(running) == normalizeInterfacePath(installed) {
		return presentation.InterfaceReload{}, false
	}
	reload := presentation.InterfaceReload{}
	if match := installedStorePackage.FindStringSubmatch(installed); match != nil {
		reload.Version = match[1]
	}
	return reload, true
}

// normalizeInterfacePath maps the wrapped program that the packaged launcher
// script starts back to the launcher name, so the running process and the
// installed command compare equal within one package.
func normalizeInterfacePath(path string) string {
	directory, name := filepath.Split(path)
	if strings.HasPrefix(name, ".") && strings.HasSuffix(name, "-wrapped") {
		name = strings.TrimSuffix(strings.TrimPrefix(name, "."), "-wrapped")
	}
	return directory + name
}

// execInterface replaces the process image; tests replace it.
var execInterface = syscall.Exec

// reopenInterface starts the installed Nixorium in place of this process once
// bubbletea has restored the terminal. It returns only when exec failed, after
// printing the manual instruction, and reports the exit code to use.
func reopenInterface(installedPath string, arguments []string, environment []string, stderr io.Writer) int {
	argv := append([]string{installedPath}, arguments...)
	if err := execInterface(installedPath, argv, environment); err != nil {
		fmt.Fprintln(stderr, reopenInterfaceHint)
	}
	return 0
}
