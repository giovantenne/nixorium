package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// receiver collects files sent to this session's desktop. It writes into a
// private folder of the home first and moves the result to the desktop only
// when everything arrived, so the student never sees half a sending. The
// agent runs as the student: the files belong to the student, folders are
// 0755 and files 0644, never executable.
type receiver struct {
	entries []classroomview.FileEntry
	staging string
	desktop string
	// next is the entry whose content is expected; written counts its bytes.
	next    int
	written int64
	file    *os.File
}

// errFiles is what the teacher sees when a sending is refused.
var errFiles = errors.New("the files could not be received")

func newReceiver(home string, entries []classroomview.FileEntry) (*receiver, error) {
	if err := classroomview.ValidateFileEntries(entries); err != nil {
		return nil, err
	}
	desktop, err := classroomview.DesktopDirectory(home)
	if err != nil {
		return nil, err
	}
	parent := filepath.Join(home, ".local", "share", "nixorium-classroom")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(parent, "incoming-")
	if err != nil {
		return nil, err
	}
	incoming := &receiver{entries: entries, staging: staging, desktop: desktop}
	for _, entry := range entries {
		if entry.Dir {
			// Explicit modes: the session's umask must not change them.
			folder := filepath.Join(staging, filepath.FromSlash(entry.Path))
			if err := os.Mkdir(folder, 0o755); err != nil {
				incoming.abort()
				return nil, err
			}
			if err := os.Chmod(folder, 0o755); err != nil {
				incoming.abort()
				return nil, err
			}
		}
	}
	if err := incoming.skipDone(); err != nil {
		incoming.abort()
		return nil, err
	}
	return incoming, nil
}

// skipDone moves past folders and empty files to the next file whose
// content is expected.
func (incoming *receiver) skipDone() error {
	for incoming.next < len(incoming.entries) {
		entry := incoming.entries[incoming.next]
		if !entry.Dir {
			if entry.Size > 0 {
				return nil
			}
			if err := incoming.open(); err != nil {
				return err
			}
			if err := incoming.file.Close(); err != nil {
				incoming.file = nil
				return err
			}
			incoming.file = nil
		}
		incoming.next++
	}
	return nil
}

func (incoming *receiver) open() error {
	if incoming.file != nil {
		return nil
	}
	name := filepath.Join(incoming.staging, filepath.FromSlash(incoming.entries[incoming.next].Path))
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	incoming.file = file
	return nil
}

func (incoming *receiver) chunk(index int, data []byte) error {
	if incoming.next >= len(incoming.entries) || index != incoming.next || len(data) == 0 || len(data) > classroomview.MaxChunkBytes {
		return errors.New("a piece of a file arrived out of order")
	}
	entry := incoming.entries[index]
	if incoming.written+int64(len(data)) > entry.Size {
		return errors.New("a file is larger than announced")
	}
	if err := incoming.open(); err != nil {
		return err
	}
	if _, err := incoming.file.Write(data); err != nil {
		return err
	}
	incoming.written += int64(len(data))
	if incoming.written == entry.Size {
		if err := incoming.file.Close(); err != nil {
			incoming.file = nil
			return err
		}
		incoming.file = nil
		incoming.next++
		incoming.written = 0
		return incoming.skipDone()
	}
	return nil
}

// finish places every top-level file and folder on the desktop and returns
// the names they got there.
func (incoming *receiver) finish() ([]string, error) {
	defer incoming.abort()
	if err := incoming.skipDone(); err != nil || incoming.next != len(incoming.entries) {
		return nil, errors.New("some files did not arrive completely")
	}
	if err := os.MkdirAll(incoming.desktop, 0o755); err != nil {
		return nil, err
	}
	placed := []string{}
	for _, entry := range incoming.entries {
		if strings.Contains(entry.Path, "/") {
			continue
		}
		name, err := place(filepath.Join(incoming.staging, entry.Path), incoming.desktop, entry.Path, entry.Dir)
		if err != nil {
			return placed, err
		}
		placed = append(placed, name)
	}
	return placed, nil
}

// place moves one file or folder to the desktop without replacing anything
// there: a taken name becomes "name (2).ext", "name (3).ext" and so on.
func place(source, desktop, name string, dir bool) (string, error) {
	base, extension := name, ""
	if !dir {
		extension = path.Ext(name)
		if extension == name {
			extension = ""
		}
		base = strings.TrimSuffix(name, extension)
	}
	for attempt := 1; attempt <= 100; attempt++ {
		candidate := name
		if attempt > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", base, attempt, extension)
		}
		target := filepath.Join(desktop, candidate)
		if dir {
			if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err := os.Rename(source, target); err != nil {
				return "", err
			}
			return candidate, nil
		}
		// A link fails when the name exists, so nothing is ever replaced.
		if err := os.Link(source, target); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}
		return candidate, nil
	}
	return "", errors.New("no free name on the desktop")
}

func (incoming *receiver) abort() {
	if incoming.file != nil {
		_ = incoming.file.Close()
		incoming.file = nil
	}
	_ = os.RemoveAll(incoming.staging)
}
