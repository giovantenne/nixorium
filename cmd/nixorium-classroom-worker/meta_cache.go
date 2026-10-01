package main

import (
	"context"
	"sync"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// metaSource is the part of the local adapter the cache needs.
type metaSource interface {
	LabMetaAtRevision(context.Context, string, string) (domain.LabMeta, error)
	GitRevision(context.Context, string) (string, error)
}

// cachedLocal evaluates labMeta of the committed revision (HEAD) once per
// revision. Uncommitted edits by the administrator therefore never change or
// break the classroom controls, and repeated requests skip Nix evaluation.
type cachedLocal struct {
	adapters.Local
	meta *metaCache
}

type metaCache struct {
	source   metaSource
	mutex    sync.Mutex
	revision string
	value    domain.LabMeta
}

func newCachedLocal() cachedLocal {
	local := adapters.Local{}
	return cachedLocal{Local: local, meta: &metaCache{source: local}}
}

func (local cachedLocal) LabMeta(ctx context.Context, repository string) (domain.LabMeta, error) {
	return local.meta.load(ctx, repository)
}

func (cache *metaCache) load(ctx context.Context, repository string) (domain.LabMeta, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	revision, err := cache.source.GitRevision(ctx, repository)
	if err != nil {
		return domain.LabMeta{}, err
	}
	if revision == cache.revision {
		return cache.value, nil
	}
	meta, err := cache.source.LabMetaAtRevision(ctx, repository, revision)
	if err != nil {
		return meta, err
	}
	cache.revision, cache.value = revision, meta
	return meta, nil
}
