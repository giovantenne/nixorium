package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
)

const maximumGitUndoFileBytes = 1024 * 1024

// Git writes this line when it reverts a commit; Nixorium's own undo commits
// carry it too, so one pattern recognizes both.
var gitRevertedCommit = regexp.MustCompile(`(?m)^This reverts commit ([0-9a-f]{40})\.?[ \t]*$`)

// GitUndoFacts reports what the last saved change is, without judging it.
// It reads the commit object itself, so no user configuration (signature
// display, encodings, diff drivers) can change what is observed.
func (local Local) GitUndoFacts(ctx context.Context, repository string) (domain.GitUndoFacts, error) {
	if err := checkTemplateResetPending(repository); err != nil {
		return domain.GitUndoFacts{}, err
	}
	head, err := local.GitRevision(ctx, repository)
	if err != nil {
		return domain.GitUndoFacts{}, err
	}
	if !fullGitObjectIDPattern.MatchString(head) {
		return domain.GitUndoFacts{}, errors.New("resolved revision is not a full Git object ID")
	}
	object, _, err := runBoundedGit(ctx, repository, 64*1024, "cat-file", "commit", head)
	if err != nil {
		return domain.GitUndoFacts{}, fmt.Errorf("read the last saved change: %w", err)
	}
	facts := parseGitUndoCommit(object)
	facts.Revision = head
	if !fullGitObjectIDPattern.MatchString(facts.Tree) {
		return domain.GitUndoFacts{}, errors.New("the last saved change has no readable tree")
	}
	branch, _, err := runBoundedGit(ctx, repository, 1024, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return domain.GitUndoFacts{}, fmt.Errorf("read the current branch: %w", err)
	}
	if branch = strings.TrimSpace(branch); branch != "HEAD" {
		facts.Branch = branch
	}
	gitDirectory, _, err := runBoundedGit(ctx, repository, 4096, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return domain.GitUndoFacts{}, fmt.Errorf("locate the Git directory: %w", err)
	}
	facts.InProgress = unfinishedGitOperation(strings.TrimSpace(gitDirectory))
	if len(facts.Parents) != 1 {
		return facts, nil
	}
	parentObject, _, err := runBoundedGit(ctx, repository, 64*1024, "cat-file", "commit", facts.Parents[0])
	if err != nil {
		return domain.GitUndoFacts{}, fmt.Errorf("read the saved change before it: %w", err)
	}
	facts.ParentTree = parseGitUndoCommit(parentObject).Tree
	if !fullGitObjectIDPattern.MatchString(facts.ParentTree) {
		return domain.GitUndoFacts{}, errors.New("the saved change before it has no readable tree")
	}
	raw, truncated, err := runBoundedGit(ctx, repository, maximumGitStatusBytes, "diff-tree", "-r", "-z", "--raw", "--no-abbrev", "--no-renames", "--no-ext-diff", "--no-textconv", facts.Parents[0], head)
	if err != nil {
		return domain.GitUndoFacts{}, fmt.Errorf("list the files of the last saved change: %w", err)
	}
	if truncated {
		facts.Truncated = true
		return facts, nil
	}
	files, err := parseGitUndoFiles(raw)
	if err != nil {
		return domain.GitUndoFacts{}, err
	}
	if len(files) > maximumGitChanges {
		facts.Truncated = true
		files = files[:maximumGitChanges]
	}
	facts.Files = files
	return facts, nil
}

// parseGitUndoCommit reads the tree, parents, committer time and subject of a
// raw commit object.
func parseGitUndoCommit(object string) domain.GitUndoFacts {
	facts := domain.GitUndoFacts{}
	header, message, _ := strings.Cut(object, "\n\n")
	for _, line := range strings.Split(header, "\n") {
		switch {
		case strings.HasPrefix(line, "tree "):
			facts.Tree = strings.TrimSpace(strings.TrimPrefix(line, "tree "))
		case strings.HasPrefix(line, "parent "):
			facts.Parents = append(facts.Parents, strings.TrimSpace(strings.TrimPrefix(line, "parent ")))
		case strings.HasPrefix(line, "committer "):
			facts.Date = parseGitCommitterDate(line)
		}
	}
	subject, _, _ := strings.Cut(message, "\n")
	facts.Subject = strings.TrimSpace(sanitizeOperationLog([]byte(subject)))
	if match := gitRevertedCommit.FindStringSubmatch(message); match != nil {
		facts.Reverts = match[1]
	}
	return facts
}

