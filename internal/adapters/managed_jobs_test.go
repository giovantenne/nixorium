package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestManagedJobReconciliation(t *testing.T) {
	started := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	unit := "nixorium-apply-controller@" + strings.Repeat("a", 40) + ".service"
	progress := domain.OperationProgress{Operation: "controller-apply", State: "running", Phase: "build", StartedAt: started.Add(time.Second)}
	for _, test := range []struct {
		name, unitState, progressState, want string
		offset                               time.Duration
		missing, invalid                     bool
	}{
		{name: "attach activating revision", unitState: "activating", progressState: "running", want: "running"},
		{name: "attach active", unitState: "active", progressState: "running", want: "running"},
		{name: "stopping is still running", unitState: "deactivating", progressState: "running", want: "running"},
		{name: "dead job", unitState: "inactive", progressState: "running", want: "interrupted"},
		{name: "failed unit stale progress", unitState: "failed", progressState: "running", want: "interrupted"},
		{name: "completed", unitState: "inactive", progressState: "completed", want: "completed"},
		{name: "failed", unitState: "failed", progressState: "failed", want: "failed"},
		{name: "old progress during new job", unitState: "activating", progressState: "running", want: "running", offset: -time.Hour},
		{name: "missing progress still conflicts", unitState: "activating", want: "running", missing: true},
		{name: "invalid progress still conflicts", unitState: "active", want: "running", invalid: true},
		{name: "no history", unitState: "inactive", want: "idle", missing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := progress
			p.State = test.progressState
			p.StartedAt = p.StartedAt.Add(test.offset)
			var err error
			if test.missing {
				err = os.ErrNotExist
			}
			if test.invalid {
				err = errors.New("invalid")
			}
			job := reconcileManagedJob("controller-apply", []managedJobUnit{{name: unit, state: test.unitState, started: started}}, p, err)
			if job.State != test.want || job.Unit != unit {
				t.Fatalf("job = %+v", job)
			}
			if job.BlocksStart() != (test.want == "running") {
				t.Fatalf("wrong conflict: %+v", job)
			}
			if test.want == "running" && (test.offset < 0 || err != nil) && job.Progress.Operation != "" {
				t.Fatal("accepted unrelated/unavailable progress")
			}
		})
	}
	job := reconcileManagedJob("pxe-prepare", []managedJobUnit{{name: "nixorium-prepare-pxe.service", state: "inactive"}}, domain.OperationProgress{State: "running"}, nil)
	if job.State != "interrupted" || job.Unit != "nixorium-prepare-pxe.service" {
		t.Fatalf("PXE = %+v", job)
	}
	job = reconcileManagedJob("controller-apply", []managedJobUnit{{name: unit, state: "activating", started: started}, {name: "nixorium-apply-controller.service", state: "activating", started: started}}, progress, nil)
	if job.Progress.Operation != "" || !job.BlocksStart() {
		t.Fatalf("ambiguous progress = %+v", job)
	}
	progress.StartedAt = started.Add(10 * time.Millisecond)
	job = reconcileManagedJob("controller-apply", []managedJobUnit{{name: unit, state: "activating", started: started.Add(20 * time.Millisecond)}}, progress, nil)
	if job.Progress.Operation != "" || !job.BlocksStart() {
		t.Fatal("accepted previous invocation from the same second")
	}
}

func TestManagedJobUnitParser(t *testing.T) {
	fixed := "Id=nixorium-prepare-pxe.service\nLoadState=loaded\nActiveState=inactive\nExecMainStartTimestamp=\n\nId=nixorium-apply-controller.service\nLoadState=not-found\nActiveState=inactive\nExecMainStartTimestamp=\n"
	instance := "\nId=nixorium-apply-controller@" + strings.Repeat("b", 40) + ".service\nLoadState=loaded\nActiveState=activating\nExecMainStartTimestamp=Wed 2026-09-30 10:00:00.123456 UTC\n"
	units, err := parseManagedJobUnits(fixed + instance)
	if err != nil || len(units) != 3 || units[2].started.Nanosecond() != 123456000 {
		t.Fatalf("%+v %v", units, err)
	}
	for _, output := range []string{"", instance, strings.Replace(fixed, "ActiveState=inactive", "ActiveState=unexpected", 1), fixed + strings.Replace(instance, strings.Repeat("b", 40), "unsafe", 1), fixed + strings.Replace(instance, "Wed 2026-09-30 10:00:00.123456 UTC", "bad", 1)} {
		if _, err := parseManagedJobUnits(output); err == nil {
			t.Fatalf("accepted invalid response: %s", output)
		}
	}
}

