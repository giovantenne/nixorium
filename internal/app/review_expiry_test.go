package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// expiryFeature builds one feature's review at a chosen clock. plan returns the
// review's expiry and token; reviewed rebuilds the plan like the command line's
// apply does and matches it against the token (nil when no command line
// re-plans).
type expiryFeature struct {
	name     string
	lifetime time.Duration
	build    func(t *testing.T) (setNow func(time.Time), plan func() (time.Time, string), reviewed func(token string) ReviewMatch)
}

func expiryFeatures() []expiryFeature {
	background := context.Background()
	return []expiryFeature{
		{"deployment recovery", deploymentRecoveryLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			revision := strings.Repeat("a", 40)
			source := &fakeDeploymentRecovery{
				present: true,
				pending: domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: revision, StartedAt: time.Now().UTC(), ControllerPID: 7,
					Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}}},
				activity: map[string]domain.DeploymentActivity{"pc01": {Reachable: true, Revision: revision}},
			}
			m := NewDeploymentRecoveryManager(source)
			return func(now time.Time) { m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, "/srv/lab", false)
					if p.State != "ready" {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				},
				func(token string) ReviewMatch {
					_, match := m.Reviewed(m.Plan(background, "/srv/lab", false), token)
					return match
				}
		}},
		{"cleanup", cleanupReviewLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			source := &fakeCleanupSource{meta: cleanupMeta(), controller: []domain.CleanupObservation{observed("aaaaaaaaaaaaaaaa", 3, 4)}, clients: map[string]domain.CleanupObservation{}}
			m := NewCleanupManager(source)
			return func(now time.Time) { m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, "/srv/lab", "controller")
					if p.State != "ready" {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				},
				func(token string) ReviewMatch {
					_, match := m.Reviewed(m.Plan(background, "/srv/lab", "controller"), token)
					return match
				}
		}},
		{"shutdown", shutdownReviewLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			_, m := shutdownFixture()
			return func(now time.Time) { m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, "/deployment", "pc01", domain.ShutdownProtectUnknown)
					if p.State != "ready" {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				},
				func(token string) ReviewMatch {
					_, match := m.Reviewed(m.Plan(background, "/deployment", "pc01", domain.ShutdownProtectUnknown), token)
					return match
				}
		}},
		{"internet", internetReviewLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			_, m := internetFixture()
			return func(now time.Time) { m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, ".", "pc01", domain.InternetBlock)
					if p.HasErrors() {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				},
				func(token string) ReviewMatch {
					_, match := m.Reviewed(m.Plan(background, ".", "pc01", domain.InternetBlock), token)
					return match
				}
		}},
		{"lock", lockReviewLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			m := NewLockManager(lockLab())
			return func(now time.Time) { m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, "/srv/lab", "pc01", domain.LockOn)
					if p.HasErrors() {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				},
				func(token string) ReviewMatch {
					_, match := m.Reviewed(m.Plan(background, "/srv/lab", "pc01", domain.LockOn), token)
					return match
				}
		}},
		{"collect", collectReviewLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			lab := newCollectLab("pc01")
			lab.desktops["pc01"] = homework
			m := NewCollectManager(lab, &fakeCollectContent{})
			var clock time.Time
			return func(now time.Time) { clock = now; m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, "/srv/lab", "pc01", "/home/teacher/Homework")
					if p.HasErrors() {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				},
				func(token string) ReviewMatch {
					_, match := ReviewedCollect(clock, m.Plan(background, "/srv/lab", "pc01", "/home/teacher/Homework"), token)
					return match
				}
		}},
		{"share", shareReviewLifetime, func(t *testing.T) (func(time.Time), func() (time.Time, string), func(string) ReviewMatch) {
			content := &fakeShareContent{files: []domain.ShareFile{{Path: "a.txt", Size: 5, SHA256: "abc"}}, data: map[int]string{0: "hello"}}
			m := NewShareManager(shareSource{lockLab(), map[string][]string{}}, content)
			return func(now time.Time) { m.now = func() time.Time { return now } },
				func() (time.Time, string) {
					p := m.Plan(background, "/srv/lab", "pc01", "t1")
					if p.HasErrors() {
						t.Fatalf("plan = %+v", p)
					}
					return p.ExpiresAt, p.ReviewToken
				}, nil
		}},
	}
}

