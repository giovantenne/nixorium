package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
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
// loopback, private, link-local and carrier-grade NAT ranges.
var assistantPrefetchRefusedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fc00::/7"),
}

var assistantPrefetchHash = regexp.MustCompile(`^sha256-[A-Za-z0-9+/]{43}=$`)
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
// network and process effects are injectable for tests.
type assistantPrefetcher struct {
	Resolve   func(context.Context, string) ([]netip.Addr, error)
	Transport http.RoundTripper
	Run       assistantCommandRunner
	Stat      func(string) (os.FileInfo, error)
}

func prefetchAssistantFile(ctx context.Context, store AssistantWorkspaceStore, call domain.AssistantToolCall) string {
	return assistantPrefetcher{
		Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		Transport: http.DefaultTransport,
		Run:       runBoundedCommand,
		Stat:      os.Lstat,
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
	for _, address := range addresses {
		address = address.Unmap()
		for _, prefix := range assistantPrefetchRefusedPrefixes {
			if prefix.Contains(address) {
				return nil, refused
			}
		}
	}
	return parsed, nil
}

// head asks for the file size and re-validates every redirect hop. A server
// that omits Content-Length, or answers HEAD with an error, is left to the
// download itself.
func (prefetcher assistantPrefetcher) head(ctx context.Context, target *url.URL) *assistantPrefetchRefusal {
	ctx, cancel := context.WithTimeout(ctx, assistantPrefetchHeadTimeout)
	defer cancel()
	client := &http.Client{Transport: prefetcher.Transport, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > assistantPrefetchRedirects {
			return &assistantPrefetchRefusal{code: "download_failed", reason: "the server redirected more than 5 times"}
		}
		if _, refusal := prefetcher.check(request.Context(), request.URL.String()); refusal != nil {
			refusal.redirect = true
			return refusal
		}
		return nil
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, target.String(), nil)
	if err != nil {
		return &assistantPrefetchRefusal{code: "invalid_url"}
	}
	response, err := client.Do(request)
	if err != nil {
		var refusal *assistantPrefetchRefusal
		if errors.As(err, &refusal) {
			return refusal
		}
		if ctx.Err() != nil {
			return &assistantPrefetchRefusal{code: "download_failed", reason: "the server did not answer within 10 seconds"}
		}
		return &assistantPrefetchRefusal{code: "download_failed", reason: "the server could not be reached"}
	}
	response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 && response.ContentLength > assistantPrefetchSizeLimit {
		return &assistantPrefetchRefusal{code: "too_large", size: response.ContentLength}
	}
	return nil
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
	if refusal = prefetcher.head(ctx, target); refusal != nil {
		return refusal.result()
	}
	runCtx, cancel := context.WithTimeout(ctx, assistantPrefetchTimeout)
	defer cancel()
	stdout, stderr, err := prefetcher.Run(runCtx, assistantPrefetchOutputLimit, "nix", "store", "prefetch-file", "--json", args.URL)
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return (&assistantPrefetchRefusal{code: "timeout", reason: "The download did not finish within 15 minutes."}).result()
	}
	if runCtx.Err() != nil {
		return (&assistantPrefetchRefusal{code: "timeout", reason: "The download was stopped."}).result()
	}
	if err != nil {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: assistantPrefetchReason(stderr, err)}).result()
	}
	var output struct {
		Hash      string `json:"hash"`
		StorePath string `json:"storePath"`
	}
	if json.Unmarshal([]byte(stdout), &output) != nil || !assistantPrefetchHash.MatchString(output.Hash) || !domain.ValidStorePath(output.StorePath) {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: "Nix returned an unexpected answer"}).result()
	}
	info, err := prefetcher.Stat(output.StorePath)
	if err != nil || !info.Mode().IsRegular() {
		return (&assistantPrefetchRefusal{code: "download_failed", reason: "the downloaded file could not be inspected"}).result()
	}
	if info.Size() > assistantPrefetchSizeLimit {
		return (&assistantPrefetchRefusal{code: "too_large", size: info.Size()}).result()
	}
	return assistantToolResult(map[string]any{"ok": true, "url": args.URL, "hash": output.Hash, "size": info.Size(), "storePath": output.StorePath})
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
