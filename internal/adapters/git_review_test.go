package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitReviewClassifiesChangesAndRedactsSettingsPasswords(t *testing.T) {
	repository := newGitReviewRepository(t)
	settings := `{
  "schemaVersion": 1,
  "lab": {
    "adminPassword": "$6$new$admin",
    "teacherPassword": "$6$new$teacher",
    "studentPassword": "$6$new$student"
  }
}
`
	writeGitReviewFile(t, repository, "lab-settings.json", settings)
	writeGitReviewFile(t, repository, "module.nix", "{ ... }: { services.openssh.enable = false; }\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", "lab-settings.json"); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "notes.txt", "not opened automatically\n")

	snapshot, err := (Local{}).GitReview(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Changes) != 3 || len(snapshot.Diffs) != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	byPath := map[string]struct {
		staged, unstaged   string
		untracked, managed bool
	}{}
	for _, change := range snapshot.Changes {
		byPath[change.Path] = struct {
			staged, unstaged   string
			untracked, managed bool
		}{change.Staged, change.Unstaged, change.Untracked, change.Managed}
	}
	if change := byPath["lab-settings.json"]; change.staged != "modified" || !change.managed {
		t.Fatalf("settings change = %+v", change)
	}
	if change := byPath["module.nix"]; change.unstaged != "modified" || change.managed {
		t.Fatalf("module change = %+v", change)
	}
	if change := byPath["notes.txt"]; !change.untracked {
		t.Fatalf("untracked change = %+v", change)
	}
	combined := snapshot.Diffs[0].Content + snapshot.Diffs[1].Content
	for _, secret := range []string{"$6$new$admin", "$6$new$teacher", "$6$new$student"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("Git review leaked %q:\n%s", secret, combined)
		}
	}
	if strings.Count(combined, `"<redacted>"`) != 6 {
		t.Fatalf("password changes were not redacted on both sides:\n%s", combined)
	}
	if !strings.Contains(combined, "services.openssh.enable = false") {
		t.Fatalf("safe unstaged patch missing:\n%s", combined)
	}
}

func TestGitReviewRefusesPrivatePathWithoutReadingAnyDiff(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "secret-key", "PRIVATE-SIGNING-MATERIAL\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", "-f", "secret-key"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (Local{}).GitReview(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Changes) != 1 || !snapshot.Changes[0].Private || len(snapshot.Diffs) != 0 {
		t.Fatalf("private change was not refused before diff: %+v", snapshot)
	}
}

func TestParseGitPorcelainHandlesRenameConflictAndControlCharacters(t *testing.T) {
	changes, err := parseGitPorcelain("R  moved-settings.json\x00lab-settings.json\x00UU conflicted\x00?? bad\x1bname\x00")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Staged != "renamed" || changes[0].OriginalPath != "lab-settings.json" || !changes[0].Managed || changes[1].Staged != "conflict" || changes[1].Unstaged != "conflict" || changes[2].Path != "bad�name" {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestGitReviewDiffArgumentsDisableDriversAndExcludePrivatePaths(t *testing.T) {
	arguments := strings.Join(gitReviewDiffArguments(true), " ")
	for _, expected := range []string{"diff --cached", "--no-ext-diff", "--no-textconv", "--no-color", ":(exclude,top)secret-key", ":(exclude,top)admin-ssh", ":(exclude,top)veyon-private-key.pem"} {
		if !strings.Contains(arguments, expected) {
			t.Fatalf("Git diff arguments omit %q: %s", expected, arguments)
		}
	}
}

func TestGitReviewBoundsLargeDiffs(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "large", strings.Repeat("a", maximumGitDiffBytes+4096)+"\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", "large"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "large"); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "large", strings.Repeat("b", maximumGitDiffBytes+4096)+"\n")
	snapshot, err := (Local{}).GitReview(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Diffs) != 1 || !snapshot.Diffs[0].Truncated || len(snapshot.Diffs[0].Content) > maximumGitDiffBytes {
		t.Fatalf("large diff was not bounded: %+v", snapshot.Diffs)
	}
}

func newGitReviewRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	if _, err := run(context.Background(), "git", "init", "-q", repository); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "lab-settings.json", "{\n  \"schemaVersion\": 1,\n  \"lab\": {\n    \"adminPassword\": \"$6$old$admin\",\n    \"teacherPassword\": \"$6$old$teacher\",\n    \"studentPassword\": \"$6$old$student\"\n  }\n}\n")
	writeGitReviewFile(t, repository, "module.nix", "{ ... }: { services.openssh.enable = true; }\n")
	writeGitReviewFile(t, repository, ".gitignore", "secret-key\nadmin-ssh\nveyon-private-key.pem\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "initial"); err != nil {
		t.Fatal(err)
	}
	return repository
}

func writeGitReviewFile(t *testing.T, repository, path, content string) {
	t.Helper()
	fullPath := filepath.Join(repository, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
