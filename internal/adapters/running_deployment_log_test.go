package adapters

import (
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The log of the running update is the newest deployment log of the lock
// holder's process started after it took the lock; an older log of a process
// that had the same number, or another process's log, is never chosen.
func TestRunningDeploymentLogPicksTheHoldersCurrentLog(t *testing.T) {
	started := time.Date(2026, 11, 18, 12, 51, 18, 900000000, time.UTC)
	holder := domain.OperationHolder{Known: true, PID: 141912, StartedAt: started}
	names := []string{
		"deploy-20261018T090000.000000000Z-141912.log", // same number, a month earlier
		"deploy-20261118T125119.118559664Z-141912.log",
		"deploy-20261118T125200.000000000Z-999.log",
		"usb-install-3f4214076d3d1000d0655465a4d2e24d.log",
		"notes.txt",
	}
	if got := runningDeploymentLog(names, holder); got != "deploy-20261118T125119.118559664Z-141912.log" {
		t.Fatalf("log = %q", got)
	}
	if got := runningDeploymentLog(names[:1], holder); got != "" {
		t.Fatalf("an earlier process's log was chosen: %q", got)
	}
	if got := runningDeploymentLog(names, domain.OperationHolder{Known: true, PID: 4242, StartedAt: started}); got != "" {
		t.Fatalf("an operation without a log got %q", got)
	}
}
