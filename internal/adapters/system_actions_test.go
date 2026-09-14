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
		{verb: "start", unit: "nixorium-harmonia.service"},
		{verb: "restart", unit: "harmonia.service"},
		{verb: "restart", unit: "nixorium-restart-cache.service"},
	} {
		if err := local.ControlSystemUnit(context.Background(), test.verb, test.unit); err == nil {
			t.Fatalf("accepted %s %s", test.verb, test.unit)
		}
	}
}

func TestServiceRestartAllowlistRejectsDirectCacheStart(t *testing.T) {
	if err := (Local{}).ControlSystemUnit(context.Background(), "start", "nixorium-harmonia.service"); err == nil {
		t.Fatal("cache start was accepted outside the restart workflow")
	}
}

func TestControllerApplyUnitAllowlistAcceptsOnlyFullRevisionInstance(t *testing.T) {
	if !controllerApplyUnitPattern.MatchString("nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567.service") {
		t.Fatal("valid revision-bound controller unit was rejected")
	}
	for _, unit := range []string{
		"nixorium-apply-controller@short.service",
		"nixorium-apply-controller@0123456789abcdef0123456789abcdef0123456g.service",
		"nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567/evil.service",
	} {
		if controllerApplyUnitPattern.MatchString(unit) {
			t.Fatalf("unsafe unit %q was accepted", unit)
		}
	}
}
