package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/giovantenne/nixorium/internal/domain"
)

// deploymentActivityCommand is a fixed read-only probe: the running system
// and revision, queued systemd jobs, and any running activation.
const deploymentActivityCommand = "nixorium-host-state 2>/dev/null; echo '--- jobs'; systemctl list-jobs --no-legend --no-pager 2>/dev/null; echo '--- activation'; pgrep -c -f '[s]witch-to-configuration' 2>/dev/null; true"

// ProcessRunning reports whether a PID still belongs to a Nixorium process.
func (Local) ProcessRunning(pid int) bool {
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	return bytes.Contains(content, []byte("nixorium"))
}

func (Local) ObserveDeploymentActivity(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.DeploymentActivity {
	tcp := probeSSHStatuses(ctx, hosts, timeout, probeHostSSH)
	return forEachHost(hosts, func(host domain.HostMeta) domain.DeploymentActivity {
		probe := tcp[host.Name]
		if probe.SSH != domain.SSHAvailable {
			return domain.DeploymentActivity{Detail: probe.Detail}
		}
		commandContext, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		command := exec.CommandContext(commandContext, "ssh", shutdownSSHArguments(host, timeout, deploymentActivityCommand)...)
		configureCommandCancellation(command)
		output := &boundedCommandBuffer{limit: 64 * 1024}
		command.Stdout = output
		if err := command.Run(); err != nil || output.truncated {
			return domain.DeploymentActivity{Detail: "management access failed"}
		}
		return parseDeploymentActivity(output.buffer.String())
	})
}

func parseDeploymentActivity(output string) domain.DeploymentActivity {
	activity := domain.DeploymentActivity{Reachable: true}
	state, rest, _ := strings.Cut(output, "--- jobs\n")
	jobs, activation, _ := strings.Cut(rest, "--- activation\n")
	if lines := strings.Fields(state); len(lines) >= 2 && validGitRevision(lines[1]) {
		activity.Revision = lines[1]
	}
	// An unreadable count is treated as activity: never guess "finished".
	fields := strings.Fields(activation)
	if len(fields) == 0 {
		activity.Activating = true
	} else if count, err := strconv.Atoi(fields[0]); err != nil || count > 0 {
		activity.Activating = true
	}
	for _, line := range strings.Split(jobs, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "No jobs") {
			activity.Activating = true
		}
	}
	return activity
}

// ArchivePendingDeployment moves the reviewed record aside under the operation
// lock, without overwriting an earlier archive. The record must be unchanged.
func (Local) ArchivePendingDeployment(expected domain.PendingDeployment) (string, error) {
	return archivePendingDeploymentAt(managedCoordinationDirectory, true, expected)
}

func archivePendingDeploymentAt(directoryPath string, managed bool, expected domain.PendingDeployment) (string, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), coordinationLockName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("open managed operation lock: %w", err)
	}
	lock := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, coordinationLockName))
	defer lock.Close()
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return "", operationBusy(lock)
	}
	recordOperationOwner(lock, "Recover interrupted client update")
	defer func() {
		clearOperationOwner(lock)
		_ = syscall.Flock(descriptor, syscall.LOCK_UN)
	}()
	current, present, err := pendingDeploymentAt(directoryPath)
	if !present {
		return "", errors.New("the interrupted update record is gone")
	}
	if err != nil {
		return "", err
	}
	if currentText, expectedText := fmt.Sprintf("%+v", current), fmt.Sprintf("%+v", expected); currentText != expectedText {
		return "", errors.New("the interrupted update record changed after review")
	}
	if err := unix.Mkdirat(int(directory.Fd()), "recovered", 0o770); err != nil && !errors.Is(err, unix.EEXIST) {
		return "", fmt.Errorf("create the recovered records directory: %w", err)
	}
	recoveredFD, err := unix.Openat(int(directory.Fd()), "recovered", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	recovered := os.NewFile(uintptr(recoveredFD), filepath.Join(directoryPath, "recovered"))
	defer recovered.Close()
	name := "deployment-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".json"
	if err := unix.Renameat2(int(directory.Fd()), deploymentPendingName, recoveredFD, name, unix.RENAME_NOREPLACE); err != nil {
		return "", fmt.Errorf("archive the interrupted update record: %w", err)
	}
	if err := recovered.Sync(); err != nil {
		return "", err
	}
	if err := directory.Sync(); err != nil {
		return "", err
	}
	return filepath.Join(directoryPath, "recovered", name), nil
}
