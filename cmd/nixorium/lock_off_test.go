package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Without the controller's classroom-view.json (this test machine has none)
// the view counts as off, and locking answers with the one plain sentence.
func TestLockCommandAnswersPlainlyWhenTheViewIsOff(t *testing.T) {
	if adapters.ClassroomViewApplied() {
		t.Skip("this machine has the classroom view on")
	}
	var stdout, stderr bytes.Buffer
	code := runLockCommand(context.Background(), t.TempDir(), options{subcommand: "plan", on: "pc01", lockAction: domain.LockOn}, &stdout, &stderr)
	if code != 1 || strings.TrimSpace(stderr.String()) != domain.ClassroomViewOffMessage || stdout.Len() != 0 {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
}
