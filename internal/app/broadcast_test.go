package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

type broadcastSession struct {
	fakeLockSession
	record *broadcastRecord
}

type broadcastRecord struct {
	mutex  sync.Mutex
	frames map[string][]string
	stops  map[string]int
}

func (session broadcastSession) ShowFrame(image []byte) error {
	session.record.mutex.Lock()
	defer session.record.mutex.Unlock()
	session.record.frames[session.name] = append(session.record.frames[session.name], string(image))
	return nil
}

func (session broadcastSession) StopBroadcast() error {
	session.record.mutex.Lock()
	defer session.record.mutex.Unlock()
	session.record.stops[session.name]++
	return nil
}

type broadcastSource struct {
	*fakeLockSource
	record *broadcastRecord
}

func (source broadcastSource) Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error) {
	session, err := source.fakeLockSource.Connect(ctx, host)
	if err != nil {
		return nil, err
	}
	return broadcastSession{session.(fakeLockSession), source.record}, nil
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	for attempt := 0; attempt < 300; attempt++ {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func TestBroadcastShowsTheLatestPictureAndStops(t *testing.T) {
	lab := lockLab()
	record := &broadcastRecord{frames: map[string][]string{}, stops: map[string]int{}}
	manager := NewBroadcastManager(broadcastSource{lab, record})
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02")
	if plan.HasErrors() || plan.ReviewToken == "" {
		t.Fatalf("plan = %+v", plan)
	}
	if _, err := manager.Start(context.Background(), plan, "sha256:other"); err == nil {
		t.Fatal("a wrong token started a showing")
	}
	broadcast, err := manager.Start(context.Background(), plan, plan.ReviewToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := broadcast.Frame([]byte("one")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		states := broadcast.States()
		return states["pc01"] == "showing" && states["pc02"] == "showing"
	})
	_ = broadcast.Frame([]byte("two"))
	waitFor(t, func() bool {
		record.mutex.Lock()
		defer record.mutex.Unlock()
		frames := record.frames["pc01"]
		return len(frames) > 0 && frames[len(frames)-1] == "two"
	})
	broadcast.Stop()
	select {
	case <-broadcast.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the showing did not end")
	}
	if record.stops["pc01"] != 1 || record.stops["pc02"] != 1 {
		t.Fatalf("stops = %v", record.stops)
	}
	if err := broadcast.Frame([]byte("three")); err == nil {
		t.Fatal("a picture was accepted after the end")
	}
}

func TestBroadcastEndsWithoutPictures(t *testing.T) {
	lab := lockLab()
	record := &broadcastRecord{frames: map[string][]string{}, stops: map[string]int{}}
	manager := NewBroadcastManager(broadcastSource{lab, record})
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01")
	broadcast, err := manager.Start(context.Background(), plan, plan.ReviewToken)
	if err != nil {
		t.Fatal(err)
	}
	var clock sync.Mutex
	offset := time.Duration(0)
	broadcast.mutex.Lock()
	broadcast.now = func() time.Time {
		clock.Lock()
		defer clock.Unlock()
		return time.Now().Add(offset)
	}
	broadcast.mutex.Unlock()
	clock.Lock()
	offset = broadcastSilence + time.Second
	clock.Unlock()
	select {
	case <-broadcast.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a silent showing did not end")
	}
	if !broadcast.Stopped() {
		t.Fatal("the showing is not stopped")
	}
}
