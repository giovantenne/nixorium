package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestActionsSkipTheComputersTheOverviewFoundOff(t *testing.T) {
	off := knownOff(func(host domain.HostMeta) bool { return host.Name == "pc02" })
	host := domain.HostMeta{Name: "pc02", IP: "192.0.2.2"}
	// No SSH is attempted for a known-off computer: the answer comes at once.
	started := time.Now()
	if _, err := (lockSource{off: off}).Connect(context.Background(), host); !errors.Is(err, classroomview.ErrNoConnection) {
		t.Fatalf("Connect: err = %v", err)
	}
	access := skipKnownOff{cachedLocal: newCachedLocal(), off: off}
	if observed := access.ObserveInternet(context.Background(), host); observed.Valid() || observed.State != "unknown" || observed.Detail != app.ClassroomOffDetail {
		t.Fatalf("ObserveInternet = %+v", observed)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("known-off computers took %v", elapsed)
	}
	// The other computers are still observed, and both answers are merged.
	other := domain.HostMeta{Name: "pc03", IP: "192.0.2.3"}
	observations := access.ShutdownObservations(context.Background(), []domain.HostMeta{host, other}, 200*time.Millisecond)
	if got := observations["pc02"]; len(observations) != 2 || got.Reachability != domain.ReachabilityUnreachable || got.Detail != app.ClassroomOffDetail {
		t.Fatalf("ShutdownObservations = %+v", observations)
	}
	if got := observations["pc03"]; got.Detail == app.ClassroomOffDetail || got.Reachability == domain.ReachabilityReachable {
		t.Fatalf("pc03 was not observed: %+v", got)
	}
}

func TestSkippingKnownOffComputersKeepsTheClassroomLease(t *testing.T) {
	// Internet and power find the classroom lease on their source by type.
	var source any = skipKnownOff{cachedLocal: newCachedLocal()}
	if _, ok := source.(interface {
		AcquireClassroomLease(string) (io.Closer, map[string]string, error)
	}); !ok {
		t.Fatal("skipKnownOff hides the classroom lease")
	}
	// A missing filter skips nothing.
	if (skipKnownOff{}).off.has(domain.HostMeta{Name: "pc01"}) {
		t.Fatal("a nil filter skipped a computer")
	}
}
