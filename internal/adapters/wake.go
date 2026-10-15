package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Wake-on-LAN on the controller. The hardware address of each student
// computer is learned from the kernel's IPv4 neighbor table right after the
// computer answered on the lab network (classroom view, computer inventory,
// or a switch-on review that found it on), and kept in a small private file
// outside Git and the Nix store. Nobody types an address.

const (
	wakeAddressFile = "wake-addresses.json"
	wakeAddressLock = "wake-addresses.lock"
	// wakeAddressRefresh is how long an unchanged address is kept without
	// writing its learning time again.
	wakeAddressRefresh = 24 * time.Hour
	wakeAddressLimit   = 1000
	wakeFileBytes      = 1 << 20
	// Three rounds over about one second, to each destination.
	wakeRounds = 3
	wakePause  = 500 * time.Millisecond
)

var (
	wakeNamePattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,62}$`)
	wakeInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:@-]{1,15}$`)
)

// WakeAddressStore keeps learned hardware addresses by computer name. The
// directory belongs to the classroom service (systemd StateDirectory, mode
// 0700, owned by admin); the file is replaced atomically with mode 0600.
type WakeAddressStore struct {
	Directory string
	// Neighbors is the kernel's IPv4 neighbor table.
	Neighbors string
	now       func() time.Time
}

func DefaultWakeAddressStore() WakeAddressStore {
	return WakeAddressStore{Directory: "/var/lib/nixorium/classroom", Neighbors: "/proc/net/arp"}
}

type wakeAddressDocument struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Computers     map[string]domain.WakeAddress `json:"computers"`
}

// WakeAddresses returns the hardware addresses learned so far. A missing,
// unsafe or damaged record reads as empty; the next learned address
// replaces it.
func (Local) WakeAddresses() map[string]domain.WakeAddress {
	return DefaultWakeAddressStore().Read()
}

// LearnWakeAddresses records, best effort, the hardware address of computers
// that just answered on the lab network.
func (Local) LearnWakeAddresses(hosts []domain.HostMeta) {
	_ = DefaultWakeAddressStore().Learn(hosts)
}

// rememberWakeAddresses learns the addresses of computers the inventory
// authenticated.
func rememberWakeAddresses(hosts []domain.HostMeta, probes map[string]domain.HostSystemProbe) {
	answered := []domain.HostMeta{}
	for _, host := range hosts {
		if probe, found := probes[host.Name]; found && probe.SystemPath != "" {
			answered = append(answered, host)
		}
	}
	_ = DefaultWakeAddressStore().Learn(answered)
}

func (store WakeAddressStore) clock() time.Time {
	if store.now != nil {
		return store.now()
	}
	return time.Now()
}

func (store WakeAddressStore) Read() map[string]domain.WakeAddress {
	directory, err := store.openDirectory()
	if err != nil {
		return map[string]domain.WakeAddress{}
	}
	defer unix.Close(directory)
	return readWakeAddresses(directory)
}

// Learn records the neighbor-table address of each host found there. One
// address belongs to one computer: a name that held it before loses it.
func (store WakeAddressStore) Learn(hosts []domain.HostMeta) error {
	if len(hosts) == 0 {
		return nil
	}
	content, err := readSmallFile(store.Neighbors, wakeFileBytes)
	if err != nil {
		return err
	}
	neighbors := neighborAddresses(content)
	found := map[string]string{}
	for _, host := range hosts {
		if mac, known := neighbors[host.IP]; known && wakeNamePattern.MatchString(host.Name) {
			found[host.Name] = mac
		}
	}
	if len(found) == 0 {
		return nil
	}
	now := store.clock().UTC()
	return store.update(func(addresses map[string]domain.WakeAddress) bool {
		changed := false
		for name, mac := range found {
			current, known := addresses[name]
			if known && current.MAC == mac && now.Sub(current.LearnedAt) < wakeAddressRefresh {
				continue
			}
			for other, address := range addresses {
				if other != name && address.MAC == mac {
					delete(addresses, other)
				}
			}
			addresses[name] = domain.WakeAddress{MAC: mac, LearnedAt: now}
			changed = true
		}
		return changed
	})
}

