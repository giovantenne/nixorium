package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type remotePreparationRoundTrip func(*http.Request) (*http.Response, error)

func (function remotePreparationRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type fakePreparationSSH struct {
	unreachable map[string]bool
}

func (connection *fakePreparationSSH) CheckCacheEndpoint(_ context.Context, cache domain.RemoteInstallCache) error {
	if connection.unreachable[cache.URL] {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (*fakePreparationSSH) VerifySignedClosure(context.Context, domain.RemoteInstallCache, string) error {
	return nil
}
func (*fakePreparationSSH) PullBundle(context.Context, domain.RemoteInstallCache, string) error {
	return nil
}
func (*fakePreparationSSH) Probe(context.Context, string) (domain.RemoteMachineFacts, error) {
	return domain.RemoteMachineFacts{}, nil
}
func (*fakePreparationSSH) VerifyWiredInterface(context.Context, string) error { return nil }

func TestRemotePreparationCacheSelectionPrefersConfiguredObservedDHCP(t *testing.T) {
	preparer, err := NewRemoteInstallPreparer("/tmp/deployment", "/tmp/state")
	if err != nil {
		t.Fatal(err)
	}
	preparer.httpClient = &http.Client{Transport: remotePreparationRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("StoreDir: /nix/store\n"))}, nil
	})}
	meta := domain.LabMeta{}
	meta.Controller.DHCPIP = "192.0.2.10"
	meta.Network.CachePort = 5000
	cache, err := preparer.selectCache(context.Background(), meta,
		[]string{"192.0.2.30/24", "192.0.2.10/24", "169.254.1.1/16", "192.0.2.30/24"},
		"cache.example:YWJjZA==", &fakePreparationSSH{unreachable: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	if cache.URL != "http://192.0.2.10:5000" {
		t.Fatalf("selected cache=%+v", cache)
	}
}

func TestRemotePreparationCacheSelectionRejectsUnresolvedAmbiguity(t *testing.T) {
	preparer, _ := NewRemoteInstallPreparer("/tmp/deployment", "/tmp/state")
	preparer.httpClient = &http.Client{Transport: remotePreparationRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("StoreDir: /nix/store\n"))}, nil
	})}
	meta := domain.LabMeta{}
	meta.Controller.DHCPIP = "192.0.2.99"
	meta.Network.CachePort = 5000
	_, err := preparer.selectCache(context.Background(), meta, []string{"192.0.2.10", "192.0.2.30"},
		"cache.example:YWJjZA==", &fakePreparationSSH{unreachable: map[string]bool{}})
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguity error=%v", err)
	}
}

func TestStrictLiveSSHRejectsUnsafeInterfaceBeforeExecution(t *testing.T) {
	connection := strictLiveSSHFixture(t)
	if err := connection.VerifyWiredInterface(context.Background(), "eth0;reboot"); err == nil {
		t.Fatal("unsafe interface reached strict SSH")
	}
}
