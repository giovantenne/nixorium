package main

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeMetaSource struct {
	evaluations int
	revision    string
	dirty       bool
	fail        bool
}

func (source *fakeMetaSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	source.evaluations++
	if source.fail {
		return domain.LabMeta{}, errors.New("evaluation failed")
	}
	meta := domain.LabMeta{}
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01-" + source.revision}}
	return meta, nil
}

func (source *fakeMetaSource) GitState(context.Context, string) (domain.GitState, error) {
	return domain.GitState{Available: true, Dirty: source.dirty}, nil
}

func (source *fakeMetaSource) GitRevision(context.Context, string) (string, error) {
	return source.revision, nil
}

func TestMetaCacheReusesOnlyACleanUnchangedRevision(t *testing.T) {
	source := &fakeMetaSource{revision: "a"}
	cache := &metaCache{source: source}
	load := func() string {
		meta, err := cache.load(context.Background(), "/srv/lab")
		if err != nil {
			t.Fatal(err)
		}
		return meta.Clients.Hosts[0].Name
	}
	load()
	if load() != "pc01-a" || source.evaluations != 1 {
		t.Fatalf("clean revision evaluated %d times", source.evaluations)
	}
	source.revision = "b"
	if load() != "pc01-b" || source.evaluations != 2 {
		t.Fatalf("new revision not evaluated: %d", source.evaluations)
	}
	source.dirty = true
	load()
	load()
	if source.evaluations != 4 {
		t.Fatalf("dirty worktree reused a cached evaluation: %d", source.evaluations)
	}
	source.dirty, source.fail = false, true
	if _, err := cache.load(context.Background(), "/srv/lab"); err == nil {
		t.Fatal("failed evaluation was hidden by the cache")
	}
}