// parseGitCommitterDate reads "committer Name <email> 1700000000 +0100".
func parseGitCommitterDate(line string) time.Time {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(fields[len(fields)-2], 10, 64)
	if err != nil {
		return time.Time{}
	}
	zone := fields[len(fields)-1]
	offset := 0
	if len(zone) == 5 && (zone[0] == '+' || zone[0] == '-') {
		hours, hoursErr := strconv.Atoi(zone[1:3])
		minutes, minutesErr := strconv.Atoi(zone[3:5])
		if hoursErr == nil && minutesErr == nil {
			offset = hours*3600 + minutes*60
			if zone[0] == '-' {
				offset = -offset
			}
		}
	}
	return time.Unix(seconds, 0).In(time.FixedZone("", offset))
}

func parseGitUndoFiles(raw string) ([]domain.GitUndoFile, error) {
	records := strings.Split(raw, "\x00")
	files := []domain.GitUndoFile{}
	for index := 0; index < len(records); index += 2 {
		if records[index] == "" {
			continue
		}
		if index+1 >= len(records) {
			return nil, errors.New("Git returned incomplete file information for the last saved change")
		}
		fields := strings.Fields(strings.TrimPrefix(records[index], ":"))
		if len(fields) != 5 || fields[4] == "" {
			return nil, errors.New("Git returned malformed file information for the last saved change")
		}
		path := records[index+1]
		special := gitUndoSpecialMode(fields[0]) || gitUndoSpecialMode(fields[1])
		for _, character := range path {
			if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
				special = true
			}
		}
		files = append(files, domain.GitUndoFile{Path: path, Change: gitStatusName(fields[4][0]), Special: special})
	}
	return files, nil
}

// A symbolic link or a submodule is written by Git in ways that depend on
// more than file content.
func gitUndoSpecialMode(mode string) bool {
	return mode == "120000" || mode == "160000"
}

func unfinishedGitOperation(gitDirectory string) string {
	for _, marker := range []struct{ name, operation string }{
		{"MERGE_HEAD", "merge"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"REVERT_HEAD", "revert"},
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
	} {
		if _, err := os.Lstat(filepath.Join(gitDirectory, marker.name)); err == nil {
			return marker.operation
		}
	}
	return ""
}

// GitFileAt reads one regular file of a saved version, bounded. The boolean
// is false when that version does not contain the path.
func (Local) GitFileAt(ctx context.Context, repository, revision, path string) ([]byte, bool, error) {
	if !fullGitObjectIDPattern.MatchString(revision) {
		return nil, false, errors.New("revision is not a full Git object ID")
	}
	listing, truncated, err := runBoundedGit(ctx, repository, 4096, "--literal-pathspecs", "ls-tree", "-z", revision, "--", path)
	if err != nil {
		return nil, false, fmt.Errorf("look up %s in the saved version: %w", path, err)
	}
	if truncated {
		return nil, false, fmt.Errorf("look up %s in the saved version: the listing is too long", path)
	}
	entry := strings.TrimSuffix(listing, "\x00")
	if entry == "" {
		return nil, false, nil
	}
	description, name, found := strings.Cut(entry, "\t")
	fields := strings.Fields(description)
	if !found || name != path || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") || !fullGitObjectIDPattern.MatchString(fields[2]) {
		return nil, false, fmt.Errorf("%s is not a regular file in the saved version", path)
	}
	content, truncated, err := runBoundedGit(ctx, repository, maximumGitUndoFileBytes, "cat-file", "blob", fields[2])
	if err != nil {
		return nil, false, fmt.Errorf("read %s from the saved version: %w", path, err)
	}
	if truncated {
		return nil, false, fmt.Errorf("%s is larger than %d bytes", path, maximumGitUndoFileBytes)
	}
	return []byte(content), true, nil
}

