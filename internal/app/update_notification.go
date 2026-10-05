package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const UpdateNotificationInterval = 24 * time.Hour

type UpdateNotificationSource interface {
	InspectUpdateInput(string) (domain.UpdateInputSnapshot, error)
	DiscoverUpdateReleases(context.Context, string) ([]domain.UpdateReleaseRef, error)
	LoadUpdateNotification(string) (domain.UpdateNotificationState, error)
	SaveUpdateNotification(string, domain.UpdateNotificationState) error
}

type UpdateNotificationManager struct {
	source UpdateNotificationSource
	now    func() time.Time
	mu     sync.Mutex
}

func NewUpdateNotificationManager(source UpdateNotificationSource) *UpdateNotificationManager {
	return &UpdateNotificationManager{source: source, now: time.Now}
}

func (m *UpdateNotificationManager) Check(ctx context.Context, repository string) (domain.UpdateNotification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, err := m.source.InspectUpdateInput(repository)
	if err != nil {
		return domain.UpdateNotification{}, err
	}
	// Unsupported references never cause an automatic network request.
	if snapshot.CurrentRef != "master" {
		if _, err := parseUpdateRelease(snapshot.CurrentRef); err != nil {
			return domain.UpdateNotification{}, nil
		}
	}
	if snapshot.CurrentRev == "" {
		return domain.UpdateNotification{}, nil
	}
	state, err := m.source.LoadUpdateNotification(repository)
	if err != nil {
		return domain.UpdateNotification{}, err
	}
	input := snapshot.SourceURL + "@" + snapshot.CurrentRev
	now := m.now().UTC()
	if state.Input != input || state.CheckedAt.IsZero() || now.Before(state.CheckedAt) || now.Sub(state.CheckedAt) >= UpdateNotificationInterval {
		if state.Input != input {
			state = domain.UpdateNotificationState{Input: input}
		}
		// Record attempts too, so offline restarts cannot create a retry storm.
		state.CheckedAt = now
		refs, discoveryErr := m.source.DiscoverUpdateReleases(ctx, snapshot.SourcePrefix)
		if discoveryErr == nil {
			state.Available = updateNotificationCandidate(snapshot, refs)
		}
		if err := m.source.SaveUpdateNotification(repository, state); err != nil {
			return domain.UpdateNotification{}, err
		}
		if discoveryErr != nil {
			return visibleUpdateNotification(state), nil
		}
	}
	return visibleUpdateNotification(state), nil
}

func (m *UpdateNotificationManager) Dismiss(repository string, notice domain.UpdateNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.source.LoadUpdateNotification(repository)
	if err != nil {
		return err
	}
	if notice.Key == "" || state.Available.Key != notice.Key {
		return nil
	}
	state.Dismissed = notice.Key
	return m.source.SaveUpdateNotification(repository, state)
}

func visibleUpdateNotification(state domain.UpdateNotificationState) domain.UpdateNotification {
	if state.Available.Key == state.Dismissed {
		return domain.UpdateNotification{}
	}
	return state.Available
}

func updateNotificationCandidate(snapshot domain.UpdateInputSnapshot, refs []domain.UpdateReleaseRef) domain.UpdateNotification {
	chosen := domain.UpdateNotification{}
	if snapshot.CurrentRef == "master" {
		for _, ref := range refs {
			if ref.Tag == "master" && ref.ObjectID != "" && ref.ObjectID != snapshot.CurrentRev {
				chosen = domain.UpdateNotification{Target: "master", Revision: ref.ObjectID, Channel: domain.UpdateChannelMoving}
			}
		}
	} else {
		current, err := parseUpdateRelease(snapshot.CurrentRef)
		if err != nil {
			return chosen
		}
		best := current
		for _, ref := range refs {
			release, err := parseUpdateRelease(ref.Tag)
			if err != nil || release.Channel() != current.Channel() || compareUpdateReleases(release, best) <= 0 {
				continue
			}
			best = release
			chosen = domain.UpdateNotification{Target: ref.Tag, Revision: ref.ObjectID, Channel: release.Channel()}
		}
	}
	if chosen.Target != "" {
		chosen.Key = strings.Join([]string{snapshot.SourcePrefix, string(chosen.Channel), chosen.Target, chosen.Revision}, "|")
	}
	return chosen
}
