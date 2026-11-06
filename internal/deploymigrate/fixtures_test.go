package deploymigrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationGateRejectsMissingFixtures(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name       string
		test       string
		git        bool
		shallow    bool
		diagnostic string
	}{
		{"history", "TestEveryStableReleaseMigratesToThisTemplate", false, false, "complete Git checkout"},
		{"tags", "TestEveryStableReleaseMigratesToThisTemplate", true, false, "release tags starting at v3.0.0"},
		{"shallow history", "TestEveryStableReleaseMigratesToThisTemplate", true, true, "complete Git checkout"},
		{"template", "TestTheTemplateOfThisReleaseNeedsNoMigration", false, false, "the template is not in this source tree"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "internal", "deploymigrate")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if fixture.git {
				if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
					t.Fatalf("initialize empty repository: %v\n%s", err, out)
				}
			}
			if fixture.shallow {
				command := exec.Command("git", "-C", root,
					"-c", "user.name=Migration test", "-c", "user.email=test@example.invalid",
					"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
					"commit", "--allow-empty", "-m", "Fixture boundary")
				if out, err := command.CombinedOutput(); err != nil {
					t.Fatalf("create shallow boundary: %v\n%s", err, out)
				}
				commit, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ".git", "shallow"), commit, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, required := range []string{"0", "1"} {
				t.Run("required="+required, func(t *testing.T) {
					command := exec.Command(executable, "-test.run", "^"+fixture.test+"$", "-test.v")
					command.Dir = directory
					command.Env = append(os.Environ(),
						"NIXORIUM_REQUIRE_MIGRATION_FIXTURES="+required,
						"GIT_CEILING_DIRECTORIES="+filepath.Dir(root))
					out, err := command.CombinedOutput()
					if required == "1" {
						if err == nil || !strings.Contains(string(out), fixture.diagnostic) || strings.Contains(string(out), "--- SKIP") {
							t.Fatalf("required fixture did not fail explicitly: %v\n%s", err, out)
						}
					} else if err != nil || !strings.Contains(string(out), "--- SKIP") {
						t.Fatalf("package build must still skip unavailable fixtures: %v\n%s", err, out)
					}
				})
			}
		})
	}
}
