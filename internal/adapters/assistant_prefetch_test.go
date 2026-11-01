package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// The SRI hash of assistantPrefetchTestBody.
const assistantPrefetchTestBody = "synthetic download\n"
const assistantPrefetchTestHash = "sha256-Fs2UJxgHZH+jYgcDoVaI1kOjO1jlwfcrA+U5C7qnbd4="
const assistantPrefetchTestStore = "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-"

type assistantPrefetchRoundTripper func(*http.Request) (*http.Response, error)

func (f assistantPrefetchRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type assistantPrefetchFixture struct {
	store     AssistantWorkspaceStore
	hosts     map[string][]netip.Addr
	responses map[string]*http.Response
	requests  []string
	runs      [][]string
	limits    []int
	run       assistantCommandRunner
	size      int64
	limit     int64
}

func newAssistantPrefetchFixture(t *testing.T) *assistantPrefetchFixture {
	t.Helper()
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	public := []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("2606:2800:220:1::248")}
	fixture := &assistantPrefetchFixture{
		store: AssistantWorkspaceStore{Directory: state},
		hosts: map[string][]netip.Addr{
			"downloads.example.org":    public,
			"cdn.example.net":          public,
			"loopback.example.org":     {netip.MustParseAddr("127.0.0.1")},
			"ten.example.org":          {netip.MustParseAddr("10.1.2.3")},
			"rfc1918-172.example.org":  {netip.MustParseAddr("172.20.0.1")},
			"rfc1918-192.example.org":  {netip.MustParseAddr("192.168.1.10")},
			"link-local.example.org":   {netip.MustParseAddr("169.254.169.254")},
			"cgnat.example.org":        {netip.MustParseAddr("100.64.0.1")},
			"unspecified.example.org":  {netip.MustParseAddr("0.0.0.0")},
			"loopback6.example.org":    {netip.MustParseAddr("::1")},
			"link-local6.example.org":  {netip.MustParseAddr("fe80::1")},
			"unique-local.example.org": {netip.MustParseAddr("fd00::1")},
			"mapped.example.org":       {netip.MustParseAddr("::ffff:192.168.0.1")},
			"mixed.example.org":        {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.1")},
			"unspecified6.example.org": {netip.MustParseAddr("::")},
		},
		responses: map[string]*http.Response{},
		size:      int64(len(assistantPrefetchTestBody)),
	}
	fixture.run = func(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
		return assistantPrefetchTestStore + args[3] + "\n", "", nil
	}
	return fixture
}

func (fixture *assistantPrefetchFixture) prefetcher() assistantPrefetcher {
	return assistantPrefetcher{
		Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			if addresses, ok := fixture.hosts[host]; ok {
				return addresses, nil
			}
			return nil, errors.New("no such host")
		},
		Transport: assistantPrefetchRoundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet {
				return nil, errors.New("unexpected method " + request.Method)
			}
			fixture.requests = append(fixture.requests, request.URL.String())
			response, ok := fixture.responses[request.URL.String()]
			if !ok {
				response = &http.Response{StatusCode: 200, ContentLength: -1, Header: http.Header{}}
			}
			clone := *response
			clone.Request = request
			body := assistantPrefetchTestBody
			if clone.StatusCode != 200 {
				body = ""
			}
			clone.Body = io.NopCloser(strings.NewReader(body))
			return &clone, nil
		}),
		Limit: fixture.limit,
		Run: func(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
			fixture.runs = append(fixture.runs, append([]string{name}, args...))
			fixture.limits = append(fixture.limits, limit)
			return fixture.run(ctx, limit, name, args...)
		},
		Stat: func(path string) (os.FileInfo, error) {
			if !strings.HasPrefix(path, assistantPrefetchTestStore) {
				return nil, os.ErrNotExist
			}
			file := filepath.Join(fixture.store.Directory, "synthetic-download")
			if err := os.WriteFile(file, nil, 0600); err != nil {
				return nil, err
			}
			if err := os.Truncate(file, fixture.size); err != nil {
				return nil, err
			}
			return os.Lstat(file)
		},
	}
}

