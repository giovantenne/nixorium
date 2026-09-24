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
