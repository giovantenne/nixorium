package adapters

import "testing"

func TestDefaultRouteInterface(t *testing.T) {
	table := "Iface Destination Gateway Flags RefCnt Use Metric Mask\nlo 0000007F 00000000 0001 0 0 0 000000FF\nenp3s0 00000000 01020304 0003 0 0 100 00000000\n"
	if got := defaultRouteInterface(table); got != "enp3s0" {
		t.Fatalf("default interface = %q", got)
	}
}
