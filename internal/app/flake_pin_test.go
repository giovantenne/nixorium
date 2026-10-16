package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

var errOutdatedPin = domain.Blocked("PIN-OUTDATED", errors.New("flake.lock does not match flake.nix (nixorium: flake.nix declares github:giovantenne/nixorium/v3.8.0 but flake.lock records github:giovantenne/nixorium/v3.3.1)"))

type outdatedPinDeploymentSource struct{ *fakeDeploymentSource }

func (outdatedPinDeploymentSource) FlakeLockConsistency(string) error { return errOutdatedPin }

type outdatedPinInspectorSource struct{ *fakeSource }

func (outdatedPinInspectorSource) FlakeLockConsistency(string) error { return errOutdatedPin }

func TestDeploymentPlanRefusesAnOutdatedPinBeforeBuilding(t *testing.T) {
	source := outdatedPinDeploymentSource{readyDeploymentSource()}
	report := NewDeploymentManager(source).Plan(context.Background(), "/deployment", "@lab")
	if report.State != "blocked" || len(report.Issues) != 1 || report.Issues[0].Next == nil || report.Issues[0].Next.Code != "PIN-OUTDATED" || !strings.Contains(report.Issues[0].Message, "v3.3.1") {
		t.Fatalf("plan = %+v", report)
	}
	execution := NewDeploymentManager(source).Execute(context.Background(), "/deployment", "@lab", "0123456789abcdef", "", nil)
	if !execution.HasErrors() || len(source.runPhases) != 0 {
		t.Fatalf("outdated pin reached Colmena: %+v phases=%v", execution, source.runPhases)
	}
	if issue, found := flakePinIssue(readyDeploymentSource(), "/deployment"); found {
		t.Fatalf("source without the check reported %+v", issue)
	}
}

func TestDoctorReportsAnOutdatedPinWithTheRepair(t *testing.T) {
	report, err := NewInspector(outdatedPinInspectorSource{readyFake()}).Doctor(context.Background(), ".", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, report.Findings, "PIN-OUTDATED", domain.LevelError)
	for _, finding := range report.Findings {
		if finding.ID == "PIN-OUTDATED" && (!strings.Contains(finding.Remediation, "Update Nixorium") || !strings.Contains(finding.Evidence, "v3.3.1")) {
			t.Fatalf("finding = %+v", finding)
		}
	}
	if report.State != "errors" {
		t.Fatalf("doctor state = %q", report.State)
	}
}

func TestUpdatePlanRepairsTheConfiguredReleaseOnlyWhenThePinIsOutdated(t *testing.T) {
	source := &fakeUpdateSource{
		revision: strings.Repeat("a", 40),
		snapshot: domain.UpdateInputSnapshot{CurrentRef: "v3.8.0", PinOutdated: "nixorium: flake.nix declares github:giovantenne/nixorium/v3.8.0 but flake.lock records github:giovantenne/nixorium/v3.3.1"},
		proposal: domain.UpdateProposal{FlakeContent: []byte("flake"), LockContent: []byte("repaired"), Diff: domain.GitDiff{Content: "+ ref"}},
		releases: []domain.UpdateReleaseRef{{Tag: "v3.8.0", ObjectID: strings.Repeat("1", 40)}},
	}
	check := NewUpdateManager(source).Check(context.Background(), ".")
	if check.PinOutdated == "" {
		t.Fatalf("check does not report the outdated pin: %+v", check)
	}
	report := NewUpdateManager(source).Plan(context.Background(), ".", "v3.8.0", false, false)
	if report.State != "ready" || source.prepared != 1 || source.lastTarget != "v3.8.0" {
		t.Fatalf("repair plan = %+v", report)
	}
	source.snapshot.PinOutdated = ""
	report = NewUpdateManager(source).Plan(context.Background(), ".", "v3.8.0", false, false)
	if report.State != "blocked" || source.prepared != 1 {
		t.Fatalf("unchanged plan = %+v", report)
	}
}
