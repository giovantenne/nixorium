package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// BroadcastManager shows the teacher's screen on students' computers. Like
// a lock it changes only the students' sessions.
type BroadcastManager struct {
	source LockSource
	now    func() time.Time
}

func NewBroadcastManager(source LockSource) *BroadcastManager {
	return &BroadcastManager{source, time.Now}
}

const (
	// broadcastSilence ends a showing that receives no picture: the page
	// sends one at least every few seconds while it runs.
	broadcastSilence = 10 * time.Second
	broadcastRetry   = 3 * time.Second
)

func (m *BroadcastManager) Plan(ctx context.Context, repository, requested string) domain.BroadcastPlan {
	lock := (&LockManager{source: m.source, now: m.now}).Plan(ctx, repository, requested, domain.LockOn)
	p := domain.BroadcastPlan{SchemaVersion: domain.SchemaVersion, Operation: "broadcast-plan", State: lock.State, Repository: lock.Repository, Requested: lock.Requested, Targets: lock.Targets, ExpiresAt: lock.ExpiresAt, Issues: lock.Issues, Message: lock.Message}
	if p.HasErrors() {
		return p
	}
	eligible := 0
	for _, target := range p.Targets {
		if target.Eligible {
			eligible++
		}
	}
	p.Message = fmt.Sprintf("Your screen covers the screen of %d of %d computers, with keyboard and mouse blocked, until you stop. Computers that are off or have nobody signed in get it when someone signs in.", eligible, len(p.Targets))
	p.ReviewToken = domain.BroadcastReviewToken(p)
	return p
}

// Broadcast is one running showing of the teacher's screen.
type Broadcast struct {
	mutex   sync.Mutex
	changed *sync.Cond
	frame   []byte
	number  int64
	last    time.Time
	stopped bool
	states  map[string]string
	now     func() time.Time
	done    chan struct{}
}

// Start begins showing on the reviewed computers. Every computer, including
// those that were not ready at the review, is tried until the showing ends.
func (m *BroadcastManager) Start(ctx context.Context, p domain.BroadcastPlan, token string) (*Broadcast, error) {
	if p.HasErrors() || token == "" || token != p.ReviewToken || token != domain.BroadcastReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		return nil, errors.New("Review expired or changed; create a fresh plan.")
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		return nil, err
	}
	identities := map[string]string{}
	for _, host := range meta.Clients.Hosts {
		identities[host.Name] = host.IP
	}
	for _, target := range p.Targets {
		if target.Name == meta.Controller.Name || identities[target.Name] != target.IP {
			return nil, errors.New("Client inventory changed; review again.")
		}
	}
	broadcast := &Broadcast{states: map[string]string{}, now: m.now, last: m.now(), done: make(chan struct{})}
	broadcast.changed = sync.NewCond(&broadcast.mutex)
	var group sync.WaitGroup
	for _, target := range p.Targets {
		broadcast.states[target.Name] = "connecting"
		group.Add(1)
		go func() {
			defer group.Done()
			broadcast.show(m.source, target.HostMeta)
		}()
	}
	go broadcast.watch()
	go func() {
		group.Wait()
		close(broadcast.done)
	}()
	return broadcast, nil
}

// Frame offers the latest picture; slow computers skip pictures.
func (broadcast *Broadcast) Frame(image []byte) error {
	if len(image) == 0 || len(image) > classroomview.MaxBroadcastBytes {
		return errors.New("the picture is empty or too large")
	}
	broadcast.mutex.Lock()
	defer broadcast.mutex.Unlock()
	if broadcast.stopped {
		return errors.New("the showing has ended")
	}
	broadcast.frame = append([]byte(nil), image...)
	broadcast.number++
	broadcast.last = broadcast.now()
	broadcast.changed.Broadcast()
	return nil
}

func (broadcast *Broadcast) Stop() {
	broadcast.mutex.Lock()
	defer broadcast.mutex.Unlock()
	broadcast.stopped = true
	broadcast.changed.Broadcast()
}

// Done is closed when every computer has stopped showing.
func (broadcast *Broadcast) Done() <-chan struct{} { return broadcast.done }

func (broadcast *Broadcast) Stopped() bool {
	broadcast.mutex.Lock()
	defer broadcast.mutex.Unlock()
	return broadcast.stopped
}

// States reports each computer: connecting, showing or waiting.
func (broadcast *Broadcast) States() map[string]string {
	broadcast.mutex.Lock()
	defer broadcast.mutex.Unlock()
	states := make(map[string]string, len(broadcast.states))
	for name, state := range broadcast.states {
		states[name] = state
	}
	return states
}

func (broadcast *Broadcast) watch() {
	for {
		time.Sleep(time.Second)
		broadcast.mutex.Lock()
		if broadcast.stopped {
			broadcast.mutex.Unlock()
			return
		}
		if broadcast.now().Sub(broadcast.last) > broadcastSilence {
			broadcast.stopped = true
			broadcast.changed.Broadcast()
		}
		broadcast.mutex.Unlock()
	}
}

func (broadcast *Broadcast) setState(name, state string) {
	broadcast.mutex.Lock()
	broadcast.states[name] = state
	broadcast.mutex.Unlock()
}

// next waits for a picture newer than sent, or the end.
func (broadcast *Broadcast) next(sent int64) ([]byte, int64, bool) {
	broadcast.mutex.Lock()
	defer broadcast.mutex.Unlock()
	for !broadcast.stopped && broadcast.number <= sent {
		broadcast.changed.Wait()
	}
	return broadcast.frame, broadcast.number, !broadcast.stopped
}

func (broadcast *Broadcast) show(source LockSource, host domain.HostMeta) {
	for !broadcast.Stopped() {
		session, err := source.Connect(context.Background(), host)
		if err != nil {
			broadcast.setState(host.Name, "waiting")
			broadcast.pause()
			continue
		}
		sent := int64(0)
		for {
			frame, number, running := broadcast.next(sent)
			if !running {
				_ = session.StopBroadcast()
				_ = session.Close()
				broadcast.setState(host.Name, "stopped")
				return
			}
			if err := session.ShowFrame(frame); err != nil {
				_ = session.Close()
				broadcast.setState(host.Name, "waiting")
				broadcast.pause()
				break
			}
			sent = number
			broadcast.setState(host.Name, "showing")
		}
	}
	broadcast.setState(host.Name, "stopped")
}

// pause waits before trying a computer again, ending early when stopped.
func (broadcast *Broadcast) pause() {
	deadline := time.Now().Add(broadcastRetry)
	for time.Now().Before(deadline) && !broadcast.Stopped() {
		time.Sleep(200 * time.Millisecond)
	}
}
