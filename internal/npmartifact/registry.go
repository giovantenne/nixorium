// Package npmartifact prepares public npm CLI packages without executing package code.
package npmartifact

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

const MaxPackages = 512
const MaxLockBytes = 512 * 1024
const MaxMetadataBytes = 8 * 1024 * 1024
const MaxMetadataTotal = 64 * 1024 * 1024
const MaxDownloadBytes = 128 * 1024 * 1024
const MaxDownloadTotal = 512 * 1024 * 1024
const MaxExpandedBytes = 1024 * 1024 * 1024
const MaxEntries = 50000

var namePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]{0,99}/)?[a-z0-9][a-z0-9._-]{0,99}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?(?:\+[A-Za-z0-9.-]+)?$`)
var selectorPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
var rangePattern = regexp.MustCompile(`^[0-9A-Za-z.*<>=~^| +_-]{1,200}$`)
var IDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

func ValidName(s string) bool { return namePattern.MatchString(s) }
func ValidSelector(s string) bool {
	return versionPattern.MatchString(s) || selectorPattern.MatchString(s)
}
func ValidVersion(s string) bool { return versionPattern.MatchString(s) }

func TarballURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host != "registry.npmjs.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Opaque != "" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	var name, file string
	if len(parts) == 3 && parts[1] == "-" {
		name, file = parts[0], parts[2]
	}
	if len(parts) == 4 && strings.HasPrefix(parts[0], "@") && parts[2] == "-" {
		name, file = parts[0]+"/"+parts[1], parts[3]
	}
	return ValidName(name) && regexp.MustCompile(`^[a-zA-Z0-9._-]+\.tgz$`).MatchString(file) && !strings.Contains(file, "..")
}

func Integrity(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "sha512-") || strings.ContainsAny(s, " \t\r\n") {
		return nil, errors.New("unsupported_integrity")
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "sha512-"))
	if err != nil || len(b) != 64 {
		return nil, errors.New("unsupported_integrity")
	}
	return b, nil
}

type Registry struct {
	HTTP *http.Client
	// Origin is set only by tests. Production constructs the fixed public origin.
	Origin   string
	Context  context.Context
	mu       sync.Mutex
	metadata int
	requests int
	cache    map[string][]byte
}

// NewRegistry reads the public npm registry through the system proxy
// (http_proxy, https_proxy, no_proxy), like the controller's other clients:
// a school whose Internet passes through a mandatory proxy can still prepare
// packages. Redirects are never followed.
func NewRegistry(ctx context.Context) *Registry {
	return &Registry{Context: ctx, Origin: "https://registry.npmjs.org", HTTP: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, cache: map[string][]byte{}}
}

func (r *Registry) Get(path string, limit int) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.Context, "GET", r.Origin+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	response, err := r.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("registry_unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("registry_unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("limit_exceeded")
	}
	return data, nil
}

func validDependencies(value any) bool {
	if value == nil {
		return true
	}
	deps, ok := value.(map[string]any)
	if !ok || len(deps) > MaxPackages {
		return false
	}
	for name, raw := range deps {
		spec, ok := raw.(string)
		if !ValidName(name) || !ok || !rangePattern.MatchString(spec) || strings.ContainsAny(spec, ":/\\@") {
			return false
		}
	}
	return true
}

func validVersion(value map[string]any, name string) bool {
	version, ok := value["version"].(string)
	if value["name"] != name || !ok || !ValidVersion(version) {
		return false
	}
	for _, field := range []string{"dependencies", "optionalDependencies", "peerDependencies"} {
		if !validDependencies(value[field]) {
			return false
		}
	}
	for _, field := range []string{"bundleDependencies", "bundledDependencies"} {
		if v := value[field]; v != nil && v != false {
			if list, ok := v.([]any); !ok || len(list) > 0 {
				return false
			}
		}
	}
	dist, ok := value["dist"].(map[string]any)
	if !ok {
		return false
	}
	tarball, ok := dist["tarball"].(string)
	if !ok || !TarballURL(tarball) {
		return false
	}
	integrity, ok := dist["integrity"].(string)
	if !ok {
		return false
	}
	_, err := Integrity(integrity)
	return err == nil
}

func (r *Registry) Metadata(name string) ([]byte, error) {
	if !ValidName(name) {
		return nil, errors.New("unsupported_dependency")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if data, ok := r.cache[name]; ok {
		return data, nil
	}
	r.requests++
	if r.requests > MaxPackages {
		return nil, errors.New("limit_exceeded")
	}
	data, err := r.Get("/"+url.PathEscape(name), MaxMetadataBytes)
	if err != nil {
		return nil, err
	}
	r.metadata += len(data)
	if r.metadata > MaxMetadataTotal {
		return nil, errors.New("limit_exceeded")
	}
	var source map[string]any
	if json.Unmarshal(data, &source) != nil {
		return nil, errors.New("invalid_metadata")
	}
	versions, ok := source["versions"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid_metadata")
	}
	clean := map[string]any{}
	for version, raw := range versions {
		v, ok := raw.(map[string]any)
		if !ok || !validVersion(v, name) || v["version"] != version {
			continue
		}
		entry := map[string]any{}
		for _, field := range []string{"name", "version", "dependencies", "optionalDependencies", "peerDependencies", "peerDependenciesMeta", "engines", "os", "cpu", "libc", "bin", "scripts", "hasInstallScript", "dist"} {
			if value, ok := v[field]; ok {
				entry[field] = value
			}
		}
		// All tarball endpoints point into the isolated namespace. Lock generation
		// never needs payloads; the broker refuses any request for one.
		dist := entry["dist"].(map[string]any)
		entry["dist"] = map[string]any{"integrity": dist["integrity"], "tarball": "http://127.0.0.1:4873" + strings.TrimPrefix(dist["tarball"].(string), "https://registry.npmjs.org")}
		clean[version] = entry
	}
	tags := map[string]any{}
	if values, ok := source["dist-tags"].(map[string]any); ok {
		for tag, version := range values {
			if v, ok := version.(string); ok && clean[v] != nil && selectorPattern.MatchString(tag) {
				tags[tag] = v
			}
		}
	}
	data, err = json.Marshal(map[string]any{"name": name, "versions": clean, "dist-tags": tags})
	if err != nil {
		return nil, err
	}
	r.cache[name] = data
	return data, nil
}

func (r *Registry) Resolve(name, selector string) (string, error) {
	data, err := r.Metadata(name)
	if err != nil {
		return "", err
	}
	var document struct {
		Versions map[string]json.RawMessage `json:"versions"`
		Tags     map[string]string          `json:"dist-tags"`
	}
	if json.Unmarshal(data, &document) != nil {
		return "", errors.New("invalid_metadata")
	}
	version := selector
	if !ValidVersion(selector) {
		version = document.Tags[selector]
	}
	if !ValidVersion(version) || document.Versions[version] == nil {
		return "", errors.New("unsupported_dependency")
	}
	return version, nil
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" || req.URL.RawQuery != "" || req.URL.Fragment != "" {
		http.Error(w, "unsupported", 400)
		return
	}
	name, err := url.PathUnescape(strings.TrimPrefix(req.URL.EscapedPath(), "/"))
	if err != nil || !ValidName(name) || strings.Contains(name, "/-/") {
		http.Error(w, "unsupported", 400)
		return
	}
	data, err := r.Metadata(name)
	if err != nil {
		http.Error(w, fmt.Sprint(err), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
