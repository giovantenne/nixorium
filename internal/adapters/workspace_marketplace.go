package adapters

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The gallery query is the documented public endpoint used by the editor.
// Requests are fixed, bounded and never follow a redirect to another host.
const marketplaceQueryURL = "https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery"

const (
	marketplaceQueryLimit    = 16 * 1024 * 1024
	marketplaceManifestLimit = 4 * 1024 * 1024
	marketplaceEntryLimit    = 100000
)

// marketplaceClient is replaceable in tests; production uses the public
// gallery and the Nix store for the download.
type marketplaceClient struct {
	queryURL string
	http     *http.Client
	prefetch func(ctx context.Context, name, url string) (hash, storePath string, err error)
	editor   func(ctx context.Context) (string, error)
}

type marketplaceVersion struct {
	Version        string `json:"version"`
	TargetPlatform string `json:"targetPlatform"`
	Properties     []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"properties"`
}

type marketplaceExtension struct {
	Publisher struct {
		PublisherName string `json:"publisherName"`
	} `json:"publisher"`
	ExtensionName    string               `json:"extensionName"`
	DisplayName      string               `json:"displayName"`
	ShortDescription string               `json:"shortDescription"`
	Versions         []marketplaceVersion `json:"versions"`
}

func (version marketplaceVersion) property(key string) string {
	for _, property := range version.Properties {
		if property.Key == key {
			return property.Value
		}
	}
	return ""
}

// ResolveMarketplaceExtension picks the newest stable Marketplace version for
// this Linux editor, downloads its exact bytes into the Nix store and checks
// the manifest. It does not change the deployment or select the extension.
func (Local) ResolveMarketplaceExtension(ctx context.Context, repository, id string) (domain.WorkspaceMarketplaceCandidate, error) {
	client := marketplaceClient{
		queryURL: marketplaceQueryURL,
		http: &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("the Marketplace answered with an unexpected redirect")
			},
		},
		prefetch: prefetchMarketplaceFile,
		editor: func(ctx context.Context) (string, error) {
			item, err := (Local{}).ResolveSoftwarePackage(ctx, repository, "vscode")
			if err != nil {
				return "", fmt.Errorf("find the pinned VS Code version: %w", err)
			}
			return item.Version, nil
		},
	}
	return client.resolve(ctx, id)
}

func (client marketplaceClient) resolve(ctx context.Context, id string) (domain.WorkspaceMarketplaceCandidate, error) {
	var candidate domain.WorkspaceMarketplaceCandidate
	id, err := domain.NormalizeWorkspaceMarketplaceID(id)
	if err != nil {
		return candidate, err
	}
	editor, err := client.editor(ctx)
	if err != nil {
		return candidate, err
	}
	extension, err := client.query(ctx, id)
	if err != nil {
		return candidate, err
	}
	version, platform, err := chooseMarketplaceVersion(extension.Versions, editor)
	if err != nil {
		return candidate, err
	}
	publisher, name := extension.Publisher.PublisherName, extension.ExtensionName
	// Check the gallery's names before they become part of a download URL.
	placeholder := "sha256-" + strings.Repeat("A", 43) + "="
	entry := domain.WorkspaceMarketplaceExtension{Publisher: &publisher, Name: &name, Version: &version.Version, Hash: &placeholder}
	if platform != "" {
		entry.Platform = &platform
	}
	if entry.Issue() != "" {
		return candidate, errors.New("the Marketplace returned an identifier or version Nixorium cannot pin")
	}
	// Match the nixpkgs fetcher exactly: the same name and URL give the same
	// store path, so the later system build finds the download offline.
	url := fmt.Sprintf("https://%s.gallery.vsassets.io/_apis/public/gallery/publisher/%s/extension/%s/%s/assetbyname/Microsoft.VisualStudio.Services.VSIXPackage",
		publisher, publisher, name, version.Version)
	if platform != "" {
		url += "?targetPlatform=" + platform
	}
	hash, storePath, err := client.prefetch(ctx, publisher+"-"+name+".vsix", url)
	if err != nil {
		return candidate, err
	}
	entry.Hash = &hash
	if issue := entry.Issue(); issue != "" {
		return candidate, errors.New("the downloaded extension could not be pinned: " + issue)
	}
	manifest, native, err := inspectMarketplaceVSIX(storePath)
	if err != nil {
		return candidate, err
	}
	if strings.ToLower(manifest.Publisher+"."+manifest.Name) != id || manifest.Version != version.Version {
		return candidate, errors.New("the downloaded package does not match the requested extension and version")
	}
	lower := func(values []string) []string {
		result := []string{}
		for _, value := range values {
			result = append(result, strings.ToLower(value))
		}
		slices.Sort(result)
		return slices.Compact(result)
	}
	return domain.WorkspaceMarketplaceCandidate{
		Entry:         entry,
		DisplayName:   safeMarketplaceText(extension.DisplayName),
		Description:   safeMarketplaceText(extension.ShortDescription),
		Engine:        manifest.Engines.VSCode,
		EditorVersion: editor,
		Dependencies:  lower(manifest.ExtensionDependencies),
		Pack:          lower(manifest.ExtensionPack),
		Native:        native,
		StorePath:     storePath,
	}, nil
}

func (client marketplaceClient) query(ctx context.Context, id string) (marketplaceExtension, error) {
	var none marketplaceExtension
	// Flags: include versions (0x1) and their properties (0x10).
	body, err := json.Marshal(map[string]any{
		"filters": []any{map[string]any{
			"criteria":   []any{map[string]any{"filterType": 7, "value": id}},
			"pageNumber": 1,
			"pageSize":   1,
		}},
		"flags": 0x11,
	})
	if err != nil {
		return none, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.queryURL, bytes.NewReader(body))
	if err != nil {
		return none, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json;api-version=3.0-preview.1")
	response, err := client.http.Do(request)
	if err != nil {
		return none, fmt.Errorf("the Marketplace could not be reached from the controller: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return none, fmt.Errorf("the Marketplace answered with HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, marketplaceQueryLimit+1))
	if err != nil {
		return none, fmt.Errorf("read the Marketplace answer: %w", err)
	}
	if len(data) > marketplaceQueryLimit {
		return none, errors.New("the Marketplace answer exceeds its size limit")
	}
	var result struct {
		Results []struct {
			Extensions []marketplaceExtension `json:"extensions"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return none, errors.New("the Marketplace answer is not valid JSON")
	}
	for _, group := range result.Results {
		for _, extension := range group.Extensions {
			if strings.ToLower(extension.Publisher.PublisherName+"."+extension.ExtensionName) == id {
				return extension, nil
			}
		}
	}
	return none, fmt.Errorf("the Marketplace has no extension %s", id)
}

