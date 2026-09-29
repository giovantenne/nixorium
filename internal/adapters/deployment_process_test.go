package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func deploymentProcessFixture(t *testing.T, script string) (domain.DeploymentPlanReport, string) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "colmena"), "#!/bin/sh\n"+script)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return domain.DeploymentPlanReport{Repository: t.TempDir(), Revision: strings.Repeat("a", 40), ColmenaSelector: "pc01", Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "192.0.2.1"}}}, directory
}

func TestDeploymentProcessPersistsIntentBeforeDispatchAndClearsOnlySuccess(t *testing.T) {
	for _, exit := range []string{"0", "7"} {
		t.Run(exit, func(t *testing.T) {
			plan, directory := deploymentProcessFixture(t, `test -s "$NIXORIUM_TEST_PENDING" || exit 99
printf 'activation output\n'
exit `+exit+"\n")
			path := filepath.Join(directory, deploymentPendingName)
			t.Setenv("NIXORIUM_TEST_PENDING", path)
			gate, err := acquireOperationGateAt(directory, false)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			err = runDeploymentPhase(context.Background(), plan, domain.DeploymentPhaseApply, &output, directory, false, time.Second*10)
			gate.Close()
			var uncertain *domain.DeploymentUnconfirmedError
			if exit == "0" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("resolved marker retained: %v", err)
				}
				gate, err = acquireOperationGateAt(directory, false)
				if err != nil {
					t.Fatal(err)
				}
				gate.Close()
				return
			}
			if !errors.As(err, &uncertain) {
				t.Fatalf("apply failure should remain uncertain: %v", err)
			}
			if !strings.Contains(output.String(), "activation output") {
				t.Fatal("lost streamed output")
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("unsafe pending intent: %v", err)
			}
			data, _ := os.ReadFile(path)
			var record struct {
				Revision string
				Targets  []domain.DeploymentTarget
			}
			if err := json.Unmarshal(data, &record); err != nil || record.Revision != plan.Revision || len(record.Targets) != 1 {
				t.Fatalf("lost reviewed identity: %s", data)
			}
			if gate, err := acquireOperationGateAt(directory, false); err == nil {
				gate.Close()
				t.Fatal("new operation bypassed pending intent")
			}
			if gate, err := acquireRecoveryGateAt(directory, false); err == nil {
				gate.Close()
				t.Fatal("USB recovery bypassed pending deployment")
			}
			if active, _ := operationActiveAt(directory, false); !active {
				t.Fatal("pending operation reported idle")
			}
		})
	}
}

func TestDeploymentProcessTimeoutBoundsLocalChildrenAndRetainsApplyEvidence(t *testing.T) {
	for _, phase := range []domain.DeploymentPhase{domain.DeploymentPhaseBuild, domain.DeploymentPhaseApply} {
		t.Run(string(phase), func(t *testing.T) {
			plan, directory := deploymentProcessFixture(t, "sleep 30 &\nwait\n")
			started := time.Now()
			err := runDeploymentPhase(context.Background(), plan, phase, io.Discard, directory, false, 100*time.Millisecond)
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 5*time.Second {
				t.Fatalf("unbounded phase: %v (%s)", err, time.Since(started))
			}
			_, statErr := os.Lstat(filepath.Join(directory, deploymentPendingName))
			if (statErr == nil) != (phase == domain.DeploymentPhaseApply) {
				t.Fatalf("wrong retained evidence after %s: %v", phase, statErr)
			}
		})
	}
}

func TestDeploymentProcessBoundsPipesInheritedAfterParentExit(t *testing.T) {
	plan, directory := deploymentProcessFixture(t, "sleep 30 &\nexit 0\n")
	started := time.Now()
	err := runDeploymentPhase(context.Background(), plan, domain.DeploymentPhaseApply, io.Discard, directory, false, 10*time.Second)
	var uncertain *domain.DeploymentUnconfirmedError
	if !errors.As(err, &uncertain) || time.Since(started) > 5*time.Second {
		t.Fatalf("inherited pipe wait not bounded: %v (%s)", err, time.Since(started))
	}
}

func TestDeploymentProcessCancelledBeforeStartDoesNotReserveRemoteWork(t *testing.T) {
	plan, directory := deploymentProcessFixture(t, "exit 0\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runDeploymentPhase(ctx, plan, domain.DeploymentPhaseApply, io.Discard, directory, false, time.Second)
	var uncertain *domain.DeploymentUnconfirmedError
	if !errors.Is(err, context.Canceled) || errors.As(err, &uncertain) {
		t.Fatalf("unstarted operation was not safely cancelled: %v", err)
	}
	if err := checkDeploymentPendingPath(directory); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentPendingGateRefusesMalformedSpecialAndSymlinkEvidence(t *testing.T) {
	for _, kind := range []string{"malformed", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			_, directory := deploymentProcessFixture(t, "exit 0\n")
			path := filepath.Join(directory, deploymentPendingName)
			var err error
			switch kind {
			case "malformed":
				err = os.WriteFile(path, []byte("{"), 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "symlink":
				err = os.Symlink(filepath.Join(directory, "missing"), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if gate, err := acquireOperationGateAt(directory, false); err == nil {
				gate.Close()
				t.Fatal("unsafe evidence bypassed gate")
			}
		})
	}
}
