package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// collectStore keeps files collected from students' desktops in the
// worker's private temporary folder until the user who asked copies them.
// Files are stored by their position in the computer's list, never by
// their (untrusted) names; a computer whose files did not all arrive
// leaves nothing behind. Collections expire when unused.
type collectStore struct {
	mutex       sync.Mutex
	root        string
	collections map[string]*collectedSet
	// reserve is the free space always left on the controller; free reads
	// the space available in a folder.
	reserve int64
	free    func(string) (int64, error)
	now     func() time.Time
}

type collectedSet struct {
	directory string
	folder    string
	computers map[string][]classroomview.FileEntry
	used      time.Time
}

const (
	collectSets = 3
	collectIdle = 30 * time.Minute
	// collectReserve keeps a gigabyte free for the controller's own work.
	collectReserve = 1 << 30
)

var errCollectMissing = errors.New("the collected files are not available")

func newCollectStore(root string) *collectStore {
	return &collectStore{root: root, collections: map[string]*collectedSet{}, reserve: collectReserve, free: freeSpace, now: time.Now}
}

func freeSpace(folder string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(folder, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// Begin reserves a collection whose files go into a folder named folder.
func (store *collectStore) Begin(folder string) (string, error) {
	if err := classroomview.ValidateSaveEntries([]classroomview.FileEntry{{Path: folder, Dir: true}}); err != nil {
		return "", err
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.pruneLocked()
	if len(store.collections) >= collectSets {
		return "", errors.New("other collected files are still being saved; try again in a few minutes")
	}
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(store.root, "collect-")
	if err != nil {
		return "", err
	}
	store.collections[id] = &collectedSet{directory: directory, folder: folder, computers: map[string][]classroomview.FileEntry{}, used: store.now()}
	return id, nil
}

// Store keeps the files of one computer. fill writes each file's pieces in
// order; when it fails, or a file is incomplete, the computer's folder is
// removed and nothing of it is kept.
func (store *collectStore) Store(collection, computer string, entries []classroomview.FileEntry, fill func(func(int, []byte) error) error) error {
	if !computerNamePattern.MatchString(computer) || len(entries) == 0 {
		return errors.New("invalid computer or files")
	}
	if err := classroomview.ValidateCollectEntries(entries); err != nil {
		return err
	}
	store.mutex.Lock()
	set, found := store.collections[collection]
	if found {
		set.used = store.now()
	}
	store.mutex.Unlock()
	if !found {
		return errCollectMissing
	}
	_, total := collectTotals(entries)
	// The user's copy needs the same space again until this one is removed.
	available, err := store.free(store.root)
	if err != nil || available < 2*total+store.reserve {
		return app.ErrCollectNoSpace
	}
	folder := filepath.Join(set.directory, computer)
	if err := os.Mkdir(folder, 0o700); err != nil {
		return err
	}
	writer := &collectWriter{folder: folder, entries: entries}
	err = fill(writer.write)
	if err == nil {
		err = writer.finish()
	}
	writer.close()
	if err != nil {
		_ = os.RemoveAll(folder)
		return err
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if current, found := store.collections[collection]; !found || current != set {
		_ = os.RemoveAll(folder)
		return errCollectMissing
	}
	set.computers[computer] = entries
	set.used = store.now()
	return nil
}

func collectTotals(entries []classroomview.FileEntry) (int, int64) {
	files, total := 0, int64(0)
	for _, entry := range entries {
		if !entry.Dir {
			files++
			total += entry.Size
		}
	}
	return files, total
}

// collectWriter writes one computer's files in list order: each piece must
// belong to the next file expected and no file grows past its size.
type collectWriter struct {
	folder  string
	entries []classroomview.FileEntry
	next    int
	written int64
	file    *os.File
}

func (writer *collectWriter) skipDone() error {
	for writer.next < len(writer.entries) {
		entry := writer.entries[writer.next]
		if !entry.Dir && entry.Size > 0 {
			return nil
		}
		if !entry.Dir {
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

func (writer *collectWriter) open() error {
	if writer.file != nil {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(writer.folder, strconv.Itoa(writer.next)), os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	writer.file = file
	return nil
}

func (writer *collectWriter) closeFile() error {
	file := writer.file
	writer.file = nil
	return file.Close()
}

func (writer *collectWriter) write(index int, data []byte) error {
	if err := writer.skipDone(); err != nil {
		return err
	}
	if writer.next >= len(writer.entries) || index != writer.next || len(data) == 0 || len(data) > classroomview.MaxChunkBytes {
		return errors.New("a piece of a file arrived out of order")
	}
	entry := writer.entries[index]
	if writer.written+int64(len(data)) > entry.Size {
		return errors.New("a file is larger than its listed size")
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
	}
	return nil
}

func (writer *collectWriter) finish() error {
	if err := writer.skipDone(); err != nil {
		return err
	}
	if writer.next != len(writer.entries) {
		return errors.New("some files did not arrive completely")
	}
	return nil
}

func (writer *collectWriter) close() {
	if writer.file != nil {
		_ = writer.file.Close()
		writer.file = nil
	}
}

// collectedItem is one item of a collection as the user saves it.
type collectedItem struct {
	domain.ShareFile
	file string
}

// Items lists a collection for saving: its folder, then one folder per
// computer (in name order) with that computer's files and folders.
func (store *collectStore) Items(collection string) ([]collectedItem, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	set, found := store.collections[collection]
	if !found || len(set.computers) == 0 {
		return nil, errCollectMissing
	}
	set.used = store.now()
	names := make([]string, 0, len(set.computers))
	for name := range set.computers {
		names = append(names, name)
	}
	sort.Strings(names)
	items := []collectedItem{{ShareFile: domain.ShareFile{Path: set.folder, Dir: true}}}
	for _, name := range names {
		prefix := set.folder + "/" + name
		items = append(items, collectedItem{ShareFile: domain.ShareFile{Path: prefix, Dir: true}})
		for index, entry := range set.computers[name] {
			item := collectedItem{ShareFile: domain.ShareFile{Path: prefix + "/" + entry.Path, Size: entry.Size, Dir: entry.Dir}}
			if !entry.Dir {
				item.file = filepath.Join(set.directory, name, strconv.Itoa(index))
			}
			items = append(items, item)
		}
	}
	return items, nil
}

// Read returns size bytes of a collected file from offset.
func (store *collectStore) Read(collection string, item collectedItem, offset int64, size int) ([]byte, error) {
	store.mutex.Lock()
	set, found := store.collections[collection]
	if found {
		set.used = store.now()
	}
	store.mutex.Unlock()
	if !found || item.file == "" || filepath.Dir(filepath.Dir(item.file)) != set.directory {
		return nil, errCollectMissing
	}
	file, err := os.Open(item.file)
	if err != nil {
		return nil, errCollectMissing
	}
	defer file.Close()
	data := make([]byte, size)
	count, err := file.ReadAt(data, offset)
	if count != size {
		if err == nil || errors.Is(err, io.EOF) {
			err = errors.New("a collected file is shorter than expected")
		}
		return nil, err
	}
	return data, nil
}

// Remove forgets a collection and removes its files.
func (store *collectStore) Remove(collection string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if set, found := store.collections[collection]; found {
		_ = os.RemoveAll(set.directory)
		delete(store.collections, collection)
	}
}

func (store *collectStore) pruneLocked() {
	for id, set := range store.collections {
		if store.now().Sub(set.used) > collectIdle {
			_ = os.RemoveAll(set.directory)
			delete(store.collections, id)
		}
	}
}

// expire removes unused collections until ctx ends.
func (store *collectStore) expire(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store.mutex.Lock()
			store.pruneLocked()
			store.mutex.Unlock()
		}
	}
}
