package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

var privateKeyContent = regexp.MustCompile(`(?i)(-----BEGIN (?:OPENSSH |RSA |EC |DSA )?PRIVATE KEY-----|AGE-SECRET-KEY-|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,})`)
var plaintextAssignment = regexp.MustCompile(`(?i)(password|passwd|api[_-]?key|token|secret)[[:space:]]*[:=][[:space:]]*["']([^"']+)["']`)

func (local Local) GitCommitProposal(ctx context.Context, repository string, paths []string) (domain.GitCommitProposal, error) {
	if len(paths) == 0 || len(paths) > maximumGitChanges {
		return domain.GitCommitProposal{}, errors.New("commit proposal requires between 1 and 1000 paths")
	}
	if err := validateGitCommitPathTypes(repository, paths); err != nil {
		return domain.GitCommitProposal{}, err
	}
	if err := rejectGitAttributes(ctx, repository, paths); err != nil {
		return domain.GitCommitProposal{}, err
	}
	index, err := os.CreateTemp("", "nixorium-git-index-*")
	if err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("create temporary Git index: %w", err)
	}
	indexPath := index.Name()
	if err := index.Close(); err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("close temporary Git index: %w", err)
	}
	if err := os.Remove(indexPath); err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("initialize temporary Git index: %w", err)
	}
	defer os.Remove(indexPath)
	defer os.Remove(indexPath + ".lock")
	environment := gitIndexEnvironment(indexPath)
	if _, _, err := runBoundedGitEnvironment(ctx, repository, environment, 16*1024, "read-tree", "HEAD"); err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("initialize proposed commit from HEAD: %w", err)
	}
	addArguments := append([]string{"add", "--"}, paths...)
	if _, _, err := runBoundedGitEnvironment(ctx, repository, environment, 16*1024, addArguments...); err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("stage selected paths in isolated proposal: %w", err)
	}
	tree, truncated, err := runBoundedGitEnvironment(ctx, repository, environment, 256, "write-tree")
	if err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("write proposed Git tree: %w", err)
	}
	if truncated || !fullGitObjectIDPattern.MatchString(strings.TrimSpace(tree)) {
		return domain.GitCommitProposal{}, errors.New("proposed Git tree ID is invalid")
	}
	diffArguments := []string{"diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-color", "--unified=3", "HEAD", "--"}
	diffArguments = append(diffArguments, paths...)
	diff, diffTruncated, err := runBoundedGitEnvironment(ctx, repository, environment, maximumGitDiffBytes, diffArguments...)
	if err != nil {
		return domain.GitCommitProposal{}, fmt.Errorf("render proposed commit: %w", err)
	}
	if diffTruncated {
		return domain.GitCommitProposal{}, errors.New("proposed commit diff exceeds the 256 KiB safety limit; commit these paths manually in smaller changes")
	}
	if strings.TrimSpace(diff) == "" {
		return domain.GitCommitProposal{}, errors.New("selected paths do not differ from HEAD")
	}
	if err := rejectSecretPatch(diff); err != nil {
		return domain.GitCommitProposal{}, err
	}
	return domain.GitCommitProposal{
		TreeID: strings.TrimSpace(tree),
		Diff:   domain.GitDiff{Scope: "proposed-commit", Content: redactGitDiff(diff)},
	}, nil
}

func validateGitCommitPathTypes(repository string, paths []string) error {
	root, err := filepath.Abs(repository)
	if err != nil {
		return fmt.Errorf("resolve Git repository path: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve Git repository symlinks: %w", err)
	}
	for _, path := range paths {
		clean := filepath.Clean(filepath.FromSlash(path))
		if path == "" || filepath.IsAbs(path) || filepath.ToSlash(clean) != path || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || path == ".git" || strings.HasPrefix(path, ".git/") {
			return fmt.Errorf("selected path %q is not repository-relative", path)
		}
		fullPath := filepath.Join(root, clean)
		info, statErr := os.Lstat(fullPath)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("inspect selected path %q: %w", path, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("selected path %q is a symbolic link; commit it manually after reviewing its target", path)
		}
		if info.IsDir() {
			return fmt.Errorf("selected path %q is a directory; select its changed files explicitly", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("selected path %q is not a regular file", path)
		}
		realPath, evalErr := filepath.EvalSymlinks(fullPath)
		if evalErr != nil {
			return fmt.Errorf("resolve selected path %q: %w", path, evalErr)
		}
		relative, relErr := filepath.Rel(realRoot, realPath)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("selected path %q traverses a symbolic link outside the repository", path)
		}
	}
	return nil
}

