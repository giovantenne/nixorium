package classroomview

import (
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

// Files sent to a student's desktop: the controller announces every entry
// (files.begin), sends each file's content in order (files.chunk, one ack
// each), then asks the agent to place them (files.end). Directories come
// before their contents; there are no links.
const (
	TypeFilesBegin = "files.begin"
	TypeFilesReady = "files.ready"
	TypeFilesChunk = "files.chunk"
	TypeFilesAck   = "files.ack"
	TypeFilesEnd   = "files.end"
	TypeFilesDone  = "files.done"
	// CodeFilesRefused: the agent did not accept the files.
	CodeFilesRefused = "files-refused"
)

// Limits of one sending.
const (
	MaxFileEntries  = 2000
	MaxFileBytes    = 500 << 20
	MaxChunkBytes   = 1 << 20
	maxNameBytes    = 255
	maxPathElements = 32
)

// FileEntry is a file or directory, by its path relative to the sent
// folder.
type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size,omitempty"`
	Dir  bool   `json:"dir,omitempty"`
}

// ValidateFileEntries refuses anything but a plain tree of named files and
// folders within the limits.
func ValidateFileEntries(entries []FileEntry) error {
	if len(entries) == 0 {
		return errors.New("nothing to send")
	}
	if len(entries) > MaxFileEntries {
		return errors.New("too many files and folders")
	}
	var total int64
	seen := map[string]bool{}
	dirs := map[string]bool{}
	for _, entry := range entries {
		if err := validFilePath(entry.Path); err != nil {
			return err
		}
		if seen[entry.Path] {
			return errors.New("a file or folder is listed twice")
		}
		seen[entry.Path] = true
		if parent := path.Dir(entry.Path); parent != "." && !dirs[parent] {
			return errors.New("a folder must come before its contents")
		}
		if entry.Dir {
			if entry.Size != 0 {
				return errors.New("a folder has no size")
			}
			dirs[entry.Path] = true
			continue
		}
		if entry.Size < 0 || entry.Size > MaxFileBytes {
			return errors.New("a file size is out of range")
		}
		total += entry.Size
		if total > MaxFileBytes {
			return errors.New("the files are larger than 500 MB")
		}
	}
	return nil
}

func validFilePath(name string) error {
	if name == "" || !utf8.ValidString(name) || path.Clean(name) != name || path.IsAbs(name) || strings.HasPrefix(name, "../") || name == ".." {
		return errors.New("a file name is not a plain relative path")
	}
	elements := strings.Split(name, "/")
	if len(elements) > maxPathElements {
		return errors.New("folders are nested too deeply")
	}
	for _, element := range elements {
		if element == "" || element == "." || element == ".." || len(element) > maxNameBytes {
			return errors.New("a file name is not a plain relative path")
		}
		for _, character := range element {
			if character < 0x20 || character == 0x7f {
				return errors.New("a file name contains control characters")
			}
		}
	}
	return nil
}
