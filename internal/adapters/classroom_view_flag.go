package adapters

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// ClassroomViewFlagPath is written by the controller's NixOS module (apply to
// the controller) and says whether this controller offers the classroom view.
const ClassroomViewFlagPath = "/etc/nixorium/classroom-view.json"

// ClassroomViewApplied tells whether the applied controller configuration
// has the classroom view on. It fails closed: a missing, unreadable, linked
// or malformed file means off.
func ClassroomViewApplied() bool {
	return classroomViewAppliedAt(ClassroomViewFlagPath)
}

func classroomViewAppliedAt(path string) bool {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil || len(data) > 1024 {
		return false
	}
	var wire struct {
		SchemaVersion int  `json:"schemaVersion"`
		Enabled       bool `json:"enabled"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil || wire.SchemaVersion != 1 {
		return false
	}
	return wire.Enabled
}
