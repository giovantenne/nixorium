package deploymigrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// releasedDrift lists the template files a migrated deployment may still
// differ in, and why no migration changes them.
var releasedDrift = map[string]string{
	// The local web server left the template in 3.10.0; a laboratory that has
	// it keeps it, so its exercises keep working.
	"modules/development.nix": "keeps the web server",
	// Only the wording of an installer error message changed in 3.6.0.
	"scripts/configure-software-profile.sh": "message wording",
	// No longer used from 3.6.0: the idle monitor launches the screensaver.
	"scripts/launch-screensaver.sh": "unused leftover",
}

// TestEveryStableReleaseMigratesToThisTemplate runs every migration on the
// deployment template of each stable release from 3.0.0, as a laboratory
// created then and never edited has it. Each migration must apply or find its
// change done, and the result must be this release's template. It needs the
// Git history with tags, so the package build skips it.
func TestEveryStableReleaseMigratesToThisTemplate(t *testing.T) {
	repository := filepath.Join("..", "..")
	out, err := exec.Command("git", "-C", repository, "tag", "--list", "v3.*").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		t.Skip("no Git history with release tags")
	}
	stable := regexp.MustCompile(`^v3\.[0-9]+\.[0-9]+$`)
	template := filepath.Join(repository, "templates", "site")
	for _, tag := range strings.Fields(string(out)) {
		if !stable.MatchString(tag) {
			continue
		}
		t.Run(tag, func(t *testing.T) {
			root := t.TempDir()
			archive := exec.Command("git", "-C", repository, "archive", tag, "templates/site")
			extract := exec.Command("tar", "-x", "-C", root, "--strip-components=2")
			extract.Stdin, _ = archive.StdoutPipe()
			if err := extract.Start(); err != nil {
				t.Fatal(err)
			}
			if err := archive.Run(); err != nil {
				t.Fatal(err)
			}
			if err := extract.Wait(); err != nil {
				t.Fatal(err)
			}
			report, err := Run(root, "3.10.0", Registry)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range report.Migrations {
				if entry.State != Applied && entry.State != Done {
					t.Errorf("%s = %s: %s", entry.ID, entry.State, entry.Detail)
				}
			}
			for _, directory := range []string{"modules", "scripts"} {
				names := map[string]bool{}
				for _, side := range []string{root, template} {
					entries, _ := os.ReadDir(filepath.Join(side, directory))
					for _, entry := range entries {
						names[entry.Name()] = true
					}
				}
				for name := range names {
					file := directory + "/" + name
					migrated, migratedErr := os.ReadFile(filepath.Join(root, file))
					current, currentErr := os.ReadFile(filepath.Join(template, file))
					if _, allowed := releasedDrift[file]; allowed {
						continue
					}
					if migratedErr != nil || currentErr != nil || string(migrated) != string(current) {
						t.Errorf("%s differs from this release's template after migrating: add or widen a migration", file)
					}
				}
			}
		})
	}
}
