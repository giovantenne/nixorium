package adapters

import (
	"context"
	"testing"
)

func TestStartSystemUnitRejectsUnknownUnit(t *testing.T) {
	if err := (Local{}).StartSystemUnit(context.Background(), "ssh.service"); err == nil {
		t.Fatal("unknown privileged unit was accepted")
	}
}

func TestControlSystemUnitRejectsUnknownVerbUnitPairs(t *testing.T) {
	local := Local{}
	for _, test := range []struct {
		verb string
		unit string
	}{
		{verb: "restart", unit: "nixorium-pxe.service"},
		{verb: "start", unit: "nixorium-pxe-network.service"},
		{verb: "stop", unit: "nixorium-pxe-recover.service"},
		{verb: "stop", unit: "nixorium-prepare-pxe.service"},
	} {
		if err := local.ControlSystemUnit(context.Background(), test.verb, test.unit); err == nil {
			t.Fatalf("accepted %s %s", test.verb, test.unit)
		}
	}
}
