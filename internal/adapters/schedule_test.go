package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func scheduleStoreForTest(t *testing.T) ScheduleStore {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "classroom")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return ScheduleStore{Directory: directory}
}

func saveSchedule(t *testing.T, store ScheduleStore, schedule domain.Schedule) {
	t.Helper()
	if err := store.Update(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
		record.Schedule = schedule
		return record, true
	}); err != nil {
		t.Fatal(err)
	}
}

var workdays = []string{"mon", "tue", "wed", "thu", "fri"}

func TestScheduleIsKeptInAPrivateFileReplacedAtomically(t *testing.T) {
	store := scheduleStoreForTest(t)
	if record := store.Read(); !record.Empty() || record.Days == nil {
		t.Fatalf("empty store = %+v", record)
	}
	saveSchedule(t, store, domain.Schedule{On: "07:50", Off: "18:00", Days: workdays})
	path := filepath.Join(store.Directory, scheduleFile)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file = %v, %v", info, err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != `{"schemaVersion":1,"on":"07:50","off":"18:00","days":["mon","tue","wed","thu","fri"]}`+"\n" {
		t.Fatalf("content = %s", content)
	}
	record := store.Read()
	if record.On != "07:50" || record.Off != "18:00" || len(record.Days) != 5 || record.LastRun != "" {
		t.Fatalf("record = %+v", record)
	}
	// Saving again replaces the file and keeps the last minute that ran.
	if err := store.Update(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
		record.LastRun = "2026-10-12T07:50"
		return record, true
	}); err != nil {
		t.Fatal(err)
	}
	saveSchedule(t, store, domain.Schedule{Off: "17:30", Days: []string{"fri"}})
	record = store.Read()
	if record.On != "" || record.Off != "17:30" || record.LastRun != "2026-10-12T07:50" {
		t.Fatalf("record = %+v", record)
	}
	entries, _ := os.ReadDir(store.Directory)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".schedule-") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
	// Nothing to save leaves the file alone.
	before, _ := os.ReadFile(path)
	if err := store.Update(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
		return domain.ScheduleRecord{}, false
	}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("an unsaved change rewrote the file")
	}
}

func TestSavingAnEmptyScheduleRemovesTheFile(t *testing.T) {
	store := scheduleStoreForTest(t)
	saveSchedule(t, store, domain.Schedule{On: "07:50", Days: workdays})
	saveSchedule(t, store, domain.Schedule{Days: []string{}})
	if _, err := os.Stat(filepath.Join(store.Directory, scheduleFile)); !os.IsNotExist(err) {
		t.Fatalf("file after clear: %v", err)
	}
	if record := store.Read(); !record.Empty() {
		t.Fatalf("record = %+v", record)
	}
	// Clearing when there is nothing to clear is fine.
	saveSchedule(t, store, domain.Schedule{Days: []string{}})
}

func TestDamagedOrUnsafeScheduleFilesReadAsNoSchedule(t *testing.T) {
	good := `{"schemaVersion":1,"on":"07:50","days":["mon"]}` + "\n"
	for name, content := range map[string]string{
		"not json":          "on at seven",
		"truncated":         `{"schemaVersion":1,"on":"07:5`,
		"wrong version":     `{"schemaVersion":2,"on":"07:50","days":["mon"]}`,
		"unknown field":     `{"schemaVersion":1,"on":"07:50","days":["mon"],"extra":true}`,
		"bad time":          `{"schemaVersion":1,"on":"7:5","days":["mon"]}`,
		"no days":           `{"schemaVersion":1,"on":"07:50","days":[]}`,
		"no times":          `{"schemaVersion":1,"days":["mon"]}`,
		"days out of order": `{"schemaVersion":1,"on":"07:50","days":["fri","mon"]}`,
		"bad last minute":   `{"schemaVersion":1,"on":"07:50","days":["mon"],"lastRun":"now"}`,
		"trailing data":     good + good,
		"too large":         `{"schemaVersion":1,"on":"07:50","days":["mon"],"pad":"` + strings.Repeat("x", scheduleFileBytes) + `"}`,
	} {
		store := scheduleStoreForTest(t)
		if err := os.WriteFile(filepath.Join(store.Directory, scheduleFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if record := store.Read(); !record.Empty() {
			t.Fatalf("%s read as %+v", name, record)
		}
		// The next save replaces the damaged file.
		saveSchedule(t, store, domain.Schedule{Off: "18:00", Days: []string{"mon"}})
		if record := store.Read(); record.Off != "18:00" {
			t.Fatalf("%s: record after save = %+v", name, record)
		}
	}
	// A readable file with the wrong mode, or a link, is not trusted.
	store := scheduleStoreForTest(t)
	path := filepath.Join(store.Directory, scheduleFile)
	if err := os.WriteFile(path, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	if record := store.Read(); record.On != "07:50" {
		t.Fatalf("good file read as %+v", record)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if record := store.Read(); !record.Empty() {
		t.Fatalf("world-readable file read as %+v", record)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.json")
	if err := os.WriteFile(elsewhere, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, path); err != nil {
		t.Fatal(err)
	}
	if record := store.Read(); !record.Empty() {
		t.Fatalf("linked file read as %+v", record)
	}
}

func TestScheduleNeedsAPrivateStateDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "classroom")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	store := ScheduleStore{Directory: directory}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	err := store.Update(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
		record.Schedule = domain.Schedule{On: "07:50", Days: workdays}
		return record, true
	})
	if err == nil {
		t.Fatal("saved into a directory others can read")
	}
	if _, statErr := os.Stat(filepath.Join(directory, scheduleFile)); !os.IsNotExist(statErr) {
		t.Fatal("a schedule file was written")
	}
	if record := (ScheduleStore{Directory: filepath.Join(directory, "missing")}).Read(); !record.Empty() {
		t.Fatalf("missing directory read as %+v", record)
	}
}

// Many runs in the same minute, in several processes in practice, start the
// minute once: the claim is made under the file lock.
func TestOnlyOneRunClaimsAMinute(t *testing.T) {
	store := scheduleStoreForTest(t)
	saveSchedule(t, store, domain.Schedule{On: "07:50", Days: workdays})
	var claimed atomic.Int32
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			err := store.Update(func(record domain.ScheduleRecord) (domain.ScheduleRecord, bool) {
				if record.LastRun == "2026-10-12T07:50" {
					return record, false
				}
				record.LastRun = "2026-10-12T07:50"
				claimed.Add(1)
				return record, true
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("%d runs claimed the minute", claimed.Load())
	}
	if record := store.Read(); record.LastRun != "2026-10-12T07:50" || record.On != "07:50" {
		t.Fatalf("record = %+v", record)
	}
}
