package adapters

import (
	"net"
	"os"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func (Local) DetectNetworkDefaults() domain.NetworkDefaults {
	routeData, _ := os.ReadFile("/proc/net/route")
	preferred := defaultRouteInterface(string(routeData))
	interfaces, err := net.Interfaces()
	if err != nil {
		return domain.NetworkDefaults{InterfaceName: preferred}
	}
	ordered := interfaces
	if preferred != "" {
		ordered = append([]net.Interface{}, interfaces...)
		for index, iface := range ordered {
			if iface.Name == preferred {
				ordered[0], ordered[index] = ordered[index], ordered[0]
				break
			}
		}
	}
	for _, iface := range ordered {
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
				return domain.NetworkDefaults{InterfaceName: iface.Name, DHCPAddress: ip.String()}
			}
		}
	}
	return domain.NetworkDefaults{InterfaceName: preferred}
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
