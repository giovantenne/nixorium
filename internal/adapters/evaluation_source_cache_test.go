package adapters

import (
	"fmt"
	"sync"
	"testing"
)

func testEvaluationCache(present map[string]bool) *evaluationSourceCache {
	return &evaluationSourceCache{limit: 3, exists: func(path string) bool { return present[path] }}
}

func testEvaluationKey() evaluationSourceKey {
	return evaluationSourceKey{metadataPath: "/nix/store/a-source", revision: "r1", credentials: "digest", sourceRevision: "s1"}
}

func TestEvaluationSourceCacheHitAndMisses(t *testing.T) {
	const stored = "/nix/store/prepared-source"
	cache := testEvaluationCache(map[string]bool{stored: true})
	key := testEvaluationKey()
	if _, ok := cache.lookup(key); ok {
		t.Fatal("empty cache hit")
	}
	cache.store(key, stored)
	if got, ok := cache.lookup(key); !ok || got != stored {
		t.Fatalf("lookup = %q, %v", got, ok)
	}
	for name, change := range map[string]func(*evaluationSourceKey){
		"tracked change":       func(k *evaluationSourceKey) { k.metadataPath = "/nix/store/b-source" },
		"requested revision":   func(k *evaluationSourceKey) { k.revision = "r2" },
		"changed credentials":  func(k *evaluationSourceKey) { k.credentials = "other" },
		"injected source file": func(k *evaluationSourceKey) { k.sourceRevision = "s2" },
	} {
		changed := key
		change(&changed)
		if _, ok := cache.lookup(changed); ok {
			t.Fatalf("%s hit the cache", name)
		}
	}
}

func TestEvaluationSourceCacheDropsMissingStorePath(t *testing.T) {
	const stored = "/nix/store/prepared-source"
	present := map[string]bool{stored: true}
	cache := testEvaluationCache(present)
	key := testEvaluationKey()
	cache.store(key, stored)
	delete(present, stored)
	if _, ok := cache.lookup(key); ok {
		t.Fatal("vanished store path reused")
	}
	if len(cache.entries) != 0 {
		t.Fatalf("stale entry kept: %v", cache.entries)
	}
	present["/nix/store/new"] = true
	cache.store(key, "/nix/store/new")
	if got, ok := cache.lookup(key); !ok || got != "/nix/store/new" {
		t.Fatalf("recomputed entry = %q, %v", got, ok)
	}
}

func TestEvaluationSourceCacheIsBoundedAndReplacesSameKey(t *testing.T) {
	present := map[string]bool{}
	cache := testEvaluationCache(present)
	for index := range 5 {
		key := testEvaluationKey()
		key.metadataPath = fmt.Sprintf("/nix/store/%d-source", index)
		path := fmt.Sprintf("/nix/store/%d-prepared", index)
		present[path] = true
		cache.store(key, path)
	}
	if len(cache.entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(cache.entries))
	}
	oldest := testEvaluationKey()
	oldest.metadataPath = "/nix/store/0-source"
	if _, ok := cache.lookup(oldest); ok {
		t.Fatal("oldest entry kept")
	}
	newest := testEvaluationKey()
	newest.metadataPath = "/nix/store/4-source"
	if _, ok := cache.lookup(newest); !ok {
		t.Fatal("newest entry lost")
	}
	cache.store(newest, "/nix/store/4-replaced")
	if len(cache.entries) != 3 {
		t.Fatalf("replacing grew the cache to %d", len(cache.entries))
	}
}

func TestEvaluationSourceCacheIsSafeForConcurrentUse(t *testing.T) {
	present := map[string]bool{"/nix/store/p": true}
	cache := testEvaluationCache(present)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.store(testEvaluationKey(), "/nix/store/p")
			cache.lookup(testEvaluationKey())
		}()
	}
	wg.Wait()
	if len(cache.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(cache.entries))
	}
}
