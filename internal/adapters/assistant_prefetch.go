package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

const (
	assistantPrefetchURLLimit    = 2048
	assistantPrefetchSizeLimit   = int64(1) << 30
	assistantPrefetchHeadTimeout = 10 * time.Second
	assistantPrefetchTimeout     = 15 * time.Minute
	assistantPrefetchRedirects   = 5
	assistantPrefetchOutputLimit = 64 * 1024
	assistantPrefetchReasonLimit = 240
)

// Addresses a public download host must never resolve to: unspecified,
// loopback, private, link-local, carrier-grade NAT, protocol-assignment,
// documentation, benchmarking, multicast and reserved ranges.
var assistantPrefetchRefusedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("ff00::/8"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// assistantPrefetchName is a valid Nix store path name.
var assistantPrefetchName = regexp.MustCompile(`^[A-Za-z0-9+_?=-][A-Za-z0-9+._?=-]{0,210}$`)
var assistantPrefetchANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

type assistantPrefetchRefusal struct {
	code     string
	reason   string
	redirect bool
	size     int64
}

func (refusal *assistantPrefetchRefusal) Error() string { return refusal.code }

func (refusal *assistantPrefetchRefusal) result() string {
	message := ""
	switch refusal.code {
	case "invalid_url":
		message = "The address must be an absolute https URL of at most 2048 characters, without user name, password, fragment, spaces or control characters."
	case "host_refused":
		message = "The address points to a local, private or reserved network host; only public internet hosts are allowed."
	case "too_large":
		message = fmt.Sprintf("The server reports %d bytes; the limit is 1 GiB (1073741824 bytes).", refusal.size)
	case "busy":
		message = "Another download is already in progress on the controller; try again when it finishes."
	case "timeout":
		message = refusal.reason
	default:
		message = "The download did not complete: " + refusal.reason + "."
	}
	if refusal.redirect && (refusal.code == "invalid_url" || refusal.code == "host_refused") {
		message = "The server redirected to an address that is not allowed. " + message
	}
	return assistantToolResult(map[string]any{"ok": false, "error": refusal.code, "message": message})
}

// assistantPrefetcher downloads one public file into the Nix store; its
// network and process effects are injectable for tests. The download itself
// runs here, not in Nix: every connection goes only to addresses checked at
// that moment, so neither a redirect nor a host name that resolves again to a
// different address reaches the laboratory network. Through a school proxy
// the proxy resolves the host, and only the host check applies.
type assistantPrefetcher struct {
	Resolve func(context.Context, string) ([]netip.Addr, error)
	Dial    func(context.Context, string, string) (net.Conn, error)
	Proxy   func(*http.Request) (*url.URL, error)
	TLS     *tls.Config
	// Transport replaces the checked transport in tests.
	Transport http.RoundTripper
	Run       assistantCommandRunner
	Stat      func(string) (os.FileInfo, error)
	// Limit replaces the 1 GiB size limit in tests.
	Limit int64
}

func prefetchAssistantFile(ctx context.Context, store AssistantWorkspaceStore, call domain.AssistantToolCall) string {
	return assistantPrefetcher{
		Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		Dial:  (&net.Dialer{Timeout: assistantPrefetchHeadTimeout}).DialContext,
		Proxy: http.ProxyFromEnvironment,
		Run:   runBoundedCommand,
		Stat:  os.Lstat,
	}.Execute(ctx, store, call)
}

// assistantPrefetchParse applies the syntactic URL rules shared by the
// requested address and every redirect hop.
func assistantPrefetchParse(raw string) (*url.URL, *assistantPrefetchRefusal) {
	invalid := &assistantPrefetchRefusal{code: "invalid_url"}
	if raw == "" || len(raw) > assistantPrefetchURLLimit || strings.Contains(raw, "#") {
		return nil, invalid
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] <= ' ' || raw[i] == 0x7f {
			return nil, invalid
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.Hostname() == "" || parsed.Fragment != "" {
		return nil, invalid
	}
	return parsed, nil
}

func (prefetcher assistantPrefetcher) check(ctx context.Context, raw string) (*url.URL, *assistantPrefetchRefusal) {
	parsed, refusal := assistantPrefetchParse(raw)
	if refusal != nil {
		return nil, refusal
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return nil, &assistantPrefetchRefusal{code: "invalid_url"}
	}
	refused := &assistantPrefetchRefusal{code: "host_refused"}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil, refused
	}
	if host == "localhost" {
		return nil, refused
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan"} {
		if strings.HasSuffix(host, suffix) {
			return nil, refused
		}
	}
	addresses, err := prefetcher.Resolve(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, &assistantPrefetchRefusal{code: "download_failed", reason: "the host name " + host + " could not be resolved"}
	}
	if !assistantPrefetchPublic(addresses) {
		return nil, refused
	}
	return parsed, nil
}

// assistantPrefetchPublic reports whether every address is outside the
// refused ranges.
func assistantPrefetchPublic(addresses []netip.Addr) bool {
	for _, address := range addresses {
		address = address.Unmap()
		for _, prefix := range assistantPrefetchRefusedPrefixes {
			if prefix.Contains(address) {
				return false
			}
		}
	}
	return true
}

// assistantPrefetchFileName is the store path name Nix itself would give the
// file: the last element of the decoded URL path, without the query.
func assistantPrefetchFileName(target *url.URL) (string, *assistantPrefetchRefusal) {
	name := path.Base(strings.TrimRight(target.Path, "/"))
	if name == "." || name == "/" {
		name = ""
	}
	if !assistantPrefetchName.MatchString(name) {
		if runes := []rune(name); len(runes) > 80 {
			name = string(runes[:80]) + "…"
		}
		return "", &assistantPrefetchRefusal{code: "download_failed", reason: fmt.Sprintf("the file name %q in the address cannot be a Nix store name", name)}
	}
	return name, nil
}

// transport connects only to checked public addresses, or to the configured
// proxy, which is the only address allowed to be on the local network.
func (prefetcher assistantPrefetcher) transport() http.RoundTripper {
	if prefetcher.Transport != nil {
		return prefetcher.Transport
	}
	var mutex sync.Mutex
	proxies := map[string]bool{}
	proxy := func(request *http.Request) (*url.URL, error) {
		if prefetcher.Proxy == nil {
			return nil, nil
		}
		proxyURL, err := prefetcher.Proxy(request)
		if proxyURL != nil {
			port := proxyURL.Port()
			if port == "" {
				port = map[string]string{"http": "80", "https": "443", "socks5": "1080"}[proxyURL.Scheme]
			}
			mutex.Lock()
			proxies[net.JoinHostPort(proxyURL.Hostname(), port)] = true
			mutex.Unlock()
		}
		return proxyURL, err
	}
	return &http.Transport{
		Proxy: proxy,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			mutex.Lock()
			viaProxy := proxies[address]
			mutex.Unlock()
			if viaProxy {
				return prefetcher.Dial(ctx, network, address)
			}
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := prefetcher.Resolve(ctx, host)
			if err != nil || len(addresses) == 0 {
				return nil, &assistantPrefetchRefusal{code: "download_failed", reason: "the host name " + host + " could not be resolved"}
			}
			if !assistantPrefetchPublic(addresses) {
				return nil, &assistantPrefetchRefusal{code: "host_refused"}
			}
			var dialErr error
			for _, candidate := range addresses {
				connection, err := prefetcher.Dial(ctx, network, net.JoinHostPort(candidate.Unmap().String(), port))
				if err == nil {
					return connection, nil
				}
				dialErr = err
			}
			return nil, dialErr
		},
		TLSClientConfig:       prefetcher.TLS,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   assistantPrefetchHeadTimeout,
		ResponseHeaderTimeout: assistantPrefetchHeadTimeout,
	}
}

