package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/giovantenne/nixorium/internal/domain"
)

const templateResetRecoveryWindow = 10 * time.Minute

// PlanTemplateResetRecovery classifies an interrupted template reset from Git
// state only and proposes the one safe way out. It changes nothing.
func (TemplateReset) PlanTemplateResetRecovery(ctx context.Context, repository string) domain.TemplateResetRecoveryPlan {
	plan := domain.TemplateResetRecoveryPlan{SchemaVersion: domain.SchemaVersion, Operation: "template-reset-recover-plan", State: "blocked", Issues: []domain.ValidationIssue{}}
	root, err := filepath.Abs(repository)
	if err != nil {
		return resetRecoveryIssue(plan, "repository", err.Error())
	}
	plan.Repository = root
	directory, err := openWorkspaceRoot(root, unix.LOCK_SH)
	if err != nil {
		return resetRecoveryIssue(plan, "repository", err.Error())
	}
	defer directory.Close()
	journal, present, err := readResetJournal(directory)
	if !present {
		plan.State = "unchanged"
		plan.Message = "No interrupted template reset is recorded."
		return plan
	}
	if err != nil {
		return resetRecoveryIssue(plan, "record", err.Error()+"; keep .git/"+resetPendingName+" and collect a support report")
	}
	plan, err = classifyResetRecovery(ctx, plan, journal)
	if err != nil {
		return resetRecoveryIssue(plan, "git", err.Error())
	}
	plan.State = "ready"
	plan.ExpiresAt = time.Now().UTC().Truncate(templateResetRecoveryWindow).Add(templateResetRecoveryWindow)
	plan.Confirmation = domain.ResetRecoveryConfirmation
	switch plan.Case {
	case domain.ResetRecoveryFinished:
		plan.Message = "The reset already completed; only its marker remains. Confirming clears the marker."
	case domain.ResetRecoveryCompleteBranch:
		plan.Message = "Every file already matches the reset; confirming moves the branch to the reset commit and clears the marker."
	case domain.ResetRecoveryNotStarted:
		plan.Message = "No file was replaced; confirming clears the marker and keeps the original configuration."
	case domain.ResetRecoveryRestore:
		plan.Confirmation = domain.ResetRestoreConfirmation
		plan.Message = "The files are a mix of the old and new template. Confirming saves the current files under a recovery reference, restores the configuration from before the reset as a new commit, and clears the marker. Untracked and ignored files are kept."
	}
	plan.ReviewToken = domain.TemplateResetRecoveryToken(plan)
	return plan
}

// ApplyTemplateResetRecovery repeats the classification under the operation
// lock and performs only the reviewed case.
func (TemplateReset) ApplyTemplateResetRecovery(ctx context.Context, plan domain.TemplateResetRecoveryPlan) domain.TemplateResetRecoveryResult {
	result := domain.TemplateResetRecoveryResult{SchemaVersion: domain.SchemaVersion, Operation: "template-reset-recover", State: "blocked", Case: plan.Case, Issues: []domain.ValidationIssue{}}
	refuse := func(message string) domain.TemplateResetRecoveryResult {
		result.Issues = append(result.Issues, domain.ValidationIssue{Field: "recovery", Message: message})
		result.Message = message
		return result
	}
	if plan.State != "ready" || plan.ReviewToken == "" || plan.ReviewToken != domain.TemplateResetRecoveryToken(plan) {
		return refuse("the recovery review token does not match the frozen plan; create a fresh review")
	}
	if !time.Now().UTC().Before(plan.ExpiresAt) {
		return refuse("the recovery review expired; create a fresh review")
	}
	gate, err := acquireManagedOperationGate()
	if err != nil {
		return refuse(err.Error())
	}
	defer gate.Close()
	gate.describe("Recover template reset")
	return applyTemplateResetRecovery(ctx, plan, result, refuse)
}

