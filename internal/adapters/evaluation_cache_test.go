package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func useEvaluationCacheDirectory(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "evaluations")
	previous := evaluationCacheDirectory
	evaluationCacheDirectory = func() (string, error) { return directory, nil }
	t.Cleanup(func() { evaluationCacheDirectory = previous })
	return directory
}

func TestEvaluationCacheOnlyKeepsReadinessOfPreparedSources(t *testing.T) {
	useEvaluationCacheDirectory(t)
	source := "path:/nix/store/0zqqvpdl52lb87zqq7jwhjkqfhb68rlf-source"
	path, cacheable := evaluationCachePath(source, "deploymentStatus")
	if !cacheable {
		t.Fatal("a prepared source must be cacheable")
	}
	for _, refused := range []struct{ flake, attribute string }{
		{"git+file:///home/admin/nixorium-deployment", "deploymentStatus"},
		{"path:/home/admin/nixorium-deployment", "deploymentStatus"},
		{source, "labMeta"},
	} {
		if _, cacheable := evaluationCachePath(refused.flake, refused.attribute); cacheable {
			t.Fatalf("%s#%s must not be cached", refused.flake, refused.attribute)
		}
	}
	other, _ := evaluationCachePath("path:/nix/store/1zqqvpdl52lb87zqq7jwhjkqfhb68rlf-source", "deploymentStatus")
	if other == path {
		t.Fatal("another source must have another entry")
	}

	if _, found := readCachedEvaluation(path); found {
		t.Fatal("nothing cached yet")
	}
	writeCachedEvaluation(path, []byte(`{"ready":true,"issues":[]}`))
	if data, found := readCachedEvaluation(path); !found || string(data) != `{"ready":true,"issues":[]}` {
		t.Fatalf("cached = %q, %v", data, found)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache file mode = %v, %v", info, err)
	}
	writeCachedEvaluation(other, []byte(`{"ready":`))
	if _, found := readCachedEvaluation(other); found {
		t.Fatal("invalid JSON must never be cached")
	}
	if err := os.WriteFile(other, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found := readCachedEvaluation(other); found {
		t.Fatal("a damaged entry must be evaluated again")
	}
}

func TestEvaluationCacheKeepsTheMostRecentResults(t *testing.T) {
	directory := useEvaluationCacheDirectory(t)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	for index := range maximumCachedEvaluations {
		name := filepath.Join(directory, fmt.Sprintf("%02d.json", index))
		if err := os.WriteFile(name, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := old.Add(time.Duration(index) * time.Second)
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	path, _ := evaluationCachePath("path:/nix/store/0zqqvpdl52lb87zqq7jwhjkqfhb68rlf-source", "deploymentStatus")
	writeCachedEvaluation(path, []byte("{}"))
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != maximumCachedEvaluations {
		t.Fatalf("entries = %d, want %d", len(entries), maximumCachedEvaluations)
	}
	if _, err := os.Stat(filepath.Join(directory, "00.json")); !os.IsNotExist(err) {
		t.Fatal("the oldest result must be removed first")
	}
	if _, found := readCachedEvaluation(path); !found {
		t.Fatal("the new result must be kept")
	}
}

func TestSoftwareValidationIsRememberedPerSourceAndCandidate(t *testing.T) {
	useEvaluationCacheDirectory(t)
	source := "path:/nix/store/0zqqvpdl52lb87zqq7jwhjkqfhb68rlf-source"
	path, cacheable := softwareValidationCachePath(source, "expression", []byte(`{"packages":["htop"]}`))
	if !cacheable {
		t.Fatal("a prepared source must be cacheable")
	}
	for _, other := range []struct {
		flake, expression, candidate string
	}{
		{"path:/nix/store/1zqqvpdl52lb87zqq7jwhjkqfhb68rlf-source", "expression", `{"packages":["htop"]}`},
		{source, "another expression", `{"packages":["htop"]}`},
		{source, "expression", `{"packages":["jq"]}`},
	} {
		if otherPath, _ := softwareValidationCachePath(other.flake, other.expression, []byte(other.candidate)); otherPath == path {
			t.Fatalf("%+v shares the entry", other)
		}
	}
	if _, cacheable := softwareValidationCachePath("path:/home/admin/nixorium-deployment", "expression", nil); cacheable {
		t.Fatal("a worktree must never be cached")
	}
	if path == func() string { p, _ := evaluationCachePath(source, "deploymentStatus"); return p }() {
		t.Fatal("software validation shares the readiness entry")
	}
}
