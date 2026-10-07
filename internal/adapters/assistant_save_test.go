package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func assistantSaveFixture(t *testing.T) (*AssistantWorkspace, AssistantWorkspaceStore, string, domain.AssistantSavePlan) {
	t.Helper()
	w, store, repo := assistantEvaluationFixture(t, false)
	if err := w.Write("course.nix", "{ value = 42; }", false); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("scripts/example.sh", "#!/bin/sh\nexit 0\n", true); err != nil {
		t.Fatal(err)
	}
	if err := w.Delete("unused.nix"); err != nil {
		t.Fatal(err)
	}
	plan := PlanAssistantSave(t.Context(), store, w.Conversation.ID)
	if plan.State != "ready" || !plan.Evaluation.OK {
		t.Fatalf("plan: %+v", plan)
	}
	return w, store, repo, plan
}
func TestAssistantSaveTransactionAndRecovery(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "save", true: "interrupted"}[interrupted], func(t *testing.T) {
			w, store, repo, plan := assistantSaveFixture(t)
			if strings.Join(plan.Added, ",") != "scripts/example.sh" || strings.Join(plan.Changed, ",") != "course.nix" || strings.Join(plan.Removed, ",") != "unused.nix" {
				t.Fatalf("paths: %+v", plan)
			}
			for _, want := range []string{"+{ value = 42; }", "-{}", "new file mode 100755", "a/course.nix", "b/course.nix"} {
				if !strings.Contains(plan.Diff, want) {
					t.Fatalf("diff missing %q: %s", want, plan.Diff)
				}
			}
			again := PlanAssistantSave(t.Context(), store, w.Conversation.ID)
			if again.ReviewToken != plan.ReviewToken {
				t.Fatal("review token is not stable")
			}
			if count, err := store.ChangedFiles(w.Conversation.ID); err != nil || count != 3 {
				t.Fatalf("count=%d: %v", count, err)
			}
			root, err := openWorkspaceRoot(repo, unix.LOCK_EX)
			if err != nil {
				t.Fatal(err)
			}
			var fault func() error
			if interrupted {
				fault = func() error { return errors.New("injected interruption") }
			}
			result := commitConfigurationTree(t.Context(), root, plan.Proposal, plan.ReviewToken, assistantCommit, fault)
			root.Close()
			if !strings.HasPrefix(result.BackupRef, "refs/nixorium/assistant-backups/") {
				t.Fatalf("backup: %+v", result)
			}
			if interrupted {
				if !result.RecoveryRequired {
					t.Fatalf("interrupted: %+v", result)
				}
				root, err = openWorkspaceRoot(repo, unix.LOCK_SH)
				if err != nil {
					t.Fatal(err)
				}
				journal, _, err := readResetJournal(root)
				root.Close()
				if err != nil || journal.Kind != "assistant-save" {
					t.Fatalf("journal: %+v %v", journal, err)
				}
				recovery := (TemplateReset{}).PlanTemplateResetRecovery(t.Context(), repo)
				if recovery.Case != domain.ResetRecoveryCompleteBranch || !strings.Contains(recovery.Message, "Assistant save") {
					t.Fatalf("recovery: %+v", recovery)
				}
				recovered := applyResetRecoveryForTest(t, recovery)
				if recovered.State != "completed" {
					t.Fatalf("recovery: %+v", recovered)
				}
			} else if result.State != "saved" || result.RecoveryRequired {
				t.Fatalf("save: %+v", result)
			}
			if got := strings.TrimSpace(workspaceTestGit(t, repo, "log", "-1", "--format=%s")); got != assistantCommit.CommitMessage {
				t.Fatal(got)
			}
			if got := strings.TrimSpace(workspaceTestGit(t, repo, "rev-parse", result.BackupRef)); got != plan.Base {
				t.Fatal("backup mismatch")
			}
			if got := workspaceTestGit(t, repo, "status", "--porcelain"); got != "" {
				t.Fatal(got)
			}
			if info, err := os.Stat(filepath.Join(repo, "scripts/example.sh")); err != nil || info.Mode().Perm() != 0755 {
				t.Fatal("executable mode not saved", err)
			}
			if _, err := os.Stat(filepath.Join(repo, "unused.nix")); !os.IsNotExist(err) {
				t.Fatal("file not removed", err)
			}
		})
	}
}
func TestAssistantSaveRefusesChangedReview(t *testing.T) {
	for _, scenario := range []string{"head", "overlay", "expired", "evaluation", "untracked"} {
		t.Run(scenario, func(t *testing.T) {
			w, store, repo, plan := assistantSaveFixture(t)
			switch scenario {
			case "head":
				workspaceTestGit(t, repo, "commit", "--allow-empty", "-qm", "Moved HEAD")
			case "overlay":
				if err := w.Write("course.nix", "{ value = 43; }", false); err != nil {
					t.Fatal(err)
				}
			case "expired":
				plan.ExpiresAt = time.Now().Add(-time.Second)
			case "evaluation":
				if err := w.Write("course.nix", "not valid nix!", false); err != nil {
					t.Fatal(err)
				}
				plan = PlanAssistantSave(t.Context(), store, w.Conversation.ID)
				if plan.State != "ready" || plan.Evaluation.OK {
					t.Fatalf("failed evaluation not reviewable: %+v", plan)
				}
			case "untracked":
				writeGitReviewFile(t, repo, "scripts/example.sh", "untracked")
			}
			before := assistantGitSnapshot(t, repo)
			result := SaveAssistant(t.Context(), store, plan)
			want := assistantSaveChanged
			if scenario == "evaluation" {
				want = assistantSaveInvalid
			}
			if result.State != "blocked" || result.Message != want {
				t.Fatalf("refusal: %+v", result)
			}
			after := assistantGitSnapshot(t, repo)
			if resetFileDigest(before) != resetFileDigest(after) {
				t.Fatal("refusal changed repository")
			}
		})
	}
}