// GitUndoProposal checks that the files of the last saved change can be put
// back as they were, and renders the patch that does it. Putting the files
// back is a checkout of the earlier tree, so the checks that protect a commit
// apply: no content-transforming attributes, no hidden index flags, no secret
// in what returns, nothing unsaved in the way.
func (local Local) GitUndoProposal(ctx context.Context, repository string, facts domain.GitUndoFacts) (domain.GitUndoProposal, error) {
	head, err := local.GitRevision(ctx, repository)
	if err != nil {
		return domain.GitUndoProposal{}, err
	}
	if head != facts.Revision {
		return domain.GitUndoProposal{}, errors.New("HEAD changed after review")
	}
	if len(facts.Parents) != 1 || !fullGitObjectIDPattern.MatchString(facts.Tree) || !fullGitObjectIDPattern.MatchString(facts.ParentTree) || len(facts.Files) == 0 {
		return domain.GitUndoProposal{}, errors.New("the last saved change has no single earlier version to go back to")
	}
	paths := make([]string, 0, len(facts.Files))
	for _, file := range facts.Files {
		if file.Special {
			return domain.GitUndoProposal{}, fmt.Errorf("%q is a symbolic link, a submodule or has control characters in its name", sanitizeOperationLog([]byte(file.Path)))
		}
		paths = append(paths, file.Path)
	}
	if err := validateGitUndoPaths(repository, facts.Files); err != nil {
		return domain.GitUndoProposal{}, err
	}
	if err := rejectGitAttributes(ctx, repository, paths); err != nil {
		return domain.GitUndoProposal{}, err
	}
	listing, truncated, err := runBoundedGit(ctx, repository, maximumGitStatusBytes, append([]string{"--literal-pathspecs", "ls-files", "-v", "-z", "--"}, paths...)...)
	if err != nil || truncated {
		return domain.GitUndoProposal{}, errors.New("cannot inspect the Git index flags of the changed files")
	}
	for _, record := range strings.Split(listing, "\x00") {
		if record != "" && record[0] != 'H' {
			return domain.GitUndoProposal{}, fmt.Errorf("%q is marked assume-unchanged or skip-worktree in Git", sanitizeOperationLog([]byte(strings.TrimPrefix(record, record[:1]+" "))))
		}
	}
	arguments := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--unified=3", facts.Tree, facts.ParentTree, "--", "."}
	for _, privatePath := range privateDeploymentPaths {
		arguments = append(arguments, ":(exclude,top)"+privatePath)
	}
	arguments = append(arguments, ":(exclude,top)"+domain.HomeFilesDirectoryName+"/imports")
	diff, truncated, err := runBoundedGit(ctx, repository, maximumGitDiffBytes, arguments...)
	if err != nil {
		return domain.GitUndoProposal{}, fmt.Errorf("render the undo: %w", err)
	}
	if truncated {
		return domain.GitUndoProposal{}, errors.New("the undo changes more than the 256 KiB review limit")
	}
	if err := rejectSecretPatch(diff); err != nil {
		return domain.GitUndoProposal{}, fmt.Errorf("the earlier version contains something that must not be saved again: %w", err)
	}
	content := redactGitDiff(diff)
	imported := 0
	for _, path := range paths {
		if homeFilesImportGitPath(path) {
			imported++
		}
	}
	if imported > 0 {
		content += fmt.Sprintf("Imported files changed: %d (their content is not shown)\n", imported)
	}
	return domain.GitUndoProposal{
		TreeID: facts.ParentTree,
		Diff:   domain.GitDiff{Scope: "undo-changes", Content: content},
	}, nil
}

