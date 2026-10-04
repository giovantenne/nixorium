package main

import (
	"context"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// internetWatcher reads the real Internet state of every computer while the
// classroom view page is open, with the same read-only check the Internet
// review uses, so cards and actions follow changes made elsewhere (the TUI,
// a restart) and not only the page's own.
type internetWatcher struct {
	observe  func(context.Context, domain.HostMeta) domain.InternetObservation
	hosts    func(context.Context) ([]domain.HostMeta, error)
	record   func(name, state string)
	interval time.Duration
	idle     time.Duration
	mutex    sync.Mutex
	wanted   time.Time
	running  bool
}

const internetWatchParallel = 8

// Touch keeps the watcher running while the page asks for computers.
func (watcher *internetWatcher) Touch() {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	watcher.wanted = time.Now()
	if !watcher.running {
		watcher.running = true
		go watcher.run()
	}
}

func (watcher *internetWatcher) watched() bool {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	if time.Since(watcher.wanted) > watcher.idle {
		watcher.running = false
		return false
	}
	return true
}

func (watcher *internetWatcher) run() {
	for watcher.watched() {
		watcher.round()
		time.Sleep(watcher.interval)
	}
}

func (watcher *internetWatcher) round() {
	ctx, cancel := context.WithTimeout(context.Background(), watcher.interval)
	defer cancel()
	hosts, err := watcher.hosts(ctx)
	if err != nil {
		return
	}
	var group sync.WaitGroup
	slots := make(chan struct{}, internetWatchParallel)
	for _, host := range hosts {
		group.Add(1)
		slots <- struct{}{}
		go func() {
			defer func() { <-slots; group.Done() }()
			// A computer that cannot answer keeps its last known state.
			if observed := watcher.observe(ctx, host); observed.State == "blocked" || observed.State == "enabled" {
				watcher.record(host.Name, observed.State)
			}
		}()
	}
	group.Wait()
}
