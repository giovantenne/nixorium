package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeInternetSource struct {
	*fakeShutdownSource
	states    map[string]domain.InternetObservation
	changed   []string
	changeErr error
	loseReply bool
}

func (s *fakeInternetSource) ObserveInternet(_ context.Context, h domain.HostMeta) domain.InternetObservation {
	return s.states[h.Name]
}
func (s *fakeInternetSource) ChangeInternet(_ context.Context, h domain.HostMeta, a domain.InternetAction, boot string) error {
	s.changed = append(s.changed, h.Name)
	if s.changeErr == nil || s.loseReply {
		o := s.states[h.Name]
		o.State = a.DesiredState()
		s.states[h.Name] = o
	}
	return s.changeErr
}
func internetFixture() (*fakeInternetSource, *InternetManager) {
	base, _ := shutdownFixture()
	s := &fakeInternetSource{fakeShutdownSource: base, states: map[string]domain.InternetObservation{"pc01": {SchemaVersion: 1, BootID: "11111111-1111-4111-8111-111111111111", State: "enabled"}}}
	m := NewInternetManager(s)
	m.now = func() time.Time { return time.Unix(1000, 0).UTC() }
	return s, m
}
func TestInternetPartialNeverQueuesOfflineClients(t *testing.T) {
	s, m := internetFixture()
	p := m.Plan(context.Background(), ".", "@lab", domain.InternetBlock)
	if p.HasErrors() || len(p.Targets) != 3 || !p.Targets[0].Eligible || p.Targets[1].Eligible {
		t.Fatalf("plan: %+v", p)
	}
	r := m.Apply(context.Background(), p, p.ReviewToken)
	if r.State != "partial" || len(s.changed) != 1 || s.changed[0] != "pc01" || r.Targets[0].State != "verified" || r.Targets[1].State != "not-sent" {
		t.Fatalf("report: %+v", r)
	}
}
func TestInternetRejectsStaleOrChangedReview(t *testing.T) {
	for _, scenario := range []string{"expiry", "action", "token", "inventory", "boot", "observed-state", "lock", "pxe"} {
		t.Run(scenario, func(t *testing.T) {
			s, m := internetFixture()
			p := m.Plan(context.Background(), ".", "pc01", domain.InternetBlock)
			token := p.ReviewToken
			switch scenario {
			case "expiry":
				m.now = func() time.Time { return p.ExpiresAt }
			case "action":
				p.Action = domain.InternetUnblock
			case "token":
				token = "changed"
			case "inventory":
				s.meta.Clients.Hosts[0].IP = "10.0.0.5"
			case "boot":
				o := s.states["pc01"]
				o.BootID = "22222222-2222-4222-8222-222222222222"
				s.states["pc01"] = o
			case "observed-state":
				o := s.states["pc01"]
				o.State = "blocked"
				s.states["pc01"] = o
			case "lock":
				s.acquireErr = errors.New("busy")
			case "pxe":
				s.listener.State = "active"
				s.network.State = "active"
				s.listener.Active = true
				s.network.Active = true
			}
			r := m.Apply(context.Background(), p, token)
			if !r.HasErrors() || len(s.changed) != 0 {
				t.Fatalf("mutation after %s: %+v", scenario, r)
			}
		})
	}
}
func TestInternetReconcilesLostReplyAndIdempotentState(t *testing.T) {
	s, m := internetFixture()
	s.changeErr = errors.New("connection lost")
	s.loseReply = true
	p := m.Plan(context.Background(), ".", "pc01", domain.InternetBlock)
	if r := m.Apply(context.Background(), p, p.ReviewToken); r.HasErrors() {
		t.Fatal(r)
	}
	p = m.Plan(context.Background(), ".", "pc01", domain.InternetBlock)
	if r := m.Apply(context.Background(), p, p.ReviewToken); r.HasErrors() || len(s.changed) != 1 {
		t.Fatal(r)
	}
	s.loseReply = false
	s.changeErr = nil
	p = m.Plan(context.Background(), ".", "pc01", domain.InternetUnblock)
	if r := m.Apply(context.Background(), p, p.ReviewToken); r.HasErrors() || s.states["pc01"].State != "enabled" {
		t.Fatal(r)
	}
}
func TestInternetUnconfirmedAndInvalidTargets(t *testing.T) {
	s, m := internetFixture()
	s.changeErr = errors.New("helper failed")
	p := m.Plan(context.Background(), ".", "pc01", domain.InternetBlock)
	r := m.Apply(context.Background(), p, p.ReviewToken)
	if r.State != "partial" || r.Targets[0].State != "unconfirmed" {
		t.Fatal(r)
	}
	for _, target := range []string{"pc99", "pc01,pc01", "pc42", ""} {
		if p := m.Plan(context.Background(), ".", target, domain.InternetBlock); !p.HasErrors() {
			t.Fatal(p)
		}
	}
}

