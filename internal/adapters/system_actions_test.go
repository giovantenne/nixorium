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
