package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestClientOperationLockConflictsWithDeploymentLock(t *testing.T) {
	stateRoot := t.TempDir()
	coordinationDirectory, err := ensureTestCoordinationDirectory(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireOperationGateAt(coordinationDirectory, false)
	if err != nil {
		t.Fatal(err)
	}
	if active, err := operationActiveAt(coordinationDirectory, false); err != nil || !active {
		t.Fatalf("active=%t error=%v", active, err)
	}
	if _, err := openDeploymentOperation(stateRoot, time.Now()); err == nil {
		t.Fatal("deployment started while shutdown held the client-operation lock")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	operation, err := openDeploymentOperation(stateRoot, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer operation.Close()
	if _, err := acquireOperationGateAt(coordinationDirectory, false); err == nil {
		t.Fatal("shutdown started while deployment held the client-operation lock")
	}
}

func TestShutdownSSHArgumentsUseOnlyFixedNonInteractiveCommand(t *testing.T) {
	arguments := shutdownSSHArguments(domain.HostMeta{Name: "pc01", IP: "192.0.2.1"}, 2500*time.Millisecond, "systemctl", "poweroff", "--no-block")
	joined := strings.Join(arguments, " ")
	for _, expected := range []string{"BatchMode=yes", "ConnectTimeout=3", "ClearAllForwardings=yes", "ForwardAgent=no", "root@192.0.2.1 systemctl poweroff --no-block"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("shutdown SSH arguments omit %q: %s", expected, joined)
		}
	}
	if strings.Contains(joined, "pc01") || strings.ContainsAny(joined, ";`$") {
		t.Fatalf("shutdown command contains an identity or shell metacharacter: %s", joined)
	}
}

func TestShutdownSessionObservationAcceptsOnlyExactHelperStates(t *testing.T) {
	directory := t.TempDir()
	ssh := filepath.Join(directory, "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\nprintf '%s\\n' \"$NIXORIUM_TEST_SESSION\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	host := domain.HostMeta{Name: "pc01", IP: "192.0.2.1"}
	for value, expected := range map[string]domain.ShutdownSessionState{"idle": domain.ShutdownSessionIdle, "active": domain.ShutdownSessionActive, "garbage": domain.ShutdownSessionUnknown} {
		t.Setenv("NIXORIUM_TEST_SESSION", value)
		observation := observeShutdownSession(context.Background(), host, time.Second)
		if observation.Session != expected || observation.SSH != domain.SSHAvailable {
			t.Fatalf("value %q observation=%+v", value, observation)
		}
	}
}

func TestDispatchShutdownTreatsConnectionLossAsUnconfirmed(t *testing.T) {
	directory := t.TempDir()
	ssh := filepath.Join(directory, "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\nprintf 'connection closed\\n' >&2\nexit 255\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	result := dispatchShutdown(context.Background(), domain.HostMeta{IP: "192.0.2.1"}, time.Second)
	if result.Accepted || !strings.Contains(result.Detail, "connection closed") {
		t.Fatalf("dispatch=%+v", result)
	}
}
