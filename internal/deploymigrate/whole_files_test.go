package deploymigrate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ttfxOnly runs the screensaver migration alone.
var ttfxOnly = Registry[2:3]

// releasedScreensaver is the 3.6.1 form of the three screensaver files.
func releasedScreensaver(t *testing.T) map[string]string {
	t.Helper()
	tree := map[string]string{}
	for path, name := range map[string]string{
		"modules/screensaver.nix":        "screensaver.nix",
		"scripts/cmd-screensaver.sh":     "cmd-screensaver.sh",
		"scripts/screensaver-monitor.sh": "screensaver-monitor.sh",
	} {
		content, err := os.ReadFile(filepath.Join("testdata", "2026-10-ttfx-screensaver", name))
		if err != nil {
			t.Fatal(err)
		}
		tree[path] = string(content)
	}
	return tree
}

func TestTheScreensaverMigrationReplacesTheReleasedFilesAndKeepsTheirMode(t *testing.T) {
	root := treeWith(t, releasedScreensaver(t))
	if err := os.Chmod(filepath.Join(root, "scripts/cmd-screensaver.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := Run(root, "3.10.0", ttfxOnly)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Applied {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if got := report.Migrations[0].Files; !slices.Equal(got, []string{"modules/screensaver.nix", "scripts/cmd-screensaver.sh", "scripts/screensaver-monitor.sh"}) {
		t.Fatalf("files = %v", got)
	}
	if got := read(t, root, "scripts/cmd-screensaver.sh"); !strings.Contains(got, "ttfx -i") || !strings.Contains(got, "--frame-rate 30") || strings.Contains(got, "tte -i") {
		t.Fatalf("cmd-screensaver.sh = %q", got)
	}
	if got := read(t, root, "modules/screensaver.nix"); !strings.Contains(got, "nixoriumTtfx") {
		t.Fatalf("screensaver.nix = %q", got)
	}
	if got := read(t, root, "scripts/screensaver-monitor.sh"); !strings.Contains(got, "IDLE_BLANK_MS=1800000") {
		t.Fatalf("screensaver-monitor.sh = %q", got)
	}
	if info, err := os.Stat(filepath.Join(root, "scripts/cmd-screensaver.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
	if !strings.Contains(read(t, root, RecordFile), "2026-10-ttfx-screensaver") {
		t.Fatalf("record = %q", read(t, root, RecordFile))
	}
}

func TestTheScreensaverMigrationTouchesNothingWhenOneFileWasEdited(t *testing.T) {
	files := releasedScreensaver(t)
	files["scripts/screensaver-monitor.sh"] = strings.Replace(files["scripts/screensaver-monitor.sh"], "IDLE_SCREENSAVER_MS=300000", "IDLE_SCREENSAVER_MS=600000", 1)
	root := treeWith(t, files)
	report, err := Run(root, "3.10.0", ttfxOnly)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Manual || !strings.Contains(report.Migrations[0].Detail, "scripts/screensaver-monitor.sh changed by hand") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if got := read(t, root, "scripts/cmd-screensaver.sh"); got != files["scripts/cmd-screensaver.sh"] {
		t.Fatalf("an unedited file was changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, RecordFile)); !os.IsNotExist(err) {
		t.Fatalf("a manual migration was recorded: %v", err)
	}
}

func TestTheScreensaverMigrationIsDoneOrPartialOnNewFiles(t *testing.T) {
	current := map[string]string{
		"modules/screensaver.nix":        string(frozen("2026-10-ttfx-screensaver", "screensaver.nix")),
		"scripts/cmd-screensaver.sh":     string(frozen("2026-10-ttfx-screensaver", "cmd-screensaver.sh")),
		"scripts/screensaver-monitor.sh": string(frozen("2026-10-ttfx-screensaver", "screensaver-monitor.sh")),
	}
	report, err := Run(treeWith(t, current), "3.10.0", ttfxOnly)
	if err != nil || report.Migrations[0].State != Done {
		t.Fatalf("report = %+v, %v", report, err)
	}
	// One file still in a released form: only that one is replaced.
	current["scripts/cmd-screensaver.sh"] = releasedScreensaver(t)["scripts/cmd-screensaver.sh"]
	report, err = Run(treeWith(t, current), "3.10.0", ttfxOnly)
	if err != nil || report.Migrations[0].State != Applied || !slices.Equal(report.Migrations[0].Files, []string{"scripts/cmd-screensaver.sh"}) {
		t.Fatalf("report = %+v, %v", report, err)
	}
}

func TestTheScreensaverMigrationIsManualWithoutTheFiles(t *testing.T) {
	report, err := Run(treeWith(t, map[string]string{"README.md": "lab\n"}), "3.10.0", ttfxOnly)
	if err != nil || report.Migrations[0].State != Manual {
		t.Fatalf("report = %+v, %v", report, err)
	}
}

func TestAMigratedScreensaverIsTheTemplateOfThisRelease(t *testing.T) {
	// A 3.6.1 deployment that runs every screensaver migration ends up with
	// the template's own files. When the template changes again, this test
	// points at the need for a new migration.
	root := treeWith(t, releasedScreensaver(t))
	report, err := Run(root, "3.10.0", Registry[2:])
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range report.Migrations {
		if entry.State != Applied {
			t.Fatalf("%s = %s: %s", entry.ID, entry.State, entry.Detail)
		}
	}
	for _, file := range []string{"modules/screensaver.nix", "scripts/cmd-screensaver.sh", "scripts/screensaver-monitor.sh"} {
		template, err := os.ReadFile(filepath.Join("..", "..", "templates", "site", file))
		if os.IsNotExist(err) {
			t.Skip("the template is not in this source tree (the package build)")
		}
		if err != nil {
			t.Fatal(err)
		}
		if read(t, root, file) != string(template) {
			t.Errorf("a migrated templates/site/%s differs from the template: add a migration", file)
		}
	}
}

func TestTheSixtyFramesMigrationAppliesOnceAndRespectsEdits(t *testing.T) {
	sixty := Registry[3:4]
	current := string(frozen("2026-10-ttfx-screensaver", "cmd-screensaver.sh"))
	root := treeWith(t, map[string]string{"scripts/cmd-screensaver.sh": current})
	report, err := Run(root, "3.10.0", sixty)
	if err != nil || report.Migrations[0].State != Applied || !strings.Contains(read(t, root, "scripts/cmd-screensaver.sh"), "--frame-rate 60 \\") {
		t.Fatalf("report = %+v, %v", report, err)
	}
	edited := strings.Replace(current, "--frame-rate 30", "--frame-rate 45", 1)
	report, err = Run(treeWith(t, map[string]string{"scripts/cmd-screensaver.sh": edited}), "3.10.0", sixty)
	if err != nil || report.Migrations[0].State != Manual {
		t.Fatalf("an edited frame rate was not left alone: %+v, %v", report, err)
	}
}
