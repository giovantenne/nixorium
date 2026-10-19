// Package systemfiles reads system configuration files of a laboratory
// computer for the Assistant. Only an allowlist of system locations is
// readable; secrets, keys, Wi-Fi credentials and every home folder are
// refused, whatever path or link leads to them. The reader never writes and
// never follows a link out of the readable locations, except into the Nix
// store where NixOS keeps the files /etc links to.
package systemfiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

const (
	// MaxFileBytes bounds the content returned for one file.
	MaxFileBytes = 256 * 1024
	// MaxEntries bounds a directory listing.
	MaxEntries = 500
	// MaxOutputBytes bounds the whole JSON answer.
	MaxOutputBytes = 512 * 1024
)

// readableRoots are the locations the Assistant may read, with everything
// below them unless refused.
var readableRoots = []string{
	"/etc",
	"/boot/grub",
	"/boot/loader",
	"/proc/cmdline",
	"/proc/cpuinfo",
	"/proc/meminfo",
	"/proc/mounts",
	"/proc/partitions",
	"/proc/swaps",
	"/proc/version",
	"/sys/class/net",
	"/sys/class/dmi/id",
	"/var/lib/nixorium",
	"/run/nixorium",
}

// refusedPrefixes are never readable, even below a readable root.
var refusedPrefixes = []string{
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/NetworkManager/system-connections",
	"/etc/wpa_supplicant",
	"/etc/ppp",
	"/etc/ipsec.secrets",
	"/etc/sudoers.d",
	"/var/lib/nixorium/keys",
	"/var/lib/nixorium/remote-install",
	"/var/lib/nixorium/assistant",
	"/var/lib/nixorium/telemetry",
	"/var/lib/nixorium/backup",
	"/sys/class/dmi/id/product_serial",
	"/sys/class/dmi/id/product_uuid",
	"/sys/class/dmi/id/board_serial",
	"/sys/class/dmi/id/chassis_serial",
}

// refusedNames are file or directory names that hold secrets or personal
// data wherever they appear.
var refusedNames = regexp.MustCompile(`(?i)(secret|private|token|passw|credential|shadow|psk|snapshot|recovery|` +
	`\.key$|_key$|-key$|^key$|\.pem$|\.p12$|\.pfx$|\.age$|^id_(rsa|dsa|ecdsa|ed25519)$|keytab|htpasswd)`)

var storeRoot = "/nix/store"

// ErrRefused is returned for a path outside the readable locations or one
// that holds secrets.
var ErrRefused = errors.New("this path is not readable by the Assistant")

// validPath accepts a plain absolute path made of common characters only,
// so it can travel safely as one SSH argument.
var validPath = regexp.MustCompile(`^/[A-Za-z0-9._@+:,=/-]{0,511}$`)

// ValidRequest checks the form of a requested path before any access.
func ValidRequest(path string) bool {
	if !validPath.MatchString(path) || strings.Contains(path, "//") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	return filepath.Clean(path) == path || path == "/"
}

// Readable reports whether a lexically clean path is inside a readable
// location and not refused.
func Readable(path string) bool {
	if refused(path) {
		return false
	}
	for _, root := range readableRoots {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func refused(path string) bool {
	for _, prefix := range refusedPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part != "" && refusedNames.MatchString(part) {
			// A public SSH key is the one key file that may be read.
			if strings.HasSuffix(part, ".pub") && !strings.Contains(strings.ToLower(part), "private") {
				continue
			}
			return true
		}
	}
	return false
}

// Entry is one item of a directory listing.
type Entry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Size int64  `json:"size,omitempty"`
}

// Result is the answer for one path.
type Result struct {
	Path      string  `json:"path"`
	Resolved  string  `json:"resolved,omitempty"`
	Kind      string  `json:"kind"`
	Size      int64   `json:"size,omitempty"`
	Truncated bool    `json:"truncated,omitempty"`
	Binary    bool    `json:"binary,omitempty"`
	Content   string  `json:"content,omitempty"`
	Entries   []Entry `json:"entries,omitempty"`
	Hidden    int     `json:"hidden,omitempty"`
}