func applyTemplateResetRecovery(ctx context.Context, plan domain.TemplateResetRecoveryPlan, result domain.TemplateResetRecoveryResult, refuse func(string) domain.TemplateResetRecoveryResult) domain.TemplateResetRecoveryResult {
	directory, err := openWorkspaceRoot(plan.Repository, unix.LOCK_EX)
	if err != nil {
		return refuse(err.Error())
	}
	defer directory.Close()
	journal, present, err := readResetJournal(directory)
	if !present || err != nil {
		return refuse("the template reset marker changed after review; create a fresh review")
	}
	current := domain.TemplateResetRecoveryPlan{Repository: plan.Repository}
	current, err = classifyResetRecovery(ctx, current, journal)
	if err != nil || current.Case != plan.Case || current.Head != plan.Head || current.Branch != plan.Branch || strings.Join(current.Changed, "\n") != strings.Join(plan.Changed, "\n") {
		return refuse("the repository changed after review; create a fresh review")
	}
	repository := plan.Repository
	switch plan.Case {
	case domain.ResetRecoveryFinished, domain.ResetRecoveryNotStarted:
		result.Revision = plan.Head
	case domain.ResetRecoveryCompleteBranch:
		if _, err := resetGit(ctx, repository, nil, 4096, "update-ref", "-m", "Nixorium template reset recovery", plan.Branch, plan.Candidate, plan.Original); err != nil {
			return refuse(err.Error())
		}
		result.Revision = plan.Candidate
	case domain.ResetRecoveryRestore:
		recovery, err := resetGit(ctx, repository, nil, 256, "stash", "create", "Files at template reset recovery")
		if err != nil {
			return refuse(err.Error())
		}
		recovery = strings.TrimSpace(recovery)
		if recovery == "" {
			recovery = plan.Head
		}
		result.RecoveryRef = "refs/nixorium/template-backups/" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-recovery"
		if _, err := resetGit(ctx, repository, nil, 4096, "update-ref", result.RecoveryRef, recovery, strings.Repeat("0", 40)); err != nil {
			return refuse(err.Error())
		}
		if _, err := resetGit(ctx, repository, nil, 4096, "read-tree", "--reset", "-u", plan.Original); err != nil {
			return refuse(err.Error() + "; the files at recovery are kept under " + result.RecoveryRef)
		}
		if !resetMatches(ctx, repository, plan.Original) {
			return refuse("the restored files do not match the original configuration; keep " + result.RecoveryRef + " and the marker")
		}
		revision := plan.Original
		if plan.Head != plan.Original {
			tree, err := resetGit(ctx, repository, nil, 256, "rev-parse", plan.Original+"^{tree}")
			if err != nil {
				return refuse(err.Error())
			}
			commit, err := resetGit(ctx, repository, nil, 256, "commit-tree", strings.TrimSpace(tree), "-p", plan.Head, "-m", "Restore the configuration from before the interrupted template reset")
			if err != nil {
				return refuse(err.Error())
			}
			revision = strings.TrimSpace(commit)
			reference := plan.Branch
			if reference == "" {
				reference = "HEAD"
			}
			if _, err := resetGit(ctx, repository, nil, 4096, "update-ref", "-m", "Nixorium template reset recovery", reference, revision, plan.Head); err != nil {
				return refuse(err.Error())
			}
		}
		result.Revision = revision
	default:
		return refuse("unknown recovery case")
	}
	if err := directory.Sync(); err != nil {
		return refuse(err.Error())
	}
	archive, err := archiveResetJournal(directory)
	if err != nil {
		return refuse("the marker could not be archived: " + err.Error())
	}
	result.State, result.Archive = "completed", archive
	result.Message = "The interrupted template reset is resolved and operations are unblocked. Review the configuration, then apply it to the controller and computers as usual."
	if result.RecoveryRef != "" {
		result.Message += " The files found at recovery are kept under " + result.RecoveryRef + "."
	}
	return result
}

