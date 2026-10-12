package npmartifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sri(data []byte) string {
	hash := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(hash[:])
}
func archive(t *testing.T, pkg string, files map[string]string, trailing int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zip := gzip.NewWriter(&buffer)
	tarball := tar.NewWriter(zip)
	files["package.json"] = pkg
	for p, text := range files {
		if err := tarball.WriteHeader(&tar.Header{Name: "package/" + p, Mode: 0755, Size: int64(len(text)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tarball.Write([]byte(text))
	}
	tarball.Close()
	zip.Write(make([]byte, trailing))
	zip.Close()
	return buffer.Bytes()
}

func TestArchiveIdentityBinsHooksAndCompressedPadding(t *testing.T) {
	good := `{"name":"example","version":"1.0.0","bin":{"example":"bin/cli.js"}}`
	for _, test := range []struct {
		name, pkg, path string
		trailing        int
		remaining       int64
		ok              bool
	}{
		{"valid", good, "bin/cli.js", 0, MaxExpandedBytes, true},
		{"identity", strings.Replace(good, `"example"`, `"other"`, 1), "bin/cli.js", 0, MaxExpandedBytes, false},
		{"injection", strings.Replace(good, "bin/cli.js", "bin/${builtins.abort}.js", 1), "bin/${builtins.abort}.js", 0, MaxExpandedBytes, false},
		{"hook-isolated-at-build", strings.TrimSuffix(good, "}") + `,"scripts":{"install":"steal"}}`, "bin/cli.js", 0, MaxExpandedBytes, true},
		{"native", good, "binding.gyp", 0, MaxExpandedBytes, false},
		{"padding", good, "bin/cli.js", 8192, 4096, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := archive(t, test.pkg, map[string]string{test.path: "#!/usr/bin/env node\nconsole.log('fixture')\n"}, test.trailing)
			dep := Dependency{Path: "node_modules/example", Version: "1.0.0", Integrity: sri(data)}
			budget := ArchiveBudget{Bytes: MaxExpandedBytes - test.remaining}
			bins, err := budget.Inspect(data, dep, true)
			if (err == nil) != test.ok {
				t.Fatal(bins, err)
			}
			if test.ok {
				dep.Integrity = sri([]byte("wrong"))
				if _, err := (&ArchiveBudget{}).Inspect(data, dep, true); err == nil {
					t.Fatal("integrity mismatch accepted")
				}
			}
		})
	}
	data := archive(t, `{"name":"helper","version":"1.0.0","bin":{"helper":"../escape"}}`, map[string]string{"../escape": "text"}, 0)
	if _, err := (&ArchiveBudget{}).Inspect(data, Dependency{Path: "node_modules/helper", Version: "1.0.0", Integrity: sri(data)}, false); err == nil {
		t.Fatal("unsafe transitive bin accepted")
	}
}

func TestRegistryRejectsRemoteSpecsAndURLsBeforeResolver(t *testing.T) {
	valid := func(version string) map[string]any {
		return map[string]any{"name": "example", "version": version, "dependencies": map[string]string{"helper": "^1.0.0"}, "dist": map[string]string{"tarball": "https://registry.npmjs.org/example/-/example-" + version + ".tgz", "integrity": sri(nil)}}
	}
	versions := map[string]any{"1.0.0": valid("1.0.0")}
	for i, spec := range []string{"git+https://evil.invalid/repo", "file:/home/admin/.npmrc", "https://evil.invalid/tgz", "npm:alias@1.0.0", "../relative"} {
		version := strings.Repeat("1", i+2) + ".0.0"
		v := valid(version)
		v["dependencies"] = map[string]string{"helper": spec}
		versions[version] = v
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"versions": versions, "dist-tags": map[string]string{"latest": "1.0.0"}})
	}))
	defer server.Close()
	registry := NewRegistry(t.Context())
	registry.Origin = server.URL
	data, err := registry.Metadata("example")
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	json.Unmarshal(data, &metadata)
	if len(metadata.Versions) != 1 || bytes.Contains(data, []byte("evil")) || bytes.Contains(data, []byte("/home/admin")) {
		t.Fatal(string(data))
	}
	if !bytes.Contains(data, []byte("http://127.0.0.1:4873/")) {
		t.Fatal("payload did not stay in namespace")
	}
	for _, p := range []string{"/../secret", "/example/-/example.tgz", "/example?token=secret", "/example%252f.."} {
		request := httptest.NewRequest("GET", p, nil)
		response := httptest.NewRecorder()
		registry.ServeHTTP(response, request)
		if response.Code == 200 {
			t.Fatal(p)
		}
	}
	for _, u := range []string{"http://registry.npmjs.org/example/-/example.tgz", "https://registry.npmjs.org:443/example/-/example.tgz", "https://user@registry.npmjs.org/example/-/example.tgz", "https://registry.npmjs.org/example/-/../evil.tgz", "https://registry.npmjs.org/example/-/x.tgz?auth=x", "https://registry.npmjs.org/example/-/%2e%2e.tgz"} {
		if TarballURL(u) {
			t.Fatal(u)
		}
	}
}

func TestLockExactIntegrityHooksAndSources(t *testing.T) {
	lock := map[string]any{"lockfileVersion": 3, "packages": map[string]any{"": map[string]any{"dependencies": map[string]string{"example": "1.0.0"}}, "node_modules/example": map[string]any{"version": "1.0.0", "resolved": "http://127.0.0.1:4873/example/-/example-1.0.0.tgz", "integrity": sri(nil)}}}
	data, _ := json.Marshal(lock)
	normalized, m, err := ValidateLock(data, "example", "1.0.0")
	if err != nil || !bytes.Contains(normalized, []byte("https://registry.npmjs.org/")) || len(m.Dependencies) != 1 {
		t.Fatal(m, err)
	}
	entry := lock["packages"].(map[string]any)["node_modules/example"].(map[string]any)
	for _, field := range []string{"link", "inBundle"} {
		entry[field] = true
		data, _ = json.Marshal(lock)
		if _, _, err := ValidateLock(data, "example", "1.0.0"); err == nil {
			t.Fatal(field)
		}
		delete(entry, field)
	}
	entry["resolved"] = "https://evil.invalid/example.tgz"
	data, _ = json.Marshal(lock)
	if _, _, err := ValidateLock(data, "example", "1.0.0"); err == nil {
		t.Fatal("external dependency accepted")
	}
}