type classroomLeaseInternetSource struct {
	*fakeInternetSource
	blocked map[string]string
}

func (s classroomLeaseInternetSource) ClassroomBlockedHosts() (map[string]string, error) {
	return s.blocked, nil
}

func (s classroomLeaseInternetSource) AcquireClassroomLease(string) (io.Closer, map[string]string, error) {
	return shutdownLease{}, s.blocked, nil
}

func TestInternetBlockRunsBesideMaintenanceAndLeavesComputersBeingUpdated(t *testing.T) {
	s, _ := internetFixture()
	s.active, s.acquireErr = true, errors.New("another operation is running")
	s.states["pc02"] = domain.InternetObservation{SchemaVersion: 1, BootID: "22222222-2222-4222-8222-222222222222", State: "enabled"}
	source := classroomLeaseInternetSource{fakeInternetSource: s, blocked: map[string]string{"pc02": domain.ClassroomBusyReason}}
	m := NewInternetManager(source)
	m.now = func() time.Time { return time.Unix(1000, 0).UTC() }
	p := m.Plan(context.Background(), ".", "pc01,pc02", domain.InternetBlock)
	if p.HasErrors() || !p.Targets[0].Eligible || p.Targets[1].Eligible || !strings.Contains(p.Message, "pc02") {
		t.Fatalf("plan: %+v", p)
	}
	r := m.Apply(context.Background(), p, p.ReviewToken)
	if len(s.changed) != 1 || s.changed[0] != "pc01" || r.Targets[1].State != "not-sent" || !strings.Contains(r.Targets[1].Detail, "being updated") {
		t.Fatalf("report: %+v changed %v", r, s.changed)
	}
	source.blocked = map[string]string{"pc01": domain.ClassroomBusyReason}
	m.source = source
	if p := m.Plan(context.Background(), ".", "pc01", domain.InternetBlock); !p.HasErrors() || !strings.Contains(p.Message, "being updated") {
		t.Fatalf("plan with only a computer being updated: %+v", p)
	}
}

func TestInternetReviewAtABlockBoundaryStillApplies(t *testing.T) {
	s, m := internetFixture()
	reviewed := time.Date(2026, 10, 23, 16, 59, 59, 0, time.UTC)
	m.now = func() time.Time { return reviewed }
	p := m.Plan(context.Background(), ".", "pc01", domain.InternetBlock)
	if !p.ExpiresAt.After(reviewed.Add(5 * time.Minute)) {
		t.Fatalf("a review at the end of a block expires at once: %v", p.ExpiresAt)
	}
	// The command line's apply runs in a new process, one second later and
	// in the next block.
	m.now = func() time.Time { return reviewed.Add(time.Second) }
	again := m.Replan(context.Background(), ".", "pc01", domain.InternetBlock, p.ReviewToken)
	if again.ReviewToken != p.ReviewToken {
		t.Fatalf("replan across the boundary changed the review: %v -> %v", p.ExpiresAt, again.ExpiresAt)
	}
	if r := m.Apply(context.Background(), again, p.ReviewToken); r.State != "completed" || len(s.changed) != 1 {
		t.Fatalf("report: %+v", r)
	}
	// Once expired, the same review is refused.
	m.now = func() time.Time { return again.ExpiresAt }
	if r := m.Apply(context.Background(), again, p.ReviewToken); r.State == "completed" {
		t.Fatalf("expired review applied: %+v", r)
	}
}