func (local Local) CommitGitPaths(ctx context.Context, repository string, paths []string, message, expectedRevision, expectedTree string) (string, error) {
	revision, err := local.GitRevision(ctx, repository)
	if err != nil {
		return "", err
	}
	if revision != expectedRevision {
		return "", errors.New("HEAD changed after review")
	}
	proposal, err := local.GitCommitProposal(ctx, repository, paths)
	if err != nil {
		return "", fmt.Errorf("revalidate commit proposal: %w", err)
	}
	if proposal.TreeID != expectedTree {
		return "", errors.New("selected path content changed after review")
	}
	commit, truncated, err := runBoundedGitInput(ctx, repository, []byte(message+"\n"), 256, "commit-tree", expectedTree, "-p", expectedRevision)
	if err != nil {
		return "", fmt.Errorf("create reviewed Git commit object: %w", err)
	}
	commit = strings.TrimSpace(commit)
	if truncated || !fullGitObjectIDPattern.MatchString(commit) {
		return "", errors.New("created Git commit object ID is invalid")
	}
	if _, _, err := runBoundedGit(ctx, repository, 16*1024, "update-ref", "-m", "nixorium reviewed commit", "HEAD", commit, expectedRevision); err != nil {
		return "", fmt.Errorf("advance HEAD to reviewed Git commit: %w", err)
	}
	resetArguments := []string{"reset", "--mixed", "--quiet", "HEAD", "--"}
	resetArguments = append(resetArguments, paths...)
	if _, _, err := runBoundedGit(ctx, repository, 16*1024, resetArguments...); err != nil {
		return "", fmt.Errorf("commit created but selected index paths could not be reconciled: %w", err)
	}
	current, err := local.GitRevision(ctx, repository)
	if err != nil {
		return "", fmt.Errorf("verify new Git revision: %w", err)
	}
	if current == revision {
		return "", errors.New("Git commit did not advance HEAD")
	}
	return current, nil
}

func runBoundedGitInput(ctx context.Context, repository string, input []byte, limit int, arguments ...string) (string, bool, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, arguments...)...)
	command.Stdin = bytes.NewReader(input)
	stdout := &boundedCommandBuffer{limit: limit}
	stderr := &boundedCommandBuffer{limit: 16 * 1024}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.buffer.String(), stdout.truncated, fmt.Errorf("git: %s", sanitizeOperationLog([]byte(message)))
	}
	return stdout.buffer.String(), stdout.truncated, nil
}

func rejectGitAttributes(ctx context.Context, repository string, paths []string) error {
	arguments := []string{"check-attr", "filter", "working-tree-encoding", "ident", "--"}
	arguments = append(arguments, paths...)
	output, truncated, err := runBoundedGit(ctx, repository, 64*1024, arguments...)
	if err != nil {
		return fmt.Errorf("inspect Git attributes: %w", err)
	}
	if truncated {
		return errors.New("Git attribute report exceeds the safety limit")
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ": unspecified") || strings.HasSuffix(line, ": unset") {
			continue
		}
		return fmt.Errorf("selected path has an active Git content-transforming attribute: %s", sanitizeOperationLog([]byte(line)))
	}
	return nil
}

func rejectSecretPatch(diff string) error {
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		added := strings.TrimPrefix(line, "+")
		if privateKeyContent.MatchString(added) {
			return errors.New("selected changes contain recognizable private key or access-token material")
		}
		match := plaintextAssignment.FindStringSubmatch(added)
		if match == nil {
			continue
		}
		value := match[2]
		if strings.HasPrefix(value, "$6$") || strings.Contains(value, "${") {
			continue
		}
		return errors.New("selected changes contain a likely plaintext password, token, API key, or secret assignment")
	}
	return nil
}

func gitIndexEnvironment(indexPath string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_INDEX_FILE=") {
			environment = append(environment, variable)
		}
	}
	return append(environment, "GIT_INDEX_FILE="+indexPath)
}

func runBoundedGitEnvironment(ctx context.Context, repository string, environment []string, limit int, arguments ...string) (string, bool, error) {
	return runBoundedGitWithEnvironment(ctx, repository, environment, limit, arguments...)
}