// download fetches target into a private file in directory, re-validating
// every redirect hop, and returns its name, size and SRI hash.
func (prefetcher assistantPrefetcher) download(ctx context.Context, target *url.URL, directory *os.File, directoryPath string) (string, int64, string, *assistantPrefetchRefusal) {
	limit := prefetcher.Limit
	if limit == 0 {
		limit = assistantPrefetchSizeLimit
	}
	failed := func(err error) *assistantPrefetchRefusal {
		var refusal *assistantPrefetchRefusal
		switch {
		case errors.As(err, &refusal):
			return refusal
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return &assistantPrefetchRefusal{code: "timeout", reason: "The download did not finish within 15 minutes."}
		case ctx.Err() != nil:
			return &assistantPrefetchRefusal{code: "timeout", reason: "The download was stopped."}
		case errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err):
			return &assistantPrefetchRefusal{code: "download_failed", reason: "the server did not answer within 10 seconds"}
		}
		return &assistantPrefetchRefusal{code: "download_failed", reason: "the server could not be reached"}
	}
	client := &http.Client{Transport: prefetcher.transport(), CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > assistantPrefetchRedirects {
			return &assistantPrefetchRefusal{code: "download_failed", reason: "the server redirected more than 5 times"}
		}
		if _, refusal := prefetcher.check(request.Context(), request.URL.String()); refusal != nil {
			refusal.redirect = true
			return refusal
		}
		return nil
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return "", 0, "", &assistantPrefetchRefusal{code: "invalid_url"}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", 0, "", failed(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", 0, "", &assistantPrefetchRefusal{code: "download_failed", reason: fmt.Sprintf("unable to download '%s': HTTP error %d", target, response.StatusCode)}
	}
	if response.ContentLength > limit {
		return "", 0, "", &assistantPrefetchRefusal{code: "too_large", size: response.ContentLength}
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", 0, "", &assistantPrefetchRefusal{code: "download_failed", reason: "the controller's Assistant state is unavailable"}
	}
	name := "download-" + hex.EncodeToString(random)
	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return "", 0, "", &assistantPrefetchRefusal{code: "download_failed", reason: "the controller's Assistant state is unavailable"}
	}
	file := os.NewFile(uintptr(fd), name)
	digest := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, digest), io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	switch {
	case copyErr != nil:
		unix.Unlinkat(int(directory.Fd()), name, 0)
		return "", 0, "", failed(copyErr)
	case size > limit:
		unix.Unlinkat(int(directory.Fd()), name, 0)
		return "", 0, "", &assistantPrefetchRefusal{code: "too_large", size: size}
	case closeErr != nil:
		unix.Unlinkat(int(directory.Fd()), name, 0)
		return "", 0, "", &assistantPrefetchRefusal{code: "download_failed", reason: "the downloaded file could not be saved"}
	}
	return filepath.Join(directoryPath, name), size, "sha256-" + base64.StdEncoding.EncodeToString(digest.Sum(nil)), nil
}

