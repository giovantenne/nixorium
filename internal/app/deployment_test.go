package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeDeploymentSource struct {
	meta        domain.LabMeta
	deployment  domain.DeploymentStatus
	git         domain.GitState
	revision    string
	revisionErr error
	runErrors   map[domain.DeploymentPhase]error
	runPhases   []domain.DeploymentPhase
	afterBuild  func()
	addresses   []string
	addressErr  error
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
	source.meta.Controller.StaticIP = "10.0.0.99"
	source.meta.Network.Interface = "lab0"
	source.addresses = []string{"10.0.0.99"}
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

func (f *fakeDeploymentSource) InterfaceAddresses(string) ([]string, error) {
	return f.addresses, f.addressErr
}

func (f *fakeDeploymentSource) RunDeploymentPhase(_ context.Context, _ string, phase domain.DeploymentPhase, selector string, output io.Writer) error {
	f.runPhases = append(f.runPhases, phase)
	if selector == "" {
		return errors.New("empty selector")
	}
	fmtOutput := "colmena " + string(phase) + " output\n"
	_, _ = io.WriteString(output, fmtOutput)
	if phase == domain.DeploymentPhaseBuild && f.afterBuild != nil {
		f.afterBuild()
	}
	return f.runErrors[phase]
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

func TestDeploymentPlanRequiresControllerDeploymentAddress(t *testing.T) {
	source := readyDeploymentSource()
	source.addresses = []string{"192.0.2.10"}
	report := NewDeploymentManager(source).Plan(context.Background(), "/deployment", "pc01")
	if !report.HasErrors() || len(report.Issues) != 1 || report.Issues[0].Field != "network" || !strings.Contains(report.Issues[0].Message, "recover PXE") {
		t.Fatalf("network-blocked plan = %+v", report)
	}
}

func TestDeploymentExecuteBuildsBeforeApplyAndStreamsOutput(t *testing.T) {
	source := readyDeploymentSource()
	progress := &bytes.Buffer{}
	report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "pc03,pc01", source.revision, "/state/deploy.log", progress)
	if report.HasErrors() || report.State != "completed" || !report.BuildCompleted || !report.ApplyCompleted || report.Phase != domain.DeploymentPhaseComplete {
		t.Fatalf("execution report = %+v", report)
	}
	if len(source.runPhases) != 2 || source.runPhases[0] != domain.DeploymentPhaseBuild || source.runPhases[1] != domain.DeploymentPhaseApply {
		t.Fatalf("phases = %v, want build then apply", source.runPhases)
	}
	if !strings.Contains(progress.String(), "colmena build output") || !strings.Contains(progress.String(), "colmena apply output") {
		t.Fatalf("progress = %q", progress.String())
	}
	if report.LogPath != "/state/deploy.log" || !report.RetrySafe {
		t.Fatalf("durability/retry fields = %+v", report)
	}
}

func TestDeploymentExecuteRejectsStaleReviewBeforeRunning(t *testing.T) {
	source := readyDeploymentSource()
	report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "pc01", "old-revision", "", io.Discard)
	if report.State != "blocked" || len(source.runPhases) != 0 || len(report.Issues) != 1 {
		t.Fatalf("stale execution = %+v, phases = %v", report, source.runPhases)
	}
}

func TestDeploymentExecuteRevalidatesAfterBuild(t *testing.T) {
	source := readyDeploymentSource()
	source.afterBuild = func() { source.revision = "changed-after-build" }
	report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "pc01", source.revision, "", io.Discard)
	if report.State != "failed" || !report.BuildCompleted || report.ApplyCompleted || report.Phase != domain.DeploymentPhasePreflight || len(source.runPhases) != 1 {
		t.Fatalf("changed execution = %+v, phases = %v", report, source.runPhases)
	}
}

func TestDeploymentExecuteReportsBuildAndPartialApplyFailures(t *testing.T) {
	for _, test := range []struct {
		phase          domain.DeploymentPhase
		buildCompleted bool
		message        string
	}{
		{phase: domain.DeploymentPhaseBuild, buildCompleted: false, message: "before any deployment"},
		{phase: domain.DeploymentPhaseApply, buildCompleted: true, message: "some targets may already have changed"},
	} {
		source := readyDeploymentSource()
		source.runErrors = map[domain.DeploymentPhase]error{test.phase: errors.New("fixture failure")}
		report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "@lab", source.revision, "", io.Discard)
		if report.State != "failed" || report.Phase != test.phase || report.BuildCompleted != test.buildCompleted || report.ApplyCompleted || !report.RetrySafe || !strings.Contains(report.Message, test.message) {
			t.Errorf("%s failure report = %+v", test.phase, report)
		}
	}
}
