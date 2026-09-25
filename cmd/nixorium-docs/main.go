// Command nixorium-docs updates deterministic documentation blocks from the
// real presentation fixtures. It has no operational adapters.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/presentation"
)

const (
	generatedStart = "<!-- BEGIN GENERATED: tui-gallery -->"
	generatedEnd   = "<!-- END GENERATED: tui-gallery -->"
	galleryPath    = "docs/tui-gallery.md"
	demoRevision   = "0123456789abcdef0123456789abcdef01234567"
	demoDate       = "2026-09-23"
)

func main() {
	check := flag.Bool("check", false, "verify generated documentation without changing files")
	write := flag.Bool("write", false, "update generated documentation blocks")
	repository := flag.String("repo", ".", "Nixorium source checkout")
	flag.Parse()
	if *check == *write {
		fmt.Fprintln(os.Stderr, "nixorium-docs: choose exactly one of --check or --write")
		os.Exit(2)
	}
	changed, err := updateGallery(*repository, *write)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixorium-docs:", err)
		os.Exit(1)
	}
	if *check && changed {
		fmt.Fprintln(os.Stderr, "nixorium-docs: generated TUI gallery is stale; run scripts/generate-docs.sh --write")
		os.Exit(1)
	}
}

func updateGallery(repository string, write bool) (bool, error) {
	path := filepath.Join(repository, galleryPath)
	info, err := os.Lstat(path)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", galleryPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, errors.New("docs/tui-gallery.md must be a regular file, not a symlink")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", galleryPath, err)
	}
	expected, err := replaceGeneratedRegion(string(current), renderGallery())
	if err != nil {
		return false, err
	}
	changed := expected != string(current)
	if !changed || !write {
		return changed, nil
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tui-gallery-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create temporary gallery: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(expected); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("write temporary gallery: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("sync temporary gallery: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close temporary gallery: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return false, fmt.Errorf("set gallery permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, fmt.Errorf("replace gallery: %w", err)
	}
	return true, nil
}

func replaceGeneratedRegion(document, generated string) (string, error) {
	if strings.Count(document, generatedStart) != 1 || strings.Count(document, generatedEnd) != 1 {
		return "", errors.New("docs/tui-gallery.md must contain exactly one generated marker pair")
	}
	start := strings.Index(document, generatedStart)
	end := strings.Index(document, generatedEnd)
	if end <= start {
		return "", errors.New("generated TUI gallery markers are out of order")
	}
	end += len(generatedEnd)
	replacement := generatedStart + "\n" + strings.TrimSpace(generated) + "\n" + generatedEnd
	return document[:start] + replacement + document[end:], nil
}

type galleryFrame struct {
	title      string
	scenarioID string
	label      string
}

func renderGallery() string {
	bundle := presentation.RenderDemoBundle(demoRevision, demoDate)
	frames := []galleryFrame{
		{title: "Overview", scenarioID: "software-all-clients", label: "Overview"},
		{title: "Pinned package search", scenarioID: "software-all-clients", label: "Find Inkscape in the pinned package set"},
		{title: "Additive software profile review", scenarioID: "software-profile", label: "Review the complete profile addition"},
		{title: "Contextual client deployment selection", scenarioID: "software-all-clients", label: "Open contextual client selection"},
		{title: "Client deployment review", scenarioID: "software-all-clients", label: "Review deployment to all five current clients"},
		{title: "Verified deployment result", scenarioID: "software-all-clients", label: "Deployment completed and verified"},
		{title: "PXE network-impact review", scenarioID: "installation", label: "Review the temporary network impact"},
		{title: "USB SSH disk review", scenarioID: "installation-usb", label: "Review physical identity, logical identity and disk"},
		{title: "USB SSH verified result", scenarioID: "installation-usb", label: "Verify the installed identity after reboot"},
		{title: "Shutdown with active sessions", scenarioID: "shutdown", label: "Active sessions will shut down; unreachable clients are not sent"},
	}
	var output strings.Builder
	for index, selection := range frames {
		if index > 0 {
			output.WriteString("\n\n")
		}
		frame := findFrame(bundle, selection.scenarioID, selection.label)
		fmt.Fprintf(&output, "## %s\n\n```text\n%s\n```", selection.title, normalizeFrame(frame.Text))
	}
	return output.String()
}

func findFrame(bundle presentation.DemoBundle, scenarioID, label string) presentation.DemoFrame {
	for _, scenario := range bundle.Scenarios {
		if scenario.ID != scenarioID {
			continue
		}
		for _, frame := range scenario.Frames {
			if frame.Label == label {
				return frame
			}
		}
	}
	panic("documentation frame not found: " + scenarioID + "/" + label)
}

func normalizeFrame(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		leading := len(line) - len(strings.TrimLeft(line, " "))
		if indent == -1 || leading < indent {
			indent = leading
		}
	}
	if indent > 0 {
		for index, line := range lines {
			if len(line) >= indent {
				lines[index] = line[indent:]
			}
		}
	}
	return strings.Join(lines, "\n")
}
