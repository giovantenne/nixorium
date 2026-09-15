package adapters

import (
	"net"
	"os"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type networkAddressCandidate struct {
	interfaceName string
	address       string
}

func (Local) DetectNetworkDefaults(excludedAddresses ...string) domain.NetworkDefaults {
	routeData, _ := os.ReadFile("/proc/net/route")
	preferred := defaultRouteInterface(string(routeData))
	interfaces, err := net.Interfaces()
	if err != nil {
		return domain.NetworkDefaults{InterfaceName: preferred}
	}
	candidates := []networkAddressCandidate{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, addressErr := iface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr == nil && ip.To4() != nil && !ip.IsLinkLocalUnicast() {
				candidates = append(candidates, networkAddressCandidate{interfaceName: iface.Name, address: ip.String()})
			}
		}
	}
	return selectNetworkDefaults(preferred, candidates, excludedAddresses)
}

func selectNetworkDefaults(preferred string, candidates []networkAddressCandidate, excludedAddresses []string) domain.NetworkDefaults {
	excluded := map[string]bool{}
	for _, address := range excludedAddresses {
		if parsed := net.ParseIP(address); parsed != nil && parsed.To4() != nil {
			excluded[parsed.String()] = true
		}
	}
	if preferred != "" {
		for _, candidate := range candidates {
			if candidate.interfaceName == preferred && !excluded[candidate.address] {
				return domain.NetworkDefaults{InterfaceName: preferred, DHCPAddress: candidate.address}
			}
		}
		return domain.NetworkDefaults{InterfaceName: preferred}
	}
	for _, candidate := range candidates {
		if !excluded[candidate.address] {
			return domain.NetworkDefaults{InterfaceName: candidate.interfaceName, DHCPAddress: candidate.address}
		}
	}
	return domain.NetworkDefaults{}
}

func defaultRouteInterface(routeTable string) string {
	for _, line := range strings.Split(routeTable, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[1] == "00000000" && fields[3] != "0000" {
			return fields[0]
		}
	}
	return ""
}
