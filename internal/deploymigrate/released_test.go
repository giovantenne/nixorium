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
}

// Package builds intentionally omit Git history and most of the template.
// The checkout CI gate requires both fixtures and must never pass by skipping.
func missingMigrationFixture(t *testing.T, message string) {
	t.Helper()
	if os.Getenv("NIXORIUM_REQUIRE_MIGRATION_FIXTURES") == "1" {
		t.Fatal(message)
	}
	t.Skip(message)
}

// TestEveryStableReleaseMigratesToThisTemplate runs every migration on the
// deployment template of each stable release from 3.0.0 and of each later
// beta (a beta can be installed), as a laboratory created then and never edited has it. Each migration must apply or find its
// change done, and the result must be this release's template. It needs the
// Git history with tags, so the package build skips it.
func TestEveryStableReleaseMigratesToThisTemplate(t *testing.T) {
	repository := filepath.Join("..", "..")
	if os.Getenv("NIXORIUM_REQUIRE_MIGRATION_FIXTURES") == "1" {
		out, err := exec.Command("git", "-C", repository, "rev-parse", "--is-shallow-repository").Output()
		if err != nil || strings.TrimSpace(string(out)) != "false" {
			t.Fatal("migration checks require a complete Git checkout (fetch-depth: 0)")
		}
		if err := exec.Command("git", "-C", repository, "rev-parse", "--verify", "refs/tags/v3.0.0^{commit}").Run(); err != nil {
			t.Fatal("migration checks require release tags starting at v3.0.0")
		}
	}
	out, err := exec.Command("git", "-C", repository, "tag", "--list", "v3.*").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		missingMigrationFixture(t, "no Git history with release tags")
	}
	stable := regexp.MustCompile(`^v3\.[0-9]+\.[0-9]+(-beta\.[0-9]+)?$`)
	template := filepath.Join(repository, "templates", "site")
	for _, tag := range strings.Fields(string(out)) {
		// v3.0.0-beta.1 predates the migrations' reach: no laboratory runs it.
		if !stable.MatchString(tag) || tag == "v3.0.0-beta.1" {
			continue
		}
		t.Run(tag, func(t *testing.T) {
			root := t.TempDir()
			archive := exec.Command("git", "-C", repository, "archive", tag, "templates/site")
			extract := exec.Command("tar", "-x", "-C", root, "--strip-components=2")
			pipe, err := extract.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			archive.Stdout = pipe
			if err := extract.Start(); err != nil {
				_ = pipe.Close()
				t.Fatal(err)
			}
			archiveErr := archive.Run()
			// Close tar input and reap it even when Git fails to start or exits
			// unsuccessfully; otherwise the extractor could keep waiting for input.
			_ = pipe.Close()
			extractErr := extract.Wait()
			if archiveErr != nil || extractErr != nil {
				t.Fatalf("extract %s: git archive: %v; tar: %v", tag, archiveErr, extractErr)
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
			if err := compareMigratedTemplate(root, template); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestTheTemplateOfThisReleaseNeedsNoMigration runs every migration on a copy
// of this release's template. A laboratory created from it must find every
// migration done: one that applies would write an older form back, and one
// that is manual would be reported at every update for ever (a manual
// migration is never recorded). It fails when a migrated whole file changes
// in templates/site and the new form is not in the Later list of the earlier
// migrations of that file.
func TestTheTemplateOfThisReleaseNeedsNoMigration(t *testing.T) {
	template := filepath.Join("..", "..", "templates", "site")
	if _, err := os.Stat(filepath.Join(template, "modules", "workstation.nix")); err != nil {
		missingMigrationFixture(t, "the template is not in this source tree (the package build)")
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(template)); err != nil {
		t.Fatal(err)
	}
	report, err := Run(root, "3.10.0", Registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Migrations) != len(Registry) {
		t.Fatalf("%d of %d migrations reported", len(report.Migrations), len(Registry))
	}
	for _, entry := range report.Migrations {
		if entry.State != Done {
			t.Errorf("%s = %s on this release's template: %s (add the template's form to the Later list of the whole-file migrations of that path)", entry.ID, entry.State, entry.Detail)
		}
	}
}