// chooseMarketplaceVersion keeps the gallery's newest-first order and picks
// the first stable Linux-compatible version the pinned editor accepts.
func chooseMarketplaceVersion(versions []marketplaceVersion, editor string) (marketplaceVersion, string, error) {
	var newestRequirement string
	for _, version := range versions {
		platform := version.TargetPlatform
		if platform != "" && platform != "linux-x64" {
			continue
		}
		if version.property("Microsoft.VisualStudio.Code.PreRelease") == "true" {
			continue
		}
		requirement := version.property("Microsoft.VisualStudio.Code.Engine")
		if requirement == "" {
			requirement = "*"
		}
		accepted, err := domain.VSCodeEngineAccepts(requirement, editor)
		if err != nil {
			continue
		}
		if !accepted {
			if newestRequirement == "" {
				newestRequirement = requirement
			}
			continue
		}
		return version, platform, nil
	}
	if newestRequirement != "" {
		return marketplaceVersion{}, "", fmt.Errorf("no stable version works with the pinned VS Code %s; the newest requires %s. Update the system and packages first", editor, newestRequirement)
	}
	return marketplaceVersion{}, "", errors.New("the Marketplace offers no stable version for Linux (x64)")
}

type marketplaceManifest struct {
	Publisher string `json:"publisher"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Engines   struct {
		VSCode string `json:"vscode"`
	} `json:"engines"`
	ExtensionDependencies []string `json:"extensionDependencies"`
	ExtensionPack         []string `json:"extensionPack"`
}

// inspectMarketplaceVSIX reads the manifest and looks for native programs
// (ELF files or Node native modules) without unpacking the archive.
func inspectMarketplaceVSIX(path string) (marketplaceManifest, bool, error) {
	var manifest marketplaceManifest
	archive, err := zip.OpenReader(path)
	if err != nil {
		return manifest, false, errors.New("the downloaded extension is not a valid package")
	}
	defer archive.Close()
	if len(archive.File) > marketplaceEntryLimit {
		return manifest, false, errors.New("the downloaded extension has too many files")
	}
	found, native := false, false
	for _, file := range archive.File {
		if file.Name == "extension/package.json" {
			if file.UncompressedSize64 > marketplaceManifestLimit {
				return manifest, false, errors.New("the extension manifest exceeds its size limit")
			}
			reader, err := file.Open()
			if err != nil {
				return manifest, false, err
			}
			data, err := io.ReadAll(io.LimitReader(reader, marketplaceManifestLimit+1))
			reader.Close()
			if err != nil || json.Unmarshal(data, &manifest) != nil {
				return manifest, false, errors.New("the extension manifest is not valid JSON")
			}
			found = true
			continue
		}
		if native || file.FileInfo().IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name, ".node") || strings.HasSuffix(file.Name, ".so") || strings.Contains(filepath.Base(file.Name), ".so.") {
			native = true
			continue
		}
		reader, err := file.Open()
		if err != nil {
			continue
		}
		magic := make([]byte, 4)
		count, _ := io.ReadFull(reader, magic)
		reader.Close()
		native = count == 4 && bytes.Equal(magic, []byte("\x7fELF"))
	}
	if !found {
		return manifest, false, errors.New("the downloaded package has no extension manifest")
	}
	return manifest, native, nil
}

var marketplaceHashPattern = regexp.MustCompile(`^sha256-[A-Za-z0-9+/]{43}=$`)

func prefetchMarketplaceFile(ctx context.Context, name, url string) (string, string, error) {
	command := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command flakes",
		"store", "prefetch-file", "--json", "--hash-type", "sha256", "--name", name, url)
	configureCommandCancellation(command)
	command.Env = workspaceEnvironment()
	output := &boundedCommandBuffer{limit: 64 * 1024}
	command.Stdout, command.Stderr = output, &boundedCommandBuffer{limit: 64 * 1024}
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("download the extension into the Nix store: %w", err)
	}
	var result struct {
		Hash      string `json:"hash"`
		StorePath string `json:"storePath"`
	}
	if output.truncated || json.Unmarshal(output.buffer.Bytes(), &result) != nil ||
		!marketplaceHashPattern.MatchString(result.Hash) || !domain.ValidStorePath(result.StorePath) {
		return "", "", errors.New("the Nix download returned incomplete metadata")
	}
	return result.Hash, result.StorePath, nil
}

func safeMarketplaceText(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, value)
	if len(value) > 200 {
		value = strings.ToValidUTF8(value[:200], "") + "…"
	}
	return strings.TrimSpace(value)
}
