package classroomview

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Saving into the home of the user who uses the controller: screenshots
// (one picture per computer) and collected desktops. The user's own process
// writes them, so they belong to that user. A save holds at most the
// pictures or desktops of maxSaveComputers computers.
const (
	maxSaveComputers = 200
	maxSaveEntries   = maxSaveComputers*(MaxFileEntries+1) + 1
	maxSaveBytes     = maxSaveComputers * MaxFileBytes
)

// ValidateSaveEntries refuses anything but a plain relative tree of named
// files and folders without backslashes, within the limits of a save.
func ValidateSaveEntries(entries []FileEntry) error {
	if len(entries) == 0 {
		return errors.New("nothing to save")
	}
	for _, entry := range entries {
		if strings.ContainsRune(entry.Path, '\\') {
			return errors.New("a file name contains a backslash")
		}
	}
	return validateTree(entries, maxSaveEntries, maxSaveBytes, errors.New("the files are too large"))
}

// TreeWriter writes a validated tree into a destination folder without
// replacing anything there. The pieces go to a private staging folder
// inside the destination first; when everything arrived, each top-level
// file or folder moves into place, and a name already taken becomes
// "name (2).ext", "name (3).ext" and so on. Folders are 0755 and files
// 0644, never executable, whatever the umask; nothing follows links.
type TreeWriter struct {
	entries     []FileEntry
	destination string
	staging     string
	// createDestination makes Finish create a missing destination.
	createDestination bool
	// next is the entry whose content is expected; written counts its bytes.
	next    int
	written int64
	file    *os.File
}

// NewTreeWriter prepares writing entries into destination, an existing
// folder.
func NewTreeWriter(destination string, entries []FileEntry) (*TreeWriter, error) {
	if err := ValidateSaveEntries(entries); err != nil {
		return nil, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", destination)
	}
	staging, err := os.MkdirTemp(destination, ".nixorium-incoming-")
	if err != nil {
		return nil, err
	}
	return startTreeWriter(&TreeWriter{entries: entries, destination: destination, staging: staging})
}

// NewStagedTreeWriter prepares writing entries into destination like
// NewTreeWriter, with two differences: the pieces go to a private folder
// under stagingParent (created when missing) so nothing appears in the
// destination before the end, and Finish creates the destination when it
// does not exist. The entries are validated as a sending to a computer,
// not as a save of many computers.
func NewStagedTreeWriter(destination, stagingParent string, entries []FileEntry) (*TreeWriter, error) {
	if err := ValidateFileEntries(entries); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stagingParent, 0o700); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(stagingParent, "incoming-")
	if err != nil {
		return nil, err
	}
	return startTreeWriter(&TreeWriter{entries: entries, destination: destination, staging: staging, createDestination: true})
}

// startTreeWriter creates the folders of the tree in the staging folder.
func startTreeWriter(writer *TreeWriter) (*TreeWriter, error) {
	staging, entries := writer.staging, writer.entries
	for _, entry := range entries {
		if entry.Dir {
			folder := filepath.Join(staging, filepath.FromSlash(entry.Path))
			if err := os.Mkdir(folder, 0o755); err != nil {
				writer.Abort()
				return nil, err
			}
			if err := os.Chmod(folder, 0o755); err != nil {
				writer.Abort()
				return nil, err
			}
		}
	}
	if err := writer.skipDone(); err != nil {
		writer.Abort()
		return nil, err
	}
	return writer, nil
}

// skipDone moves past folders and empty files to the next file whose
// content is expected.
func (writer *TreeWriter) skipDone() error {
	for writer.next < len(writer.entries) {
		entry := writer.entries[writer.next]
		if !entry.Dir {
			if entry.Size > 0 {
				return nil
			}
			if err := writer.open(); err != nil {
				return err
			}
			if err := writer.closeFile(); err != nil {
				return err
			}
		}
		writer.next++
	}
	return nil
}

func (writer *TreeWriter) open() error {
	if writer.file != nil {
		return nil
	}
	name := filepath.Join(writer.staging, filepath.FromSlash(writer.entries[writer.next].Path))
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	writer.file = file
	return nil
}

func (writer *TreeWriter) closeFile() error {
	file := writer.file
	writer.file = nil
	return file.Close()
}

// Write appends one piece to the file at index, which must be the next
// file expected; a file never grows past its announced size.
func (writer *TreeWriter) Write(index int, data []byte) error {
	if writer.next >= len(writer.entries) || index != writer.next || len(data) == 0 || len(data) > MaxChunkBytes {
		return errors.New("a piece of a file arrived out of order")
	}
	entry := writer.entries[index]
	if writer.written+int64(len(data)) > entry.Size {
		return errors.New("a file is larger than announced")
	}
	if err := writer.open(); err != nil {
		return err
	}
	if _, err := writer.file.Write(data); err != nil {
		return err
	}
	writer.written += int64(len(data))
	if writer.written == entry.Size {
		if err := writer.closeFile(); err != nil {
			return err
		}
		writer.next++
		writer.written = 0
		return writer.skipDone()
	}
	return nil
}

// Finish moves every top-level file and folder into the destination and
// returns the names they got there. The staging folder is always removed.
func (writer *TreeWriter) Finish() ([]string, error) {
	defer writer.Abort()
	if err := writer.skipDone(); err != nil || writer.next != len(writer.entries) {
		return nil, errors.New("some files did not arrive completely")
	}
	if writer.createDestination {
		if err := EnsureFolder(writer.destination); err != nil {
			return nil, err
		}
	}
	placed := []string{}
	for _, entry := range writer.entries {
		if strings.Contains(entry.Path, "/") {
			continue
		}
		name, err := placeNew(filepath.Join(writer.staging, entry.Path), writer.destination, entry.Path, entry.Dir)
		if err != nil {
			return placed, err
		}
		placed = append(placed, name)
	}
	return placed, nil
}

// Abort removes everything not yet moved into place.
func (writer *TreeWriter) Abort() {
	if writer.file != nil {
		_ = writer.file.Close()
		writer.file = nil
	}
	_ = os.RemoveAll(writer.staging)
}

// placeNew moves one file or folder into a folder without replacing
// anything there: a taken name becomes "name (2).ext", "name (3).ext" and
// so on. A file is linked (which fails on an existing name) and a folder
// renamed with RENAME_NOREPLACE.
func placeNew(source, folder, name string, dir bool) (string, error) {
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
		target := filepath.Join(folder, candidate)
		var err error
		if dir {
			err = unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
		} else {
			err = os.Link(source, target)
		}
		switch {
		case err == nil:
			return candidate, nil
		case errors.Is(err, os.ErrExist), errors.Is(err, unix.EEXIST), errors.Is(err, unix.ENOTEMPTY):
			continue
		default:
			return "", err
		}
	}
	return "", errors.New("no free name in the folder")
}

// EnsureFolder creates a folder and its missing parents with mode 0755;
// existing folders keep their modes.
func EnsureFolder(folder string) error {
	if !filepath.IsAbs(folder) {
		return errors.New("the folder is not an absolute path")
	}
	folder = filepath.Clean(folder)
	if info, err := os.Stat(folder); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a folder", folder)
		}
		return nil
	}
	if err := EnsureFolder(filepath.Dir(folder)); err != nil {
		return err
	}
	if err := os.Mkdir(folder, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	} else if err == nil {
		return os.Chmod(folder, 0o755)
	}
	return nil
}
