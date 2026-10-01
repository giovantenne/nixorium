package main

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeMetaSource struct {
	evaluations []string
	revision    string
	fail        bool
}

func (source *fakeMetaSource) LabMetaAtRevision(_ context.Context, _ string, revision string) (domain.LabMeta, error) {
	source.evaluations = append(source.evaluations, revision)
	if source.fail {
		return domain.LabMeta{}, errors.New("evaluation failed")
	}
	meta := domain.LabMeta{}
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01-" + revision}}
	return meta, nil
}

func (source *fakeMetaSource) GitRevision(context.Context, string) (string, error) {
	return source.revision, nil
}

func TestMetaCacheEvaluatesEachCommittedRevisionOnce(t *testing.T) {
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
	if load() != "pc01-a" || len(source.evaluations) != 1 {
		t.Fatalf("revision evaluated %v", source.evaluations)
	}
	source.revision = "b"
	if load() != "pc01-b" || len(source.evaluations) != 2 || source.evaluations[1] != "b" {
		t.Fatalf("new revision not evaluated at HEAD: %v", source.evaluations)
	}
	source.revision, source.fail = "c", true
	if _, err := cache.load(context.Background(), "/srv/lab"); err == nil {
		t.Fatal("failed evaluation was hidden by the cache")
	}
	source.fail = false
	if load() != "pc01-c" {
		t.Fatal("a failed evaluation was cached")
	}
}
