package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// A prepared evaluation source is a content-addressed store path: any tracked
// change, pinned input or account hash gives another path, so the same path
// always evaluates to the same value. Results of the expensive readiness
// evaluation (it forces the whole workspace check, about 20 s and 1 GB) are
// therefore kept on disk and shared by the review, the post-build
// revalidation and later commands on the same source.
var cachedEvaluationAttributes = map[string]bool{"deploymentStatus": true}

const (
	maximumCachedEvaluation  = 1024 * 1024
	maximumCachedEvaluations = 64
)

// evaluationCacheDirectory is a variable so tests can use their own.
var evaluationCacheDirectory = func() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "nixorium", "evaluations"), nil
}

// evaluationCachePath names the cached result of attribute for flake, or
// reports that the result is not cacheable: only prepared store sources are,
// never a worktree or a Git reference.
func evaluationCachePath(flake, attribute string) (string, bool) {
	source, prepared := strings.CutPrefix(flake, "path:")
	if !prepared || !domain.ValidStorePath(source) || !cachedEvaluationAttributes[attribute] {
		return "", false
	}
	directory, err := evaluationCacheDirectory()
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256([]byte(source + "\x00" + attribute))
	return filepath.Join(directory, hex.EncodeToString(digest[:])+".json"), true
}

func readCachedEvaluation(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > maximumCachedEvaluation || !json.Valid(data) {
		return nil, false
	}
	return data, true
}

// writeCachedEvaluation is best effort: a failure only means the next
// command evaluates again.
func writeCachedEvaluation(path string, data []byte) {
	if len(data) > maximumCachedEvaluation || !json.Valid(data) {
		return
	}
	directory := filepath.Dir(path)
	if os.MkdirAll(directory, 0o700) != nil {
		return
	}
	pruneEvaluationCache(directory)
	file, err := os.CreateTemp(directory, ".evaluation-*")
	if err != nil {
		return
	}
	temporary := file.Name()
	_, writeErr := file.Write(data)
	if closeErr := file.Close(); writeErr != nil || closeErr != nil || os.Rename(temporary, path) != nil {
		_ = os.Remove(temporary)
	}
}

// pruneEvaluationCache keeps the most recent results, leaving room for one.
func pruneEvaluationCache(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	type cached struct {
		name    string
		modTime int64
	}
	results := []cached{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if info, err := entry.Info(); err == nil {
			results = append(results, cached{entry.Name(), info.ModTime().UnixNano()})
		}
	}
	if len(results) < maximumCachedEvaluations {
		return
	}
	slices.SortFunc(results, func(left, right cached) int {
		switch {
		case left.modTime < right.modTime:
			return -1
		case left.modTime > right.modTime:
			return 1
		}
		return strings.Compare(left.name, right.name)
	})
	for _, result := range results[:len(results)-maximumCachedEvaluations+1] {
		_ = os.Remove(filepath.Join(directory, result.name))
	}
}
