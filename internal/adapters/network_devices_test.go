package adapters

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func fakeSysClassNet(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	device := func(name, kind string, markers ...string) {
		directory := filepath.Join(root, name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "type"), []byte(kind+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if err := os.Mkdir(filepath.Join(directory, marker), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	device("lo", "772")
	device("enp0s3", "1", "device")
	device("wlp2s0", "1", "device", "wireless")
	device("br0", "1", "bridge")
	device("veth1", "1")
	device("enp0s3.10", "1", "lower_enp0s3")
	device("tun0", "65534")
	return root
}

const ipAddressFixture = `[{"ifindex":1,"ifname":"lo","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8,"valid_life_time":4294967295}]},
{"ifindex":2,"ifname":"enp0s3","addr_info":[{"family":"inet","local":"10.0.0.99","prefixlen":24,"valid_life_time":4294967295},{"family":"inet","local":"192.168.0.198","prefixlen":24,"dynamic":true,"valid_life_time":86000}]},
{"ifindex":3,"ifname":"br0","addr_info":[{"family":"inet","local":"192.168.122.1","prefixlen":24,"valid_life_time":3600}]}]`

func TestObserveLocalNetworkClassifiesDevicesAndAddresses(t *testing.T) {
	root := fakeSysClassNet(t)
	observation := observeLocalNetwork(context.Background(), root, func(context.Context) ([]byte, error) { return []byte(ipAddressFixture), nil })
	kinds := []string{}
	for _, device := range observation.Devices {
		kinds = append(kinds, device.Name+":"+device.Kind)
	}
	if got := strings.Join(kinds, ","); got != "br0:wired,enp0s3:wired,enp0s3.10:wired,tun0:other,veth1:virtual,wlp2s0:wireless" {
		t.Fatalf("devices = %s", got)
	}
	if !observation.Lifetimes || observation.Detail != "" {
		t.Fatalf("observation = %+v", observation)
	}
	enp := observation.Devices[1]
	if len(enp.Addresses) != 2 || enp.Addresses[0] != (domain.NetworkAddress{Address: "10.0.0.99"}) || enp.Addresses[1] != (domain.NetworkAddress{Address: "192.168.0.198", Dynamic: true}) {
		t.Fatalf("enp0s3 addresses = %+v", enp.Addresses)
	}
	if bridge := observation.Devices[0]; len(bridge.Addresses) != 1 || !bridge.Addresses[0].Dynamic {
		t.Fatalf("finite lifetime is not dynamic: %+v", bridge)
	}

	fallback := observeLocalNetwork(context.Background(), root, func(context.Context) ([]byte, error) { return nil, errors.New("ip: not found") })
	if fallback.Lifetimes || len(fallback.Devices) != 6 || fallback.Detail == "" {
		t.Fatalf("fallback = %+v", fallback)
	}
	missing := observeLocalNetwork(context.Background(), filepath.Join(root, "absent"), func(context.Context) ([]byte, error) { return nil, nil })
	if len(missing.Devices) != 0 || missing.Detail == "" {
		t.Fatalf("missing root = %+v", missing)
	}
}

func TestParseClientNetworkDevicesIsStrict(t *testing.T) {
	observation := parseClientNetworkDevices("wlp3s0 wireless\nenp3s0 wired\n\n")
	if !observation.Reachable || len(observation.Devices) != 2 || observation.Devices[0].Name != "enp3s0" || observation.Devices[1].Kind != domain.NetworkDeviceWireless {
		t.Fatalf("observation = %+v", observation)
	}
	for _, output := range []string{"enp3s0 wired extra\n", "enp3s0 cable\n", "$(reboot) wired\n", strings.Repeat("eth0 wired\n", maximumNetworkDevices+1)} {
		if observation := parseClientNetworkDevices(output); observation.Reachable {
			t.Fatalf("accepted %q: %+v", output, observation)
		}
	}
}

func TestClientNetworkDevicesCommandMatchesTheControllerClassification(t *testing.T) {
	root := fakeSysClassNet(t)
	output, err := exec.Command("bash", "-c", strings.ReplaceAll(clientNetworkDevicesCommand, "/sys/class/net", root)).Output()
	if err != nil {
		t.Fatal(err)
	}
	observation := parseClientNetworkDevices(string(output))
	local := observeLocalNetwork(context.Background(), root, func(context.Context) ([]byte, error) { return []byte("[]"), nil })
	describe := func(devices []domain.NetworkDevice) string {
		parts := []string{}
		for _, device := range devices {
			parts = append(parts, device.Name+":"+device.Kind)
		}
		return strings.Join(parts, ",")
	}
	if !observation.Reachable || describe(observation.Devices) != describe(local.Devices) {
		t.Fatalf("remote %q differs from local %q (output %q)", describe(observation.Devices), describe(local.Devices), output)
	}
}

func TestClientNetworkDevicesCommandOnlyReads(t *testing.T) {
	for _, forbidden := range []string{">", "rm ", "ip link", "nmcli", "systemctl", "tee"} {
		if strings.Contains(clientNetworkDevicesCommand, forbidden) {
			t.Fatalf("probe command contains %q: %s", forbidden, clientNetworkDevicesCommand)
		}
	}
	arguments := strings.Join(shutdownSSHArguments(domain.HostMeta{Name: "pc01", IP: "10.0.0.1"}, 3e9, clientNetworkDevicesCommand), " ")
	for _, required := range []string{"BatchMode=yes", "ConnectTimeout=3", "PasswordAuthentication=no", "root@10.0.0.1 for d in /sys/class/net/*"} {
		if !strings.Contains(arguments, required) {
			t.Fatalf("missing %q in %s", required, arguments)
		}
	}
}
