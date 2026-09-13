package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestHostsTextIncludesTypedComputerState(t *testing.T) {
	report := domain.HostsReport{
		State: "partial",
		Hosts: []domain.HostStatus{
			{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
			{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown},
		},
	}
	var output bytes.Buffer
	HostsText(&output, report)
	for _, expected := range []string{
		"Nixorium computers: PARTIAL",
		"SSH available:      1/2",
		"pc01       10.0.0.1        network=reachable",
		"pc02       10.0.0.2        network=unknown",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("status output omits %q:\n%s", expected, output.String())
		}
	}
}
