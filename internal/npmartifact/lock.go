package npmartifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
)

type Dependency struct {
	Path      string `json:"path"`
	Version   string `json:"version"`
	URL       string `json:"url"`
	Integrity string `json:"integrity"`
	Optional  bool   `json:"optional,omitempty"`
}
type Manifest struct {
	SchemaVersion   int               `json:"schemaVersion"`
	Package         string            `json:"package"`
	Version         string            `json:"version"`
	NodeRuntime     string            `json:"nodeRuntime"`
	ArtifactID      string            `json:"artifactID"`
	Bins            map[string]string `json:"bins"`
	Dependencies    []Dependency      `json:"dependencies"`
	OmittedOptional []string          `json:"omittedOptional"`
}
type lockPackage struct {
	Version          string            `json:"version"`
	Resolved         string            `json:"resolved"`
	Integrity        string            `json:"integrity"`
	Optional         bool              `json:"optional"`
	Dev              bool              `json:"dev"`
	Link             bool              `json:"link"`
	InBundle         bool              `json:"inBundle"`
	HasInstallScript bool              `json:"hasInstallScript"`
	Dependencies     map[string]string `json:"dependencies"`
}

func packagePath(p string) bool {
	parts := strings.Split(p, "/node_modules/")
	for i, part := range parts {
		if i == 0 {
			part = strings.TrimPrefix(part, "node_modules/")
			if part == parts[i] {
				return false
			}
		}
		if !ValidName(part) {
			return false
		}
	}
	return len(parts) <= 12
}

func ValidateLock(data []byte, name, version string) ([]byte, Manifest, error) {
	m := Manifest{SchemaVersion: 1, Package: name, Version: version, Bins: map[string]string{}, Dependencies: []Dependency{}, OmittedOptional: []string{}}
	if len(data) > MaxLockBytes {
		return nil, m, errors.New("limit_exceeded")
	}
	var document struct {
		LockfileVersion int                    `json:"lockfileVersion"`
		Packages        map[string]lockPackage `json:"packages"`
	}
	if json.Unmarshal(data, &document) != nil || document.LockfileVersion != 3 || len(document.Packages) < 2 || len(document.Packages) > MaxPackages+1 {
		return nil, m, errors.New("invalid_lock")
	}
	root, ok := document.Packages[""]
	if !ok || len(root.Dependencies) != 1 || root.Dependencies[name] != version {
		return nil, m, errors.New("invalid_lock")
	}
	paths := make([]string, 0, len(document.Packages))
	for p := range document.Packages {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		v := document.Packages[p]
		if !packagePath(p) || !ValidVersion(v.Version) || v.Dev || v.Link || v.InBundle {
			return nil, m, errors.New("unsupported_dependency")
		}
		resolved := strings.Replace(v.Resolved, "http://127.0.0.1:4873/", "https://registry.npmjs.org/", 1)
		if !TarballURL(resolved) {
			return nil, m, errors.New("unsupported_dependency")
		}
		if _, err := Integrity(v.Integrity); err != nil {
			return nil, m, err
		}
		m.Dependencies = append(m.Dependencies, Dependency{p, v.Version, resolved, v.Integrity, v.Optional})
		if v.Optional {
			m.OmittedOptional = append(m.OmittedOptional, p)
		}
	}
	selected, ok := document.Packages["node_modules/"+name]
	if !ok || selected.Optional || selected.Version != version {
		return nil, m, errors.New("invalid_lock")
	}
	// Preserve npm's complete lock metadata, changing only the confined proxy URL.
	data = bytes.ReplaceAll(data, []byte("http://127.0.0.1:4873/"), []byte("https://registry.npmjs.org/"))
	return data, m, nil
}

type ArchiveBudget struct {
	Bytes     int64
	Entries   int
	Downloads int64
}

