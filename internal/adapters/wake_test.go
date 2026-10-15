package adapters

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const neighborTable = `IP address       HW type     Flags       HW address            Mask     Device
10.42.0.11       0x1         0x2         52:54:00:AB:CD:11     *        enp0s3
10.42.0.12       0x1         0x0         00:00:00:00:00:00     *        enp0s3
10.42.0.13       0x1         0x6         52:54:00:ab:cd:13     *        enp0s3
10.42.0.14       0x1         0xa         52:54:00:ab:cd:14     *        enp0s3
10.42.0.15       0x1         0x2         52:54:00:ab:cd:15     *        enp0s3
10.42.0.15       0x1         0x2         52:54:00:ab:cd:99     *        wlp2s0
10.42.0.16       0x20        0x2         52:54:00:ab:cd:16     *        ib0
10.42.0.17       0x1         0x2         01:00:5e:00:00:01     *        enp0s3
`

func TestNeighborAddressesKeepOnlyResolvedEthernetEntries(t *testing.T) {
	got := neighborAddresses([]byte(neighborTable))
	want := map[string]string{"10.42.0.11": "52:54:00:ab:cd:11", "10.42.0.13": "52:54:00:ab:cd:13"}
	if len(got) != len(want) {
		t.Fatalf("neighbors = %v", got)
	}
	for address, mac := range want {
		if got[address] != mac {
			t.Fatalf("neighbors = %v", got)
		}
	}
}

func wakeStore(t *testing.T) (WakeAddressStore, *time.Time) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "classroom")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	neighbors := filepath.Join(t.TempDir(), "arp")
	if err := os.WriteFile(neighbors, []byte(neighborTable), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	store := WakeAddressStore{Directory: directory, Neighbors: neighbors}
	store.now = func() time.Time { return now }
	return store, &now
}

func TestWakeAddressesAreLearnedIntoAPrivateFile(t *testing.T) {
	store, now := wakeStore(t)
	if got := store.Read(); len(got) != 0 {
		t.Fatalf("empty store = %v", got)
	}
	hosts := []domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}, {Name: "pc02", IP: "10.42.0.12"}, {Name: "pc03", IP: "10.42.0.13"}}
	if err := store.Learn(hosts); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Directory, wakeAddressFile)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record = %v, %v", info, err)
	}
	got := store.Read()
	if len(got) != 2 || got["pc01"].MAC != "52:54:00:ab:cd:11" || got["pc03"].MAC != "52:54:00:ab:cd:13" || !got["pc01"].LearnedAt.Equal(*now) {
		t.Fatalf("addresses = %+v", got)
	}
	entries, _ := os.ReadDir(store.Directory)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wake-addresses-") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
	// An unchanged address is not written again until it is a day old.
	before, _ := os.ReadFile(path)
	*now = now.Add(time.Hour)
	if err := store.Learn(hosts); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("an unchanged address was written again")
	}
	*now = now.Add(wakeAddressRefresh)
	if err := store.Learn(hosts[:1]); err != nil {
		t.Fatal(err)
	}
	if got := store.Read(); !got["pc01"].LearnedAt.Equal(*now) || got["pc03"].LearnedAt.Equal(*now) {
		t.Fatalf("refreshed addresses = %+v", got)
	}
}

func TestWakeAddressBelongsToOneComputer(t *testing.T) {
	store, _ := wakeStore(t)
	if err := store.Learn([]domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}}); err != nil {
		t.Fatal(err)
	}
	// The same card now answers as pc05 (the computers were renamed).
	if err := store.Learn([]domain.HostMeta{{Name: "pc05", IP: "10.42.0.11"}}); err != nil {
		t.Fatal(err)
	}
	if got := store.Read(); len(got) != 1 || got["pc05"].MAC != "52:54:00:ab:cd:11" {
		t.Fatalf("addresses = %+v", got)
	}
}

