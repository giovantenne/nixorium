package domain

import (
	"testing"
	"time"
)

func TestShutdownReviewTokenBindsTargetsPolicyAndExpiry(t *testing.T) {
	base := ShutdownPlanReport{
		Repository: "/deployment", Requested: "pc01", Policy: ShutdownProtectUnknown,
		ExpiresAt: time.Unix(100, 0),
		Targets:   []ShutdownTargetPlan{{Name: "pc01", IP: "10.0.0.1", Reachability: ReachabilityReachable, SSH: SSHAvailable, Session: ShutdownSessionIdle, Eligible: true}},
	}
	token := ShutdownReviewToken(base)
	changed := base
	changed.Policy = ShutdownAcknowledgeUnknown
	if token == ShutdownReviewToken(changed) {
		t.Fatal("review token did not bind the session policy")
	}
	changed = base
	changed.Targets = append([]ShutdownTargetPlan(nil), base.Targets...)
	changed.Targets[0].IP = "10.0.0.2"
	if token == ShutdownReviewToken(changed) {
		t.Fatal("review token did not bind target identity and address")
	}
	changed = base
	changed.ExpiresAt = changed.ExpiresAt.Add(time.Second)
	if token == ShutdownReviewToken(changed) {
		t.Fatal("review token did not bind expiry")
	}
}

func TestShutdownApplyPartialIsAnError(t *testing.T) {
	if !(ShutdownApplyReport{State: "partial"}).HasErrors() {
		t.Fatal("partial shutdown result must require attention")
	}
}
