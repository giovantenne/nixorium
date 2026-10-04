package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestInternetWatcherFollowsTheComputersWhileWatched(t *testing.T) {
	var mutex sync.Mutex
	states := map[string]string{"pc01": "blocked", "pc02": "unknown"}
	recorded := map[string]string{"pc02": "enabled"}
	rounds := 0
	watcher := &internetWatcher{
		observe: func(_ context.Context, host domain.HostMeta) domain.InternetObservation {
			mutex.Lock()
			defer mutex.Unlock()
			return domain.InternetObservation{State: states[host.Name]}
		},
		hosts: func(context.Context) ([]domain.HostMeta, error) {
			mutex.Lock()
			rounds++
			mutex.Unlock()
			return []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}}, nil
		},
		record: func(name, state string) {
			mutex.Lock()
			defer mutex.Unlock()
			recorded[name] = state
		},
		interval: 20 * time.Millisecond,
		idle:     100 * time.Millisecond,
	}
	watcher.Touch()
	read := func(name string) string {
		mutex.Lock()
		defer mutex.Unlock()
		return recorded[name]
	}
	waitUntil(t, func() bool { return read("pc01") == "blocked" })
	// A computer that cannot answer keeps its last known state.
	if read("pc02") != "enabled" {
		t.Fatalf("pc02 = %q", read("pc02"))
	}
	mutex.Lock()
	states["pc01"] = "enabled"
	mutex.Unlock()
	waitUntil(t, func() bool { return read("pc01") == "enabled" })
	// Without the page asking, the watcher stops.
	time.Sleep(300 * time.Millisecond)
	mutex.Lock()
	before := rounds
	mutex.Unlock()
	time.Sleep(100 * time.Millisecond)
	mutex.Lock()
	after := rounds
	mutex.Unlock()
	if after != before {
		t.Fatalf("the watcher kept running: %d → %d rounds", before, after)
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	for attempt := 0; attempt < 200; attempt++ {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}
