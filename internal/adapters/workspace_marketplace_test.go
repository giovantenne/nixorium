package adapters

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func marketplaceVSIX(t *testing.T, manifest map[string]any, extra map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "extension.vsix")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	data, _ := json.Marshal(manifest)
	entries := map[string][]byte{"extension/package.json": data, "extension/README.md": []byte("readme")}
	for name, content := range extra {
		entries[name] = content
	}
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write(content)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	return path
}

func marketplaceFixture(t *testing.T, versions string, vsix string) (marketplaceClient, *[]string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || !strings.Contains(string(body), `"value":"platformio.platformio-ide"`) {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		io.WriteString(w, `{"results":[{"extensions":[{"publisher":{"publisherName":"platformio"},"extensionName":"platformio-ide","displayName":"PlatformIO IDE","shortDescription":"Embedded\u001b[31m development","versions":`+versions+`}]}]}`)
	}))
	t.Cleanup(server.Close)
	downloads := []string{}
	return marketplaceClient{
		queryURL: server.URL,
		http:     server.Client(),
		prefetch: func(_ context.Context, name, url string) (string, string, error) {
			downloads = append(downloads, name+" "+url)
			return "sha256-" + strings.Repeat("B", 43) + "=", vsix, nil
		},
		editor: func(context.Context) (string, error) { return "1.119.0", nil },
	}, &downloads
}

const marketplaceVersions = `[
 {"version":"3.4.0","targetPlatform":"linux-x64","properties":[{"key":"Microsoft.VisualStudio.Code.Engine","value":"^1.125.0"}]},
 {"version":"3.3.9","targetPlatform":"linux-x64","properties":[{"key":"Microsoft.VisualStudio.Code.Engine","value":"^1.65.0"},{"key":"Microsoft.VisualStudio.Code.PreRelease","value":"true"}]},
 {"version":"3.3.4","targetPlatform":"win32-x64","properties":[{"key":"Microsoft.VisualStudio.Code.Engine","value":"^1.65.0"}]},
 {"version":"3.3.4","targetPlatform":"linux-x64","properties":[{"key":"Microsoft.VisualStudio.Code.Engine","value":"^1.65.0"}]}
]`

func TestMarketplaceResolvesNewestCompatibleStableLinuxVersion(t *testing.T) {
	vsix := marketplaceVSIX(t, map[string]any{
		"publisher": "platformio", "name": "platformio-ide", "version": "3.3.4",
		"engines": map[string]any{"vscode": "^1.65.0"}, "extensionDependencies": []string{"ms-vscode.cpptools", "MS-vscode.CppTools"},
	}, map[string][]byte{"extension/bin/tool": []byte("\x7fELF\x02\x01")})
	client, downloads := marketplaceFixture(t, marketplaceVersions, vsix)
	candidate, err := client.resolve(t.Context(), "PlatformIO.PlatformIO-IDE")
	if err != nil {
		t.Fatal(err)
	}
	if *candidate.Entry.Version != "3.3.4" || *candidate.Entry.Platform != "linux-x64" || candidate.Entry.Issue() != "" {
		t.Fatalf("wrong version: %+v", candidate.Entry)
	}
	if len(*downloads) != 1 || *downloads != nil && (*downloads)[0] != "platformio-platformio-ide.vsix https://platformio.gallery.vsassets.io/_apis/public/gallery/publisher/platformio/extension/platformio-ide/3.3.4/assetbyname/Microsoft.VisualStudio.Services.VSIXPackage?targetPlatform=linux-x64" {
		t.Fatalf("unexpected download %v", *downloads)
	}
	if !candidate.Native || strings.Join(candidate.Dependencies, ",") != "ms-vscode.cpptools" || candidate.EditorVersion != "1.119.0" {
		t.Fatalf("manifest inspection: %+v", candidate)
	}
	if strings.ContainsRune(candidate.Description, '\x1b') {
		t.Fatal("terminal control text was not removed")
	}
}

func TestMarketplaceRefusals(t *testing.T) {
	good := map[string]any{"publisher": "platformio", "name": "platformio-ide", "version": "3.3.4", "engines": map[string]any{"vscode": "^1.65.0"}}
	for name, c := range map[string]struct {
		versions string
		manifest map[string]any
		want     string
	}{
		"only newer engines":   {`[{"version":"3.4.0","properties":[{"key":"Microsoft.VisualStudio.Code.Engine","value":"^1.125.0"}]}]`, good, "newest requires ^1.125.0"},
		"only other platforms": {`[{"version":"3.3.4","targetPlatform":"darwin-arm64","properties":[]}]`, good, "no stable version for Linux"},
		"prerelease version":   {`[{"version":"3.3.4-beta","properties":[]}]`, good, "cannot pin"},
		"manifest mismatch":    {`[{"version":"3.3.4","properties":[]}]`, map[string]any{"publisher": "other", "name": "platformio-ide", "version": "3.3.4"}, "does not match"},
		"version mismatch":     {`[{"version":"3.3.4","properties":[]}]`, map[string]any{"publisher": "platformio", "name": "platformio-ide", "version": "3.3.5"}, "does not match"},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := marketplaceFixture(t, c.versions, marketplaceVSIX(t, c.manifest, nil))
			if _, err := client.resolve(t.Context(), "platformio.platformio-ide"); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
	client, downloads := marketplaceFixture(t, marketplaceVersions, "")
	if _, err := client.resolve(t.Context(), "platformio"); err == nil || len(*downloads) != 0 {
		t.Fatal("accepted an invalid identifier")
	}
	if _, err := client.resolve(t.Context(), "other.extension"); err == nil {
		t.Fatal("accepted a query answer for another extension")
	}
}

func TestMarketplaceRejectsRedirectsAndLargeAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://example.invalid/", http.StatusFound)
			return
		}
		w.Write([]byte(strings.Repeat(" ", marketplaceQueryLimit+1)))
	}))
	defer server.Close()
	client := marketplaceClient{http: server.Client()}
	client.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.queryURL = server.URL + "/redirect"
	if _, err := client.query(t.Context(), "a.b"); err == nil {
		t.Fatal("followed a redirect")
	}
	client.queryURL = server.URL
	if _, err := client.query(t.Context(), "a.b"); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("unbounded answer: %v", err)
	}
}
