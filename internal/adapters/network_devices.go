package adapters

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	sysClassNet               = "/sys/class/net"
	maximumNetworkDevices     = 64
	maximumNetworkProbeOutput = 16 * 1024
	maximumIPAddressOutput    = 256 * 1024
	// The kernel reports a permanent address lifetime as this value.
	permanentAddressLifetime = 4294967295
)

var networkDeviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,31}$`)

// clientNetworkDevicesCommand is a fixed read-only probe: one "name kind"
// line per network device, classified like the controller's own devices.
const clientNetworkDevicesCommand = `for d in /sys/class/net/*; do n=${d##*/}; [ "$n" = lo ] && continue; k=other; if [ -d "$d/wireless" ] || [ -e "$d/phy80211" ]; then k=wireless; elif [ "$(cat "$d/type")" = 1 ]; then if [ -e "$d/device" ] || [ -d "$d/bridge" ] || [ -d "$d/bonding" ] || { set -- "$d"/lower_*; [ -e "$1" ]; }; then k=wired; else k=virtual; fi; fi; printf '%s %s\n' "$n" "$k"; done`

// ControllerNetworkDevices lists this controller's network devices from
// /sys/class/net with their IPv4 addresses from `ip -4 -j address`. It is
// read-only and bounded in time and output.
func (Local) ControllerNetworkDevices(ctx context.Context) domain.NetworkObservation {
	return observeLocalNetwork(ctx, sysClassNet, runIPAddressJSON)
}

func observeLocalNetwork(ctx context.Context, root string, addresses func(context.Context) ([]byte, error)) domain.NetworkObservation {
	entries, err := os.ReadDir(root)
	if err != nil {
		return domain.NetworkObservation{Devices: []domain.NetworkDevice{}, Detail: "network devices are unreadable"}
	}
	observation := domain.NetworkObservation{Reachable: true, Devices: []domain.NetworkDevice{}}
	index := map[string]int{}
	for _, entry := range entries {
		name := entry.Name()
		if name == "lo" || !networkDeviceNamePattern.MatchString(name) || len(observation.Devices) >= maximumNetworkDevices {
			continue
		}
		index[name] = len(observation.Devices)
		observation.Devices = append(observation.Devices, domain.NetworkDevice{Name: name, Kind: classifySysNetDevice(filepath.Join(root, name))})
	}
	output, err := addresses(ctx)
	parsed, parseErr := parseIPAddressJSON(output)
	if err == nil && parseErr == nil {
		observation.Lifetimes = true
		for name, values := range parsed {
			if position, found := index[name]; found {
				observation.Devices[position].Addresses = values
			}
		}
		return observation
	}
	// Without `ip`, fall back to the kernel's address list. Lifetimes are then
	// unknown, so no address is called dynamic.
	observation.Detail = "address lifetimes are unavailable"
	for position, device := range observation.Devices {
		iface, lookupErr := net.InterfaceByName(device.Name)
		if lookupErr != nil {
			continue
		}
		values, addressErr := iface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, value := range values {
			ip, _, cidrErr := net.ParseCIDR(value.String())
			if cidrErr == nil && ip.To4() != nil {
				observation.Devices[position].Addresses = append(observation.Devices[position].Addresses, domain.NetworkAddress{Address: ip.String()})
			}
		}
	}
	return observation
}

// classifySysNetDevice reads only sysfs attributes: wireless devices have a
// wireless directory or phy80211 link; Ethernet-type devices with hardware,
// bridges, bonds and devices stacked on another one (a VLAN has a lower_*
// link) count as wired; other Ethernet-type devices (veth, dummy) are virtual.
func classifySysNetDevice(directory string) string {
	if sysfsEntryExists(filepath.Join(directory, "wireless")) || sysfsEntryExists(filepath.Join(directory, "phy80211")) {
		return domain.NetworkDeviceWireless
	}
	content, err := os.ReadFile(filepath.Join(directory, "type"))
	if err != nil || strings.TrimSpace(string(content)) != "1" {
		return domain.NetworkDeviceOther
	}
	for _, marker := range []string{"device", "bridge", "bonding"} {
		if sysfsEntryExists(filepath.Join(directory, marker)) {
			return domain.NetworkDeviceWired
		}
	}
	if lower, _ := filepath.Glob(filepath.Join(directory, "lower_*")); len(lower) > 0 {
		return domain.NetworkDeviceWired
	}
	return domain.NetworkDeviceVirtual
}

func sysfsEntryExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func runIPAddressJSON(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	stdout, _, err := runBoundedCommand(ctx, maximumIPAddressOutput, "ip", "-4", "-j", "address", "show")
	return []byte(stdout), err
}

type ipAddressRecord struct {
	Name     string `json:"ifname"`
	AddrInfo []struct {
		Family        string `json:"family"`
		Local         string `json:"local"`
		Dynamic       bool   `json:"dynamic"`
		ValidLifetime *int64 `json:"valid_life_time"`
	} `json:"addr_info"`
}

func parseIPAddressJSON(data []byte) (map[string][]domain.NetworkAddress, error) {
	var records []ipAddressRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	addresses := map[string][]domain.NetworkAddress{}
	for _, record := range records {
		for _, info := range record.AddrInfo {
			ip := net.ParseIP(info.Local)
			if info.Family != "inet" || ip == nil || ip.To4() == nil {
				continue
			}
			dynamic := info.Dynamic || (info.ValidLifetime != nil && *info.ValidLifetime != permanentAddressLifetime)
			addresses[record.Name] = append(addresses[record.Name], domain.NetworkAddress{Address: ip.String(), Dynamic: dynamic})
		}
	}
	return addresses, nil
}

// ClientNetworkDevices lists the network devices of each client through the
// same bounded, parallel SSH access as the inventory probe. Computers that do
// not answer are reported as not reachable; nothing is changed.
func (Local) ClientNetworkDevices(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.NetworkObservation {
	tcp := probeSSHStatuses(ctx, hosts, timeout, probeHostSSH)
	return forEachHost(hosts, func(host domain.HostMeta) domain.NetworkObservation {
		probe := tcp[host.Name]
		if probe.SSH != domain.SSHAvailable {
			detail := probe.Detail
			if detail == "" {
				detail = "no answer"
			}
			return domain.NetworkObservation{Detail: detail}
		}
		commandContext, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
		defer cancel()
		command := exec.CommandContext(commandContext, "ssh", shutdownSSHArguments(host, timeout, clientNetworkDevicesCommand)...)
		configureCommandCancellation(command)
		output := &boundedCommandBuffer{limit: maximumNetworkProbeOutput}
		command.Stdout = output
		if err := command.Run(); err != nil || output.truncated {
			return domain.NetworkObservation{Detail: "management access failed"}
		}
		return parseClientNetworkDevices(output.buffer.String())
	})
}

func parseClientNetworkDevices(output string) domain.NetworkObservation {
	observation := domain.NetworkObservation{Reachable: true, Devices: []domain.NetworkDevice{}}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !networkDeviceNamePattern.MatchString(fields[0]) || len(observation.Devices) >= maximumNetworkDevices {
			return domain.NetworkObservation{Detail: "the computer returned an unexpected device list"}
		}
		switch fields[1] {
		case domain.NetworkDeviceWired, domain.NetworkDeviceWireless, domain.NetworkDeviceVirtual, domain.NetworkDeviceOther:
		default:
			return domain.NetworkObservation{Detail: "the computer returned an unexpected device list"}
		}
		observation.Devices = append(observation.Devices, domain.NetworkDevice{Name: fields[0], Kind: fields[1]})
	}
	sort.Slice(observation.Devices, func(i, j int) bool { return observation.Devices[i].Name < observation.Devices[j].Name })
	return observation
}
