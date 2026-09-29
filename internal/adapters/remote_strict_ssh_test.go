package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestStrictLiveSSHUsesPinnedNonInteractivePolicy(t *testing.T) {
	connection := strictLiveSSHFixture(t)
	arguments := strings.Join(connection.arguments("/nix/store/00000000000000000000000000000000-bundle/bin/helper probe"), " ")
	for _, expected := range []string{
		"-F /dev/null", "IdentitiesOnly=yes", "IdentityAgent=none", "StrictHostKeyChecking=yes",
		"HostKeyAlgorithms=ssh-ed25519", "BatchMode=yes", "PasswordAuthentication=no",
		"KbdInteractiveAuthentication=no", "ClearAllForwardings=yes", "ForwardAgent=no",
		"PermitLocalCommand=no", "ProxyCommand=none", "ProxyJump=none", "UpdateHostKeys=no",
		"ControlMaster=no", "CanonicalizeHostname=no", "root@192.0.2.20",
		"ConnectionAttempts=1", "ServerAliveInterval=10", "ServerAliveCountMax=3",
	} {
		if !strings.Contains(arguments, expected) {
			t.Fatalf("strict SSH arguments omit %q: %s", expected, arguments)
		}
	}
	for _, forbidden := range []string{"accept-new", "StrictHostKeyChecking=no", "UserKnownHostsFile=/dev/null"} {
		if strings.Contains(arguments, forbidden) {
			t.Fatalf("strict SSH arguments contain %q: %s", forbidden, arguments)
		}
	}
}

func TestStrictLiveSSHPullsOnlySignedApprovedBundle(t *testing.T) {
	connection := strictLiveSSHFixture(t)
	directory := t.TempDir()
	argumentsPath := filepath.Join(directory, "arguments")
	environmentPath := filepath.Join(directory, "environment")
	stdinPath := filepath.Join(directory, "stdin")
	executable := filepath.Join(directory, "ssh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$NIXORIUM_TEST_ARGUMENTS\"\nprintf '%s' \"${SSH_AUTH_SOCK+x}:${SSH_ASKPASS+x}:${DISPLAY+x}\" >\"$NIXORIUM_TEST_ENVIRONMENT\"\ncat >\"$NIXORIUM_TEST_STDIN\"\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	connection.sshExecutable = executable
	t.Setenv("NIXORIUM_TEST_ARGUMENTS", argumentsPath)
	t.Setenv("NIXORIUM_TEST_ENVIRONMENT", environmentPath)
	t.Setenv("NIXORIUM_TEST_STDIN", stdinPath)
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent")
	t.Setenv("SSH_ASKPASS", "/tmp/askpass")
	t.Setenv("DISPLAY", ":99")
	cache := domain.RemoteInstallCache{URL: "http://192.0.2.10:5000", PublicKey: "cache.example:YWJjZA=="}
	bundle := "/nix/store/11111111111111111111111111111111-remote-installer"
	if err := connection.PullBundle(context.Background(), cache, bundle); err != nil {
		t.Fatal(err)
	}
	arguments, _ := os.ReadFile(argumentsPath)
	joined := strings.ReplaceAll(string(arguments), "\n", " ")
	for _, expected := range []string{bundle, "--from " + cache.URL, "trusted-public-keys " + cache.PublicKey, "require-sigs true", "fallback false", "builders ''"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bundle pull omits %q: %s", expected, joined)
		}
	}
	environment, _ := os.ReadFile(environmentPath)
	if string(environment) != "::" {
		t.Fatalf("interactive SSH environment leaked: %q", environment)
	}
	stdin, _ := os.ReadFile(stdinPath)
	if len(stdin) != 0 {
		t.Fatalf("bundle pull sent unexpected stdin: %q", stdin)
	}
}

func TestStrictLiveSSHValidatesPlanBeforeTransfer(t *testing.T) {
	connection := strictLiveSSHFixture(t)
	bundle := "/nix/store/11111111111111111111111111111111-remote-installer"
	if err := connection.ValidatePlan(context.Background(), bundle, []byte(`{"schemaVersion":1}`)); err == nil {
		t.Fatal("invalid plan reached strict SSH")
	}
	plan := validRemoteReservationPlan()
	data, _ := json.Marshal(plan)
	directory := t.TempDir()
	executable := filepath.Join(directory, "ssh")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\ncat >/dev/null\nprintf 'valid\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	connection.sshExecutable = executable
	if err := connection.ValidatePlan(context.Background(), bundle, data); err != nil {
		t.Fatal(err)
	}
}

func TestStrictLiveSSHDispatchesPlanOnStdinAndDecodesAcknowledgement(t *testing.T) {
	connection := strictLiveSSHFixture(t)
	plan := validRemoteReservationPlan()
	bundle := "/nix/store/11111111111111111111111111111111-remote-installer"
	directory := t.TempDir()
	stdinPath := filepath.Join(directory, "stdin")
	executable := filepath.Join(directory, "ssh")
	receipt := `{"schemaVersion":1,"operationId":"0123456789abcdef0123456789abcdef","state":"accepted","phase":"preflight","sequence":1,"mutationStarted":false,"diskMayBeModified":false,"installed":false,"message":"accepted"}`
	script := "#!/bin/sh\ncat >\"$NIXORIUM_TEST_STDIN\"\nprintf '%s\\n' '" + receipt + "'\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	connection.sshExecutable = executable
	t.Setenv("NIXORIUM_TEST_STDIN", stdinPath)
	acknowledgement, err := connection.Dispatch(context.Background(), bundle, plan)
	if err != nil || acknowledgement.State != "accepted" || acknowledgement.Sequence != 1 {
		t.Fatalf("acknowledgement=%+v error=%v", acknowledgement, err)
	}
	content, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeRemoteInstallPlan(content)
	if err != nil || decoded.OperationID != plan.OperationID {
		t.Fatalf("dispatched plan=%+v error=%v", decoded, err)
	}
}

