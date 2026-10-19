package adapters

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

// writeFakeSSH puts an ssh command that runs script in front of the real one.
func writeFakeSSH(t *testing.T, script string) {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "ssh"), []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCurrentSystemProbeParsesOnlyStandardOutput(t *testing.T) {
	system := "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system"
	revision := strings.Repeat("a", 40)
	writeFakeSSH(t, "echo 'Warning: noise on stderr' >&2\nprintf '%s\\n%s\\n' '"+system+"' '"+revision+"'")
	probe := probeCurrentSystemArguments(context.Background(), 5*time.Second, nil)
	if probe.SystemPath != system || probe.Revision != revision || probe.Detail != "" {
		t.Fatalf("probe = %+v", probe)
	}
}

func TestCurrentSystemProbeReadsTheIdentityNumber(t *testing.T) {
	system := "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system"
	revision := strings.Repeat("a", 40)
	writeFakeSSH(t, "printf '%s\\n%s\\n7\\n' '"+system+"' '"+revision+"'")
	if probe := probeCurrentSystemArguments(context.Background(), 5*time.Second, nil); probe.Number != 7 || probe.Revision != revision {
		t.Fatalf("probe = %+v", probe)
	}
	for _, third := range []string{"07", "0", "x", "12345"} {
		writeFakeSSH(t, "printf '%s\\n%s\\n"+third+"\\n' '"+system+"' '"+revision+"'")
		if probe := probeCurrentSystemArguments(context.Background(), 5*time.Second, nil); probe.SystemPath != "" {
			t.Fatalf("third line %q accepted: %+v", third, probe)
		}
	}
}

func TestCurrentSystemProbeKeepsWhatAnInvalidAnswerSaid(t *testing.T) {
	const prefix = "authenticated system observation returned invalid state"
	for _, test := range []struct {
		name, script, detail string
	}{
		{"silent", "exit 0", prefix},
		{"stdout", "printf 'line one\\nline\\ttwo\\001\\n'", prefix + ` (stdout: "line one\nline\ttwo\x01")`},
		{"both", "echo out; echo err >&2", prefix + ` (stdout: "out"; stderr: "err")`},
		{"stderr only", "echo 'Connection reset' >&2", prefix + ` (stderr: "Connection reset")`},
		{"long", "head -c 1000 /dev/zero | tr '\\0' x", prefix + ` (stdout: "` + strings.Repeat("x", 160) + `")`},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFakeSSH(t, test.script)
			probe := probeCurrentSystemArguments(context.Background(), 5*time.Second, nil)
			if probe.Revision != "" || probe.SystemPath != "" || probe.Detail != test.detail {
				t.Fatalf("probe = %+v, want detail %q", probe, test.detail)
			}
		})
	}
}

func TestCurrentSystemProbeFailureDetailComesFromStandardError(t *testing.T) {
	writeFakeSSH(t, "echo 'partial output'\necho '@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@' >&2\nexit 255")
	probe := probeCurrentSystemArguments(context.Background(), 5*time.Second, nil)
	if probe.HostKeyCondition != domain.HostKeyChanged || !strings.HasPrefix(probe.Detail, "authenticated system observation failed: @@@ WARNING") {
		t.Fatalf("probe = %+v", probe)
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