func TestManagedJobConflictDoesNotDispatchStart(t *testing.T) {
	directory := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" != show ]; then echo forbidden-start > \"$MARKER\"; exit 1; fi\nprintf '%s\\n' 'Id=nixorium-prepare-pxe.service' 'LoadState=loaded' 'ActiveState=activating' 'ExecMainStartTimestamp=' '' 'Id=nixorium-apply-controller.service' 'LoadState=loaded' 'ActiveState=inactive' 'ExecMainStartTimestamp='\n"
	if err := os.WriteFile(filepath.Join(directory, "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, "started")
	t.Setenv("PATH", directory)
	t.Setenv("MARKER", marker)
	for _, unit := range []string{"nixorium-prepare-pxe.service", "nixorium-apply-controller.service", "nixorium-apply-controller@" + strings.Repeat("a", 40) + ".service", "nixorium-pxe.service"} {
		err := (Local{}).ControlSystemUnit(context.Background(), "start", unit)
		if err == nil || !strings.Contains(err.Error(), "already running") {
			t.Fatalf("%s: %v", unit, err)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("conflicting start dispatched")
	}
	if err := os.WriteFile(filepath.Join(directory, "systemctl"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := (Local{}).StartSystemUnit(context.Background(), "nixorium-prepare-pxe.service"); err == nil {
		t.Fatal("unavailable unit state allowed a start")
	}
}

// Opt-in only: an isolated management VM, never the contributor's real lab.
func TestManagedJobsSystemdIntegration(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_MANAGED_JOBS") != "1" {
		t.Skip("management VM only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unit := "nixorium-apply-controller@" + strings.Repeat("b", 40) + ".service"
	path := filepath.Join("/run/systemd/system", unit)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("test unit already exists: %v", err)
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[Service]\nType=oneshot\nExecStart="+sleep+" 60\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = run(context.Background(), "systemctl", "stop", unit)
		_ = os.Remove(path)
		_, _ = run(context.Background(), "systemctl", "daemon-reload")
	})
	if _, err := run(ctx, "systemctl", "daemon-reload"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, "systemctl", "start", "--no-block", unit); err != nil {
		t.Fatal(err)
	}
	for {
		units, err := readManagedJobUnits(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ready := false
		for _, u := range units {
			if u.name == unit && u.state == "activating" && !u.started.IsZero() {
				ready = true
			}
		}
		if ready {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(20 * time.Millisecond)
	}
	progress := domain.OperationProgress{SchemaVersion: 1, Operation: "controller-apply", State: "running", Phase: "build", StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Recent: []string{"VM job in progress"}}
	data, _ := json.Marshal(progress)
	progressPath := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(progressPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	old := operationProgressPaths["controller-apply"]
	operationProgressPaths["controller-apply"] = progressPath
	t.Cleanup(func() { operationProgressPaths["controller-apply"] = old })
	jobs, err := (Local{}).ObserveManagedJobs(ctx)
	if err != nil || jobs[0].State != "running" || jobs[0].Unit != unit || jobs[0].Progress.Phase != "build" {
		t.Fatalf("attached: %+v %v", jobs, err)
	}
	if err := (Local{}).StartSystemUnit(ctx, unit); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("duplicate start: %v", err)
	}
	if _, err := run(ctx, "systemctl", "stop", unit); err != nil {
		t.Fatal(err)
	}
	jobs, err = (Local{}).ObserveManagedJobs(ctx)
	if err != nil || jobs[0].State != "interrupted" {
		t.Fatalf("interrupted: %+v %v", jobs, err)
	}
}
