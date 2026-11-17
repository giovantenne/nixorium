package main

import (
	"context"
	"maps"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// knownOff tells whether the classroom overview found a computer switched off
// moments ago (app.ClassroomViewHub.KnownOff). While the page is open, and
// shortly after, the classroom actions skip such a computer at once instead
// of waiting for its SSH timeout, and name it as switched off as before,
// whether they come from the page or from the menu through this service.
// Switch on, which is for the computers that are off, does not use it.
type knownOff func(domain.HostMeta) bool

func (off knownOff) has(host domain.HostMeta) bool { return off != nil && off(host) }

// Connect opens a channel to the computer's agent, unless the overview found
// it switched off.
func (source lockSource) Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error) {
	if source.off.has(host) {
		return nil, classroomview.ErrNoConnection
	}
	return source.ClassroomAgentConnector.Connect(ctx, host)
}

// skipKnownOff is the controller access of the Internet and power actions,
// which skips the computers the overview found switched off.
type skipKnownOff struct {
	cachedLocal
	off knownOff
}

// ObserveInternet observes a computer the way a switched-off one answers:
// unknown, so it is not eligible.
func (source skipKnownOff) ObserveInternet(ctx context.Context, host domain.HostMeta) domain.InternetObservation {
	if source.off.has(host) {
		return domain.InternetObservation{State: "unknown", Detail: app.ClassroomOffDetail}
	}
	return source.cachedLocal.ObserveInternet(ctx, host)
}

// ShutdownObservations observes only the computers not known to be off; the
// others are unreachable.
func (source skipKnownOff) ShutdownObservations(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.ShutdownObservation {
	observations := map[string]domain.ShutdownObservation{}
	var ask []domain.HostMeta
	for _, host := range hosts {
		if source.off.has(host) {
			observations[host.Name] = domain.ShutdownObservation{Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Session: domain.ShutdownSessionUnknown, Detail: app.ClassroomOffDetail}
			continue
		}
		ask = append(ask, host)
	}
	if len(ask) > 0 {
		maps.Copy(observations, source.cachedLocal.ShutdownObservations(ctx, ask, timeout))
	}
	return observations
}
