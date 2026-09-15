package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

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
	current     map[string]domain.HostSystemProbe
	recorded    map[string]domain.LastSuccessfulDeployment
	recordErr   error
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
	source.current = map[string]domain.HostSystemProbe{}
	for _, host := range source.meta.Clients.Hosts {
		source.current[host.Name] = domain.HostSystemProbe{
			SystemPath: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-" + host.Name + "-system",
			Revision:   source.revision,
		}
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

func (f *fakeDeploymentSource) CurrentSystems(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.HostSystemProbe {
	result := map[string]domain.HostSystemProbe{}
	for _, host := range hosts {
		if probe, found := f.current[host.Name]; found {
			result[host.Name] = probe
		}
	}
	return result
}

func (f *fakeDeploymentSource) RecordSuccessfulDeployments(_ string, records map[string]domain.LastSuccessfulDeployment) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.recorded = records
	return nil
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
	var observed []domain.DeploymentProgress
	report := NewDeploymentManager(source).ExecuteWithProgress(context.Background(), "/deployment", "pc03,pc01", source.revision, "/state/deploy.log", progress, func(update domain.DeploymentProgress) {
		observed = append(observed, update)
	})
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
	if report.Verification.Attempted != 2 || report.Verification.Verified != 2 || report.Verification.Recorded != 2 || len(source.recorded) != 2 {
		t.Fatalf("verification = %+v, recorded = %+v", report.Verification, source.recorded)
	}
	if len(observed) < 6 || observed[0].Phase != domain.DeploymentPhaseBuild || observed[len(observed)-1].Phase != domain.DeploymentPhaseComplete || observed[len(observed)-1].Completed != 4 {
		t.Fatalf("typed progress = %+v", observed)
	}
	for _, update := range observed {
		if strings.Contains(update.Activity, "colmena build output") || strings.Contains(update.Activity, "colmena apply output") {
			t.Fatalf("raw command output crossed typed progress boundary: %+v", update)
		}
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
		{phase: domain.DeploymentPhaseApply, buildCompleted: true, message: "authenticated reconciliation"},
	} {
		source := readyDeploymentSource()
		source.runErrors = map[domain.DeploymentPhase]error{test.phase: errors.New("fixture failure")}
		report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "@lab", source.revision, "", io.Discard)
		if report.State != "failed" || report.Phase != test.phase || report.BuildCompleted != test.buildCompleted || report.ApplyCompleted || !report.RetrySafe || !strings.Contains(report.Message, test.message) {
			t.Errorf("%s failure report = %+v", test.phase, report)
		}
	}
}

func TestDeploymentExecuteRecordsOnlyAuthenticatedMatchingTargetsAfterPartialApply(t *testing.T) {
	source := readyDeploymentSource()
	source.runErrors = map[domain.DeploymentPhase]error{domain.DeploymentPhaseApply: errors.New("partial fixture failure")}
	source.current["pc02"] = domain.HostSystemProbe{
		SystemPath: "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-pc02-system",
		Revision:   "fedcba9876543210",
	}
	delete(source.current, "pc03")
	manager := NewDeploymentManager(source)
	manager.now = func() time.Time { return time.Unix(123, 0) }
	report := manager.Execute(context.Background(), "/deployment", "@lab", source.revision, "", io.Discard)
	if report.State != "failed" || report.Phase != domain.DeploymentPhaseApply || report.ApplyCompleted {
		t.Fatalf("partial apply report = %+v", report)
	}
	if report.Verification.Attempted != 3 || report.Verification.Verified != 1 || report.Verification.Recorded != 1 {
		t.Fatalf("verification = %+v", report.Verification)
	}
	if len(source.recorded) != 1 || source.recorded["pc01"].Revision != source.revision || !source.recorded["pc01"].VerifiedAt.Equal(time.Unix(123, 0)) {
		t.Fatalf("recorded = %+v", source.recorded)
	}
}

func TestDeploymentExecuteReportsIncompleteVerificationAfterSuccessfulApply(t *testing.T) {
	source := readyDeploymentSource()
	delete(source.current, "pc02")
	report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "pc01,pc02", source.revision, "", io.Discard)
	if report.State != "partial" || report.Phase != domain.DeploymentPhaseVerify || !report.ApplyCompleted || !report.HasErrors() {
		t.Fatalf("incomplete verification report = %+v", report)
	}
	if report.Verification.Verified != 1 || report.Verification.Recorded != 1 || len(source.recorded) != 1 {
		t.Fatalf("verification = %+v, recorded = %+v", report.Verification, source.recorded)
	}
}

func TestDeploymentExecuteReportsHistoryWriteFailureAfterSuccessfulApply(t *testing.T) {
	source := readyDeploymentSource()
	source.recordErr = errors.New("read-only state")
	report := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "pc01", source.revision, "", io.Discard)
	if report.State != "failed" || report.Phase != domain.DeploymentPhaseVerify || !report.ApplyCompleted || report.Verification.Verified != 1 || report.Verification.Recorded != 0 || !strings.Contains(report.Message, "recording verification failed") {
		t.Fatalf("history failure report = %+v", report)
	}
}
