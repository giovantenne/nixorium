package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The holder of the operation lock describes itself inside the lock file so
// that a refused operation can say what is running, who started it and when.
// The description is informational only: the flock remains the lock, and a
// description that cannot be confirmed against a live process is ignored.

// OperationBusyMessage starts every refusal caused by a held operation lock.
const OperationBusyMessage = "another Nixorium controller or client operation is already running"

const operationOwnerMaxBytes = 1024

type operationOwner struct {
	Operation string    `json:"operation"`
	User      string    `json:"user"`
	PID       int       `json:"pid"`
	PIDStart  string    `json:"pidStart"`
	StartedAt time.Time `json:"startedAt"`
}

// OperationBusyError reports a held lock and, when it can be confirmed, its
// holder.
type OperationBusyError struct {
	Operation string
	User      string
	StartedAt time.Time
	Known     bool
}

func (err *OperationBusyError) Error() string {
	if !err.Known {
		return OperationBusyMessage + "; open its progress in Nixorium or wait for it to finish"
	}
	return OperationBusyMessage + ": " + err.Holder()
}

// Holder describes the running operation in a few words.
func (err *OperationBusyError) Holder() string {
	if !err.Known {
		return "an operation whose owner is not recorded"
	}
	description := err.Operation
	if err.User != "" && err.User != "root" {
		description += ", started by " + err.User
	} else {
		description += ", started"
	}
	return description + " at " + err.StartedAt.Local().Format("15:04")
}

// Labels for privileged units that record only their program name.
var operationOwnerLabels = map[string]string{
	"nixorium-apply-controller":             "Apply to controller",
	"nixorium-prepare-pxe":                  "Prepare network installation",
	"nixorium-restart-cache":                "Restart the binary cache",
	"nixorium-install-secrets":              "Install controller keys",
	"nixorium-clean-controller-generations": "Free disk space",
	"nixorium-pxe":                          "Network installation",
	"nixorium-pxe-recover":                  "Recover controller networking",
	"nixorium-remote-worker":                "USB installation",
	"nixorium-classroom-worker":             "Classroom controls",
}

func defaultOperationLabel() string {
	name := filepath.Base(os.Args[0])
	if label, found := operationOwnerLabels[name]; found {
		return label
	}
	return "Nixorium operation"
}

// recordOperationOwner replaces the lock content with this process. It is
// called only while holding the lock; failures leave an unknown owner.
func recordOperationOwner(lock *os.File, label string) {
	if lock == nil {
		return
	}
	start, err := processStartTime(os.Getpid())
	if err != nil {
		return
	}
	name := ""
	if current, lookupErr := user.Current(); lookupErr == nil {
		name = current.Username
	}
	// The classroom worker runs as the administrator on behalf of the teacher.
	if filepath.Base(os.Args[0]) == "nixorium-classroom-worker" && label != operationOwnerLabels["nixorium-classroom-worker"] {
		label, name = label+" from classroom controls", ""
	}
	content, err := json.Marshal(operationOwner{Operation: label, User: name, PID: os.Getpid(), PIDStart: start, StartedAt: time.Now().UTC()})
	if err != nil {
		return
	}
	if err := lock.Truncate(0); err != nil {
		return
	}
	_, _ = lock.WriteAt(append(content, '\n'), 0)
}

func clearOperationOwner(lock *os.File) {
	if lock != nil {
		_ = lock.Truncate(0)
	}
}

// operationBusy builds the refusal for a held lock from its recorded owner.
func operationBusy(lock *os.File) *OperationBusyError {
	busy := &OperationBusyError{}
	if lock == nil {
		return busy
	}
	buffer := make([]byte, operationOwnerMaxBytes+1)
	count, _ := lock.ReadAt(buffer, 0)
	if count == 0 || count > operationOwnerMaxBytes {
		return busy
	}
	var owner operationOwner
	if json.Unmarshal(buffer[:count], &owner) != nil || owner.PID <= 0 || owner.Operation == "" || owner.StartedAt.IsZero() {
		return busy
	}
	start, err := processStartTime(owner.PID)
	if err != nil || start != owner.PIDStart {
		return busy
	}
	operation := owner.Operation
	if label, found := operationOwnerLabels[operation]; found {
		operation = label
	}
	if len(operation) > 80 || strings.ContainsAny(operation, "\n\r\x1b") {
		return busy
	}
	busy.Operation, busy.User, busy.StartedAt, busy.Known = operation, owner.User, owner.StartedAt, true
	return busy
}

// processStartTime returns field 22 of /proc/PID/stat, which identifies a
// process together with its PID.
func processStartTime(pid int) (string, error) {
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	text := string(content)
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return "", fmt.Errorf("process %d status is malformed", pid)
	}
	fields := strings.Fields(text[end+1:])
	// Field 3 (state) is the first after the command name; start time is 22.
	if len(fields) < 20 {
		return "", fmt.Errorf("process %d status is malformed", pid)
	}
	return fields[19], nil
}
