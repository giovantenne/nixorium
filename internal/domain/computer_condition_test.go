package domain

import (
	"strings"
	"testing"
)

func TestComputerConditionDoesNotConfuseConnectivityWithConfiguration(t *testing.T) {
	for _, h := range []HostStatus{
		{Reachability: ReachabilityUnreachable},
		{Reachability: ReachabilityReachable, SSH: SSHUnavailable},
		{SSH: SSHAvailable, Deployment: DeploymentUnknown},
	} {
		level, label, guidance := ComputerCondition(h)
		if level == LevelOK || label == "" || guidance == "" {
			t.Fatalf("invalid condition: %s %s %s", level, label, guidance)
		}
	}
}

func TestChangedHostKeyRequiresPhysicalIdentityReview(t *testing.T) {
	level, label, guidance := ComputerCondition(HostStatus{HostKeyCondition: HostKeyChanged, SSH: SSHAvailable, Deployment: DeploymentCurrent})
	if level != LevelError || label != "SSH key changed" || !strings.Contains(guidance, "physical-console fingerprint") {
		t.Fatalf("unsafe condition: %s %s %s", level, label, guidance)
	}
}
