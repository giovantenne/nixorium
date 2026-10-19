package adapters

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const maximumCandidateSourceBytes = 512 * 1024 * 1024

// candidateFlakeSource returns an immutable local flake made of the committed
// deployment tree with reviewed replacements for flake.nix and flake.lock.
//
// Nix records the "original" reference of a locked input from the flake.nix
// it locks. A candidate lock must therefore be computed from the proposed
// flake.nix, not from the current one with an override, or the saved pair
// disagrees and every immutable copy of the deployment (Colmena) fails.
// Account hashes are injected exactly as for EvaluationSource; private keys
// and ignored files are never copied.
func candidateFlakeSource(ctx context.Context, repository string, replacements map[string][]byte) (string, error) {
	for name := range replacements {
		if name != "flake.nix" && name != "flake.lock" && !validMigrationFile(name) && name != domain.DeploymentMigrationsFile {
			return "", fmt.Errorf("candidate source cannot replace %s", name)
		}
	}
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return "", err
	}
	revision, err := runOutput(ctx, "git", "-C", repository, "rev-parse", "--verify", "HEAD^{commit}")
	revision = strings.TrimSpace(revision)
	if err != nil || !validGitRevision(revision) {
		return "", errors.New("candidate source needs a committed deployment revision")
	}
	stage, err := os.MkdirTemp("", "nixorium-update-candidate-*")
	if err != nil {
		return "", fmt.Errorf("create candidate source: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := extractCommittedTree(ctx, repository, revision, stage); err != nil {
		return "", fmt.Errorf("copy committed deployment source: %w", err)
	}
	credentials, _, err := readCredentials(repository)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return "", err
	default:
		if err := injectEvaluationAccounts(stage, stage, credentials); err != nil {
			return "", err
		}
	}
	data, _ := json.Marshal(revision)
	if err := os.WriteFile(filepath.Join(stage, ".nixorium-source-revision.json"), data, 0600); err != nil {
		return "", err
	}
	for name, content := range replacements {
		path := filepath.Join(stage, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		// A nil replacement removes the file (a deployment migration).
		if content == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			return "", fmt.Errorf("write candidate %s: %w", name, err)
		}
	}
	output, err := runBoundedNix(ctx, 4096, "store", "add-path", "--name", "source", stage)
	if err != nil {
		return "", errors.New("cannot import the candidate deployment source")
	}
	storePath := strings.TrimSpace(output)
	if !domain.ValidStorePath(storePath) {
		return "", errors.New("Nix returned an invalid candidate source path")
	}
	return "path:" + storePath, nil
}

// extractCommittedTree writes the tracked files of one commit into target.
// The update planner requires a clean worktree, so this is the same content
// the Git fetcher would read.
func extractCommittedTree(ctx context.Context, repository, revision, target string) error {
	command := exec.CommandContext(ctx, "git", "-C", repository, "archive", "--format=tar", revision)
	configureCommandCancellation(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &boundedCommandBuffer{limit: 16 * 1024}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return err
	}
	extractErr := extractCandidateArchive(io.LimitReader(stdout, maximumCandidateSourceBytes+1), target)
	if extractErr != nil {
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := command.Wait()
	if extractErr != nil {
		return extractErr
	}
	if waitErr != nil {
		return fmt.Errorf("git archive: %s", sanitizeOperationLog(stderr.buffer.Bytes()))
	}
	return nil
}

func extractCandidateArchive(input io.Reader, target string) error {
	reader := tar.NewReader(input)
	total := int64(0)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		relative := filepath.Clean(strings.TrimSuffix(header.Name, "/"))
		if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") {
			return errors.New("deployment source contains an unsafe path")
		}
		for _, private := range privateDeploymentPaths {
			if relative == private {
				return errors.New("private file found in the tracked deployment source")
			}
		}
		if relative == ".nixorium-source-revision.json" {
			return errors.New("reserved evaluation metadata must not be tracked")
		}
		destination := filepath.Join(target, relative)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(destination, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
		case tar.TypeSymlink:
			resolved := filepath.Clean(filepath.Join(filepath.Dir(relative), header.Linkname))
			if filepath.IsAbs(header.Linkname) || resolved == ".." || strings.HasPrefix(resolved, "../") {
				return errors.New("deployment source contains an escaping symlink")
			}
			if err := os.Symlink(header.Linkname, destination); err != nil {
				return err
			}
		case tar.TypeReg:
			total += header.Size
			if header.Size < 0 || total > maximumCandidateSourceBytes {
				return errors.New("deployment source exceeds the candidate size limit")
			}
			file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(0600|(header.Mode&0111)))
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("deployment source contains a special file")
		}
	}
}
