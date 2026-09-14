package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	maximumGitStatusBytes = 1024 * 1024
	maximumGitChanges     = 1000
	maximumGitDiffBytes   = 256 * 1024
)

var managedDeploymentPaths = map[string]bool{
	"lab-settings.json":         true,
	"keys/cache-public-key":     true,
	"keys/admin-ssh.pub":        true,
	"keys/veyon-public-key.pem": true,
}

var passwordJSONValue = regexp.MustCompile(`("(?:admin|teacher|student)Password"[[:space:]]*:[[:space:]]*)"(?:\\.|[^"\\])*"`)
var fullGitObjectIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type boundedCommandBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (writer *boundedCommandBuffer) Write(content []byte) (int, error) {
	written := len(content)
	remaining := writer.limit - writer.buffer.Len()
	if remaining > 0 {
		if remaining > len(content) {
			remaining = len(content)
		}
		_, _ = writer.buffer.Write(content[:remaining])
	}
	if remaining < len(content) {
		writer.truncated = true
	}
	return written, nil
}

func (Local) GitReview(ctx context.Context, repository string) (domain.GitReviewSnapshot, error) {
	status, truncated, err := runBoundedGit(ctx, repository, maximumGitStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return domain.GitReviewSnapshot{}, fmt.Errorf("inspect Git worktree: %w", err)
	}
	if truncated {
		return domain.GitReviewSnapshot{}, errors.New("Git worktree status exceeds the 1 MiB safety limit")
	}
	changes, err := parseGitPorcelain(status)
	if err != nil {
		return domain.GitReviewSnapshot{}, err
	}
	if len(changes) > maximumGitChanges {
		return domain.GitReviewSnapshot{}, fmt.Errorf("Git worktree has more than %d changed paths", maximumGitChanges)
	}
	snapshot := domain.GitReviewSnapshot{Changes: changes, Diffs: []domain.GitDiff{}}
	for _, change := range changes {
		if change.Private {
			return snapshot, nil
		}
	}
	for _, scope := range []struct {
		name string
		args []string
	}{
		{name: "staged", args: gitReviewDiffArguments(true)},
		{name: "unstaged", args: gitReviewDiffArguments(false)},
	} {
		content, diffTruncated, diffErr := runBoundedGit(ctx, repository, maximumGitDiffBytes, scope.args...)
		if diffErr != nil {
			return domain.GitReviewSnapshot{}, fmt.Errorf("read %s Git diff: %w", scope.name, diffErr)
		}
		content = redactGitDiff(content)
		if content != "" || diffTruncated {
			snapshot.Diffs = append(snapshot.Diffs, domain.GitDiff{Scope: scope.name, Content: content, Truncated: diffTruncated})
		}
	}
	currentStatus, currentTruncated, err := runBoundedGit(ctx, repository, maximumGitStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return domain.GitReviewSnapshot{}, fmt.Errorf("recheck Git worktree: %w", err)
	}
	if currentTruncated || currentStatus != status {
		return domain.GitReviewSnapshot{}, errors.New("Git worktree changed during review; retry against a stable worktree")
	}
	return snapshot, nil
}

func gitReviewDiffArguments(staged bool) []string {
	arguments := []string{"diff"}
	if staged {
		arguments = append(arguments, "--cached")
	}
	arguments = append(arguments, "--no-ext-diff", "--no-textconv", "--no-color", "--unified=3", "--", ".")
	for _, privatePath := range privateDeploymentPaths {
		arguments = append(arguments, ":(exclude,top)"+privatePath)
	}
	return arguments
}

func parseGitPorcelain(output string) ([]domain.GitChange, error) {
	records := strings.Split(output, "\x00")
	changes := make([]domain.GitChange, 0, len(records))
	for index := 0; index < len(records); index++ {
		record := records[index]
		if record == "" {
			continue
		}
		if len(record) < 4 || record[2] != ' ' {
			return nil, errors.New("Git returned malformed porcelain status")
		}
		x, y := record[0], record[1]
		path := record[3:]
		original := ""
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			index++
			if index >= len(records) || records[index] == "" {
				return nil, errors.New("Git returned an incomplete rename status")
			}
			original = records[index]
		}
		change := domain.GitChange{
			Path:         sanitizeOperationLog([]byte(path)),
			OriginalPath: sanitizeOperationLog([]byte(original)),
			Managed:      managedDeploymentPaths[filepath.ToSlash(path)] || managedDeploymentPaths[filepath.ToSlash(original)],
			Private:      isPrivateDeploymentPath(path) || isPrivateDeploymentPath(original),
		}
		if x == '?' && y == '?' {
			change.Untracked = true
		} else if isUnmergedGitStatus(x, y) {
			change.Staged = "conflict"
			change.Unstaged = "conflict"
		} else {
			change.Staged = gitStatusName(x)
			change.Unstaged = gitStatusName(y)
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func isUnmergedGitStatus(index, worktree byte) bool {
	status := string([]byte{index, worktree})
	switch status {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	default:
		return false
	}
}

func gitStatusName(status byte) string {
	switch status {
	case ' ':
		return ""
	case 'A':
		return "added"
	case 'M':
		return "modified"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "type-changed"
	case 'U':
		return "conflict"
	default:
		return "unknown"
	}
}

func isPrivateDeploymentPath(path string) bool {
	path = filepath.ToSlash(path)
	for _, privatePath := range privateDeploymentPaths {
		if path == privatePath {
			return true
		}
	}
	return false
}

func redactGitDiff(content string) string {
	content = strings.ToValidUTF8(content, "�")
	content = passwordJSONValue.ReplaceAllString(content, `${1}"<redacted>"`)
	return sanitizeOperationLog([]byte(content))
}

func runBoundedGit(ctx context.Context, repository string, limit int, arguments ...string) (string, bool, error) {
	return runBoundedGitWithEnvironment(ctx, repository, nil, limit, arguments...)
}

func runBoundedGitWithEnvironment(ctx context.Context, repository string, environment []string, limit int, arguments ...string) (string, bool, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, arguments...)...)
	if environment != nil {
		command.Env = environment
	}
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
