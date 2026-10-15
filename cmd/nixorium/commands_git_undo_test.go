package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestGitUndoArguments(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		valid     bool
	}{
		{[]string{"git", "undo", "plan"}, true},
		{[]string{"git", "undo", "plan", "--json", "--repo", "/srv/lab"}, true},
		{[]string{"git", "undo", "apply", "--expect", "sha256:review"}, true},
		{[]string{"git", "undo", "apply", "--expect", "sha256:review", "--yes", "--json"}, true},
		{[]string{"git", "undo", "--help"}, true},
		{[]string{"git", "undo", "apply", "--help"}, true},
		{[]string{"git", "undo"}, false},
		{[]string{"git", "undo", "review"}, false},
		{[]string{"git", "undo", "apply"}, false},
		{[]string{"git", "undo", "apply", "--yes"}, false},
		{[]string{"git", "undo", "plan", "--expect", "sha256:review"}, false},
		{[]string{"git", "undo", "plan", "--yes"}, false},
		{[]string{"git", "undo", "plan", "--paths", "lab-settings.json"}, false},
		{[]string{"git", "undo", "apply", "--expect", "sha256:review", "extra"}, false},
	} {
		parsed, err := parseArguments(test.arguments)
		if (err == nil) != test.valid {
			t.Fatalf("%q: valid=%t, err=%v", test.arguments, test.valid, err)
		}
		if test.valid && !parsed.help && (parsed.command != "git" || parsed.subcommand != "undo-"+test.arguments[2]) {
			t.Fatalf("%q parsed as %+v", test.arguments, parsed)
		}
	}
	parsed, err := parseArguments([]string{"git", "undo", "apply", "--expect", "sha256:x", "--yes", "--json", "--repo", "/srv/lab"})
	if err != nil || parsed.expect != "sha256:x" || !parsed.yes || !parsed.json || parsed.repository != "/srv/lab" {
		t.Fatalf("parsed = %+v, err = %v", parsed, err)
	}
	if help, err := parseArguments([]string{"git", "undo", "plan", "-h"}); err != nil || !help.help {
		t.Fatalf("help = %+v, err = %v", help, err)
	}
	// The sibling commands keep their own rules.
	if _, err := parseArguments([]string{"git", "commit", "plan", "--paths", "lab-settings.json"}); err != nil {
		t.Fatal(err)
	}
	if _, err := parseArguments([]string{"git", "undo", "commit"}); err == nil {
		t.Fatal("git undo accepted another git subcommand")
	}
}

func TestGitUndoUsageNamesBothSteps(t *testing.T) {
	var output bytes.Buffer
	usage(&output)
	for _, line := range []string{"git undo plan", "git undo apply --expect <review-token> [--yes]"} {
		if !strings.Contains(output.String(), line) {
			t.Fatalf("usage does not mention %q", line)
		}
	}
}

func undoTestGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "gc.auto=0"}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

