package adapters

import (
	"strings"
	"testing"
)

func controllerChangeCommit(t *testing.T, repository string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		writeGitReviewFile(t, repository, name, content)
		workspaceTestGit(t, repository, "add", name)
	}
	workspaceTestGit(t, repository, "commit", "-qm", "change")
	return strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
}

func controllerLock(nixorium, nixpkgs string) string {
	return `{"nodes":{"nixorium":{"locked":{"rev":"` + nixorium + `"}},"nixpkgs":{"locked":{"rev":"` + nixpkgs + `"}},` +
		`"root":{"inputs":{"nixorium":"nixorium","nixpkgs":"nixpkgs"}}},"root":"root","version":7}` + "\n"
}

func TestControllerChangesNameEachKindOfChange(t *testing.T) {
	repository := workspaceRepository(t)
	applied := controllerChangeCommit(t, repository, map[string]string{"flake.lock": controllerLock("a", "a"), "lab-settings.json": "{}\n"})
	check := func(files map[string]string, want ...string) string {
		t.Helper()
		reviewed := controllerChangeCommit(t, repository, files)
		changes, known := controllerChanges(t.Context(), repository, applied, reviewed)
		if !known || strings.Join(changes, "|") != strings.Join(want, "|") {
			t.Fatalf("changes for %v: %v %t, want %v", files, changes, known, want)
		}
		return reviewed
	}
	check(map[string]string{"lab-settings.json": `{"a":1}` + "\n"}, "Saved settings")
	// Changes accumulate since the recorded activation.
	check(map[string]string{"lab-software.json": "{}\n", "workspace-profile.json": "{}\n"}, "Saved settings", "Software selection", "Student preferences")
	check(map[string]string{"flake.lock": controllerLock("b", "a")}, "Saved settings", "Software selection", "Student preferences", "Nixorium version")
	applied = strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	check(map[string]string{"flake.lock": controllerLock("b", "b"), "modules/local.nix": "{}\n"}, "System and packages", "Local modules, assets and other files")
	applied = strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	if changes, known := controllerChanges(t.Context(), repository, applied, applied); !known || len(changes) != 0 {
		t.Fatalf("identical revisions: %v %t", changes, known)
	}
	for _, missing := range []string{"", strings.Repeat("0", 40), "not-a-revision"} {
		if changes, known := controllerChanges(t.Context(), repository, missing, applied); known || changes != nil {
			t.Fatalf("unknown applied revision %q reported %v", missing, changes)
		}
	}
}
