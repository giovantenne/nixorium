package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
)

func TestDiscardKeepsABackupAndRestoresTheSavedVersion(t *testing.T) {
	repository := workspaceRepository(t)
	writeGitReviewFile(t, repository, "lab-settings.json", "{\"saved\":true}\n")
	writeGitReviewFile(t, repository, "notes.txt", "kept\n")
	workspaceTestGit(t, repository, "add", "lab-settings.json", "notes.txt")
	workspaceTestGit(t, repository, "commit", "-qm", "saved")
	writeGitReviewFile(t, repository, "lab-settings.json", "{\"mistake\":true}\n")
	writeGitReviewFile(t, repository, "notes.txt", "also edited\n")
	writeGitReviewFile(t, repository, "untracked.txt", "new\n")
	manager := app.NewGitDiscardManager(Local{})
	plan := manager.Plan(t.Context(), repository, "lab-settings.json")
	if plan.HasErrors() || plan.Confirmation != "DISCARD" || !strings.Contains(plan.Diff.Content, "mistake") {
		t.Fatalf("plan = %+v", plan)
	}
	if blocked := manager.Plan(t.Context(), repository, "untracked.txt"); !blocked.HasErrors() {
		t.Fatal("an untracked file was offered for discard")
	}
	if refused := manager.Apply(t.Context(), repository, "lab-settings.json", "sha256:wrong"); refused.State == "completed" {
		t.Fatal("a wrong token discarded changes")
	}
	result := manager.Apply(t.Context(), repository, "lab-settings.json", plan.ReviewToken)
	if result.State != "completed" || !strings.HasPrefix(result.BackupRef, "refs/nixorium/discard-backups/") {
		t.Fatalf("result = %+v", result)
	}
	content, _ := os.ReadFile(filepath.Join(repository, "lab-settings.json"))
	if string(content) != "{\"saved\":true}\n" {
		t.Fatalf("restored content = %q", content)
	}
	if saved := workspaceTestGit(t, repository, "show", result.BackupRef+":lab-settings.json"); !strings.Contains(saved, "mistake") {
		t.Fatalf("backup lost the discarded content: %q", saved)
	}
	if other, _ := os.ReadFile(filepath.Join(repository, "notes.txt")); string(other) != "also edited\n" {
		t.Fatal("an unselected file was changed")
	}
	if _, err := os.Stat(filepath.Join(repository, "untracked.txt")); err != nil {
		t.Fatal("an untracked file was removed")
	}
}