func TestDamagedOrUnsafeWakeRecordReadsAsEmptyAndIsReplaced(t *testing.T) {
	store, _ := wakeStore(t)
	path := filepath.Join(store.Directory, wakeAddressFile)
	for name, content := range map[string]string{
		"not json":      "{",
		"unknown field": `{"schemaVersion":1,"computers":{},"extra":true}`,
		"other version": `{"schemaVersion":2,"computers":{"pc01":{"mac":"52:54:00:ab:cd:11","learnedAt":"2026-10-15T09:00:00Z"}}}`,
		"bad name":      `{"schemaVersion":1,"computers":{"pc/01":{"mac":"52:54:00:ab:cd:11","learnedAt":"2026-10-15T09:00:00Z"}}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := store.Read(); len(got) != 0 {
			t.Fatalf("%s: addresses = %+v", name, got)
		}
	}
	// A record others could read is not trusted either.
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"computers":{"pc01":{"mac":"52:54:00:ab:cd:11","learnedAt":"2026-10-15T09:00:00Z"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := store.Read(); len(got) != 1 {
		t.Fatalf("valid record = %+v", got)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := store.Read(); len(got) != 0 {
		t.Fatalf("world-readable record = %+v", got)
	}
	if err := store.Learn([]domain.HostMeta{{Name: "pc03", IP: "10.42.0.13"}}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if got := store.Read(); len(got) != 1 || got["pc03"].MAC == "" || info.Mode().Perm() != 0o600 {
		t.Fatalf("replaced record = %+v, mode %v", got, info.Mode())
	}
}

func TestWakeRecordRefusesASharedDirectory(t *testing.T) {
	store, _ := wakeStore(t)
	if err := os.Chmod(store.Directory, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := store.Learn([]domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}}); err == nil {
		t.Fatal("a group-accessible directory was written")
	}
	missing := WakeAddressStore{Directory: filepath.Join(t.TempDir(), "absent"), Neighbors: store.Neighbors}
	if err := missing.Learn([]domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}}); err == nil || len(missing.Read()) != 0 {
		t.Fatal("a missing directory was created or read")
	}
}

func TestWakeRoundsSendTheMagicPacketThreeTimes(t *testing.T) {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback UDP:", err)
	}
	defer listener.Close()
	sender, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	packet, _ := domain.MagicPacket("52:54:00:ab:cd:11")
	results := sendWakeRounds(context.Background(), sender, []net.Addr{listener.LocalAddr()}, map[string][]byte{"52:54:00:ab:cd:11": packet}, 3, time.Millisecond)
	if err, found := results["52:54:00:ab:cd:11"]; !found || err != nil {
		t.Fatalf("results = %v", results)
	}
	buffer := make([]byte, 512)
	for range 3 {
		_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
		count, _, err := listener.ReadFrom(buffer)
		if err != nil || !bytes.Equal(buffer[:count], packet) {
			t.Fatalf("received % x, %v", buffer[:count], err)
		}
	}
	// A cancelled request stops after the first round.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closed, _ := net.ListenPacket("udp4", "127.0.0.1:0")
	closed.Close()
	if results := sendWakeRounds(ctx, closed, []net.Addr{listener.LocalAddr()}, map[string][]byte{"52:54:00:ab:cd:11": packet}, 3, time.Hour); results["52:54:00:ab:cd:11"] == nil {
		t.Fatal("a closed socket reported a sent packet")
	}
}

func TestWakeDestinationsAreTheLabBroadcastAndTheLimitedBroadcast(t *testing.T) {
	destinations := wakeDestinations(domain.WakeNetwork{Broadcast: "10.42.0.255"})
	if len(destinations) != 2 || destinations[0].String() != "10.42.0.255:9" || destinations[1].String() != "255.255.255.255:9" {
		t.Fatalf("destinations = %v", destinations)
	}
	if destinations := wakeDestinations(domain.WakeNetwork{}); len(destinations) != 1 || destinations[0].String() != "255.255.255.255:9" {
		t.Fatalf("destinations without a subnet = %v", destinations)
	}
}
