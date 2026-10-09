package app

import (
	"errors"
	"testing"
)

type unbackedDeploymentSource struct{ *fakeDeploymentSource }

func (unbackedDeploymentSource) RemoteBackupRequired(string) error {
	return errors.New("current keys have no verified remote backup")
}
func TestDeploymentDoesNotRequireBackup(t *testing.T) {
	source := unbackedDeploymentSource{readyDeploymentSource()}
	plan := NewDeploymentManager(source).Plan(t.Context(), "/lab", "@lab")
	if plan.HasErrors() {
		t.Fatalf("optional backup blocked deployment: %+v", plan)
	}
	if len(source.runPhases) != 0 {
		t.Fatal("unbacked deployment started")
	}
}

type unbackedPXESource struct{ *fakePXELifecycleSource }

func (unbackedPXESource) RemoteBackupRequired(string) error {
	return errors.New("current keys have no verified remote backup")
}
func TestPXEDoesNotRequireBackup(t *testing.T) {
	source := unbackedPXESource{readyPXESource()}
	plan := NewPXELifecycle(source).PlanStart(t.Context(), "/lab")
	if plan.State != "ready" {
		t.Fatalf("optional backup blocked PXE: %+v", plan)
	}
}

type unbackedRecoverySource struct{ fakeRecoverySource }

func (unbackedRecoverySource) BackupDue(string) (string, bool) {
	return "No current backup is recorded. Backup is optional.", true
}

func TestBackupReminderDoesNotBlockOperations(t *testing.T) {
	report := NewRecoveryInspector(unbackedRecoverySource{}).Observe(t.Context(), "/lab")
	condition, found := report.Condition("backup-due")
	if !found || condition.Blocks != "" {
		t.Fatalf("backup must be advisory: %+v", report)
	}
}
