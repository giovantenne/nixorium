package domain

import "testing"

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
