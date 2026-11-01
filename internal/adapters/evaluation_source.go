package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/giovantenne/nixorium/internal/domain"
)

// EvaluationSource starts from the Git fetcher's tracked-only source, then
// injects only account hashes. Private keys and other ignored files are never
// copied. The resulting immutable source is local Nix input, never Git input.
func (Local) EvaluationSource(ctx context.Context, repository, revision string) (string, error) {
	reference, err := rawDeploymentFlakeReference(repository)
	if err != nil {
		return "", err
	}
	if revision != "" {
		if !validGitRevision(revision) {
			return "", errors.New("invalid evaluation revision")
		}
		reference += "?rev=" + revision
	}
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return "", err
	}
	credentials, credentialsData, err := readCredentials(repository)
	if errors.Is(err, os.ErrNotExist) {
		return reference, nil
	}
	if err != nil {
		return "", err
	}
	output, err := runBoundedNix(ctx, 4*1024*1024, "flake", "metadata", reference, "--json", "--no-write-lock-file", "--no-update-lock-file")
	if err != nil {
		return "", errors.New("cannot prepare the tracked deployment source; check the pinned flake inputs")
	}
	var metadata struct {
		Path          string `json:"path"`
		Revision      string `json:"revision"`
		DirtyRevision string `json:"dirtyRevision"`
	}
	if json.Unmarshal([]byte(output), &metadata) != nil || !domain.ValidStorePath(metadata.Path) || (revision != "" && metadata.Revision != revision) {
		return "", errors.New("Nix returned an invalid deployment source")
	}
	sourceRevision := metadata.Revision
	if sourceRevision == "" {
		sourceRevision = metadata.DirtyRevision
	}
	// metadata.Path is content-addressed, so the same key always produces the
	// same prepared source. Only the copy, injection and import are skipped;
	// the metadata call above still detects every tracked change.
	digest := sha256.Sum256(credentialsData)
	key := evaluationSourceKey{
		metadataPath:   metadata.Path,
		revision:       revision,
		credentials:    hex.EncodeToString(digest[:]),
		sourceRevision: sourceRevision,
	}
	if storePath, ok := preparedEvaluationSources.lookup(key); ok {
		return "path:" + storePath, nil
	}
	// Newer Nix versions can return a lazy source path from metadata without
	// materializing it. Archive the pinned source before reading its files.
	if _, err := os.Stat(metadata.Path); errors.Is(err, os.ErrNotExist) {
		archived, err := runBoundedNix(ctx, 4*1024*1024, "flake", "archive", reference, "--json", "--no-write-lock-file", "--no-update-lock-file")
		var result struct {
			Path string `json:"path"`
		}
		if err != nil || json.Unmarshal([]byte(archived), &result) != nil || result.Path != metadata.Path {
			return "", errors.New("cannot materialize the reviewed deployment source; retry after checking pinned inputs")
		}
	}
	stage, err := os.MkdirTemp("", "nixorium-evaluation-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := copyEvaluationSource(metadata.Path, stage); err != nil {
		return "", fmt.Errorf("copy tracked source: %w", err)
	}
	if err := injectEvaluationAccounts(stage, metadata.Path, credentials); err != nil {
		return "", err
	}
	if sourceRevision != "" {
		data, _ := json.Marshal(sourceRevision)
		if err := os.WriteFile(filepath.Join(stage, ".nixorium-source-revision.json"), data, 0600); err != nil {
			return "", err
		}
	}
	output, err = runBoundedNix(ctx, 4096, "store", "add-path", "--name", "source", stage)
	if err != nil {
		return "", errors.New("cannot import the prepared local evaluation source")
	}
	storePath := strings.TrimSpace(output)
	if !domain.ValidStorePath(storePath) {
		return "", errors.New("Nix returned an invalid prepared source path")
	}
	preparedEvaluationSources.store(key, storePath)
	return "path:" + storePath, nil
}

