package classroomview

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Collecting the files of a student's desktop, the reverse of sending: the
// controller asks for the list (collect.list, answered by collect.listed
// with every regular file and folder of the XDG desktop folder), then asks
// for each file's content in order (collect.chunk with the entry's Index,
// answered by collect.data with the next piece of at most MaxChunkBytes),
// and ends with collect.end (answered by collect.done). The controller never
// names a path: it refers to the agent's own list by position. The agent
// only reads; it never changes or removes the student's files.
const (
	TypeCollectList   = "collect.list"
	TypeCollectListed = "collect.listed"
	TypeCollectChunk  = "collect.chunk"
	TypeCollectData   = "collect.data"
	TypeCollectEnd    = "collect.end"
	TypeCollectDone   = "collect.done"
	// CodeCollectTooMuch: the desktop holds more than one collection may
	// take (MaxFileEntries items, MaxFileBytes or MaxCollectPathBytes).
	CodeCollectTooMuch = "collect-too-much"
	// CodeCollectRefused: the agent could not list or read the desktop, or
	// a file changed while it was read.
	CodeCollectRefused = "collect-refused"
)

// MaxCollectPathBytes bounds the names of one desktop's list, so that the
// list always fits in one message.
const MaxCollectPathBytes = 512 << 10

// ErrCollectTooMuch means a desktop is beyond the limits of one collection.
var ErrCollectTooMuch = errors.New("the desktop holds more than 2000 items or 500 MB")

// Collector reads the files on the desktop of a session's user. Sessions of
// agents that predate collecting answer CodeUnsupported.
type Collector interface {
	// ListDesktop lists the desktop's regular files and folders, folders
	// before their contents, and counts the entries left out because their
	// names cannot be collected.
	ListDesktop() ([]FileEntry, int, error)
	// ReadDesktopFile reads the file at a position of the list from its
	// start and hands over its pieces in order; size is its listed size.
	ReadDesktopFile(index int, size int64, piece func([]byte) error) error
	// EndCollect forgets the list on the agent.
	EndCollect() error
}

// ValidateCollectEntries checks a desktop list received from a student
// computer, which is untrusted: a plain relative tree (no absolute paths,
// "..", links or special files, which a list cannot even express), names
// without control characters or backslashes, bounded depth and length, and
// the limits of one collection. An empty list is valid. Limits beyond which
// nothing is collected return ErrCollectTooMuch.
func ValidateCollectEntries(entries []FileEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > MaxFileEntries {
		return ErrCollectTooMuch
	}
	var total, names int64
	for _, entry := range entries {
		if strings.ContainsRune(entry.Path, '\\') {
			return errors.New("a file name contains a backslash")
		}
		if !entry.Dir && entry.Size > 0 {
			total += entry.Size
		}
		names += int64(len(entry.Path))
	}
	if total > MaxFileBytes || names > MaxCollectPathBytes {
		return ErrCollectTooMuch
	}
	return ValidateFileEntries(entries)
}

// collectableName tells whether one file or folder name can travel in a
// collection: valid UTF-8 without control characters or backslashes.
func collectableName(name string) bool {
	if name == "" || len(name) > maxNameBytes || !utf8.ValidString(name) || strings.ContainsRune(name, '\\') {
		return false
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

// ListDesktop lists a desktop folder for collecting, in order, folders
// before their contents. Hidden names, links and special files are left out
// silently; names that cannot be collected (control characters, a
// backslash, invalid UTF-8) and folders nested too deeply are left out and
// counted. A missing desktop is empty. Beyond the limits of one collection
// it stops with ErrCollectTooMuch.
func ListDesktop(desktop string) ([]FileEntry, int, error) {
	entries := []FileEntry{}
	skipped := 0
	info, err := os.Lstat(desktop)
	if errors.Is(err, os.ErrNotExist) {
		return entries, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !info.IsDir() {
		// A desktop that is a link or a file has nothing to collect.
		return entries, 0, nil
	}
	var total, names int64
	err = filepath.WalkDir(desktop, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable folder inside the desktop is left out; an
			// unreadable desktop is an error.
			if name != desktop && entry != nil && entry.IsDir() {
				skipped++
				return filepath.SkipDir
			}
			return err
		}
		if name == desktop {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(desktop, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !collectableName(entry.Name()) || strings.Count(relative, "/") >= maxPathElements {
			skipped++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		item := FileEntry{Path: relative, Dir: entry.IsDir()}
		if !item.Dir {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			item.Size = info.Size()
			total += item.Size
		}
		names += int64(len(relative))
		entries = append(entries, item)
		if len(entries) > MaxFileEntries || total > MaxFileBytes || names > MaxCollectPathBytes {
			return ErrCollectTooMuch
		}
		return nil
	})
	if err != nil {
		return nil, skipped, err
	}
	return entries, skipped, nil
}
