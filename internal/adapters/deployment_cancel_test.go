package adapters

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAClientCommandStoppedBeforeItRunsSaysSo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, status, err := runClientSSH(ctx, "10.0.0.1", "true")
	if status != -1 || !errors.Is(err, errClientCommandNotStarted) {
		t.Fatalf("status = %d, err = %v", status, err)
	}
}

func TestAStoppedUpdateStartsNoFurtherActivation(t *testing.T) {
	// nix-copy-closure succeeds, so only the activation step can decide.
	bin := t.TempDir()
	copied := filepath.Join(bin, "copied")
	scripts := map[string]string{
		"nix-copy-closure": "#!/bin/sh\ntouch " + copied + "\n",
		"ssh":              "#!/bin/sh\nexit 0\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, err := ensureTestCoordinationDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	target := domain.DeploymentTarget{Name: "pc01", IP: "10.0.0.1"}
	plan := domain.DeploymentPlanReport{Repository: "/srv/lab", Revision: strings.Repeat("b", 40), Targets: []domain.DeploymentTarget{target}}
	record, err := createDeploymentPending(directory, plan, []domain.PendingActivation{{Name: "pc01", State: domain.ActivationPlanned, ActivationID: strings.Repeat("c", 32)}})
	if err != nil {
		t.Fatal(err)
	}
	defer record.close()

	// Every activation slot is taken by computers already activating; the
	// operator stops the update while pc01 waits for one.
	ctx, cancel := context.WithCancel(context.Background())
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	done := make(chan clientActivation)
	var log bytes.Buffer
	go func() {
		done <- applyClientSystem(ctx, target, target.IP, "/nix/store/"+strings.Repeat("a", 32)+"-system", strings.Repeat("c", 32), &lockedWriter{writer: &log}, record, make(chan struct{}, 1), slots)
	}()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(copied); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the copy did not run")
		}
	}
	// Let the copy return; pc01 then waits for an activation slot.
	time.Sleep(500 * time.Millisecond)
	cancel()
	outcome := <-done
	if outcome.started || outcome.detail != "stopped before activation" {
		t.Fatalf("outcome = %+v\n%s", outcome, log.String())
	}
	if state := record.activation("pc01").State; state != domain.ActivationPlanned {
		t.Fatalf("pending state = %q, want planned", state)
	}
}