// validateGitUndoPaths refuses a worktree that cannot take the earlier
// version. Git overwrites ignored files without asking, so a file that the
// saved change removed must not exist in the folder, ignored or not.
func validateGitUndoPaths(repository string, files []domain.GitUndoFile) error {
	root, err := filepath.Abs(repository)
	if err != nil {
		return fmt.Errorf("resolve Git repository path: %w", err)
	}
	for _, file := range files {
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		if file.Path == "" || filepath.IsAbs(file.Path) || filepath.ToSlash(clean) != file.Path || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || file.Path == ".git" || strings.HasPrefix(file.Path, ".git/") {
			return fmt.Errorf("%q is not a repository-relative path", sanitizeOperationLog([]byte(file.Path)))
		}
		if file.Change != "deleted" {
			continue
		}
		_, statErr := os.Lstat(filepath.Join(root, clean))
		switch {
		case statErr == nil:
			return fmt.Errorf("%q exists in this folder but is not saved in Git (it may be an ignored file); the undo would overwrite it", file.Path)
		case !errors.Is(statErr, os.ErrNotExist):
			return fmt.Errorf("%q cannot be checked: %w", file.Path, statErr)
		}
	}
	return nil
}

// UndoGitCommit saves the reviewed undo: a commit whose tree is the saved
// version before HEAD and whose parent is HEAD. The index and the files move
// to that tree first, with a two-tree read-tree that refuses to overwrite
// anything edited, so a refusal leaves HEAD where it was; then HEAD advances
// only from the reviewed revision. No hook, signing helper, remote or push is
// involved.
func (local Local) UndoGitCommit(ctx context.Context, repository string, facts domain.GitUndoFacts, expectedTree, message string) (string, error) {
	revision, err := local.GitRevision(ctx, repository)
	if err != nil {
		return "", err
	}
	if revision != facts.Revision {
		return "", errors.New("HEAD changed after review")
	}
	proposal, err := local.GitUndoProposal(ctx, repository, facts)
	if err != nil {
		return "", fmt.Errorf("revalidate the undo: %w", err)
	}
	if proposal.TreeID != expectedTree {
		return "", errors.New("the saved versions changed after review")
	}
	if err := requireCleanGitWorktree(ctx, repository); err != nil {
		return "", err
	}
	commit, truncated, err := runBoundedGitInput(ctx, repository, []byte(strings.TrimRight(message, "\n")+"\n"), 256, "commit-tree", expectedTree, "-p", revision)
	if err != nil {
		return "", fmt.Errorf("create the undo commit object: %w", err)
	}
	commit = strings.TrimSpace(commit)
	if truncated || !fullGitObjectIDPattern.MatchString(commit) {
		return "", errors.New("created Git commit object ID is invalid")
	}
	if _, _, err := runBoundedGit(ctx, repository, 16*1024, "read-tree", "-m", "-u", revision, commit); err != nil {
		return "", fmt.Errorf("the files could not be moved back: %w", err)
	}
	if _, _, err := runBoundedGit(ctx, repository, 16*1024, "update-ref", "-m", "nixorium undo of the last saved change", "HEAD", commit, revision); err != nil {
		if _, _, rollback := runBoundedGit(ctx, repository, 16*1024, "read-tree", "-m", "-u", commit, revision); rollback != nil {
			return "", fmt.Errorf("advance HEAD to the undo commit: %w; the files also could not be put back, so run `nixorium git review`", err)
		}
		return "", fmt.Errorf("advance HEAD to the undo commit: %w", err)
	}
	current, err := local.GitRevision(ctx, repository)
	if err != nil {
		return "", fmt.Errorf("verify the new Git revision: %w", err)
	}
	if current == revision {
		return "", errors.New("the undo did not advance HEAD")
	}
	if err := requireCleanGitWorktree(ctx, repository); err != nil {
		return current, fmt.Errorf("the undo was saved but %w", err)
	}
	return current, nil
}

func requireCleanGitWorktree(ctx context.Context, repository string) error {
	status, truncated, err := runBoundedGit(ctx, repository, maximumGitStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect Git worktree: %w", err)
	}
	if truncated || status != "" {
		return errors.New("the configuration has unsaved changes")
	}
	return nil
}
