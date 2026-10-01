package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

const maximumPendingDeploymentBytes = 256 * 1024

// PendingDeployment reads the durable record of an interrupted client update.
// An unsafe or invalid record is reported as an error, never ignored.
func (Local) PendingDeployment() (domain.PendingDeployment, bool, error) {
	return pendingDeploymentAt(managedCoordinationDirectory)
}

func pendingDeploymentAt(directory string) (domain.PendingDeployment, bool, error) {
	var record domain.PendingDeployment
	path := filepath.Join(directory, deploymentPendingName)
	data, mode, err := readRegularFileNoFollowLimit(path, maximumPendingDeploymentBytes)
	if errors.Is(err, os.ErrNotExist) {
		return record, false, nil
	}
	if err != nil {
		return record, true, err
	}
	if mode&0o077 != 0 {
		return record, true, errors.New("the pending deployment record is not private")
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, true, errors.New("the pending deployment record is not valid JSON")
	}
	if record.SchemaVersion != 1 || !filepath.IsAbs(record.Repository) || !validGitRevision(record.Revision) || len(record.Targets) == 0 || record.StartedAt.IsZero() {
		return record, true, errors.New("the pending deployment record is incomplete")
	}
	return record, true, nil
}

// TemplateResetPending reports an interrupted template reset marker.
func (Local) TemplateResetPending(repository string) bool {
	_, err := os.Lstat(filepath.Join(repository, ".git", resetPendingName))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

// OperationLockHolder reports a held operation lock with its confirmed
// holder. Pending deployments and reservations are reported separately.
func (Local) OperationLockHolder() (domain.OperationHolder, bool) {
	return operationLockHolderAt(managedCoordinationDirectory, true)
}

func operationLockHolderAt(directoryPath string, managed bool) (domain.OperationHolder, bool) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return domain.OperationHolder{}, false
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), coordinationLockName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return domain.OperationHolder{}, false
	}
	lock := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, coordinationLockName))
	defer lock.Close()
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return operationBusy(lock).OperationHolder, true
	}
	_ = syscall.Flock(descriptor, syscall.LOCK_UN)
	return domain.OperationHolder{}, false
}

// LabSettingsIssues decodes the managed settings without Nix evaluation.
func (Local) LabSettingsIssues(repository string) []domain.ValidationIssue {
	data, err := os.ReadFile(filepath.Join(repository, "lab-settings.json"))
	if err != nil {
		return []domain.ValidationIssue{{Field: "lab-settings.json", Message: "cannot read the laboratory settings: " + err.Error()}}
	}
	_, issues := domain.DecodeLabSettings(data)
	return issues
}

// ControllerActivationDrift reports when this controller runs another system
// than its last successful reviewed activation, for example after booting an
// older generation. It is empty when no activation was recorded.
func (Local) ControllerActivationDrift() string {
	data, mode, err := readRegularFileNoFollowLimit(controllerActivationPath, maximumControllerActivationBytes)
	if err != nil || mode != 0o644 {
		return ""
	}
	record, err := domain.DecodeControllerActivation(data)
	if err != nil || record.SystemPath == "" {
		return ""
	}
	running, err := filepath.EvalSymlinks("/run/current-system")
	if err != nil || running == record.SystemPath || !strings.HasPrefix(running, "/nix/store/") {
		return ""
	}
	return "the running system differs from the last applied configuration"
}

// ClockSynchronized asks systemd whether network time is synchronized.
func (Local) ClockSynchronized() (bool, error) {
	output, err := run(context.Background(), "timedatectl", "show", "--property=NTPSynchronized", "--value")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "yes", nil
}