// assistantPrefetchReason reduces Nix diagnostics to one sanitized line.
func assistantPrefetchReason(stderr string, err error) string {
	text := stderr
	if strings.TrimSpace(text) == "" && err != nil {
		text = strings.TrimPrefix(err.Error(), "nix: ")
	}
	text = sanitizeOperationLog([]byte(assistantPrefetchANSI.ReplaceAllString(text, "")))
	line := ""
	for _, candidate := range strings.Split(text, "\n") {
		candidate = strings.TrimSpace(candidate)
		if strings.HasPrefix(candidate, "error:") {
			line = strings.TrimSpace(strings.TrimPrefix(candidate, "error:"))
			break
		}
		if line == "" && candidate != "" {
			line = candidate
		}
	}
	line = strings.TrimRight(strings.Join(strings.Fields(line), " "), ".")
	if line == "" {
		return "Nix could not download the file"
	}
	if runes := []rune(line); len(runes) > assistantPrefetchReasonLimit {
		line = string(runes[:assistantPrefetchReasonLimit]) + "…"
	}
	return line
}

func (prefetcher assistantPrefetcher) Execute(ctx context.Context, store AssistantWorkspaceStore, call domain.AssistantToolCall) string {
	var args struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(call.Arguments, &args) != nil {
		return assistantToolFailure(nil)
	}
	checkCtx, cancelCheck := context.WithTimeout(ctx, assistantPrefetchHeadTimeout)
	target, refusal := prefetcher.check(checkCtx, args.URL)
	cancelCheck()
	if refusal != nil {
		return refusal.result()
	}
	lock, err := assistantPrefetchRootDirectory(store)
	if err != nil {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: "the controller's Assistant state is unavailable"}).result()
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); errors.Is(err, unix.EWOULDBLOCK) {
		return (&assistantPrefetchRefusal{code: "busy"}).result()
	} else if err != nil {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: "the controller's Assistant state is unavailable"}).result()
	}
	// A download left by an interrupted earlier run is never resumed.
	if names, err := lock.Readdirnames(-1); err == nil {
		for _, name := range names {
			if strings.HasPrefix(name, "download-") {
				unix.Unlinkat(int(lock.Fd()), name, 0)
			}
		}
	}
	name, refusal := assistantPrefetchFileName(target)
	if refusal != nil {
		return refusal.result()
	}
	runCtx, cancel := context.WithTimeout(ctx, assistantPrefetchTimeout)
	defer cancel()
	downloaded, size, hash, refusal := prefetcher.download(runCtx, target, lock, filepath.Join(store.Directory, "prefetch"))
	if refusal != nil {
		return refusal.result()
	}
	defer os.Remove(downloaded)
	stdout, stderr, err := prefetcher.Run(runCtx, assistantPrefetchOutputLimit, "nix", "store", "add-file", "--name", name, downloaded)
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return (&assistantPrefetchRefusal{code: "timeout", reason: "The download did not finish within 15 minutes."}).result()
	}
	if runCtx.Err() != nil {
		return (&assistantPrefetchRefusal{code: "timeout", reason: "The download was stopped."}).result()
	}
	if err != nil {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: assistantPrefetchReason(stderr, err)}).result()
	}
	storePath := strings.TrimSpace(stdout)
	if !domain.ValidStorePath(storePath) || !strings.HasSuffix(storePath, "-"+name) {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: "Nix returned an unexpected answer"}).result()
	}
	info, err := prefetcher.Stat(storePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: "the downloaded file could not be inspected"}).result()
	}
	return assistantToolResult(map[string]any{"ok": true, "url": args.URL, "hash": hash, "size": size, "storePath": storePath})
}

// assistantPrefetchRootDirectory holds the prefetch lock beside the npm
// artifact roots, with the same private ownership checks.
func assistantPrefetchRootDirectory(store AssistantWorkspaceStore) (*os.File, error) {
	root, err := store.state().root()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return assistantDirectory(root, "prefetch", true)
}
