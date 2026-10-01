package main

import (
	"bytes"
	"os"
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

func TestRecordedWriterKeepsTheTerminalForInteractiveChecks(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := recordWriter(file, &nextStepRecorder{})
	if recorded, ok := writer.(recordedWriter); !ok || recorded.file != file {
		t.Fatal("recorded writer lost the original file")
	}
	if interactiveWriter(writer) {
		t.Fatal("a regular file was treated as a terminal")
	}
}
