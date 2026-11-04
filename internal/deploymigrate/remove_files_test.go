package deploymigrate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var screensaverInCoreOnly = registryWith("screensaver-in-core")

// releasedLastScreensaver is a deployment created with 3.12.0: the form every
// earlier screensaver migration leaves, and the 3.5.0 leftover.
func releasedLastScreensaver(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{"modules/shared.nix": releasedShared}
	for _, name := range []string{"modules/screensaver.nix", "scripts/cmd-screensaver.sh", "scripts/screensaver-monitor.sh"} {
		content, err := releasedTemplateFile(t, "v3.12.0", name)
		if err != nil {
			t.Skip("no Git history with release tags")
		}
		files[name] = content
	}
	return files
}

// releasedTemplateFile reads a template file of a released tag.
func releasedTemplateFile(t *testing.T, tag, name string) (string, error) {
	t.Helper()
	out, err := exec.Command("git", "-C", filepath.Join("..", ".."), "show", tag+":templates/site/"+name).Output()
	return string(out), err
}

const leftover = "#!/usr/bin/env bash\n"

func TestTheScreensaverLeavesTheDeployment(t *testing.T) {
	files := releasedLastScreensaver(t)
	root := treeWith(t, files)
	report, err := Run(root, "3.13.0", screensaverInCoreOnly)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Applied {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if got := report.Migrations[0].Files; !slices.Equal(got, []string{"modules/shared.nix", "modules/screensaver.nix", "scripts/cmd-screensaver.sh", "scripts/screensaver-monitor.sh"}) {
		t.Fatalf("files = %v", got)
	}
	if got := read(t, root, "modules/shared.nix"); strings.Contains(got, "screensaver") || !strings.Contains(got, "./workstation.nix") {
		t.Fatalf("shared.nix = %q", got)
	}
	for _, name := range []string{"modules/screensaver.nix", "scripts/cmd-screensaver.sh", "scripts/screensaver-monitor.sh"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was kept: %v", name, err)
		}
	}
	if report, err = Run(root, "3.13.0", screensaverInCoreOnly); err != nil || len(report.Migrations) != 0 {
		t.Fatalf("the recorded migration ran again: %+v, %v", report, err)
	}
}

func TestTheScreensaverLeftoverGoesOnlyInItsReleasedForm(t *testing.T) {
	released, err := releasedTemplateFile(t, "v3.5.0", "scripts/launch-screensaver.sh")
	if err != nil {
		t.Skip("no Git history with release tags")
	}
	for _, form := range []struct {
		content string
		kept    bool
	}{{released, false}, {leftover, true}} {
		files := releasedLastScreensaver(t)
		files["scripts/launch-screensaver.sh"] = form.content
		root := treeWith(t, files)
		report, err := Run(root, "3.13.0", screensaverInCoreOnly)
		if err != nil || report.Migrations[0].State != Applied {
			t.Fatalf("report = %+v, %v", report, err)
		}
		if _, err := os.Stat(filepath.Join(root, "scripts/launch-screensaver.sh")); os.IsNotExist(err) == form.kept {
			t.Fatalf("leftover kept = %v, want %v", !os.IsNotExist(err), form.kept)
		}
	}
}

func TestAnEditedScreensaverStaysWhole(t *testing.T) {
	files := releasedLastScreensaver(t)
	files["scripts/screensaver-monitor.sh"] = strings.Replace(files["scripts/screensaver-monitor.sh"], "IDLE_SCREENSAVER_MS=300000", "IDLE_SCREENSAVER_MS=600000", 1)
	root := treeWith(t, files)
	report, err := Run(root, "3.13.0", screensaverInCoreOnly)
	if err != nil || report.Migrations[0].State != Manual || !strings.Contains(report.Migrations[0].Detail, "scripts/screensaver-monitor.sh changed by hand") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	for name, content := range files {
		if read(t, root, name) != content {
			t.Fatalf("%s was changed", name)
		}
	}
}

func TestAScreensaverImportedElsewhereStays(t *testing.T) {
	// The import was moved by hand: the files may be used elsewhere.
	files := releasedLastScreensaver(t)
	files["modules/shared.nix"] = strings.Replace(releasedShared, "    ./screensaver.nix\n", "", 1)
	root := treeWith(t, files)
	report, err := Run(root, "3.13.0", screensaverInCoreOnly)
	if err != nil || report.Migrations[0].State != Manual {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if read(t, root, "modules/screensaver.nix") != files["modules/screensaver.nix"] {
		t.Fatal("the module was removed")
	}
}

func TestAFailedRemovalRestoresEverything(t *testing.T) {
	files := releasedLastScreensaver(t)
	root := treeWith(t, files)
	original := removeFile
	t.Cleanup(func() { removeFile = original })
	removeFile = func(path string) error {
		if strings.HasSuffix(path, "screensaver-monitor.sh") {
			return errors.New("read-only")
		}
		return os.Remove(path)
	}
	report, err := Run(root, "3.13.0", screensaverInCoreOnly)
	if err != nil || report.Migrations[0].State != Manual || !strings.Contains(report.Migrations[0].Detail, "Nothing was changed") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	for name, content := range files {
		if read(t, root, name) != content {
			t.Fatalf("%s was not restored", name)
		}
	}
}
