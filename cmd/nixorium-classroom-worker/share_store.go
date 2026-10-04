package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// shareStore keeps files prepared for sending to students' desktops, in the
// worker's private temporary folder. The page and the teacher's terminal
// upload them in pieces; a transfer is complete when every file has its
// announced size. Transfers expire when unused, and few exist at a time.
type shareStore struct {
	mutex     sync.Mutex
	root      string
	transfers map[string]*shareTransfer
	now       func() time.Time
}

type shareTransfer struct {
	directory string
	entries   []classroomview.FileEntry
	written   []int64
	digests   []string
	used      time.Time
}

const (
	shareTransfers = 3
	shareIdle      = 30 * time.Minute
)

var errShareMissing = errors.New("the prepared files are not available")

func newShareStore(root string) *shareStore {
	return &shareStore{root: root, transfers: map[string]*shareTransfer{}, now: time.Now}
}

// Begin reserves a transfer for the listed files and folders.
func (store *shareStore) Begin(entries []classroomview.FileEntry) (string, error) {
	if err := classroomview.ValidateFileEntries(entries); err != nil {
		return "", err
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.pruneLocked()
	if len(store.transfers) >= shareTransfers {
		return "", errors.New("other files are being prepared; try again in a few minutes")
	}
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(store.root, "share-")
	if err != nil {
		return "", err
	}
	store.transfers[id] = &shareTransfer{
		directory: directory,
		entries:   append([]classroomview.FileEntry(nil), entries...),
		written:   make([]int64, len(entries)),
		digests:   make([]string, len(entries)),
		used:      store.now(),
	}
	return id, nil
}

// Write appends one piece to a file; pieces arrive in order.
func (store *shareStore) Write(id string, index int, offset int64, data []byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	transfer, found := store.transfers[id]
	if !found {
		return errShareMissing
	}
	transfer.used = store.now()
	if index < 0 || index >= len(transfer.entries) || transfer.entries[index].Dir || len(data) == 0 || len(data) > classroomview.MaxChunkBytes {
		return errors.New("a piece of a file is invalid")
	}
	entry := transfer.entries[index]
	if offset != transfer.written[index] || offset+int64(len(data)) > entry.Size {
		return errors.New("a piece of a file arrived out of order")
	}
	flags := os.O_WRONLY | os.O_APPEND
	if offset == 0 {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(store.path(transfer, index), flags, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	transfer.written[index] += int64(len(data))
	if transfer.written[index] == entry.Size {
		digest, err := fileDigest(store.path(transfer, index))
		if err != nil {
			return err
		}
		transfer.digests[index] = digest
	}
	return nil
}

// Files lists a complete transfer; empty files need no upload.
func (store *shareStore) Files(id string) ([]domain.ShareFile, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	transfer, found := store.transfers[id]
	if !found {
		return nil, errShareMissing
	}
	transfer.used = store.now()
	files := make([]domain.ShareFile, len(transfer.entries))
	for index, entry := range transfer.entries {
		file := domain.ShareFile{Path: entry.Path, Size: entry.Size, Dir: entry.Dir}
		if !entry.Dir {
			if transfer.written[index] != entry.Size {
				return nil, errors.New("some files were not prepared completely")
			}
			file.SHA256 = transfer.digests[index]
			if entry.Size == 0 {
				file.SHA256 = emptyDigest
			}
		}
		files[index] = file
	}
	return files, nil
}

func (store *shareStore) Open(id string, index int) (io.ReadCloser, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	transfer, found := store.transfers[id]
	if !found || index < 0 || index >= len(transfer.entries) || transfer.entries[index].Dir {
		return nil, errShareMissing
	}
	transfer.used = store.now()
	return os.Open(store.path(transfer, index))
}

// Remove forgets a transfer once it was sent.
func (store *shareStore) Remove(id string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if transfer, found := store.transfers[id]; found {
		_ = os.RemoveAll(transfer.directory)
		delete(store.transfers, id)
	}
}

func (store *shareStore) pruneLocked() {
	for id, transfer := range store.transfers {
		if store.now().Sub(transfer.used) > shareIdle {
			_ = os.RemoveAll(transfer.directory)
			delete(store.transfers, id)
		}
	}
}

// Files are stored by position, never by their (untrusted) names.
func (store *shareStore) path(transfer *shareTransfer, index int) string {
	return filepath.Join(transfer.directory, strconv.Itoa(index))
}

var emptyDigest = func() string {
	sum := sha256.Sum256(nil)
	return hex.EncodeToString(sum[:])
}()

func fileDigest(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
