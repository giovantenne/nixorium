package adapters

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
