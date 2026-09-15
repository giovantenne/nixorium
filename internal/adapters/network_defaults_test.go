package adapters

import "testing"

func TestDefaultRouteInterface(t *testing.T) {
	table := "Iface Destination Gateway Flags RefCnt Use Metric Mask\nlo 0000007F 00000000 0001 0 0 0 000000FF\nenp3s0 00000000 01020304 0003 0 0 100 00000000\n"
	if got := defaultRouteInterface(table); got != "enp3s0" {
		t.Fatalf("default interface = %q", got)
	}
}

func TestSelectNetworkDefaultsUsesDefaultRouteAndExcludesStaticAddress(t *testing.T) {
	candidates := []networkAddressCandidate{
		{interfaceName: "virbr0", address: "192.168.122.1"},
		{interfaceName: "enp0s3", address: "10.0.0.99"},
		{interfaceName: "enp0s3", address: "192.168.1.42"},
	}
	defaults := selectNetworkDefaults("enp0s3", candidates, []string{"10.0.0.99"})
	if defaults.InterfaceName != "enp0s3" || defaults.DHCPAddress != "192.168.1.42" {
		t.Fatalf("defaults = %+v", defaults)
	}
}

func TestSelectNetworkDefaultsDoesNotGuessAnotherInterface(t *testing.T) {
	candidates := []networkAddressCandidate{
		{interfaceName: "enp0s3", address: "10.0.0.99"},
		{interfaceName: "enp0s8", address: "192.168.56.10"},
	}
	defaults := selectNetworkDefaults("enp0s3", candidates, []string{"10.0.0.99"})
	if defaults.InterfaceName != "enp0s3" || defaults.DHCPAddress != "" {
		t.Fatalf("defaults = %+v; another interface must not be guessed", defaults)
	}
}

func TestSelectNetworkDefaultsFallsBackWithoutDefaultRoute(t *testing.T) {
	candidates := []networkAddressCandidate{{interfaceName: "enp0s8", address: "192.168.56.10"}}
	defaults := selectNetworkDefaults("", candidates, nil)
	if defaults.InterfaceName != "enp0s8" || defaults.DHCPAddress != "192.168.56.10" {
		t.Fatalf("defaults = %+v", defaults)
	}
}
