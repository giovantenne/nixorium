package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const deploymentPendingName = "deployment-pending.json"

func deploymentPhaseTimeout(phase domain.DeploymentPhase) time.Duration {
	if phase == domain.DeploymentPhaseApply {
		return 2 * time.Hour
	}
	return 6 * time.Hour
}

// A pending record is written and synced before dispatch, while the caller
// holds the fleet gate. Even process death or controller reboot leaves the
// affected operation blocked until an administrator reviews remote completion.
func createDeploymentPending(directory *os.File, plan domain.DeploymentPlanReport) (*os.File, error) {
	if !filepath.IsAbs(plan.Repository) || !validGitRevision(plan.Revision) || len(plan.Targets) == 0 || plan.ColmenaSelector == "" {
		return nil, errors.New("deployment pending identity is invalid")
	}
	content, err := json.Marshal(struct {
		SchemaVersion int                       `json:"schemaVersion"`
		Repository    string                    `json:"repository"`
		Revision      string                    `json:"revision"`
		Targets       []domain.DeploymentTarget `json:"targets"`
		StartedAt     time.Time                 `json:"startedAt"`
		ControllerPID int                       `json:"controllerPid"`
	}{1, plan.Repository, plan.Revision, plan.Targets, time.Now().UTC(), os.Getpid()})
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Openat(int(directory.Fd()), deploymentPendingName, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), deploymentPendingName)
	if _, err = file.Write(append(content, '\n')); err == nil {
		err = file.Sync()
	}
	if err == nil {
		err = directory.Sync()
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func clearDeploymentPending(directory, pending *os.File) error {
	// Never unlink a replacement supplied after dispatch.
	path := filepath.Join(directory.Name(), deploymentPendingName)
	original, err := pending.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) {
		return errors.New("deployment pending record changed during execution")
	}
	if err := syscall.Unlinkat(int(directory.Fd()), deploymentPendingName); err != nil {
		return err
	}
	return directory.Sync()
}

func checkDeploymentPendingAt(directory *os.File) error {
	return checkDeploymentPendingPath(directory.Name())
}

func checkDeploymentPendingPath(directory string) error {
	_, err := os.Lstat(filepath.Join(directory, deploymentPendingName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect pending deployment: %w", err)
	}
	return errors.New("an unfinished client deployment blocks new operations; inspect deployment-pending.json and follow the deployment recovery guide")
}

func runDeploymentPhase(ctx context.Context, plan domain.DeploymentPlanReport, phase domain.DeploymentPhase, output io.Writer, coordinationDirectory string, managed bool, timeout time.Duration) error {
	arguments, err := deploymentCommand(phase, plan.ColmenaSelector)
	if err != nil {
		return err
	}
	phaseContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(phaseContext, "colmena", arguments...)
	// Bound both process lifetime and inherited output pipes. Killing only
	// Colmena can otherwise leave local ssh children holding the log pipe open.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
	command.Dir = plan.Repository
	command.Stdout, command.Stderr = output, output
	var directory, pending *os.File
	if phase == domain.DeploymentPhaseApply {
		cleanup, err := configureColmenaSSH(command)
		if err != nil {
			return fmt.Errorf("prepare Colmena SSH policy: %w", err)
		}
		defer cleanup()
		directory, err = openCoordinationDirectory(coordinationDirectory, managed)
		if err != nil {
			return err
		}
		defer directory.Close()
		pending, err = createDeploymentPending(directory, plan)
		if err != nil {
			return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("persist deployment intent; no new apply started: %w", err)}
		}
		defer pending.Close()
	}
	if err := command.Start(); err != nil {
		if pending != nil {
			if clearErr := clearDeploymentPending(directory, pending); clearErr != nil {
				return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("apply never started, but clearing its intent failed: %w", clearErr)}
			}
		}
		return fmt.Errorf("start colmena %s: %w", phase, err)
	}
	err = command.Wait()
	if err != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	if phaseContext.Err() != nil {
		err = phaseContext.Err()
	}
	if err != nil {
		result := fmt.Errorf("colmena %s: %w", phase, err)
		if pending != nil {
			return &domain.DeploymentUnconfirmedError{Err: result}
		}
		return result
	}
	if pending != nil {
		if err := clearDeploymentPending(directory, pending); err != nil {
			return &domain.DeploymentUnconfirmedError{Err: fmt.Errorf("apply returned, but clearing its pending record failed: %w", err)}
		}
	}
	return nil
}
