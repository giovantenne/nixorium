package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestParseDeploymentActivity(t *testing.T) {
	revision := strings.Repeat("a", 40)
	idle := parseDeploymentActivity("/nix/store/x-system\n" + revision + "\n--- jobs\n--- activation\n0\n")
	if !idle.Reachable || idle.Activating || idle.Revision != revision {
		t.Fatalf("idle = %+v", idle)
	}
	for _, output := range []string{
		"/nix/store/x\n" + revision + "\n--- jobs\n42 nixos-activation.service start running\n--- activation\n0\n",
		"/nix/store/x\n" + revision + "\n--- jobs\n--- activation\n1\n",
		"/nix/store/x\n" + revision + "\n--- jobs\n--- activation\n",
	} {
		if !parseDeploymentActivity(output).Activating {
			t.Fatalf("activity not detected in %q", output)
		}
	}
	if parseDeploymentActivity("--- jobs\nNo jobs running.\n--- activation\n0\n").Activating {
		t.Fatal("the empty job message was treated as activity")
	}
}

func TestArchivePendingDeploymentMovesOnlyTheReviewedRecord(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: strings.Repeat("b", 40), Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}}, StartedAt: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC), ControllerPID: 99}
	data, _ := json.Marshal(record)
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), data, 0600); err != nil {
		t.Fatal(err)
	}
	read, present, err := pendingDeploymentAt(directory)
	if !present || err != nil {
		t.Fatalf("read = %+v %v %v", read, present, err)
	}
	changed := read
	changed.Revision = strings.Repeat("c", 40)
	if _, err := archivePendingDeploymentAt(directory, false, changed); err == nil {
		t.Fatal("a different record was archived")
	}
	archive, err := archivePendingDeploymentAt(directory, false, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, deploymentPendingName)); !os.IsNotExist(err) {
		t.Fatal("pending record still blocks operations")
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	if _, present, _ := pendingDeploymentAt(directory); present {
		t.Fatal("record still present")
	}
}

func TestPendingDeploymentRejectsUnsafeRecords(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), []byte(`{"schemaVersion":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, present, err := pendingDeploymentAt(directory); !present || err == nil {
		t.Fatal("unsafe record accepted")
	}
}

func writeFakeProcess(t *testing.T, root string, pid int, command, start string) {
	t.Helper()
	directory := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	// Field 22 is the start time; the command name may contain spaces.
	stat := strconv.Itoa(pid) + " (nixorium x) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 " + start + " 0 0\n"
	if err := os.WriteFile(filepath.Join(directory, "stat"), []byte(stat), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "cmdline"), []byte(command), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestProcessRunningBindsTheRecorderToItsStartTime(t *testing.T) {
	root := t.TempDir()
	writeFakeProcess(t, root, 42, "/usr/bin/git\x00-C\x00/home/admin/nixorium", "9001")
	if !processRunningAt(root, 42, "9001") {
		t.Fatal("matching start time not recognised")
	}
	if processRunningAt(root, 42, "9002") {
		t.Fatal("a process with another start time was taken for the recorder")
	}
	if processRunningAt(root, 43, "9001") {
		t.Fatal("a missing process was reported running")
	}
	// Records without a start time keep the command-line check.
	if !processRunningAt(root, 42, "") {
		t.Fatal("old record: a Nixorium command line was not recognised")
	}
	writeFakeProcess(t, root, 44, "/usr/bin/vim\x00notes.txt", "1")
	if processRunningAt(root, 44, "") {
		t.Fatal("old record: an unrelated process was reported running")
	}
}

func TestPendingDeploymentStartTimeIsOptionalWhenReading(t *testing.T) {
	directory, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := `{"schemaVersion":1,"repository":"/srv/lab","revision":"` + strings.Repeat("b", 40) + `","targets":[{"name":"pc01","ip":"10.0.0.1"}],"startedAt":"2026-10-01T10:00:00Z","controllerPid":99}`
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	read, present, err := pendingDeploymentAt(directory)
	if !present || err != nil || read.ControllerPID != 99 || read.ControllerPIDStart != "" {
		t.Fatalf("old record rejected: %+v %v %v", read, present, err)
	}
}

func TestCreateDeploymentPendingRecordsTheProcessStartTime(t *testing.T) {
	root, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	plan := domain.DeploymentPlanReport{Repository: "/srv/lab", Revision: strings.Repeat("b", 40), ColmenaSelector: "@clients", Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}}}
	file, err := createDeploymentPending(directory, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	read, present, err := pendingDeploymentAt(root)
	if !present || err != nil {
		t.Fatalf("read = %+v %v %v", read, present, err)
	}
	want, err := processStartTime(os.Getpid())
	if err != nil {
		t.Skip("process start time is unavailable here")
	}
	if read.ControllerPID != os.Getpid() || read.ControllerPIDStart != want {
		t.Fatalf("pending identity = %d/%q, want %d/%q", read.ControllerPID, read.ControllerPIDStart, os.Getpid(), want)
	}
	if !(Local{}).ProcessRunning(read.ControllerPID, read.ControllerPIDStart) {
		t.Fatal("the running recorder was not recognised")
	}
}
