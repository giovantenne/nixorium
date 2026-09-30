package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type managedJobUnit struct {
	name, state string
	started     time.Time
}

// ObserveManagedJobs deliberately uses only local systemd and bounded progress
// files: reopening the dashboard must not evaluate a deployment or resume it.
func (local Local) ObserveManagedJobs(ctx context.Context) ([]domain.ManagedJob, error) {
	units, err := readManagedJobUnits(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]domain.ManagedJob, 0, 2)
	for _, operation := range []string{"controller-apply", "pxe-prepare"} {
		data, readErr := local.ReadOperationProgress(operation)
		var progress domain.OperationProgress
		if readErr == nil {
			progress, readErr = domain.DecodeOperationProgress(data)
			if readErr == nil && progress.Operation != operation {
				readErr = errors.New("progress operation mismatch")
			}
		}
		jobs = append(jobs, reconcileManagedJob(operation, units, progress, readErr))
	}
	return jobs, nil
}

func managedUnitOperation(name string) string {
	switch {
	case name == "nixorium-prepare-pxe.service":
		return "pxe-prepare"
	case name == "nixorium-apply-controller.service", controllerApplyUnitPattern.MatchString(name):
		return "controller-apply"
	default:
		return ""
	}
}

func readManagedJobUnits(ctx context.Context) ([]managedJobUnit, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "systemctl", "show", "nixorium-prepare-pxe.service", "nixorium-apply-controller.service", "nixorium-apply-controller@*.service", "--property=Id,LoadState,ActiveState,ExecMainStartTimestamp", "--timestamp=us+utc", "--no-pager")
	configureCommandCancellation(command)
	command.Env = environmentWithValue(environmentWithValue(os.Environ(), "LC_ALL", "C"), "TZ", "UTC")
	output := &boundedCommandBuffer{limit: 1024 * 1024}
	command.Stdout = output
	// Do not pass arbitrary systemctl diagnostics into the terminal.
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("managed job unit state is unavailable: %w", err)
	}
	if output.truncated {
		return nil, errors.New("managed job unit response exceeds the safety limit")
	}
	return parseManagedJobUnits(output.buffer.String())
}

func parseManagedJobUnits(output string) ([]managedJobUnit, error) {
	units := []managedJobUnit{}
	fixed := map[string]bool{}
	for _, block := range strings.Split(strings.TrimSpace(output), "\n\n") {
		values := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				values[key] = value
			}
		}
		name := values["Id"]
		if managedUnitOperation(name) == "" {
			return nil, errors.New("unexpected managed job unit response")
		}
		fixed[name] = true
		state := values["ActiveState"]
		switch state {
		case "active", "activating", "deactivating", "reloading", "inactive", "failed":
		default:
			return nil, errors.New("unknown managed job unit state")
		}
		if values["LoadState"] != "loaded" && values["LoadState"] != "not-found" {
			return nil, errors.New("managed job unit could not be loaded")
		}
		unit := managedJobUnit{name: name, state: state}
		if timestamp := values["ExecMainStartTimestamp"]; timestamp != "" {
			var err error
			unit.started, err = time.Parse("Mon 2006-01-02 15:04:05 MST", timestamp)
			if err != nil {
				return nil, errors.New("invalid managed job start timestamp")
			}
		}
		units = append(units, unit)
	}
	if !fixed["nixorium-prepare-pxe.service"] || !fixed["nixorium-apply-controller.service"] {
		return nil, errors.New("incomplete managed job unit response")
	}
	return units, nil
}

func jobUnitRunning(unit managedJobUnit) bool {
	return unit.state != "inactive" && unit.state != "failed"
}

func reconcileManagedJob(operation string, units []managedJobUnit, progress domain.OperationProgress, progressErr error) domain.ManagedJob {
	job := domain.ManagedJob{Operation: operation, State: "idle", Unit: "nixorium-apply-controller*"}
	if operation == "pxe-prepare" {
		job.Unit = "nixorium-prepare-pxe.service"
	}
	var selected managedJobUnit
	active := 0
	for _, unit := range units {
		if managedUnitOperation(unit.name) != operation {
			continue
		}
		if jobUnitRunning(unit) {
			active++
			if !jobUnitRunning(selected) || selected.name == "" || unit.started.After(selected.started) {
				selected = unit
			}
		} else if active == 0 && unit.started.After(selected.started) {
			selected = unit
		}
	}
	if selected.name != "" {
		job.Unit = selected.name
	}
	if active > 0 {
		job.State = "running"
		job.Detail = "Managed work is still running; viewing it does not start another operation."
		// Request microsecond UTC timestamps to exclude an earlier invocation,
		// even when both started within the same second. A missing timestamp,
		// concurrent instances or an older progress record cannot identify this job.
		if active == 1 && progressErr == nil && !selected.started.IsZero() && !progress.StartedAt.Before(selected.started) {
			job.Progress = progress
		} else {
			job.Detail = "Managed work is running; matching progress is not available yet."
		}
		return job
	}
	if progressErr != nil {
		if !errors.Is(progressErr, os.ErrNotExist) {
			job.Detail = "The managed progress record is unavailable or invalid."
		}
		return job
	}
	job.Progress = progress
	job.State = progress.State
	if progress.State == "running" {
		job.State = "interrupted"
		job.Detail = "Progress was left running, but no managed unit is running. Inspect the journal before a fresh review."
	} else {
		job.Detail = "The managed job ended. This record is not verification of the current configuration; run a fresh review."
	}
	return job
}

// Recheck at the existing adapter mutation boundary, including CLI callers.
// This is not a replacement for the root job's coordination locks/rechecks.
func checkManagedJobConflict(ctx context.Context) error {
	units, err := readManagedJobUnits(ctx)
	if err != nil {
		return err
	}
	for _, unit := range units {
		if jobUnitRunning(unit) {
			return fmt.Errorf("%s is already running; view its progress and wait before starting conflicting work", unit.name)
		}
	}
	return nil
}