func TestStrictLiveSSHFetchesTheBoundedOperationLog(t *testing.T) {
	connection := strictLiveSSHFixture(t)
	directory := t.TempDir()
	argumentsPath := filepath.Join(directory, "arguments")
	executable := filepath.Join(directory, "ssh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$NIXORIUM_TEST_ARGUMENTS\"\nprintf '/mnt is already occupied\\n'\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	connection.sshExecutable = executable
	t.Setenv("NIXORIUM_TEST_ARGUMENTS", argumentsPath)
	operationID := "0123456789abcdef0123456789abcdef"
	bundle := "/nix/store/11111111111111111111111111111111-remote-installer"
	content, err := connection.OperationLog(context.Background(), bundle, operationID)
	if err != nil || string(content) != "/mnt is already occupied\n" {
		t.Fatalf("content=%q error=%v", content, err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	command := bundle + "/bin/nixorium-remote-client-installer log " + operationID
	if !strings.Contains(string(arguments), command) {
		t.Fatalf("operation log command missing from SSH arguments: %s", arguments)
	}
}

func TestStrictLiveSSHRevalidatesEthernetAndWiFiWithoutRelaxingIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		iface     string
		wantError string
	}{
		{name: "ethernet", iface: "enp0s2"},
		{name: "wifi", iface: "wlp2s0"},
		{name: "changed-interface", iface: "wlp2s0", wantError: "declared live network changed"},
		{name: "changed-address", iface: "wlp2s0", wantError: "declared live network changed"},
		{name: "changed-boot", iface: "wlp2s0", wantError: "remote boot identity"},
		{name: "changed-disk", iface: "wlp2s0", wantError: "reviewed disk identity"},
		{name: "cache-unreachable", iface: "wlp2s0", wantError: "revalidate remote cache endpoint"},
		{name: "signature-failure", iface: "wlp2s0", wantError: "verify signed closure metadata"},
		{name: "unsafe-interface", iface: "wlp2s0;reboot", wantError: "host identity is invalid"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			connection := strictLiveSSHFixture(t)
			plan := validRemoteReservationPlan()
			plan.Host.Interface = scenario.iface
			preparation := validInstalledVerificationPreparation(plan.OperationID)
			preparation.Host = plan.Host
			preparation.HostKeyPublic = plan.HostKeyPublic
			preparation.HostFingerprint = "SHA256:" + strings.Repeat("A", 43)
			facts := preparation.Facts
			facts.Interfaces = []domain.RemoteNetworkInterface{{Name: scenario.iface, Addresses: []string{plan.Host.LiveIP}}}
			facts.Disks = []domain.RemoteDisk{plan.Disk}
			facts.Disks[0].Eligible = true
			switch scenario.name {
			case "changed-interface":
				facts.Interfaces[0].Name = "wlp3s0"
			case "changed-address":
				facts.Interfaces[0].Addresses = []string{"192.0.2.21"}
			case "changed-boot":
				facts.BootID = "ffffffff-89ab-cdef-0123-456789abcdef"
			case "changed-disk":
				facts.Disks[0].Serial = "replacement"
			}
			data, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			factsPath := filepath.Join(directory, "facts.json")
			if err := os.WriteFile(factsPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(directory, "ssh")
			// Only the existing cache, signed metadata, inventory, and plan checks
			// may run. A reintroduced wireless-interface refusal fails this fixture.
			script := `#!/bin/sh
for command do :; done
case "$command" in
  */nix-cache-info)
    test "$NIXORIUM_TEST_SCENARIO" != cache-unreachable || exit 1
    printf 'StoreDir: /nix/store\n' ;;
  *' path-info '*)
    test "$NIXORIUM_TEST_SCENARIO" != signature-failure || exit 1 ;;
  *' probe') cat "$NIXORIUM_TEST_FACTS" ;;
  *' validate-plan') cat >/dev/null; printf 'valid\n' ;;
  *) exit 99 ;;
esac
`
			if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			connection.sshExecutable = executable
			t.Setenv("NIXORIUM_TEST_FACTS", factsPath)
			t.Setenv("NIXORIUM_TEST_SCENARIO", scenario.name)
			err = connection.Revalidate(context.Background(), preparation, plan)
			if scenario.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), scenario.wantError) {
				t.Fatalf("expected %q, got %v", scenario.wantError, err)
			}
		})
	}
}

func strictLiveSSHFixture(t *testing.T) *StrictLiveSSH {
	t.Helper()
	directory := t.TempDir()
	privateKey := filepath.Join(directory, "id_ed25519")
	knownHosts := filepath.Join(directory, "known_hosts")
	for _, path := range []string{privateKey, knownHosts} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	connection, err := NewStrictLiveSSH(VerifiedLiveSession{Address: "192.0.2.20", Port: 22, PrivateKeyPath: privateKey, KnownHostsPath: knownHosts})
	if err != nil {
		t.Fatal(err)
	}
	return connection
}
