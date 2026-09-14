package app

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeControllerSource struct {
	meta       domain.LabMeta
	deployment domain.DeploymentStatus
	git        domain.GitState
	revision   string
	current    bool
	detail     string
	stateErr   error
	startErr   error
	unit       string
}

func readyControllerSource() *fakeControllerSource {
	source := &fakeControllerSource{
		deployment: domain.DeploymentStatus{Ready: true},
		git:        domain.GitState{Available: true},
		revision:   "0123456789abcdef0123456789abcdef01234567",
		detail:     "reviewed controller configuration is not active",
	}
	source.meta.Controller.Name = "pc99"
	return source
}

func (f *fakeControllerSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}
func (f *fakeControllerSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	return f.deployment, nil
}
func (f *fakeControllerSource) GitState(context.Context, string) (domain.GitState, error) {
	return f.git, nil
}
func (f *fakeControllerSource) GitRevision(context.Context, string) (string, error) {
	return f.revision, nil
}
func (f *fakeControllerSource) ControllerState(context.Context, string) (bool, string, error) {
	return f.current, f.detail, f.stateErr
}

func TestControllerPlanBlocksUnavailableDesiredState(t *testing.T) {
	source := readyControllerSource()
	source.stateErr = errors.New("cannot evaluate desired controller generation")
	report := NewControllerManager(source).Plan(context.Background(), "/deployment")
	if !report.HasErrors() || len(report.Issues) != 1 || report.Issues[0].Field != "controller" {
		t.Fatalf("plan = %+v", report)
	}
}
func (f *fakeControllerSource) StartSystemUnit(_ context.Context, unit string) error {
	f.unit = unit
	if f.startErr == nil {
		f.current = true
		f.detail = "active system matches the reviewed controller configuration"
	}
	return f.startErr
}

func TestControllerPlanBindsCleanReadyRevision(t *testing.T) {
	source := readyControllerSource()
	report := NewControllerManager(source).Plan(context.Background(), "/deployment")
	if report.HasErrors() || report.State != "ready" || report.Controller != "pc99" || report.Revision != source.revision || report.Confirmation != "REBUILD pc99" {
		t.Fatalf("plan = %+v", report)
	}
	source.current = true
	if report := NewControllerManager(source).Plan(context.Background(), "/deployment"); report.State != "current" || report.HasErrors() {
		t.Fatalf("current plan = %+v", report)
	}
}

func TestControllerPlanBlocksDirtyOrUnreadyDeployment(t *testing.T) {
	source := readyControllerSource()
	source.git = domain.GitState{Available: true, Dirty: true, Changes: 2}
	source.deployment = domain.DeploymentStatus{Issues: []string{"placeholder remains"}}
	report := NewControllerManager(source).Plan(context.Background(), "/deployment")
	if !report.HasErrors() || len(report.Issues) != 2 {
		t.Fatalf("plan = %+v", report)
	}
}

func TestControllerApplyUsesRevisionInstanceAndVerifiesActiveSystem(t *testing.T) {
	source := readyControllerSource()
	report := NewControllerManager(source).Apply(context.Background(), "/deployment", source.revision)
	if report.HasErrors() || !report.Applied || !report.Verified || report.Phase != domain.ControllerRebuildPhaseComplete || source.unit != ControllerApplyUnit(source.revision) {
		t.Fatalf("report = %+v, unit = %q", report, source.unit)
	}
}

func TestControllerApplyRejectsStaleReviewAndReportsUnitFailure(t *testing.T) {
	source := readyControllerSource()
	manager := NewControllerManager(source)
	if report := manager.Apply(context.Background(), "/deployment", "fedcba9876543210fedcba9876543210fedcba98"); !report.HasErrors() || source.unit != "" || report.Phase != domain.ControllerRebuildPhasePreflight {
		t.Fatalf("stale report = %+v, unit = %q", report, source.unit)
	}
	source.startErr = errors.New("activation failed")
	if report := manager.Apply(context.Background(), "/deployment", source.revision); !report.HasErrors() || report.Phase != domain.ControllerRebuildPhaseApply || report.Applied {
		t.Fatalf("failure report = %+v", report)
	}
}
