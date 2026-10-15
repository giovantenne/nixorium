package domain

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// Network device kinds observed read-only in /sys/class/net.
const (
	NetworkDeviceWired    = "wired"
	NetworkDeviceWireless = "wireless"
	NetworkDeviceVirtual  = "virtual"
	NetworkDeviceOther    = "other"
)

// NetworkAddress is one IPv4 address of a network device. Dynamic means it
// has a finite lifetime, as addresses from a DHCP server do; a static address
// such as the laboratory address never expires.
type NetworkAddress struct {
	Address string `json:"address"`
	Dynamic bool   `json:"dynamic,omitempty"`
}

// NetworkDevice is one network device of a computer.
type NetworkDevice struct {
	Name      string           `json:"name"`
	Kind      string           `json:"kind"`
	Addresses []NetworkAddress `json:"addresses,omitempty"`
}

// NetworkObservation is the read-only device list of one computer. Reachable
// is meaningful only for clients; the controller is observed locally.
type NetworkObservation struct {
	Reachable bool            `json:"reachable"`
	Devices   []NetworkDevice `json:"devices"`
	// Detail explains a failed or partial observation.
	Detail string `json:"detail,omitempty"`
	// Lifetimes is true when address lifetimes were read, so Dynamic is
	// meaningful.
	Lifetimes bool `json:"lifetimes,omitempty"`
}

// Preflight states of a network device check.
const (
	DevicePresent     = "present"
	DeviceMissing     = "missing"
	DeviceNotWired    = "not-wired"
	DeviceUnknown     = "unknown"
	DeviceUnreachable = "unreachable"
)

// NetworkPreflight records read-only checks of the network devices that a
// reviewed settings change would use. It never changes a computer.
type NetworkPreflight struct {
	Controller  *ControllerNetworkCheck `json:"controller,omitempty"`
	Clients     []ClientNetworkCheck    `json:"clients,omitempty"`
	DHCPAddress *DHCPAddressCheck       `json:"dhcpAddress,omitempty"`
	// Acknowledged lists the computers the administrator chose to save
	// anyway for, although the named device is missing there.
	Acknowledged []string `json:"acknowledged,omitempty"`
}

// ControllerNetworkCheck is the controller part of the preflight.
type ControllerNetworkCheck struct {
	Interface  string          `json:"interface"`
	Previous   string          `json:"previous"`
	State      string          `json:"state"`
	Blocking   bool            `json:"blocking"`
	DHCPDevice string          `json:"dhcpDevice,omitempty"`
	Devices    []NetworkDevice `json:"devices"`
	Message    string          `json:"message"`
}

