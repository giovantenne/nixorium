package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// The schedule of switching the student computers on and off lives next to
// the learned Wake-on-LAN addresses, in the classroom service's own state
// directory (systemd StateDirectory, mode 0700, owned by the service user),
// outside Git, the deployment and the backup. The file is replaced
// atomically with mode 0600 under a lock; a missing, unsafe or damaged file
// reads as "no schedule".

const (
	scheduleFile      = "schedule.json"
	scheduleLock      = "schedule.lock"
	scheduleFileBytes = 16 << 10
)

// ScheduleStore keeps the schedule file in a private state directory.
type ScheduleStore struct {
	Directory string
}

func DefaultScheduleStore() ScheduleStore {
	return ScheduleStore{Directory: DefaultWakeAddressStore().Directory}
}

// ReadSchedule returns the schedule record, or the empty record.
func (Local) ReadSchedule() domain.ScheduleRecord { return DefaultScheduleStore().Read() }

// UpdateSchedule changes the record under its lock: change returns the new
// record and whether to save it. Saving an empty schedule removes the file.
func (Local) UpdateSchedule(change func(domain.ScheduleRecord) (domain.ScheduleRecord, bool)) error {
	return DefaultScheduleStore().Update(change)
}

func emptyScheduleRecord() domain.ScheduleRecord {
	return domain.ScheduleRecord{SchemaVersion: 1, Schedule: domain.Schedule{Days: []string{}}}
}

func (store ScheduleStore) Read() domain.ScheduleRecord {
	directory, err := openPrivateStateDirectory(store.Directory)
	if err != nil {
		return emptyScheduleRecord()
	}
	defer unix.Close(directory)
	return readScheduleRecord(directory)
}

func (store ScheduleStore) Update(change func(domain.ScheduleRecord) (domain.ScheduleRecord, bool)) error {
	directory, err := openPrivateStateDirectory(store.Directory)
	if err != nil {
		return err
	}
	defer unix.Close(directory)
	unlock, err := lockPrivateState(directory, scheduleLock)
	if err != nil {
		return err
	}
	defer unlock()
	next, save := change(readScheduleRecord(directory))
	if !save {
		return nil
	}
	if next.Empty() {
		if err := unix.Unlinkat(directory, scheduleFile, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
		return unix.Fsync(directory)
	}
	next.SchemaVersion = 1
	if next.Days == nil {
		next.Days = []string{}
	}
	content, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return replacePrivateFile(directory, scheduleFile, ".schedule-", content)
}

func readScheduleRecord(directory int) domain.ScheduleRecord {
	descriptor, err := unix.Openat(directory, scheduleFile, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return emptyScheduleRecord()
	}
	file := os.NewFile(uintptr(descriptor), scheduleFile)
	defer file.Close()
	if !wakeFileSafe(descriptor) {
		return emptyScheduleRecord()
	}
	content, err := io.ReadAll(io.LimitReader(file, scheduleFileBytes+1))
	if err != nil || len(content) > scheduleFileBytes {
		return emptyScheduleRecord()
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var record domain.ScheduleRecord
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF || record.SchemaVersion != 1 || record.Empty() || !record.Valid() {
		return emptyScheduleRecord()
	}
	return record
}