func classifyResetRecovery(ctx context.Context, plan domain.TemplateResetRecoveryPlan, journal resetJournal) (domain.TemplateResetRecoveryPlan, error) {
	repository := plan.Repository
	plan.Branch, plan.Original, plan.Candidate, plan.BackupRef = journal.Branch, journal.OriginalRevision, journal.CandidateRevision, journal.BackupRef
	for _, name := range []string{"index.lock", "HEAD.lock", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		if _, err := os.Lstat(filepath.Join(repository, ".git", name)); !errors.Is(err, os.ErrNotExist) {
			return plan, errors.New("Git is in the middle of another operation (" + name + "); finish or abort it first")
		}
	}
	head, err := resetGit(ctx, repository, nil, 256, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return plan, err
	}
	plan.Head = strings.TrimSpace(head)
	branch := ""
	if output, err := resetGit(ctx, repository, nil, 4096, "symbolic-ref", "-q", "HEAD"); err == nil {
		branch = strings.TrimSpace(output)
	}
	switch {
	case plan.Head == plan.Candidate && resetMatches(ctx, repository, plan.Candidate):
		plan.Case = domain.ResetRecoveryFinished
	case plan.Head == plan.Original && branch == journal.Branch && resetMatches(ctx, repository, plan.Candidate):
		plan.Case = domain.ResetRecoveryCompleteBranch
	case plan.Head == plan.Original && resetMatches(ctx, repository, plan.Original):
		plan.Case = domain.ResetRecoveryNotStarted
	default:
		plan.Case = domain.ResetRecoveryRestore
		plan.Branch = branch
		changed := map[string]bool{}
		for _, arguments := range [][]string{{"diff", "--name-only", "--no-ext-diff", "--no-textconv", plan.Original, "--"}, {"diff", "--cached", "--name-only", "--no-ext-diff", "--no-textconv", plan.Original, "--"}} {
			output, err := resetGit(ctx, repository, nil, 256*1024, arguments...)
			if err != nil {
				return plan, err
			}
			for _, name := range strings.Split(strings.TrimSpace(output), "\n") {
				if name != "" {
					changed[name] = true
				}
			}
		}
		plan.Changed = make([]string, 0, len(changed))
		for name := range changed {
			plan.Changed = append(plan.Changed, name)
		}
		sort.Strings(plan.Changed)
		untracked, err := resetGit(ctx, repository, nil, 256*1024, "ls-files", "--others", "--exclude-standard")
		if err != nil {
			return plan, err
		}
		original, err := resetGit(ctx, repository, nil, 1024*1024, "ls-tree", "-r", "--name-only", plan.Original)
		if err != nil {
			return plan, err
		}
		tracked := map[string]bool{}
		for _, name := range strings.Split(original, "\n") {
			tracked[name] = true
		}
		for _, name := range strings.Split(strings.TrimSpace(untracked), "\n") {
			if name != "" && tracked[name] {
				return plan, errors.New("the untracked file " + name + " would be overwritten by the restore; move it aside first")
			}
		}
	}
	return plan, nil
}

func resetMatches(ctx context.Context, repository, revision string) bool {
	if _, err := resetGit(ctx, repository, nil, 4096, "diff", "--quiet", "--no-ext-diff", "--no-textconv", revision, "--"); err != nil {
		return false
	}
	_, err := resetGit(ctx, repository, nil, 4096, "diff", "--cached", "--quiet", "--no-ext-diff", "--no-textconv", revision, "--")
	return err == nil
}

func readResetJournal(root *os.File) (resetJournal, bool, error) {
	var journal resetJournal
	data, mode, err := readRegularFileNoFollowLimit(filepath.Join(root.Name(), ".git", resetPendingName), 64*1024)
	if errors.Is(err, os.ErrNotExist) {
		return journal, false, nil
	}
	if err != nil {
		return journal, true, err
	}
	if mode&0o077 != 0 {
		return journal, true, errors.New("the template reset marker is not private")
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return journal, true, errors.New("the template reset marker is not valid JSON")
	}
	if journal.SchemaVersion != 1 || !strings.HasPrefix(journal.Branch, "refs/heads/") || !fullGitObjectIDPattern.MatchString(journal.OriginalRevision) ||
		!fullGitObjectIDPattern.MatchString(journal.CandidateRevision) || !strings.HasPrefix(journal.BackupRef, "refs/nixorium/template-backups/") {
		return journal, true, errors.New("the template reset marker is incomplete")
	}
	return journal, true, nil
}

// archiveResetJournal moves the marker into .git/nixorium-recovered without
// overwriting an earlier archive.
func archiveResetJournal(root *os.File) (string, error) {
	gitDirectory, err := resetGitDirectory(root)
	if err != nil {
		return "", err
	}
	defer gitDirectory.Close()
	if err := unix.Mkdirat(int(gitDirectory.Fd()), "nixorium-recovered", 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return "", err
	}
	archiveFD, err := unix.Openat(int(gitDirectory.Fd()), "nixorium-recovered", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	archive := os.NewFile(uintptr(archiveFD), filepath.Join(gitDirectory.Name(), "nixorium-recovered"))
	defer archive.Close()
	name := "template-reset-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".json"
	if err := unix.Renameat2(int(gitDirectory.Fd()), resetPendingName, archiveFD, name, unix.RENAME_NOREPLACE); err != nil {
		return "", err
	}
	if err := archive.Sync(); err != nil {
		return "", err
	}
	if err := gitDirectory.Sync(); err != nil {
		return "", err
	}
	return filepath.Join(archive.Name(), name), nil
}

func resetRecoveryIssue(plan domain.TemplateResetRecoveryPlan, field, message string) domain.TemplateResetRecoveryPlan {
	plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: field, Message: message})
	if plan.Message == "" {
		plan.Message = message
	}
	return plan
}