func (fixture *assistantPrefetchFixture) execute(t *testing.T, ctx context.Context, raw string) map[string]any {
	t.Helper()
	arguments, _ := json.Marshal(map[string]string{"url": raw})
	result := fixture.prefetcher().Execute(ctx, fixture.store, domain.AssistantToolCall{Name: "prefetch", Arguments: arguments})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatal(result, err)
	}
	if decoded["ok"] == false {
		if message, _ := decoded["message"].(string); message == "" {
			t.Fatal("failure without message", result)
		}
	}
	return decoded
}

func redirect(location string) *http.Response {
	return &http.Response{StatusCode: http.StatusFound, ContentLength: 0, Header: http.Header{"Location": {location}}}
}

func TestAssistantPrefetchURLValidation(t *testing.T) {
	for _, test := range []struct {
		url, code string
	}{
		{"", "invalid_url"},
		{"/releases/tool.AppImage", "invalid_url"},
		{"downloads.example.org/tool.AppImage", "invalid_url"},
		{"http://downloads.example.org/tool.AppImage", "invalid_url"},
		{"ftp://downloads.example.org/tool.AppImage", "invalid_url"},
		{"file:///etc/passwd", "invalid_url"},
		{"https:downloads.example.org/tool.AppImage", "invalid_url"},
		{"https://user:pass@downloads.example.org/tool.AppImage", "invalid_url"},
		{"https://user@downloads.example.org/tool.AppImage", "invalid_url"},
		{"https:///tool.AppImage", "invalid_url"},
		{"https://:443/tool.AppImage", "invalid_url"},
		{"https://downloads.example.org/tool.AppImage#section", "invalid_url"},
		{"https://downloads.example.org/tool.AppImage#", "invalid_url"},
		{"https://downloads.example.org/tool .AppImage", "invalid_url"},
		{"https://downloads.example.org/tool.AppImage\n", "invalid_url"},
		{"https://downloads.example.org/" + strings.Repeat("a", 2048), "invalid_url"},
		{"https://93.184.216.34/tool.AppImage", "host_refused"},
		{"https://[2606:2800:220:1::248]/tool.AppImage", "host_refused"},
		{"https://127.0.0.1/tool.AppImage", "host_refused"},
		{"https://localhost/tool.AppImage", "host_refused"},
		{"https://LOCALHOST./tool.AppImage", "host_refused"},
		{"https://cache.localhost/tool.AppImage", "host_refused"},
		{"https://printer.local/tool.AppImage", "host_refused"},
		{"https://git.corp.internal/tool.AppImage", "host_refused"},
		{"https://nas.lan/tool.AppImage", "host_refused"},
		{"https://loopback.example.org/tool.AppImage", "host_refused"},
		{"https://ten.example.org/tool.AppImage", "host_refused"},
		{"https://rfc1918-172.example.org/tool.AppImage", "host_refused"},
		{"https://rfc1918-192.example.org/tool.AppImage", "host_refused"},
		{"https://link-local.example.org/tool.AppImage", "host_refused"},
		{"https://cgnat.example.org/tool.AppImage", "host_refused"},
		{"https://unspecified.example.org/tool.AppImage", "host_refused"},
		{"https://unspecified6.example.org/tool.AppImage", "host_refused"},
		{"https://loopback6.example.org/tool.AppImage", "host_refused"},
		{"https://link-local6.example.org/tool.AppImage", "host_refused"},
		{"https://unique-local.example.org/tool.AppImage", "host_refused"},
		{"https://mapped.example.org/tool.AppImage", "host_refused"},
		{"https://mixed.example.org/tool.AppImage", "host_refused"},
		{"https://unknown.example.org/tool.AppImage", "download_failed"},
	} {
		fixture := newAssistantPrefetchFixture(t)
		result := fixture.execute(t, t.Context(), test.url)
		if result["ok"] != false || result["error"] != test.code {
			t.Fatalf("%q: %v", test.url, result)
		}
		if len(fixture.requests) != 0 || len(fixture.runs) != 0 {
			t.Fatalf("%q contacted the network: %v %v", test.url, fixture.requests, fixture.runs)
		}
	}
	fixture := newAssistantPrefetchFixture(t)
	exact := "https://downloads.example.org/tool?" + strings.Repeat("a", 2048-len("https://downloads.example.org/tool?"))
	if result := fixture.execute(t, t.Context(), exact); result["ok"] != true {
		t.Fatal("2048-character URL refused", result)
	}
}

