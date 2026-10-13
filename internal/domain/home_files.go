package domain

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	HomeFilesFileName      = "home-files.json"
	HomeFilesDirectoryName = "home-files"
	HomeFilesMaxBytes      = 1024 * 1024
	HomeFilesMaxEntries    = 512
	HomeFilesMaxFileBytes  = 16 * 1024 * 1024
	HomeFilesMaxTotalBytes = 64 * 1024 * 1024
	HomeFilesMaxTextBytes  = 64 * 1024
)

// HomeFilesProfile adds non-secret initial files to the common student seed.
// Source bytes are captured copies, never references to the controller home.
type HomeFilesProfile struct {
	SchemaVersion int        `json:"schemaVersion"`
	Files         []HomeFile `json:"files,omitempty"`
	Directories   []string   `json:"directories,omitempty"`
}

type HomeFile struct {
	Destination string  `json:"destination"`
	Text        *string `json:"text,omitempty"`
	Source      *string `json:"source,omitempty"`
	Executable  bool    `json:"executable,omitempty"`
}

// HomeFilesSeedEntry is integrity and mode metadata for copied regular files.
// Bytes are already in the immutable seed; metadata carries no source path.
type HomeFilesSeedEntry struct {
	Destination string `json:"destination"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Executable  bool   `json:"executable"`
}

//go:embed home_files_policy.json
var homeFilesPolicyJSON []byte
var homeFilesPolicy = func() (p struct {
	ProtectedRoots      []string `json:"protectedRoots"`
	SensitiveComponents []string `json:"sensitiveComponents"`
	SensitivePrefixes   []string `json:"sensitivePrefixes"`
	ScaffoldDirectories []string `json:"scaffoldDirectories"`
}) {
	if err := json.Unmarshal(homeFilesPolicyJSON, &p); err != nil {
		panic(err)
	}
	return
}()
var homeFilesSource = regexp.MustCompile(`^imports/[0-9a-f]{64}$`)
var homeFilesSecret = regexp.MustCompile(`(?i)(-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----|AGE-SECRET-KEY-|AKIA[0-9A-Z]{16}|(?:sk-|ghp_|github_pat_)[A-Za-z0-9_-]{16,}|\$6\$|Bearer[ \t]+[A-Za-z0-9._~+/=-]{16,})`)

func HomeFilesSensitiveContent(data []byte) bool { return homeFilesSecret.Match(data) }
func HomeFilesSensitivePath(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if slices.Contains(homeFilesPolicy.SensitiveComponents, part) {
			return true
		}
		for _, prefix := range homeFilesPolicy.SensitivePrefixes {
			if strings.HasPrefix(part, prefix) {
				return true
			}
		}
	}
	return false
}

// ValidHomeFilesDestination refuses credentials, launch hooks and managed seed
// files. A student owns the resulting files and can change their session modes.
func ValidHomeFilesDestination(path string, directory bool) bool {
	if path == "" || len(path) > 512 || !utf8.ValidString(path) || strings.Contains(path, "\\") {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) > 12 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, char := range part {
			if unicode.IsControl(char) {
				return false
			}
		}
	}
	if HomeFilesSensitivePath(path) {
		return false
	}
	for _, protected := range homeFilesPolicy.ProtectedRoots {
		if path == protected || strings.HasPrefix(path, protected+"/") || (!directory && strings.HasPrefix(protected, path+"/")) {
			return false
		}
	}
	return directory || !slices.Contains(homeFilesPolicy.ScaffoldDirectories, path)
}

func checkHomeFilesJSON(d *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("nesting limit exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return errors.New("malformed JSON")
	}
	if token == nil {
		return errors.New("null is not supported")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		allowed := []string{"destination", "text", "source", "executable"}
		if depth == 0 {
			allowed = []string{"schemaVersion", "files", "directories"}
		}
		if depth != 0 && depth != 2 {
			return errors.New("unexpected object")
		}
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return errors.New("malformed object")
			}
			if seen[key] {
				return errors.New("duplicate field")
			}
			seen[key] = true
			if !slices.Contains(allowed, key) {
				return errors.New("unsupported field")
			}
			if err := checkHomeFilesJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := checkHomeFilesJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected delimiter")
	}
	_, err = d.Token()
	return err
}

func DecodeHomeFilesProfile(data []byte) (HomeFilesProfile, []ValidationIssue) {
	invalid := func(message string) (HomeFilesProfile, []ValidationIssue) {
		return HomeFilesProfile{}, []ValidationIssue{{Field: "$", Message: message}}
	}
	if len(data) > HomeFilesMaxBytes || !utf8.Valid(data) {
		return invalid("expected UTF-8 JSON of at most 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := checkHomeFilesJSON(d, 0); err != nil {
		return invalid(err.Error())
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return invalid("unexpected trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var p HomeFilesProfile
	if err := d.Decode(&p); err != nil {
		return invalid("unsupported field or invalid field type")
	}
	if p.SchemaVersion != 1 {
		return invalid("unsupported schemaVersion")
	}
	if len(p.Files)+len(p.Directories) > HomeFilesMaxEntries {
		return invalid("too many home entries")
	}
	seen := map[string]bool{}
	for _, dir := range p.Directories {
		if !ValidHomeFilesDestination(dir, true) || seen[dir] {
			return invalid("invalid, protected or duplicate directory")
		}
		seen[dir] = true
	}
	for _, file := range p.Files {
		if !ValidHomeFilesDestination(file.Destination, false) || seen[file.Destination] {
			return invalid("invalid, protected or duplicate file destination")
		}
		seen[file.Destination] = true
		if (file.Text == nil) == (file.Source == nil) {
			return invalid("a file requires exactly one of text or source")
		}
		if file.Source != nil && !homeFilesSource.MatchString(*file.Source) {
			return invalid("invalid imported source")
		}
		if file.Text != nil && (len(*file.Text) > HomeFilesMaxTextBytes || strings.ContainsRune(*file.Text, 0) || HomeFilesSensitiveContent([]byte(*file.Text))) {
			return invalid("text is too large or contains credential material")
		}
	}
	for _, file := range p.Files {
		for path := range seen {
			if strings.HasPrefix(path, file.Destination+"/") {
				return invalid("a file conflicts with another destination")
			}
		}
	}
	slices.Sort(p.Directories)
	slices.SortFunc(p.Files, func(a, b HomeFile) int { return strings.Compare(a.Destination, b.Destination) })
	return p, nil
}

func MarshalHomeFilesProfile(p HomeFilesProfile) ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	normalized, issues := DecodeHomeFilesProfile(data)
	if len(issues) != 0 {
		return nil, errors.New(issues[0].Message)
	}
	data, err = json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data)+1 > HomeFilesMaxBytes {
		return nil, errors.New("normalized home profile exceeds 1 MiB")
	}
	return append(data, '\n'), nil
}