// A review made just before a grid line must stay valid for its whole
// lifetime, and a command line that rebuilds the review at any time within
// that lifetime must still match the confirmed token.
func TestReviewsStayValidForTheirFullLifetime(t *testing.T) {
	for _, created := range []time.Time{
		time.Date(2026, 10, 23, 10, 9, 50, 500_000_000, time.UTC),
		time.Date(2026, 10, 23, 10, 4, 59, 999_000_000, time.UTC),
		time.Date(2026, 10, 23, 10, 10, 0, 0, time.UTC),
	} {
		for _, feature := range expiryFeatures() {
			t.Run(feature.name+" "+created.Format("15:04:05.000"), func(t *testing.T) {
				setNow, plan, reviewed := feature.build(t)
				setNow(created)
				expires, token := plan()
				if expires.Sub(created) < feature.lifetime {
					t.Fatalf("review made at %s expires at %s, before its %s lifetime", created, expires, feature.lifetime)
				}
				if reviewed == nil {
					return
				}
				for _, delay := range []time.Duration{0, time.Second, 11 * time.Second, time.Minute, feature.lifetime / 2, feature.lifetime - time.Second} {
					setNow(created.Add(delay))
					if match := reviewed(token); match != ReviewMatched {
						t.Fatalf("a re-plan %s after the review reported %v", delay, match)
					}
				}
				setNow(expires)
				if match := reviewed(token); match != ReviewExpired {
					t.Fatalf("a re-plan at the expiry reported %v, not expired", match)
				}
				setNow(created)
				if match := reviewed("sha256:other"); match != ReviewChanged {
					t.Fatalf("a foreign token reported %v", match)
				}
				if match := reviewed(""); match != ReviewChanged {
					t.Fatalf("an empty token reported %v", match)
				}
			})
		}
	}
}

func TestReviewExpiryRoundsUpToTheGrid(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 9, 50, 0, time.UTC)
	if got := reviewExpiry(now, 10*time.Minute); !got.Equal(time.Date(2026, 10, 23, 10, 20, 0, 0, time.UTC)) {
		t.Fatalf("expiry = %s", got)
	}
	exact := time.Date(2026, 10, 23, 10, 10, 0, 0, time.UTC)
	if got := reviewExpiry(exact, 10*time.Minute); got.Sub(exact) < 10*time.Minute {
		t.Fatalf("expiry %s is shorter than the lifetime", got)
	}
	if got := reviewExpiries(now, 10*time.Minute); len(got) != 11 || got[0].Before(got[len(got)-1]) {
		t.Fatalf("candidate expiries = %v", got)
	}
}

// An internet review applied through the rebuilt plan is accepted until its
// expiry, never refused for moving to another grid step.
func TestInternetRebuiltReviewAppliesNearTheEndOfItsLifetime(t *testing.T) {
	s, m := internetFixture()
	created := time.Date(2026, 10, 23, 10, 9, 50, 0, time.UTC)
	m.now = func() time.Time { return created }
	p := m.Plan(context.Background(), ".", "pc01", domain.InternetBlock)
	m.now = func() time.Time { return created.Add(internetReviewLifetime - time.Second) }
	rebuilt, match := m.Reviewed(m.Plan(context.Background(), ".", "pc01", domain.InternetBlock), p.ReviewToken)
	if match != ReviewMatched {
		t.Fatalf("match = %v", match)
	}
	if r := m.Apply(context.Background(), rebuilt, p.ReviewToken); r.State != "completed" || len(s.changed) != 1 {
		t.Fatalf("report: %+v", r)
	}
}
