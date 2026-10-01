package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestFailedCommandEndsWithTheNextStep(t *testing.T) {
	recorder := &nextStepRecorder{}
	_, _ = recorder.Write([]byte("Error: deployment worktree has 2 changed path(s)\nError: another Nixorium controller or client operation is already running\nagain: deployment worktree has 2 changed path(s)"))
	var output bytes.Buffer
	recorder.report(&output)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "Next (GIT-DIRTY)") || !strings.HasPrefix(lines[1], "Next (OP-BUSY)") {
		t.Fatalf("report = %q", output.String())
	}
}
