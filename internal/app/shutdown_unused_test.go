package app

import (
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestShutdownUnusedSessionsAreEligibleWithoutDataLossWording(t *testing.T) {
	observation := domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnused}
	for _, policy := range []domain.ShutdownSessionPolicy{domain.ShutdownProtectUnknown, domain.ShutdownAcknowledgeUnknown} {
		eligible, detail := shutdownObservationEligible(observation, policy)
		if !eligible || !strings.Contains(detail, "not in use") || strings.Contains(detail, "unsaved") {
			t.Fatalf("unused session with %s: %t %q", policy, eligible, detail)
		}
	}
}
