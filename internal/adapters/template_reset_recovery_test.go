package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func interruptedReset(t *testing.T) (string, domain.TemplateResetResult) {
	t.Helper()
	root, plan := resetFixture(t)
	result := applyResetTransaction(t.Context(), root, plan, func() error { return errors.New("injected interruption") })
	if result.State != "recovery-required" {
		t.Fatalf("interruption = %+v", result)
	}
	root.Close()
	return plan.Repository, result
}

func applyResetRecoveryForTest(t *testing.T, plan domain.TemplateResetRecoveryPlan) domain.TemplateResetRecoveryResult {
	t.Helper()
	result := domain.TemplateResetRecoveryResult{Case: plan.Case}
	return applyTemplateResetRecovery(t.Context(), plan, result, func(message string) domain.TemplateResetRecoveryResult {
		t.Fatalf("recovery refused: %s", message)
		return result
	})
}

func TestResetRecoveryCompletesTheBranch(t *testing.T) {
	repository, _ := interruptedReset(t)
	plan := TemplateReset{}.PlanTemplateResetRecovery(t.Context(), repository)
	if plan.State != "ready" || plan.Case != domain.ResetRecoveryCompleteBranch || plan.Confirmation != "RECOVERED" {
		t.Fatalf("plan = %+v", plan)
	}
	result := applyResetRecoveryForTest(t, plan)
	if result.State != "completed" || result.Revision != plan.Candidate {
		t.Fatalf("result = %+v", result)
	}
	head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD"))
	if head != plan.Candidate {
		t.Fatalf("HEAD = %s, want %s", head, plan.Candidate)
	}
	if _, err := (Local{}).GitState(t.Context(), repository); err != nil {
		t.Fatalf("operations still blocked: %v", err)
	}
	if after := (TemplateReset{}).PlanTemplateResetRecovery(t.Context(), repository); after.State != "unchanged" {
		t.Fatalf("after = %+v", after)
	}
}

func TestResetRecoveryRestoresAMixedCheckout(t *testing.T) {
	repository, _ := interruptedReset(t)
	writeGitReviewFile(t, repository, "lab-settings.json", "{\"edited\":\"after interruption\"}\n")
	plan := TemplateReset{}.PlanTemplateResetRecovery(t.Context(), repository)
	if plan.State != "ready" || plan.Case != domain.ResetRecoveryRestore || plan.Confirmation != "RESTORE" || len(plan.Changed) == 0 {
		t.Fatalf("plan = %+v", plan)
	}
	result := applyResetRecoveryForTest(t, plan)
	if result.State != "completed" || result.RecoveryRef == "" {
		t.Fatalf("result = %+v", result)
	}
	workspaceTestGit(t, repository, "diff", "--exit-code", plan.Original)
	workspaceTestGit(t, repository, "diff", "--exit-code", "HEAD")
	// The branch never moved, so restoring the files needs no new commit.
	if head := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD")); plan.Head != plan.Original || head != plan.Original {
		t.Fatalf("HEAD = %s, plan head %s, original %s", head, plan.Head, plan.Original)
	}
	saved := workspaceTestGit(t, repository, "show", result.RecoveryRef+":lab-settings.json")
	if !strings.Contains(saved, "after interruption") {
		t.Fatalf("recovery ref lost the edited file: %q", saved)
	}
	for _, name := range []string{"secret-key", "private/config", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(repository, name)); err != nil {
			t.Fatalf("%s was not preserved: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repository, ".git", resetPendingName)); !os.IsNotExist(err) {
		t.Fatal("marker still blocks operations")
	}
}

func TestResetRecoveryRefusesAChangedRepository(t *testing.T) {
	repository, _ := interruptedReset(t)
	plan := TemplateReset{}.PlanTemplateResetRecovery(t.Context(), repository)
	writeGitReviewFile(t, repository, "lab-settings.json", "{\"changed\":\"after review\"}\n")
	refused := ""
	applyTemplateResetRecovery(t.Context(), plan, domain.TemplateResetRecoveryResult{}, func(message string) domain.TemplateResetRecoveryResult {
		refused = message
		return domain.TemplateResetRecoveryResult{}
	})
	if !strings.Contains(refused, "changed after review") {
		t.Fatalf("refusal = %q", refused)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git", resetPendingName)); err != nil {
		t.Fatal("marker removed despite refusal")
	}
}

func TestResetRecoveryRestoresOnTopOfAMovedBranch(t *testing.T) {
	repository, _ := interruptedReset(t)
	plan := TemplateReset{}.PlanTemplateResetRecovery(t.Context(), repository)
	// Simulate a branch that moved to the reset commit while files diverged.
	workspaceTestGit(t, repository, "update-ref", plan.Branch, plan.Candidate, plan.Original)
	writeGitReviewFile(t, repository, "lab-settings.json", "{\"edited\":\"diverged\"}\n")
	plan = TemplateReset{}.PlanTemplateResetRecovery(t.Context(), repository)
	if plan.Case != domain.ResetRecoveryRestore || plan.Head != plan.Candidate {
		t.Fatalf("plan = %+v", plan)
	}
	result := applyResetRecoveryForTest(t, plan)
	if parent := strings.TrimSpace(workspaceTestGit(t, repository, "rev-parse", "HEAD^")); parent != plan.Candidate || result.Revision == plan.Original {
		t.Fatalf("restore commit parent = %s, result %+v", parent, result)
	}
	workspaceTestGit(t, repository, "diff", "--exit-code", plan.Original, "HEAD")
}
