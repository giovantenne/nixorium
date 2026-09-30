package main

import (
	"context"
	"sync"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// metaSource is the part of the local adapter the cache needs.
type metaSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
}

// cachedLocal evaluates labMeta once per committed revision. A clean worktree
// at the same HEAD evaluates to the same identities, so classroom requests
// reuse the result instead of starting a new Nix evaluation each time. A
// dirty worktree is always evaluated again.
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
	revision := cache.cleanRevision(ctx, repository)
	if revision != "" && revision == cache.revision {
		return cache.value, nil
	}
	meta, err := cache.source.LabMeta(ctx, repository)
	if err != nil {
		return meta, err
	}
	// Remember only a revision that was clean before and after evaluation.
	if revision != "" && cache.cleanRevision(ctx, repository) == revision {
		cache.revision, cache.value = revision, meta
	} else {
		cache.revision = ""
	}
	return meta, nil
}

func (cache *metaCache) cleanRevision(ctx context.Context, repository string) string {
	state, err := cache.source.GitState(ctx, repository)
	if err != nil || !state.Available || state.Dirty {
		return ""
	}
	revision, err := cache.source.GitRevision(ctx, repository)
	if err != nil {
		return ""
	}
	return revision
}