// Read returns a file's text or a directory's listing. Refused entries of a
// listing are counted in Hidden, not named.
func Read(path string) (Result, error) {
	if !ValidRequest(path) || !Readable(path) {
		return Result{}, ErrRefused
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, os.ErrNotExist
		}
		return Result{}, fmt.Errorf("resolve %s: %w", path, err)
	}
	if resolved != path && !Readable(resolved) && !(strings.HasPrefix(resolved, storeRoot+"/") && !refused(resolved)) {
		return Result{}, ErrRefused
	}
	descriptor, err := syscall.Open(resolved, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
			return Result{}, os.ErrPermission
		}
		return Result{}, fmt.Errorf("open %s: %w", path, err)
	}
	file := os.NewFile(uintptr(descriptor), resolved)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Result{}, err
	}
	result := Result{Path: path}
	if resolved != path {
		result.Resolved = resolved
	}
	switch {
	case info.IsDir():
		return listDirectory(file, path, result)
	case info.Mode().IsRegular():
		return readText(file, info, result)
	default:
		result.Kind = "special"
		return result, nil
	}
}

func readText(file *os.File, info os.FileInfo, result Result) (Result, error) {
	result.Kind, result.Size = "file", info.Size()
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		return Result{}, err
	}
	if len(data) > MaxFileBytes {
		data, result.Truncated = data[:MaxFileBytes], true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		result.Binary = true
		return result, nil
	}
	result.Content = withholdSecretLines(strings.ToValidUTF8(string(data), "�"))
	return result, nil
}

// secretLine matches a line that sets a password, key or token, such as a
// GRUB password_pbkdf2 entry or a key = value pair.
var secretLine = regexp.MustCompile(`(?i)(password|passwd|secret|token|psk|private[_-]?key|api[_-]?key)`)

// withholdSecretLines keeps the first word of a line that may carry a
// secret, so the file's structure stays readable, and withholds the rest.
func withholdSecretLines(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if !secretLine.MatchString(line) {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		first, _, _ := strings.Cut(trimmed, " ")
		first, _, _ = strings.Cut(first, "=")
		lines[index] = indent + first + " [withheld]"
	}
	return strings.Join(lines, "\n")
}

func listDirectory(directory *os.File, path string, result Result) (Result, error) {
	result.Kind = "directory"
	names, err := directory.Readdirnames(-1)
	if err != nil {
		return Result{}, err
	}
	sort.Strings(names)
	for _, name := range names {
		child := path + "/" + name
		if path == "/" {
			child = "/" + name
		}
		if !Readable(child) {
			result.Hidden++
			continue
		}
		if len(result.Entries) == MaxEntries {
			result.Truncated = true
			break
		}
		entry := Entry{Name: name, Kind: "other"}
		if info, err := os.Lstat(child); err == nil {
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				entry.Kind = "link"
			case info.IsDir():
				entry.Kind = "directory"
			case info.Mode().IsRegular():
				entry.Kind, entry.Size = "file", info.Size()
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

// Encode returns the JSON answer, bounded.
func Encode(result Result) ([]byte, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxOutputBytes {
		return nil, errors.New("answer too large")
	}
	return data, nil
}

// Decode reads an answer strictly and checks it again on the controller
// side: a computer cannot return a path it was not asked for, or content for
// a refused path.
func Decode(data []byte, requested string) (Result, error) {
	if len(data) > MaxOutputBytes {
		return Result{}, errors.New("answer too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, errors.New("invalid answer")
	}
	if result.Path != requested || !Readable(requested) {
		return Result{}, errors.New("answer does not match the request")
	}
	if result.Resolved != "" && !Readable(result.Resolved) && !(strings.HasPrefix(result.Resolved, storeRoot+"/") && !refused(result.Resolved)) {
		return Result{}, errors.New("answer resolves to a refused path")
	}
	switch result.Kind {
	case "file", "directory", "special":
	default:
		return Result{}, errors.New("invalid answer kind")
	}
	if len(result.Content) > MaxFileBytes || len(result.Entries) > MaxEntries {
		return Result{}, errors.New("answer exceeds its bounds")
	}
	for _, entry := range result.Entries {
		if strings.ContainsAny(entry.Name, "/\x00") || !Readable(strings.TrimSuffix(requested, "/")+"/"+entry.Name) {
			return Result{}, errors.New("answer lists a refused entry")
		}
	}
	return result, nil
}
