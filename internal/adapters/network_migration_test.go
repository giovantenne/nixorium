package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestTransitionProbeTriesTheNewAddressFirst(t *testing.T) {
	reachable := domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
	silent := domain.SSHProbe{Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Detail: "timeout"}
	host := domain.HostMeta{Name: "pc01", IP: "10.1.0.1", PreviousIP: "10.0.0.1"}
	for _, test := range []struct {
		name    string
		answers map[string]domain.SSHProbe
		network domain.HostNetwork
		dialed  string
	}{
		{"moved", map[string]domain.SSHProbe{"10.1.0.1": reachable}, domain.HostNetworkCurrent, "10.1.0.1"},
		{"still on the previous network", map[string]domain.SSHProbe{"10.0.0.1": reachable}, domain.HostNetworkPrevious, "10.1.0.1,10.0.0.1"},
		{"off", map[string]domain.SSHProbe{}, domain.HostNetworkUnknown, "10.1.0.1,10.0.0.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dialed := []string{}
			probe := probeTransitionSSH(host, func(address string) domain.SSHProbe {
				dialed = append(dialed, address)
				if answer, found := test.answers[address]; found {
					return answer
				}
				return silent
			})
			if probe.Network != test.network || strings.Join(dialed, ",") != test.dialed {
				t.Fatalf("probe = %+v, dialed %v", probe, dialed)
			}
		})
	}
	// Outside a change of laboratory network nothing else is tried or labelled.
	dialed := 0
	probe := probeTransitionSSH(domain.HostMeta{Name: "pc01", IP: "10.0.0.1"}, func(string) domain.SSHProbe { dialed++; return silent })
	if dialed != 1 || probe.Network != "" {
		t.Fatalf("probe = %+v after %d dials", probe, dialed)
	}
}

func TestRekeyKnownHostsCopiesTrustedKeysOnly(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "known_hosts")
	backups := filepath.Join(directory, "backups")
	line := func(host string, key ssh.PublicKey) string { return knownhosts.Line([]string{host}, key) }
	first, second, third, other := testKnownHostKey(t), testKnownHostKey(t), testKnownHostKey(t), testKnownHostKey(t)
	original := strings.Join([]string{
		knownhosts.HashHostname("10.0.0.1") + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(first))),
		line("10.0.0.2", second),
		line("10.1.0.2", second),
		line("10.0.0.3", third),
		line("10.1.0.3", other),
		line("unrelated.example", other),
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	moves := []domain.KnownHostMove{
		{Name: "pc01", From: "10.0.0.1", To: "10.1.0.1"},
		{Name: "pc02", From: "10.0.0.2", To: "10.1.0.2"},
		{Name: "pc03", From: "10.0.0.3", To: "10.1.0.3"},
		{Name: "pc04", From: "10.0.0.4", To: "10.1.0.4"},
	}
	id := "0123456789abcdef0123456789abcdef"
	report, err := rekeyKnownHosts(path, backups, id, moves)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(report.Added, ",") != "pc01" || strings.Join(report.Already, ",") != "pc02" ||
		strings.Join(report.Conflicts, ",") != "pc03" || strings.Join(report.Missing, ",") != "pc04" {
		t.Fatalf("report = %+v", report)
	}
	updated, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(updated), original) || !strings.HasSuffix(string(updated), line("10.1.0.1", first)+"\n") {
		t.Fatalf("known hosts:\n%s", updated)
	}
	// The conflicting key for pc03's new address is kept, never replaced.
	if !strings.Contains(string(updated), line("10.1.0.3", other)) || strings.Contains(string(updated), line("10.1.0.3", third)) {
		t.Fatalf("conflict changed:\n%s", updated)
	}
	if backup, err := os.ReadFile(filepath.Join(backups, "known_hosts-"+id+".bak")); err != nil || string(backup) != original {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	// A second pass finds everything recorded and writes nothing.
	again, err := rekeyKnownHosts(path, backups, "fedcba9876543210fedcba9876543210", moves)
	if err != nil || len(again.Added) != 0 || strings.Join(again.Already, ",") != "pc01,pc02" {
		t.Fatalf("again = %+v, %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(backups, "known_hosts-fedcba9876543210fedcba9876543210.bak")); !os.IsNotExist(err) {
		t.Fatalf("unchanged pass wrote a backup: %v", err)
	}
	if _, err := rekeyKnownHosts(path, backups, id, []domain.KnownHostMove{{Name: "pc01", From: "10.0.0.1", To: "example.org"}}); err == nil {
		t.Fatal("a non-address destination was accepted")
	}
}

func TestLabMetaCarriesPreviousAddressesDuringATransition(t *testing.T) {
	data := []byte(`{"schemaVersion":2,"controller":{"name":"pc99","staticIp":"10.1.0.99"},
"clients":{"count":2,"hosts":[{"name":"pc01","ip":"10.1.0.1"},{"name":"pc02","ip":"10.1.0.2"}]},
"network":{"base":"10.1.0.0","prefixLength":24,"ifaceName":"eth1","transition":{"base":"10.0.0.0","prefixLength":24,"controllerIp":"10.0.0.99","startedAt":"2026-10-15T08:00:00Z",
"clients":[{"name":"pc01","ip":"10.0.0.1"},{"name":"pc02","ip":"10.0.0.2"}]}}}`)
	var meta domain.LabMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Clients.Hosts[0].PreviousIP != "10.0.0.1" || meta.Clients.Hosts[1].PreviousIP != "10.0.0.2" || meta.Controller.StaticIP != "10.1.0.99" {
		t.Fatalf("meta = %+v", meta.Clients.Hosts)
	}
	moves := domain.TransitionKnownHostMoves(meta)
	if len(moves) != 2 || moves[1] != (domain.KnownHostMove{Name: "pc02", From: "10.0.0.2", To: "10.1.0.2"}) {
		t.Fatalf("moves = %+v", moves)
	}
}
