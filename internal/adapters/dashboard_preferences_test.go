package adapters

import (
	"github.com/giovantenne/nixorium/internal/domain"
	"os"
	"testing"
)

func TestDashboardPreferencesPersistSeparatelyFromDeployment(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := t.TempDir()
	local := Local{}
	accepted, err := local.DisclaimerAccepted(repo)
	if err != nil || accepted {
		t.Fatal(accepted, err)
	}
	if err := local.AcceptDisclaimer(repo); err != nil {
		t.Fatal(err)
	}
	accepted, err = local.DisclaimerAccepted(repo)
	if err != nil || !accepted {
		t.Fatal(accepted, err)
	}
	state := domain.UpdateNotificationState{Dismissed: "release"}
	if err := local.SaveUpdateNotification(repo, state); err != nil {
		t.Fatal(err)
	}
	read, err := local.LoadUpdateNotification(repo)
	if err != nil || read.Dismissed != "release" {
		t.Fatal(read, err)
	}
	accepted, _ = local.DisclaimerAccepted(t.TempDir())
	if accepted {
		t.Fatal("acknowledgement leaked to another lab")
	}
	entries, _ := os.ReadDir(repo)
	if len(entries) != 0 {
		t.Fatal("preferences changed deployment")
	}
	path, _ := dashboardPreferencePath(repo, "disclaimer")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir() + "/target"
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := local.DisclaimerAccepted(repo); err == nil {
		t.Fatal("followed symlink")
	}
	if err := local.AcceptDisclaimer(repo); err == nil {
		t.Fatal("overwrote symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("changed symlink target")
	}
}
