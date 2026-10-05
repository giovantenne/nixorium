package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type notificationFixture struct {
	snapshot domain.UpdateInputSnapshot
	refs     []domain.UpdateReleaseRef
	state    domain.UpdateNotificationState
	calls    int
	err      error
}

func (f *notificationFixture) InspectUpdateInput(string) (domain.UpdateInputSnapshot, error) {
	return f.snapshot, nil
}
func (f *notificationFixture) DiscoverUpdateReleases(context.Context, string) ([]domain.UpdateReleaseRef, error) {
	f.calls++
	return f.refs, f.err
}
func (f *notificationFixture) LoadUpdateNotification(string) (domain.UpdateNotificationState, error) {
	return f.state, nil
}
func (f *notificationFixture) SaveUpdateNotification(_ string, s domain.UpdateNotificationState) error {
	f.state = s
	return nil
}

func TestUpdateNotificationFollowsChosenChannel(t *testing.T) {
	refs := []domain.UpdateReleaseRef{{Tag: "master", ObjectID: "new"}, {Tag: "v3.1.0", ObjectID: "stable"}, {Tag: "v4.0.0-beta.1", ObjectID: "beta"}, {Tag: "v3.0.0", ObjectID: "old"}, {Tag: "invalid"}}
	for _, tc := range []struct{ current, revision, target string }{
		{"master", "old", "master"}, {"master", "new", ""}, {"v3.0.0", "old", "v3.1.0"},
		{"v3.1.0", "stable", ""}, {"v3.2.0", "future", ""}, {"v3.0.0-beta.1", "old", "v4.0.0-beta.1"},
		{"other-branch", "old", ""}, {"v3.0.0", "", ""},
	} {
		t.Run(tc.current+tc.revision, func(t *testing.T) {
			f := &notificationFixture{snapshot: domain.UpdateInputSnapshot{SourceURL: "github:owner/repo/" + tc.current, SourcePrefix: "owner/repo", CurrentRef: tc.current, CurrentRev: tc.revision}, refs: refs}
			m := NewUpdateNotificationManager(f)
			notice, err := m.Check(context.Background(), "/repo")
			if err != nil || notice.Target != tc.target {
				t.Fatalf("got %+v, %v", notice, err)
			}
			if (tc.current == "other-branch" || tc.revision == "") && f.calls != 0 {
				t.Fatal("unsupported pin contacted upstream")
			}
		})
	}
}
func TestUpdateNotificationCacheDismissalAndOfflineRetry(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	f := &notificationFixture{snapshot: domain.UpdateInputSnapshot{SourceURL: "github:owner/repo/master", SourcePrefix: "owner/repo", CurrentRef: "master", CurrentRev: "old"}, refs: []domain.UpdateReleaseRef{{Tag: "master", ObjectID: "new"}}}
	m := NewUpdateNotificationManager(f)
	m.now = func() time.Time { return now }
	notice, err := m.Check(context.Background(), "/repo")
	if err != nil || notice.Key == "" {
		t.Fatal(notice, err)
	}
	if err := m.Dismiss("/repo", notice); err != nil {
		t.Fatal(err)
	}
	// A restart and hourly polling reuse the durable check and dismissal.
	m = NewUpdateNotificationManager(f)
	m.now = func() time.Time { return now.Add(time.Hour) }
	notice, err = m.Check(context.Background(), "/repo")
	if err != nil || notice.Key != "" || f.calls != 1 {
		t.Fatal(notice, err, f.calls)
	}
	now = now.Add(24 * time.Hour)
	f.refs = []domain.UpdateReleaseRef{{Tag: "master", ObjectID: "newer"}}
	notice, err = m.Check(context.Background(), "/repo")
	if err != nil || notice.Revision != "newer" || f.calls != 2 {
		t.Fatal(notice, err, f.calls)
	}
	// A changed local pin invalidates the cached notification immediately.
	f.snapshot.CurrentRev = "newer"
	notice, err = m.Check(context.Background(), "/repo")
	if err != nil || notice.Key != "" {
		t.Fatal(notice, err)
	}
	now = now.Add(24 * time.Hour)
	f.err = errors.New("offline")
	_, _ = m.Check(context.Background(), "/repo")
	calls := f.calls
	_, _ = m.Check(context.Background(), "/repo")
	if f.calls != calls {
		t.Fatal("offline poll retried before interval")
	}
}