// neighborAddresses reads /proc/net/arp: complete Ethernet entries by IPv4
// address. Proxy entries are skipped, and an address seen with two
// different hardware addresses is left out.
func neighborAddresses(content []byte) map[string]string {
	result := map[string]string{}
	conflicting := map[string]bool{}
	for index, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if index == 0 || len(fields) < 6 || fields[1] != "0x1" || net.ParseIP(fields[0]).To4() == nil {
			continue
		}
		flags, err := strconv.ParseUint(strings.TrimPrefix(fields[2], "0x"), 16, 32)
		// ATF_COM (0x2): resolved; ATF_PUBL (0x8): published for another host.
		if err != nil || flags&0x2 == 0 || flags&0x8 != 0 {
			continue
		}
		mac, err := domain.NormalizeMAC(fields[3])
		if err != nil {
			continue
		}
		if previous, seen := result[fields[0]]; seen && previous != mac {
			conflicting[fields[0]] = true
		}
		result[fields[0]] = mac
	}
	for address := range conflicting {
		delete(result, address)
	}
	return result
}

func readSmallFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("file is too large")
	}
	return content, nil
}

// openDirectory opens the private state directory without following links.
func (store WakeAddressStore) openDirectory() (int, error) {
	directory, err := unix.Open(store.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(directory, &stat); err != nil || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		unix.Close(directory)
		return -1, errors.New("the Wake-on-LAN state directory is not private to this account")
	}
	return directory, nil
}

func wakeFileSafe(descriptor int) bool {
	var stat unix.Stat_t
	return unix.Fstat(descriptor, &stat) == nil && stat.Mode&unix.S_IFMT == unix.S_IFREG &&
		stat.Uid == uint32(os.Geteuid()) && stat.Mode&0o777 == 0o600 && stat.Nlink == 1
}

