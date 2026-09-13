package app

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeDeploymentSource struct {
	meta        domain.LabMeta
	deployment  domain.DeploymentStatus
	git         domain.GitState
	revision    string
	revisionErr error
}

func readyDeploymentSource() *fakeDeploymentSource {
	source := &fakeDeploymentSource{
		deployment: domain.DeploymentStatus{Ready: true},
		git:        domain.GitState{Available: true},
		revision:   "0123456789abcdef",
	}
	source.meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc02", IP: "10.0.0.2"},
		{Name: "pc03", IP: "10.0.0.3"},
	}
	return source
}

func (f *fakeDeploymentSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}

func (f *fakeDeploymentSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	return f.deployment, nil
}

func (f *fakeDeploymentSource) GitState(context.Context, string) (domain.GitState, error) {
	return f.git, nil
}

func (f *fakeDeploymentSource) GitRevision(context.Context, string) (string, error) {
	return f.revision, f.revisionErr
}

func TestDeploymentPlanSelectsOneSeveralOrAllClients(t *testing.T) {
	manager := NewDeploymentManager(readyDeploymentSource())
	for _, test := range []struct {
		requested string
		selector  string
		targets   int
	}{
		{requested: "pc02", selector: "pc02", targets: 1},
		{requested: "pc03, pc01", selector: "pc03,pc01", targets: 2},
		{requested: "@lab", selector: "@lab", targets: 3},
	} {
		report := manager.Plan(context.Background(), "/deployment", test.requested)
		if report.HasErrors() || report.State != "ready" || report.ColmenaSelector != test.selector || len(report.Targets) != test.targets || !report.BuildFirst {
			t.Errorf("plan %q = %+v", test.requested, report)
		}
	}
}

func TestDeploymentPlanRejectsUnknownDuplicateAndEmptyTargets(t *testing.T) {
	manager := NewDeploymentManager(readyDeploymentSource())
	for _, requested := range []string{"pc99", "pc01,pc01", ""} {
		report := manager.Plan(context.Background(), "/deployment", requested)
		if !report.HasErrors() || report.State != "blocked" {
			t.Errorf("invalid plan %q = %+v", requested, report)
		}
	}
}

func TestDeploymentPlanFailsClosedOnReadinessGitAndRevision(t *testing.T) {
	source := readyDeploymentSource()
	source.deployment = domain.DeploymentStatus{Ready: false, Issues: []string{"cache key missing"}}
	source.git = domain.GitState{Available: true, Dirty: true, Changes: 2}
	source.revisionErr = errors.New("no HEAD")

	report := NewDeploymentManager(source).Plan(context.Background(), "/deployment", "pc01")
	if len(report.Issues) != 3 || report.Revision != "" || report.State != "blocked" {
		t.Fatalf("blocked plan = %+v, want readiness, Git, and revision issues", report)
	}
}