func (budget *ArchiveBudget) Inspect(data []byte, dep Dependency, selected bool) (map[string]string, error) {
	expected, err := Integrity(dep.Integrity)
	if err != nil {
		return nil, err
	}
	actual := sha512.Sum512(data)
	if !bytes.Equal(actual[:], expected) {
		return nil, errors.New("integrity_mismatch")
	}
	budget.Downloads += int64(len(data))
	if len(data) > MaxDownloadBytes || budget.Downloads > MaxDownloadTotal {
		return nil, errors.New("limit_exceeded")
	}
	zip, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("invalid_archive")
	}
	defer zip.Close()
	remaining := int64(MaxExpandedBytes) - budget.Bytes
	limited := &io.LimitedReader{R: zip, N: remaining + 1}
	reader := tar.NewReader(limited)
	var declared int64
	files := map[string]bool{}
	hashes := map[string]string{}
	modes := map[string]int64{}
	var manifest []byte
	expectedName := strings.TrimPrefix(dep.Path, "node_modules/")
	if index := strings.LastIndex(dep.Path, "/node_modules/"); index >= 0 {
		expectedName = dep.Path[index+len("/node_modules/"):]
	}
	archiveRoot := ""
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid_archive")
		}
		p := strings.TrimSuffix(header.Name, "/")
		root, relative, hasRelative := strings.Cut(p, "/")
		if archiveRoot == "" {
			if root != "package" && root != path.Base(expectedName) {
				return nil, errors.New("unsafe_archive")
			}
			archiveRoot = root
		}
		if root != archiveRoot {
			return nil, errors.New("unsafe_archive")
		}
		p = "package"
		if hasRelative {
			p += "/" + relative
		}
		if p != "package" && !strings.HasPrefix(p, "package/") || strings.ContainsAny(p, "\\\x00") || header.Size < 0 {
			return nil, errors.New("unsafe_archive")
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".." || part == "" {
				return nil, errors.New("unsafe_archive")
			}
		}
		p = path.Clean(p)
		// Legacy tar regular files may use a zero type byte; avoid the deprecated
		// TypeRegA alias while keeping those archives supported.
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 && header.Typeflag != tar.TypeDir {
			return nil, errors.New("unsafe_archive")
		}
		if header.Mode&06000 != 0 {
			return nil, errors.New("unsafe_archive")
		}
		budget.Entries++
		declared += header.Size
		if budget.Entries > MaxEntries || declared > remaining {
			return nil, errors.New("limit_exceeded")
		}
		if header.Typeflag != tar.TypeDir {
			if strings.Contains(p, "/node_modules/") {
				return nil, errors.New("unsupported_bundled_dependency")
			}
			if strings.HasSuffix(p, ".node") || p == "package/binding.gyp" {
				return nil, errors.New("unsupported_native_dependency")
			}
			if p == "package/package.json" {
				if header.Size > MaxLockBytes {
					return nil, errors.New("limit_exceeded")
				}
				manifest, err = io.ReadAll(io.LimitReader(reader, MaxLockBytes+1))
				if err != nil {
					return nil, errors.New("invalid_archive")
				}
				hash := sha256.Sum256(manifest)
				if files[p] && (hashes[p] != string(hash[:]) || modes[p] != header.Mode) {
					return nil, errors.New("unsafe_archive")
				}
				hashes[p] = string(hash[:])
			} else {
				hash := sha256.New()
				if _, err = io.Copy(hash, reader); err != nil {
					return nil, errors.New("invalid_archive")
				}
				if files[p] && (hashes[p] != string(hash.Sum(nil)) || modes[p] != header.Mode) {
					return nil, errors.New("unsafe_archive")
				}
				hashes[p] = string(hash.Sum(nil))
			}
			files[p] = true
			modes[p] = header.Mode
		}
	}
	// Account for gzip padding/trailing data as well as file bodies, verify the
	// gzip checksum, and refuse bombs concealed after the tar end marker.
	if _, err = io.Copy(io.Discard, limited); err != nil {
		return nil, errors.New("invalid_archive")
	}
	consumed := remaining + 1 - limited.N
	if consumed > remaining {
		return nil, errors.New("limit_exceeded")
	}
	budget.Bytes += consumed
	var pkg struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Bin     json.RawMessage   `json:"bin"`
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(manifest, &pkg) != nil || pkg.Name != expectedName || pkg.Version != dep.Version {
		return nil, errors.New("invalid_archive")
	}
	bins := map[string]string{}
	if len(pkg.Bin) > 0 && string(pkg.Bin) != "null" {
		var single string
		if json.Unmarshal(pkg.Bin, &single) == nil {
			bins[path.Base(pkg.Name)] = single
		} else if json.Unmarshal(pkg.Bin, &bins) != nil {
			return nil, errors.New("invalid_bin")
		}
	}
	if selected && len(bins) == 0 || len(bins) > 16 {
		return nil, errors.New("unsupported_no_cli")
	}
	for name, p := range bins {
		p = strings.TrimPrefix(p, "./")
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}$`).MatchString(name) || !regexp.MustCompile(`^[A-Za-z0-9_./@-]+$`).MatchString(p) || path.Clean(p) != p || strings.HasPrefix(p, "/") || !files["package/"+p] {
			return nil, errors.New("invalid_bin")
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".." || part == "." {
				return nil, errors.New("invalid_bin")
			}
		}
		bins[name] = p
	}
	if !selected {
		return nil, nil
	}
	return bins, nil
}