// ClientNetworkCheck is the preflight of one client computer.
type ClientNetworkCheck struct {
	Name      string   `json:"name"`
	IP        string   `json:"ip"`
	Interface string   `json:"interface"`
	Previous  string   `json:"previous,omitempty"`
	State     string   `json:"state"`
	Devices   []string `json:"devices,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

// DHCPAddressCheck compares a chosen controller DHCP address with the
// addresses the controller has now.
type DHCPAddressCheck struct {
	Address  string   `json:"address"`
	Detected []string `json:"detected"`
	Found    bool     `json:"found"`
	Message  string   `json:"message,omitempty"`
}

// ControllerInterfaceFor resolves the controller's laboratory device in the
// same order as mkLab: per-computer override, controller name, fallback.
func ControllerInterfaceFor(lab LabSettings) string {
	if name := lab.HostInterfaceNames[fmt.Sprintf("pc%02d", lab.MasterHostNumber)]; name != "" {
		return name
	}
	return lab.ControllerInterface()
}

// ClientInterfaceFor resolves a client's device name. A client is configured
// by name only when an override or the client name is set (ADR 0017);
// otherwise it uses its wired card whatever the name, and explicit is false.
func ClientInterfaceFor(lab LabSettings, host string) (name string, explicit bool) {
	if name := lab.HostInterfaceNames[host]; name != "" {
		return name, true
	}
	if lab.ClientInterfaceName != "" {
		return lab.ClientInterfaceName, true
	}
	return "", false
}

// NetworkPreflightNeeds reports which checks a reviewed change needs: the
// controller when its device name changes, and each existing client whose
// configured device name changes. New clients are not installed yet.
func NetworkPreflightNeeds(base, candidate LabSettingsFile) (controller bool, clients []HostMeta) {
	if candidate.Lab.DeploymentMode == "controller" {
		return false, nil
	}
	controller = ControllerInterfaceFor(base.Lab) != ControllerInterfaceFor(candidate.Lab)
	if base.Lab.DeploymentMode == "controller" {
		return controller, nil
	}
	for number := 1; number <= min(base.Lab.PCCount, candidate.Lab.PCCount); number++ {
		name := fmt.Sprintf("pc%02d", number)
		after, explicit := ClientInterfaceFor(candidate.Lab, name)
		before, wasExplicit := ClientInterfaceFor(base.Lab, name)
		if !explicit || (wasExplicit && before == after) {
			continue
		}
		address, err := LabHostAddress(base.Lab, number)
		if err != nil {
			continue
		}
		required := true
		clients = append(clients, HostMeta{Name: name, IP: address, Interface: after, InterfaceExplicit: &required})
	}
	return controller, clients
}

// LabHostAddress is the static laboratory address of host number n.
func LabHostAddress(lab LabSettings, number int) (string, error) {
	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", lab.NetworkBase, lab.NetworkPrefix))
	if err != nil || !prefix.Addr().Is4() || prefix.Masked() != prefix || prefix.Bits() < 1 || prefix.Bits() > 30 {
		return "", fmt.Errorf("laboratory network is not a canonical IPv4 prefix")
	}
	if number < 1 || uint64(number) >= (uint64(1)<<(32-prefix.Bits()))-1 {
		return "", fmt.Errorf("host number %d does not fit in the laboratory prefix", number)
	}
	octets := prefix.Addr().As4()
	value := uint32(octets[0])<<24 | uint32(octets[1])<<16 | uint32(octets[2])<<8 | uint32(octets[3]) + uint32(number)
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}).String(), nil
}

// DHCPDevice names the device carrying the controller's DHCP address: the
// saved address when the controller has it, otherwise the first device with
// a dynamic (leased) address.
func DHCPDevice(devices []NetworkDevice, savedAddress string) string {
	for _, device := range devices {
		for _, address := range device.Addresses {
			if address.Address == savedAddress {
				return device.Name
			}
		}
	}
	for _, device := range devices {
		for _, address := range device.Addresses {
			if address.Dynamic && !linkLocal(address.Address) {
				return device.Name
			}
		}
	}
	return ""
}

func linkLocal(address string) bool {
	parsed, err := netip.ParseAddr(address)
	return err == nil && parsed.IsLinkLocalUnicast()
}

// DescribeNetworkDevices lists devices for a sentence, for example
// "enp0s3 (192.168.0.198, carries the DHCP address), wlp2s0".
func DescribeNetworkDevices(devices []NetworkDevice, dhcpDevice string) string {
	parts := make([]string, 0, len(devices))
	for _, device := range devices {
		details := []string{}
		for _, address := range device.Addresses {
			if !linkLocal(address.Address) {
				details = append(details, address.Address)
			}
		}
		if device.Name == dhcpDevice {
			details = append(details, "carries the DHCP address")
		}
		if device.Kind == NetworkDeviceWireless {
			details = append(details, "wireless")
		}
		if len(details) == 0 {
			parts = append(parts, device.Name)
		} else {
			parts = append(parts, device.Name+" ("+strings.Join(details, ", ")+")")
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// CheckControllerInterface classifies the controller's chosen device. A
// missing or non-wired device blocks the save; a device that does not carry
// the DHCP address is only a warning.
func CheckControllerInterface(previous, chosen string, observation NetworkObservation, savedDHCP string) ControllerNetworkCheck {
	check := ControllerNetworkCheck{Interface: chosen, Previous: previous, Devices: observation.Devices}
	if check.Devices == nil {
		check.Devices = []NetworkDevice{}
	}
	if len(observation.Devices) == 0 {
		check.State = DeviceUnknown
		detail := observation.Detail
		if detail == "" {
			detail = "no network device was found"
		}
		check.Message = "The controller's network devices could not be read (" + detail + "). Check that " + chosen + " exists before applying."
		return check
	}
	check.DHCPDevice = DHCPDevice(observation.Devices, savedDHCP)
	list := DescribeNetworkDevices(observation.Devices, check.DHCPDevice)
	for _, device := range observation.Devices {
		if device.Name != chosen {
			continue
		}
		if device.Kind != NetworkDeviceWired {
			check.State, check.Blocking = DeviceNotWired, true
			kind := "not a wired network device"
			if device.Kind == NetworkDeviceWireless {
				kind = "a wireless device"
			}
			check.Message = fmt.Sprintf("The controller's %s is %s; the laboratory needs a wired one. Its devices are: %s.", chosen, kind, list)
			return check
		}
		check.State = DevicePresent
		switch {
		case check.DHCPDevice == "" || check.DHCPDevice == chosen:
			check.Message = fmt.Sprintf("The controller has %s. Its devices are: %s.", chosen, list)
		default:
			check.Message = fmt.Sprintf("The controller has %s, but its DHCP address is on %s. Check that the laboratory cable is on %s before applying. Its devices are: %s.", chosen, check.DHCPDevice, chosen, list)
		}
		return check
	}
	check.State, check.Blocking = DeviceMissing, true
	check.Message = fmt.Sprintf("The controller has no network device called %s. Its devices are: %s.", chosen, list)
	return check
}

// CheckClientInterface classifies one client from its read-only observation.
func CheckClientInterface(host HostMeta, previous string, observation NetworkObservation) ClientNetworkCheck {
	check := ClientNetworkCheck{Name: host.Name, IP: host.IP, Interface: host.Interface, Previous: previous}
	if !observation.Reachable {
		check.State = DeviceUnreachable
		check.Detail = observation.Detail
		return check
	}
	for _, device := range observation.Devices {
		check.Devices = append(check.Devices, device.Name)
		if device.Name == host.Interface {
			check.State = DevicePresent
		}
	}
	if check.State == "" {
		check.State = DeviceMissing
	}
	return check
}

// Missing lists clients where the named device is missing.
func (preflight *NetworkPreflight) Missing() []string {
	if preflight == nil {
		return nil
	}
	names := []string{}
	for _, client := range preflight.Clients {
		if client.State == DeviceMissing {
			names = append(names, client.Name)
		}
	}
	return names
}

// Unreachable lists clients that could not be checked.
func (preflight *NetworkPreflight) Unreachable() []string {
	if preflight == nil {
		return nil
	}
	names := []string{}
	for _, client := range preflight.Clients {
		if client.State == DeviceUnreachable {
			names = append(names, client.Name)
		}
	}
	return names
}

// SaveRefusal returns why the reviewed settings must not be saved, or "".
// A controller without the chosen wired device always refuses. A client
// without its named device refuses unless the administrator acknowledged
// exactly that computer.
func (preflight *NetworkPreflight) SaveRefusal() string {
	if preflight == nil {
		return ""
	}
	if preflight.Controller != nil && preflight.Controller.Blocking {
		return preflight.Controller.Message + " Choose one of its wired devices."
	}
	acknowledged := map[string]bool{}
	for _, name := range preflight.Acknowledged {
		acknowledged[name] = true
	}
	blocked := []string{}
	for _, name := range preflight.Missing() {
		if !acknowledged[name] {
			blocked = append(blocked, name)
		}
	}
	if len(blocked) == 0 {
		return ""
	}
	sort.Strings(blocked)
	return fmt.Sprintf("The named network device is missing on %s. Correct the name, or choose to save anyway for the computers listed.", strings.Join(blocked, ", "))
}

// ControllerDHCPCandidates lists the controller's current addresses that can
// be its DHCP address: IPv4, not link-local, outside the static laboratory
// network, and dynamic. Without lifetime information (dynamic unknown) every
// remaining address is offered.
func ControllerDHCPCandidates(observation NetworkObservation, lab LabSettings) []DHCPAddressChoice {
	labAddress, _ := ControllerStaticAddress(lab)
	labPrefix, prefixErr := netip.ParsePrefix(fmt.Sprintf("%s/%d", lab.NetworkBase, lab.NetworkPrefix))
	choices := []DHCPAddressChoice{}
	seen := map[string]bool{}
	for _, device := range observation.Devices {
		for _, address := range device.Addresses {
			parsed, err := netip.ParseAddr(address.Address)
			if err != nil || !parsed.Is4() || parsed.IsLinkLocalUnicast() || parsed.IsLoopback() || address.Address == labAddress || seen[address.Address] {
				continue
			}
			if prefixErr == nil && labPrefix.Contains(parsed) {
				continue
			}
			if observation.Lifetimes && !address.Dynamic {
				continue
			}
			seen[address.Address] = true
			choices = append(choices, DHCPAddressChoice{Address: address.Address, Device: device.Name})
		}
	}
	return choices
}

// DHCPAddressChoice is one detected controller address offered as the DHCP
// address, with the device that carries it.
type DHCPAddressChoice struct {
	Address string `json:"address"`
	Device  string `json:"device"`
}

// ControllerAddresses is what the settings form detects on the controller.
type ControllerAddresses struct {
	Choices []DHCPAddressChoice `json:"choices"`
	// Detected is false when the controller's addresses could not be read;
	// the form then keeps a typed value.
	Detected bool   `json:"detected"`
	Detail   string `json:"detail,omitempty"`
}

// CheckDHCPAddress warns when a chosen address is not one the controller has.
func CheckDHCPAddress(address string, choices []DHCPAddressChoice) DHCPAddressCheck {
	check := DHCPAddressCheck{Address: address, Detected: []string{}}
	for _, choice := range choices {
		check.Detected = append(check.Detected, choice.Address)
		if choice.Address == address {
			check.Found = true
		}
	}
	if !check.Found {
		found := "none"
		if len(check.Detected) > 0 {
			found = strings.Join(check.Detected, ", ")
		}
		check.Message = fmt.Sprintf("%s is not one of the controller's current DHCP addresses (found: %s). Check it before preparing network installation.", address, found)
	}
	return check
}
