package adapters

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

type commandStartedWriter struct {
	once  sync.Once
	ready chan struct{}
}

func (w *commandStartedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return len(p), nil
}

func TestReadCommandCancellationKillsInheritedPipeOwners(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// The child inherits stdout. Cancelling only the shell would leave Run
	// waiting for that pipe, even though the requested read is already over.
	command := exec.CommandContext(ctx, "sh", "-c", "sleep 30 & echo ready; wait")
	configureCommandCancellation(command)
	command.WaitDelay = 10 * time.Second // A group kill must finish before this fallback.
	output := &commandStartedWriter{ready: make(chan struct{})}
	command.Stdout, command.Stderr = output, io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	select {
	case <-output.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not start")
	}
	start := time.Now()
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled command reported success")
		}
		if time.Since(start) >= command.WaitDelay {
			t.Fatal("child kept its output pipe until WaitDelay instead of being cancelled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled read is still waiting for an inherited pipe")
	}
}

func TestReadCommandBoundsPipesAfterParentExit(t *testing.T) {
	command := exec.CommandContext(t.Context(), "sh", "-c", "sleep 30 & exit 0")
	configureCommandCancellation(command)
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	select {
	case err := <-finished:
		if err != exec.ErrWaitDelay {
			t.Fatalf("inherited output was not bounded: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read waited indefinitely after its parent exited")
	}
}