// evaluationSourceCacheLimit bounds how many prepared sources one process
// remembers; a review, an update and their phases use the latest one or two.
const evaluationSourceCacheLimit = 8

// evaluationSourceKey identifies one prepared source. The credentials are
// represented only by the SHA-256 of the file, never by their content.
type evaluationSourceKey struct {
	metadataPath   string
	revision       string
	credentials    string
	sourceRevision string
}

type evaluationSourceEntry struct {
	key       evaluationSourceKey
	storePath string
}

// evaluationSourceCache remembers, for this process only, the store path made
// from a key. It is never written to disk. An entry is reused only while its
// store path still exists, so garbage collection falls back to recomputing.
type evaluationSourceCache struct {
	mu      sync.Mutex
	limit   int
	entries []evaluationSourceEntry // oldest first
	exists  func(string) bool
}

var preparedEvaluationSources = &evaluationSourceCache{
	limit: evaluationSourceCacheLimit,
	exists: func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	},
}

func (c *evaluationSourceCache) lookup(key evaluationSourceKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, entry := range c.entries {
		if entry.key != key {
			continue
		}
		if !c.exists(entry.storePath) {
			c.entries = append(c.entries[:index], c.entries[index+1:]...)
			return "", false
		}
		return entry.storePath, true
	}
	return "", false
}

func (c *evaluationSourceCache) store(key evaluationSourceKey, storePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, entry := range c.entries {
		if entry.key == key {
			c.entries = append(c.entries[:index], c.entries[index+1:]...)
			break
		}
	}
	c.entries = append(c.entries, evaluationSourceEntry{key, storePath})
	if extra := len(c.entries) - c.limit; extra > 0 {
		c.entries = append([]evaluationSourceEntry(nil), c.entries[extra:]...)
	}
}

// injectEvaluationAccounts reads the tracked settings from source and writes
// them, with the account hashes of the private credentials file, into stage.
func injectEvaluationAccounts(stage, source string, credentials domain.LabCredentials) error {
	public, _, err := readRecoveryFile(filepath.Join(source, settingsFileName), 1024*1024)
	if err != nil {
		return fmt.Errorf("read tracked settings from %s: %w", source, err)
	}
	settings, issues := domain.DecodeLabSettings(public)
	if len(issues) != 0 {
		return domain.Blocked("SETTINGS-INVALID", errors.New("deployment settings are invalid; repair them in Change settings"))
	}
	if settings.Lab.CredentialsVersion != credentials.Version {
		return errors.New("account credentials do not match this configuration; set all passwords again or restore its complete backup")
	}
	if settings.Lab.AdminPassword != "" || settings.Lab.TeacherPassword != "" || settings.Lab.StudentPassword != "" {
		return errors.New("refusing password hashes in tracked settings")
	}
	settings.Lab.AdminPassword, settings.Lab.TeacherPassword, settings.Lab.StudentPassword = credentials.Admin, credentials.Teacher, credentials.Student
	injected, err := domain.MarshalLabSettings(settings)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, settingsFileName), injected, 0600)
}

func copyEvaluationSource(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		for _, private := range privateDeploymentPaths {
			if relative == private {
				return errors.New("private file found in the tracked evaluation source")
			}
		}
		if relative == ".nixorium-source-revision.json" {
			return errors.New("reserved evaluation metadata must not be tracked")
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.Mkdir(destination, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Relative links must stay in the immutable source; no link may
			// import an unrelated local file.
			resolved := filepath.Clean(filepath.Join(filepath.Dir(relative), link))
			if filepath.IsAbs(link) || resolved == ".." || strings.HasPrefix(resolved, "../") {
				return errors.New("deployment source contains an escaping symlink")
			}
			return os.Symlink(link, destination)
		}
		if !entry.Type().IsRegular() {
			return errors.New("deployment source contains a special file")
		}
		data, mode, err := readRecoveryFile(path, 128*1024*1024)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, os.FileMode(0600|(mode&0111)))
	})
}
