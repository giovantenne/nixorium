package app

import (
	"errors"
	"testing"
)

type unbackedDeploymentSource struct{ *fakeDeploymentSource }

func (unbackedDeploymentSource) RemoteBackupRequired(string) error {
	return errors.New("current keys have no verified remote backup")
}
func TestDeploymentRequiresRemoteKeyBackupBeforeEvaluation(t *testing.T) {
	source := unbackedDeploymentSource{&fakeDeploymentSource{}}
	plan := NewDeploymentManager(source).Plan(t.Context(), "/lab", "@lab")
	if !plan.HasErrors() || len(plan.Issues) != 1 || plan.Issues[0].Field != "backup" {
		t.Fatalf("missing backup not reported: %+v", plan)
	}
	if len(source.runPhases) != 0 {
		t.Fatal("unbacked deployment started")
	}
}

type unbackedPXESource struct{ *fakePXELifecycleSource }

func (unbackedPXESource) RemoteBackupRequired(string) error {
	return errors.New("current keys have no verified remote backup")
}
func TestPXERequiresRemoteKeyBackup(t *testing.T) {
	source := unbackedPXESource{&fakePXELifecycleSource{}}
	plan := NewPXELifecycle(source).PlanStart(t.Context(), "/lab")
	if plan.State == "ready" {
		t.Fatal("PXE allowed unbacked keys")
	}
}