func readWakeAddresses(directory int) map[string]domain.WakeAddress {
	addresses := map[string]domain.WakeAddress{}
	descriptor, err := unix.Openat(directory, wakeAddressFile, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return addresses
	}
	file := os.NewFile(uintptr(descriptor), wakeAddressFile)
	defer file.Close()
	if !wakeFileSafe(descriptor) {
		return addresses
	}
	content, err := io.ReadAll(io.LimitReader(file, wakeFileBytes+1))
	if err != nil || len(content) > wakeFileBytes {
		return addresses
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var document wakeAddressDocument
	if decoder.Decode(&document) != nil || document.SchemaVersion != 1 {
		return addresses
	}
	for name, address := range document.Computers {
		mac, err := domain.NormalizeMAC(address.MAC)
		if err != nil || mac != address.MAC || !wakeNamePattern.MatchString(name) || len(addresses) >= wakeAddressLimit {
			continue
		}
		addresses[name] = address
	}
	return addresses
}

// update changes the record under its lock and replaces the file only when
// something changed.
func (store WakeAddressStore) update(change func(map[string]domain.WakeAddress) bool) error {
	directory, err := store.openDirectory()
	if err != nil {
		return err
	}
	defer unix.Close(directory)
	lock, err := unix.Openat(directory, wakeAddressLock, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	defer unix.Close(lock)
	if !wakeFileSafe(lock) {
		return errors.New("the Wake-on-LAN lock file is unsafe")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || time.Now().After(deadline) {
			return errors.New("the Wake-on-LAN record is busy")
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	addresses := readWakeAddresses(directory)
	if !change(addresses) {
		return nil
	}
	return writeWakeAddresses(directory, addresses)
}

func writeWakeAddresses(directory int, addresses map[string]domain.WakeAddress) error {
	if len(addresses) > wakeAddressLimit {
		names := make([]string, 0, len(addresses))
		for name := range addresses {
			names = append(names, name)
		}
		// The oldest observations go first.
		sort.Slice(names, func(i, j int) bool { return addresses[names[i]].LearnedAt.Before(addresses[names[j]].LearnedAt) })
		for _, name := range names[:len(addresses)-wakeAddressLimit] {
			delete(addresses, name)
		}
	}
	content, err := json.Marshal(wakeAddressDocument{SchemaVersion: 1, Computers: addresses})
	if err != nil {
		return err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	name := ".wake-addresses-" + hex.EncodeToString(nonce)
	descriptor, err := unix.Openat(directory, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), name)
	defer unix.Unlinkat(directory, name, 0)
	// The mode is explicit whatever the process umask.
	err = file.Chmod(0o600)
	if err == nil {
		_, err = file.Write(append(content, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := unix.Renameat(directory, name, directory, wakeAddressFile); err != nil {
		return err
	}
	return unix.Fsync(directory)
}

// SendWakePackets sends the magic packet of each address as UDP to port 9,
// to the lab subnet's directed broadcast address and to 255.255.255.255,
// three times over about one second. It needs no privilege: the socket only
// asks for broadcast, binds to the controller's lab address when it is
// present, and to the lab interface where the kernel allows it. An address
// gets an error only when none of its packets left.
func (Local) SendWakePackets(ctx context.Context, network domain.WakeNetwork, macs []string) map[string]error {
	results := map[string]error{}
	packets := map[string][]byte{}
	for _, mac := range macs {
		packet, err := domain.MagicPacket(mac)
		if err != nil {
			results[mac] = err
			continue
		}
		packets[mac] = packet
	}
	if len(packets) == 0 {
		return results
	}
	connection, err := openWakeSocket(ctx, network)
	if err != nil {
		for mac := range packets {
			results[mac] = err
		}
		return results
	}
	defer connection.Close()
	for mac, err := range sendWakeRounds(ctx, connection, wakeDestinations(network), packets, wakeRounds, wakePause) {
		results[mac] = err
	}
	return results
}

func wakeDestinations(network domain.WakeNetwork) []net.Addr {
	destinations := []net.Addr{}
	if broadcast := net.ParseIP(network.Broadcast).To4(); broadcast != nil {
		destinations = append(destinations, &net.UDPAddr{IP: broadcast, Port: domain.WakePort})
	}
	return append(destinations, &net.UDPAddr{IP: net.IPv4bcast, Port: domain.WakePort})
}

func openWakeSocket(ctx context.Context, network domain.WakeNetwork) (net.PacketConn, error) {
	device := ""
	if wakeInterfacePattern.MatchString(network.Interface) {
		device = network.Interface
	}
	config := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var optionErr error
		err := raw.Control(func(descriptor uintptr) {
			optionErr = unix.SetsockoptInt(int(descriptor), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
			if optionErr == nil && device != "" {
				// Kernels before 5.7 allow this only with CAP_NET_RAW; the
				// source address below still selects the lab interface.
				_ = unix.SetsockoptString(int(descriptor), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, device)
			}
		})
		if err != nil {
			return err
		}
		return optionErr
	}}
	if source := net.ParseIP(network.Source).To4(); source != nil {
		// The controller's static lab address is absent while network
		// installation runs; any address will do then.
		if connection, err := config.ListenPacket(ctx, "udp4", net.JoinHostPort(source.String(), "0")); err == nil {
			return connection, nil
		}
	}
	return config.ListenPacket(ctx, "udp4", "0.0.0.0:0")
}

// sendWakeRounds writes every packet to every destination in each round.
func sendWakeRounds(ctx context.Context, connection net.PacketConn, destinations []net.Addr, packets map[string][]byte, rounds int, pause time.Duration) map[string]error {
	macs := make([]string, 0, len(packets))
	for mac := range packets {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	sent := map[string]bool{}
	failures := map[string]error{}
	for round := range rounds {
		if round > 0 && !wakeWait(ctx, pause) {
			break
		}
		for _, mac := range macs {
			for _, destination := range destinations {
				if _, err := connection.WriteTo(packets[mac], destination); err != nil {
					failures[mac] = err
				} else {
					sent[mac] = true
				}
			}
		}
	}
	results := map[string]error{}
	for _, mac := range macs {
		if sent[mac] {
			results[mac] = nil
			continue
		}
		if failures[mac] == nil {
			failures[mac] = errors.New("no packet was sent")
		}
		results[mac] = failures[mac]
	}
	return results
}

// wakeWait pauses between rounds; false means the request was cancelled.
func wakeWait(ctx context.Context, pause time.Duration) bool {
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