func TestAssistantPrefetchSuccess(t *testing.T) {
	fixture := newAssistantPrefetchFixture(t)
	raw := "https://downloads.example.org/releases/v1.2.3/tool-1.2.3.AppImage?download=1"
	arguments, _ := json.Marshal(map[string]string{"url": raw})
	got := fixture.prefetcher().Execute(t.Context(), fixture.store, domain.AssistantToolCall{Name: "prefetch", Arguments: arguments})
	want := `{"hash":"` + assistantPrefetchTestHash + `","ok":true,"size":19,"storePath":"` + assistantPrefetchTestStore + `tool-1.2.3.AppImage","url":"` + raw + `"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !slices.Equal(fixture.requests, []string{raw}) {
		t.Fatal(fixture.requests)
	}
	downloaded := filepath.Join(fixture.store.Directory, "prefetch", "download-")
	if len(fixture.runs) != 1 || len(fixture.runs[0]) != 6 || !slices.Equal(fixture.runs[0][:5], []string{"nix", "store", "add-file", "--name", "tool-1.2.3.AppImage"}) || !strings.HasPrefix(fixture.runs[0][5], downloaded) || fixture.limits[0] != 64*1024 {
		t.Fatal(fixture.runs, fixture.limits)
	}
	if _, err := os.Lstat(fixture.runs[0][5]); !os.IsNotExist(err) {
		t.Fatal("the downloaded file was kept after it entered the store", err)
	}
	if info, err := os.Lstat(filepath.Join(fixture.store.Directory, "prefetch")); err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatal("prefetch lock directory", info, err)
	}
}

func TestAssistantPrefetchRedirectRevalidation(t *testing.T) {
	start := "https://downloads.example.org/latest"
	for _, test := range []struct {
		name     string
		chain    map[string]*http.Response
		code     string
		redirect bool
	}{
		{"public relative and absolute hops", map[string]*http.Response{start: redirect("/v1/tool"), "https://downloads.example.org/v1/tool": redirect("https://cdn.example.net/blob")}, "", false},
		{"downgrade to http", map[string]*http.Response{start: redirect("http://cdn.example.net/blob")}, "invalid_url", true},
		{"userinfo hop", map[string]*http.Response{start: redirect("https://user:pass@cdn.example.net/blob")}, "invalid_url", true},
		{"private hop", map[string]*http.Response{start: redirect("https://ten.example.org/blob")}, "host_refused", true},
		{"IP literal hop", map[string]*http.Response{start: redirect("https://169.254.169.254/latest/meta-data")}, "host_refused", true},
		{"local name hop", map[string]*http.Response{start: redirect("https://printer.local/blob")}, "host_refused", true},
		{"unresolvable hop", map[string]*http.Response{start: redirect("https://unknown.example.org/blob")}, "download_failed", false},
	} {
		fixture := newAssistantPrefetchFixture(t)
		fixture.responses = test.chain
		result := fixture.execute(t, t.Context(), start)
		if test.code == "" {
			if result["ok"] != true || result["url"] != start || len(fixture.requests) != 3 || len(fixture.runs) != 1 || fixture.runs[0][4] != "latest" {
				t.Fatal(test.name, result, fixture.requests, fixture.runs)
			}
			continue
		}
		message, _ := result["message"].(string)
		if result["error"] != test.code || len(fixture.runs) != 0 || strings.HasPrefix(message, "The server redirected") != test.redirect {
			t.Fatal(test.name, result, fixture.runs)
		}
	}
	chain := func(hops int) map[string]*http.Response {
		responses := map[string]*http.Response{}
		for hop := 0; hop < hops; hop++ {
			responses["https://downloads.example.org/"+strconv.Itoa(hop)] = redirect("https://downloads.example.org/" + strconv.Itoa(hop+1))
		}
		return responses
	}
	fixture := newAssistantPrefetchFixture(t)
	fixture.responses = chain(5)
	if result := fixture.execute(t, t.Context(), "https://downloads.example.org/0"); result["ok"] != true || len(fixture.requests) != 6 {
		t.Fatal("five redirects refused", result, fixture.requests)
	}
	fixture = newAssistantPrefetchFixture(t)
	fixture.responses = chain(6)
	if result := fixture.execute(t, t.Context(), "https://downloads.example.org/0"); result["error"] != "download_failed" || !strings.Contains(result["message"].(string), "more than 5 times") || len(fixture.runs) != 0 {
		t.Fatal("six redirects followed", result)
	}
}

func TestAssistantPrefetchSize(t *testing.T) {
	raw := "https://downloads.example.org/tool.tar.gz"
	for _, test := range []struct {
		status int
		length int64
		code   string
	}{
		{200, 1<<30 + 1, "too_large"},
		{200, 1 << 30, ""},
		{200, -1, ""},
		{404, -1, "download_failed"},
	} {
		fixture := newAssistantPrefetchFixture(t)
		fixture.responses[raw] = &http.Response{StatusCode: test.status, ContentLength: test.length, Header: http.Header{}}
		result := fixture.execute(t, t.Context(), raw)
		if result["error"] == nil && test.code != "" || test.code != "" && (result["error"] != test.code || len(fixture.runs) != 0) || test.code == "" && result["ok"] != true {
			t.Fatal(test, result, fixture.runs)
		}
	}
	fixture := newAssistantPrefetchFixture(t)
	fixture.responses[raw] = &http.Response{StatusCode: 404, ContentLength: -1, Header: http.Header{}}
	if result := fixture.execute(t, t.Context(), raw); result["message"] != "The download did not complete: unable to download 'https://downloads.example.org/tool.tar.gz': HTTP error 404." {
		t.Fatal(result)
	}
	// A server that sends more than it announced is cut at the limit.
	fixture = newAssistantPrefetchFixture(t)
	fixture.limit = int64(len(assistantPrefetchTestBody)) - 1
	if result := fixture.execute(t, t.Context(), raw); result["error"] != "too_large" || len(fixture.runs) != 0 {
		t.Fatal("download above the limit accepted", result)
	}
	if names, _ := os.ReadDir(filepath.Join(fixture.store.Directory, "prefetch")); len(names) != 0 {
		t.Fatal("partial download kept", names)
	}
	// What the store holds must be what was downloaded.
	fixture = newAssistantPrefetchFixture(t)
	fixture.size = 1
	if result := fixture.execute(t, t.Context(), raw); result["error"] != "download_failed" {
		t.Fatal("store file of another size accepted", result)
	}
}

func TestAssistantPrefetchStoreName(t *testing.T) {
	for raw, name := range map[string]string{
		"https://downloads.example.org/a/tool.AppImage?download=1": "tool.AppImage",
		"https://downloads.example.org/releases/v1/":               "v1",
		"https://downloads.example.org/a/tool%2B1.deb":             "tool+1.deb",
	} {
		fixture := newAssistantPrefetchFixture(t)
		if result := fixture.execute(t, t.Context(), raw); result["ok"] != true || fixture.runs[0][4] != name || result["storePath"] != assistantPrefetchTestStore+name {
			t.Fatal(raw, result, fixture.runs)
		}
	}
	for _, raw := range []string{"https://downloads.example.org/", "https://downloads.example.org/a/b%20c.txt", "https://downloads.example.org/a/tool;x", "https://downloads.example.org/.hidden"} {
		fixture := newAssistantPrefetchFixture(t)
		if result := fixture.execute(t, t.Context(), raw); result["error"] != "download_failed" || len(fixture.requests) != 0 || len(fixture.runs) != 0 {
			t.Fatal(raw, result, fixture.requests)
		}
	}
	fixture := newAssistantPrefetchFixture(t)
	fixture.run = func(context.Context, int, string, ...string) (string, string, error) {
		return assistantPrefetchTestStore + "other.AppImage\n", "", nil
	}
	if result := fixture.execute(t, t.Context(), "https://downloads.example.org/tool.AppImage"); result["error"] != "download_failed" {
		t.Fatal("a store path of another name accepted", result)
	}
}

func TestAssistantPrefetchRunnerFailures(t *testing.T) {
	raw := "https://downloads.example.org/missing.deb"
	fixture := newAssistantPrefetchFixture(t)
	fixture.run = func(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
		stderr := "\x1b[31;1merror:\x1b[0m cannot connect to socket at '/nix/var/nix/daemon-socket/socket'\n\n       details\n"
		return "", stderr, errors.New("nix: " + sanitizeOperationLog([]byte(stderr)))
	}
	result := fixture.execute(t, t.Context(), raw)
	if result["error"] != "download_failed" || result["message"] != "The download did not complete: cannot connect to socket at '/nix/var/nix/daemon-socket/socket'." {
		t.Fatal(result)
	}
	for _, stdout := range []string{"", "not a path", "/tmp/missing.deb", assistantPrefetchTestStore + "missing.deb\n" + assistantPrefetchTestStore + "missing.deb"} {
		fixture = newAssistantPrefetchFixture(t)
		fixture.run = func(context.Context, int, string, ...string) (string, string, error) { return stdout, "", nil }
		if result := fixture.execute(t, t.Context(), raw); result["error"] != "download_failed" {
			t.Fatal(stdout, result)
		}
	}
	fixture = newAssistantPrefetchFixture(t)
	fixture.run = func(context.Context, int, string, ...string) (string, string, error) {
		return "", strings.Repeat("x", 1000), errors.New("nix: failed")
	}
	if result := fixture.execute(t, t.Context(), raw); result["error"] != "download_failed" || len([]rune(result["message"].(string))) > 300 {
		t.Fatal("reason not bounded", result)
	}
	fixture = newAssistantPrefetchFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	fixture.run = func(context.Context, int, string, ...string) (string, string, error) {
		cancel()
		return "", "", context.Canceled
	}
	if result := fixture.execute(t, ctx, raw); result["error"] != "timeout" {
		t.Fatal(result)
	}
	fixture = newAssistantPrefetchFixture(t)
	fixture.run = func(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > assistantPrefetchTimeout || time.Until(deadline) < assistantPrefetchTimeout-time.Minute {
			t.Error("missing download deadline")
		}
		return "", "", context.DeadlineExceeded
	}
	if result := fixture.execute(t, t.Context(), raw); result["error"] != "download_failed" {
		t.Fatal("runner error without expired context must be a download failure", result)
	}
}

func TestAssistantPrefetchBusy(t *testing.T) {
	fixture := newAssistantPrefetchFixture(t)
	lock, err := assistantPrefetchRootDirectory(fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if result := fixture.execute(t, t.Context(), "https://downloads.example.org/tool.AppImage"); result["error"] != "busy" || len(fixture.requests) != 0 || len(fixture.runs) != 0 {
		t.Fatal(result, fixture.requests, fixture.runs)
	}
	unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	if result := fixture.execute(t, t.Context(), "https://downloads.example.org/tool.AppImage"); result["ok"] != true {
		t.Fatal("lock not released", result)
	}
}

func TestAssistantPrefetchRequiresConsent(t *testing.T) {
	w, store, _ := assistantWorkspaceFixture(t)
	tools := AssistantWorkspaceTools{Store: store}
	call := domain.AssistantToolCall{Name: "prefetch", Arguments: []byte(`{"url":"https://downloads.example.org/tool.AppImage"}`)}
	if got := tools.Execute(t.Context(), w.Conversation, call); got != `{"ok":false,"error":"consent_required"}` {
		t.Fatal(got)
	}
	if got := tools.Execute(t.Context(), w.Conversation, domain.AssistantToolCall{Name: "prefetch", Arguments: []byte(`{"url":"https://downloads.example.org/a","sha256":"x"}`)}); !strings.Contains(got, "invalid_arguments") {
		t.Fatal(got)
	}
	if err := store.state().WriteConsent(); err != nil {
		t.Fatal(err)
	}
	if got := tools.Execute(t.Context(), w.Conversation, domain.AssistantToolCall{Name: "prefetch", Arguments: []byte(`{"url":"http://downloads.example.org/tool.AppImage"}`)}); !strings.Contains(got, `"error":"invalid_url"`) {
		t.Fatal("consented call did not reach the prefetch validation", got)
	}
}

// The real transport connects only to addresses it checks itself: a host that
// resolves again to a private address after the first check (DNS rebinding)
// is never contacted, and every redirect hop is checked again when dialed.
func TestAssistantPrefetchConnectsOnlyToCheckedAddresses(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/moved" {
			http.Redirect(writer, request, "https://example.com/tool.AppImage", http.StatusFound)
			return
		}
		io.WriteString(writer, assistantPrefetchTestBody)
	}))
	defer server.Close()
	public, private := netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.7")
	run := func(t *testing.T, raw string, answers ...[]netip.Addr) (map[string]any, []string) {
		t.Helper()
		fixture := newAssistantPrefetchFixture(t)
		prefetcher := fixture.prefetcher()
		lookups := 0
		prefetcher.Resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
			if host != "example.com" {
				return nil, errors.New("no such host")
			}
			answer := answers[min(lookups, len(answers)-1)]
			lookups++
			return answer, nil
		}
		var dialed []string
		prefetcher.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}
		prefetcher.TLS = server.Client().Transport.(*http.Transport).TLSClientConfig
		prefetcher.Transport = nil
		arguments, _ := json.Marshal(map[string]string{"url": raw})
		var result map[string]any
		if err := json.Unmarshal([]byte(prefetcher.Execute(t.Context(), fixture.store, domain.AssistantToolCall{Name: "prefetch", Arguments: arguments})), &result); err != nil {
			t.Fatal(err)
		}
		return result, dialed
	}
	if result, dialed := run(t, "https://example.com/moved", []netip.Addr{public}); result["ok"] != true || result["hash"] != assistantPrefetchTestHash || result["storePath"] != assistantPrefetchTestStore+"moved" || !slices.Equal(dialed, []string{"93.184.216.34:443"}) {
		t.Fatal("checked download", result, dialed)
	}
	if result, dialed := run(t, "https://example.com/tool.AppImage", []netip.Addr{public}, []netip.Addr{private}); result["error"] != "host_refused" || len(dialed) != 0 {
		t.Fatal("rebinding to a private address was contacted", result, dialed)
	}
	if result, dialed := run(t, "https://example.com/tool.AppImage", []netip.Addr{public}, []netip.Addr{public, private}); result["error"] != "host_refused" || len(dialed) != 0 {
		t.Fatal("a mixed answer was contacted", result, dialed)
	}
}

func TestAssistantPrefetchRefusesReservedRanges(t *testing.T) {
	for _, address := range []string{"192.0.0.8", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.251", "239.255.255.250", "240.0.0.1", "255.255.255.255", "ff02::1", "2001:db8::1"} {
		if assistantPrefetchPublic([]netip.Addr{netip.MustParseAddr(address)}) {
			t.Error(address, "accepted")
		}
	}
	for _, address := range []string{"93.184.216.34", "1.1.1.1", "2606:4700::1111"} {
		if !assistantPrefetchPublic([]netip.Addr{netip.MustParseAddr(address)}) {
			t.Error(address, "refused")
		}
	}
}
