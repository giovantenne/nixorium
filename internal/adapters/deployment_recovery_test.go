package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
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
