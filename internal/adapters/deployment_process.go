package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const deploymentPendingName = "deployment-pending.json"

// deploymentPhaseTimeout bounds a phase. The apply bound grows with the
// computers, so that copies and activations proceeding normally on a large
// laboratory are never cut off at once and left unknown.
func deploymentPhaseTimeout(phase domain.DeploymentPhase, targets int) time.Duration {
	if phase != domain.DeploymentPhaseApply {
		return 6 * time.Hour
	}
	copies := time.Duration((targets+clientCopyParallelism-1)/clientCopyParallelism) * clientCopyTimeout
	activations := time.Duration((targets+clientActivationParallelism-1)/clientActivationParallelism) * clientActivationTimeout
	return max(2*time.Hour, copies+activations+10*time.Minute)
}

// A pending record is written and synced before dispatch, while the caller
// holds the fleet gate. Even process death or controller reboot leaves the
// affected computers blocked until their outcome is known or reviewed.
// The engine then rewrites it atomically as each computer progresses.
type pendingRecord struct {
	mutex     sync.Mutex
	directory *os.File
	file      *os.File
	record    domain.PendingDeployment
}

func createDeploymentPending(directory *os.File, plan domain.DeploymentPlanReport, activations []domain.PendingActivation) (*pendingRecord, error) {
	if !filepath.IsAbs(plan.Repository) || !validGitRevision(plan.Revision) || len(plan.Targets) == 0 {
		return nil, errors.New("deployment pending identity is invalid")
	}
	// The start time binds the record to this process; without it the record
	// is still written, and the review falls back to the PID alone.
	start, _ := processStartTime(os.Getpid())
	record := domain.PendingDeployment{SchemaVersion: 1, Repository: plan.Repository, Revision: plan.Revision, Targets: plan.Targets,
		StartedAt: time.Now().UTC(), ControllerPID: os.Getpid(), ControllerPIDStart: start, Activations: activations}
	content, err := json.Marshal(record)
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
	return &pendingRecord{directory: directory, file: file, record: record}, nil
}

func (pending *pendingRecord) activation(name string) domain.PendingActivation {
	pending.mutex.Lock()
	defer pending.mutex.Unlock()
	return pending.record.ActivationOf(name)
}

// update changes one computer's progress and replaces the record atomically.
// It refuses when the record on disk is no longer the one this process wrote.
func (pending *pendingRecord) update(name string, change func(*domain.PendingActivation)) error {
	return pending.rewrite(func(record *domain.PendingDeployment) error {
		for index := range record.Activations {
			if record.Activations[index].Name == name {
				change(&record.Activations[index])
				return nil
			}
		}
		return fmt.Errorf("%s is not part of the pending record", name)
	})
}

func (pending *pendingRecord) rewrite(change func(*domain.PendingDeployment) error) error {
	pending.mutex.Lock()
	defer pending.mutex.Unlock()
	if err := pending.unchanged(); err != nil {
		return err
	}
	next := pending.record
	next.Activations = append([]domain.PendingActivation(nil), pending.record.Activations...)
	if err := change(&next); err != nil {
		return err
	}
	content, err := json.Marshal(next)
	if err != nil {
		return err
	}
	const temporaryName = deploymentPendingName + ".next"
	_ = syscall.Unlinkat(int(pending.directory.Fd()), temporaryName)
	fd, err := syscall.Openat(int(pending.directory.Fd()), temporaryName, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), deploymentPendingName)
	if _, err = file.Write(append(content, '\n')); err == nil {
		err = file.Sync()
	}
	if err == nil {
		err = syscall.Renameat(int(pending.directory.Fd()), temporaryName, int(pending.directory.Fd()), deploymentPendingName)
	}
	if err == nil {
		err = pending.directory.Sync()
	}
	if err != nil {
		file.Close()
		_ = syscall.Unlinkat(int(pending.directory.Fd()), temporaryName)
		return err
	}
	pending.file.Close()
	pending.file, pending.record = file, next
	return nil
}

func (pending *pendingRecord) unchanged() error {
	original, err := pending.file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(filepath.Join(pending.directory.Name(), deploymentPendingName))
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) {
		return errors.New("deployment pending record changed during execution")
	}
	return nil
}

// clear removes the record once every computer's outcome is known. It never
// unlinks a replacement supplied by someone else.
func (pending *pendingRecord) clear() error {
	pending.mutex.Lock()
	defer pending.mutex.Unlock()
	if err := pending.unchanged(); err != nil {
		return err
	}
	if err := syscall.Unlinkat(int(pending.directory.Fd()), deploymentPendingName); err != nil {
		return err
	}
	pending.file.Close()
	return pending.directory.Sync()
}

// close keeps the record on disk for a later review, noting that this
// process no longer acts on any computer.
func (pending *pendingRecord) close() error {
	err := pending.rewrite(func(record *domain.PendingDeployment) error {
		record.ApplyReturned = true
		return nil
	})
	pending.mutex.Lock()
	defer pending.mutex.Unlock()
	pending.file.Close()
	return err
}

// ownDeploymentApplies counts the client applies of this process that hold
// a pending record. Recovery review uses it to tell a returned apply of this
// process from one that is still running.
var ownDeploymentApplies atomic.Int32

// OwnDeploymentApplyRunning reports whether this process is applying a
// client update right now.
func (Local) OwnDeploymentApplyRunning() bool {
	return ownDeploymentApplies.Load() > 0
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
	return domain.Blocked("DEPLOY-PENDING", errors.New(domain.MessageDeploymentPending+"; inspect deployment-pending.json and follow the deployment recovery guide"))
}

func runDeploymentPhase(ctx context.Context, plan domain.DeploymentPlanReport, phase domain.DeploymentPhase, output io.Writer, coordinationDirectory string, managed bool, timeout time.Duration) error {
	phaseContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch phase {
	case domain.DeploymentPhaseBuild:
		if err := buildClientSystems(phaseContext, plan, output); err != nil {
			if phaseContext.Err() != nil {
				return fmt.Errorf("build: %w", phaseContext.Err())
			}
			return fmt.Errorf("build: %w", err)
		}
		return nil
	case domain.DeploymentPhaseApply:
		directory, err := openCoordinationDirectory(coordinationDirectory, managed)
		if err != nil {
			return err
		}
		defer directory.Close()
		// Count before the record exists and until this function returns,
		// so a recovery review in this process never sees it unowned.
		ownDeploymentApplies.Add(1)
		defer ownDeploymentApplies.Add(-1)
		return applyClientSystems(phaseContext, plan, output, directory, managed)
	default:
		return fmt.Errorf("unsupported deployment phase %q", phase)
	}
}
