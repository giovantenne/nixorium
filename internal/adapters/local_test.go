package adapters

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestReadRegularFileNoFollowRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readRegularFileNoFollow(link); err == nil {
		t.Fatal("secure read accepted a symlink")
	}
}

func TestGitStateCountsChangedPaths(t *testing.T) {
	directory := t.TempDir()
	if _, err := run(context.Background(), "git", "init", "-q", directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "one"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "two"), []byte("2"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := (Local{}).GitState(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty || state.Changes != 2 || len(state.Paths) != 2 {
		t.Fatalf("state = %+v, want two changes", state)
	}
}

func TestGitRevisionReturnsCommittedHead(t *testing.T) {
	directory := t.TempDir()
	if _, err := run(context.Background(), "git", "init", "-q", directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "tracked"), []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", directory, "add", "tracked"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", directory, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "initial"); err != nil {
		t.Fatal(err)
	}
	want, err := run(context.Background(), "git", "-C", directory, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := (Local{}).GitRevision(context.Background(), directory)
	if err != nil || revision != strings.TrimSpace(want) {
		t.Fatalf("revision = %q, error = %v, want %q", revision, err, strings.TrimSpace(want))
	}
}

func TestGitStatePreservesPorcelainPaths(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
		paths  []string
	}{
		{"unstaged", " M lab-settings.json\x00", []string{"lab-settings.json"}},
		{"staged", "M  lab-settings.json\x00", []string{"lab-settings.json"}},
		{"both", "MM lab-settings.json\x00", []string{"lab-settings.json"}},
		{"untracked", "?? lab-settings.json\x00", []string{"lab-settings.json"}},
		{"rename", "R  renamed.json\x00lab-settings.json\x00", []string{"renamed.json", "lab-settings.json"}},
		{"spaces", " M path with spaces.json\x00", []string{"path with spaces.json"}},
		{"controls", " M path\nwith\ttabs\x00", []string{"path\nwith\ttabs"}},
		{"clean", "", []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			// Git diagnostics must never become status records.
			script := "#!/bin/sh\nprintf '%s' '" + test.status + "'\necho 'warning: test diagnostic' >&2\n"
			// A shell script cannot contain NUL bytes; printf interprets the escapes.
			script = strings.ReplaceAll(script, "\x00", "\\0")
			script = strings.Replace(script, "printf '%s'", "printf '%b'", 1)
			if err := os.WriteFile(filepath.Join(directory, "git"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
			state, err := (Local{}).GitState(context.Background(), t.TempDir())
			count := 1
			if test.status == "" {
				count = 0
			}
			if err != nil || !state.Available || state.Dirty != (count > 0) || state.Changes != count || !reflect.DeepEqual(state.Paths, test.paths) {
				t.Fatalf("state = %+v, err = %v, want paths %q", state, err, test.paths)
			}
		})
	}
}

func TestDeploymentCommandIsFixedAndUsesArgumentArray(t *testing.T) {
	tests := []struct {
		phase domain.DeploymentPhase
		want  string
	}{
		{phase: domain.DeploymentPhaseBuild, want: "build --on pc01,pc02 --verbose --color never"},
		{phase: domain.DeploymentPhaseApply, want: "apply switch --on pc01,pc02 --verbose --color never"},
	}
	for _, test := range tests {
		arguments, err := deploymentCommand(test.phase, "pc01,pc02")
		if err != nil || strings.Join(arguments, " ") != test.want {
			t.Errorf("%s command = %q, %v", test.phase, strings.Join(arguments, " "), err)
		}
	}
	if _, err := deploymentCommand(domain.DeploymentPhaseComplete, "pc01"); err == nil {
		t.Fatal("unsupported deployment phase was accepted")
	}
	if _, err := deploymentCommand(domain.DeploymentPhaseBuild, ""); err == nil {
		t.Fatal("empty deployment selector was accepted")
	}
}

func TestConfigureColmenaSSHUsesPrivateSupportedConfig(t *testing.T) {
	command := exec.Command("true")
	command.Env = []string{"PATH=/bin", "SSH_CONFIG_FILE=/tmp/untrusted"}
	cleanup, err := configureColmenaSSH(command)
	if err != nil {
		t.Fatal(err)
	}
	path := ""
	pathEnvironmentPreserved := false
	for _, entry := range command.Env {
		if entry == "PATH=/bin" {
			pathEnvironmentPreserved = true
		}
		if strings.HasPrefix(entry, "SSH_CONFIG_FILE=") {
			if path != "" {
				t.Fatalf("duplicate SSH_CONFIG_FILE entries: %q", command.Env)
			}
			path = strings.TrimPrefix(entry, "SSH_CONFIG_FILE=")
		}
	}
	if path == "" || path == "/tmp/untrusted" {
		t.Fatalf("SSH_CONFIG_FILE = %q", path)
	}
	if !pathEnvironmentPreserved {
		t.Fatalf("existing command environment was not preserved: %q", command.Env)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Host *", "BatchMode yes", "PasswordAuthentication no", "StrictHostKeyChecking accept-new", "KbdInteractiveAuthentication no", "ConnectTimeout 10", "ConnectionAttempts 1", "ServerAliveInterval 10", "ServerAliveCountMax 3"} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("SSH config omits %q:\n%s", expected, content)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("SSH config mode = %v, want 0600", info.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("SSH config still exists after cleanup: %v", err)
	}
}

func TestCurrentSystemSSHCommandIsFixedAndNonInteractive(t *testing.T) {
	arguments := sshCurrentSystemArguments(domain.HostMeta{Name: "pc01", IP: "192.0.2.1"}, 2500*time.Millisecond)
	joined := strings.Join(arguments, " ")
	for _, expected := range []string{"BatchMode=yes", "ConnectTimeout=3", "PasswordAuthentication=no", "StrictHostKeyChecking=accept-new", "root@192.0.2.1 nixorium-host-state"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("SSH arguments omit %q: %s", expected, joined)
		}
	}
	if !validSystemPath("/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-pc01") || validSystemPath("/tmp/system") || validSystemPath("/nix/store/path with space") || !validGitRevision("0123456789abcdef0123456789abcdef01234567") || validGitRevision("not-a-revision") {
		t.Fatal("host-state validation accepted or rejected the wrong value")
	}
}

func TestParseProcNetFindsListeningPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tcp")
	content := "  sl  local_address rem_address   st\n   0: 0100007F:1F90 00000000:00000000 0A 00000000:00000000\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	uses, err := parseProcNet(path, "tcp", "0A")
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 1 || uses[0].Protocol != "tcp" || uses[0].Port != 8080 {
		t.Fatalf("uses = %+v, want tcp/8080", uses)
	}
}

func TestCacheHealthValidatesNixMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintln(writer, "StoreDir: /nix/store")
	}))
	defer server.Close()
	address, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	if err := (Local{}).CacheHealth(context.Background(), address, port); err != nil {
		t.Fatal(err)
	}
}

func TestClassifySSHErrorDistinguishesRefusalAndUnreachable(t *testing.T) {
	refused := classifySSHError(syscall.ECONNREFUSED)
	if refused.Reachability != domain.ReachabilityReachable || refused.SSH != domain.SSHUnavailable {
		t.Fatalf("refused = %+v, want reachable without SSH", refused)
	}
	unreachable := classifySSHError(syscall.EHOSTUNREACH)
	if unreachable.Reachability != domain.ReachabilityUnreachable || unreachable.SSH != domain.SSHUnknown {
		t.Fatalf("unreachable = %+v, want unreachable with unknown SSH", unreachable)
	}
}

func TestSSHStatusBoundsConcurrencyAndKeepsEveryHost(t *testing.T) {
	hosts := make([]domain.HostMeta, 24)
	for index := range hosts {
		hosts[index] = domain.HostMeta{Name: fmt.Sprintf("pc%02d", index+1), IP: "192.0.2.1"}
	}
	var active int32
	var maximum int32
	probe := func(context.Context, domain.HostMeta, time.Duration) domain.SSHProbe {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maximum)
			if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
	}

	statuses := probeSSHStatuses(context.Background(), hosts, time.Second, probe)
	if len(statuses) != len(hosts) {
		t.Fatalf("statuses = %d, want %d", len(statuses), len(hosts))
	}
	if maximum < 2 || maximum > maximumConcurrentSSHProbes {
		t.Fatalf("maximum concurrency = %d, want 2..%d", maximum, maximumConcurrentSSHProbes)
	}
}

func TestCurrentSystemObservationBoundsConcurrencyAndKeepsEveryHost(t *testing.T) {
	hosts := make([]domain.HostMeta, 24)
	for index := range hosts {
		hosts[index] = domain.HostMeta{Name: fmt.Sprintf("pc%02d", index+1), IP: "192.0.2.1"}
	}
	var active int32
	var maximum int32
	probe := func(context.Context, domain.HostMeta, time.Duration) domain.HostSystemProbe {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maximum)
			if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return domain.HostSystemProbe{SystemPath: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-system", Revision: "0123456789abcdef0123456789abcdef01234567"}
	}

	statuses := probeCurrentSystems(context.Background(), hosts, time.Second, probe)
	if len(statuses) != len(hosts) {
		t.Fatalf("statuses = %d, want %d", len(statuses), len(hosts))
	}
	if maximum < 2 || maximum > maximumConcurrentSSHProbes {
		t.Fatalf("maximum concurrency = %d, want 2..%d", maximum, maximumConcurrentSSHProbes)
	}
}