// undoDeployment is a deployment repository with one ordinary saved change on
// top of its first version.
func undoDeployment(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repository := t.TempDir()
	undoTestGit(t, repository, "init", "-q", ".")
	for name, content := range map[string]string{"flake.nix": "{ outputs = { self }: {}; }\n", ".gitignore": "secret-key\nadmin-ssh\n", "lab-software.json": "{\n  \"schemaVersion\": 1,\n  \"packages\": []\n}\n"} {
		if err := os.WriteFile(filepath.Join(repository, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	undoTestGit(t, repository, "add", ".")
	undoTestGit(t, repository, "commit", "-qm", "Initial commit")
	if err := os.WriteFile(filepath.Join(repository, "lab-software.json"), []byte("{\n  \"schemaVersion\": 1,\n  \"packages\": [{\"package\": \"vlc\", \"scope\": {\"kind\": \"all-clients\"}}]\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	undoTestGit(t, repository, "add", "lab-software.json")
	undoTestGit(t, repository, "commit", "-qm", "chore: update laboratory software")
	return repository
}

func runUndo(t *testing.T, arguments ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), arguments, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestGitUndoPlanAndApplyEndToEnd(t *testing.T) {
	repository := undoDeployment(t)
	saved := undoTestGit(t, repository, "rev-parse", "HEAD")

	code, stdout, stderr := runUndo(t, "git", "undo", "plan", "--repo", repository)
	if code != 0 {
		t.Fatalf("plan exit %d: %s%s", code, stdout, stderr)
	}
	for _, expected := range []string{
		"Undo the last saved change: READY",
		"Last change:  chore: update laboratory software",
		"modified      lab-software.json",
		"\"Undo: chore: update laboratory software\"",
		"Apply to controller so this controller uses the configuration saved before that change.",
		"Update computers so the clients use it too;",
		"nixorium git undo apply --expect sha256:",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("plan output lacks %q:\n%s", expected, stdout)
		}
	}
	if undoTestGit(t, repository, "rev-parse", "HEAD") != saved {
		t.Fatal("plan changed HEAD")
	}

	code, stdout, _ = runUndo(t, "git", "undo", "plan", "--repo", repository, "--json")
	var plan domain.GitUndoPlanReport
	if code != 0 || json.Unmarshal([]byte(stdout), &plan) != nil || plan.State != "ready" || plan.ReviewToken == "" || plan.Undone.Revision != saved {
		t.Fatalf("json plan exit %d: %s", code, stdout)
	}

	if code, _, stderr = runUndo(t, "git", "undo", "apply", "--repo", repository, "--expect", "sha256:stale", "--yes"); code != 1 || !strings.Contains(stderr, "Nothing was undone") {
		t.Fatalf("stale token exit %d: %s", code, stderr)
	}
	if undoTestGit(t, repository, "rev-parse", "HEAD") != saved {
		t.Fatal("a stale token changed HEAD")
	}
	// Without --yes a non-interactive apply refuses before changing anything.
	if code, _, stderr = runUndo(t, "git", "undo", "apply", "--repo", repository, "--expect", plan.ReviewToken); code != 2 || !strings.Contains(stderr, "interactive terminal or explicit --yes") {
		t.Fatalf("non-interactive exit %d: %s", code, stderr)
	}

	code, stdout, stderr = runUndo(t, "git", "undo", "apply", "--repo", repository, "--expect", plan.ReviewToken, "--yes", "--json")
	var result domain.GitUndoReport
	if code != 0 || json.Unmarshal([]byte(stdout), &result) != nil || result.State != "completed" || !result.Committed || result.PreviousRevision != saved || len(result.Impacts) != 2 {
		t.Fatalf("apply exit %d: %s%s", code, stdout, stderr)
	}
	if undoTestGit(t, repository, "log", "-1", "--format=%s") != "Undo: chore: update laboratory software" || undoTestGit(t, repository, "status", "--porcelain") != "" {
		t.Fatalf("history:\n%s", undoTestGit(t, repository, "log", "--oneline"))
	}
	records, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "nixorium", "operations", "records.json"))
	if err != nil || !strings.Contains(string(records), "git-undo") || strings.Contains(result.Message, "not recorded") {
		t.Fatalf("the outcome was not recorded (%v): %s", err, records)
	}

	// The undo is itself a saved change: undoing it again is refused and the
	// refusal says why in words.
	code, stdout, _ = runUndo(t, "git", "undo", "plan", "--repo", repository)
	if code != 1 || !strings.Contains(stdout, "Undo the last saved change: BLOCKED") || !strings.Contains(stdout, "already undoes an earlier one") {
		t.Fatalf("second plan exit %d: %s", code, stdout)
	}
}

func TestGitUndoRefusalsAreReadableAndCarryTheNextStep(t *testing.T) {
	repository := undoDeployment(t)
	if err := os.WriteFile(filepath.Join(repository, "notes.txt"), []byte("unsaved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runUndo(t, "git", "undo", "plan", "--repo", repository)
	if code != 1 || !strings.Contains(stdout, "Blocked: deployment worktree has 1 changed path(s)") || !strings.Contains(stderr, "Next (GIT-DIRTY)") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, _ = runUndo(t, "git", "undo", "plan", "--repo", repository, "--json")
	var plan domain.GitUndoPlanReport
	if code != 1 || json.Unmarshal([]byte(stdout), &plan) != nil || len(plan.Issues) != 1 || plan.Issues[0].Next == nil || plan.Issues[0].Next.Code != "GIT-DIRTY" {
		t.Fatalf("json exit %d: %s", code, stdout)
	}
}
