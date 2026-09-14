package presentation

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestHostsTextIncludesTypedComputerState(t *testing.T) {
	report := domain.HostsReport{
		State:           "partial",
		DesiredRevision: "0123456789abcdef",
		Deployment:      domain.HostDeploymentSummary{Current: 1, Unknown: 1},
		HistoryDetail:   "one old history file was ignored",
		Hosts: []domain.HostStatus{
			{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Deployment: domain.DeploymentCurrent, CurrentSystem: "/nix/store/current", CurrentRevision: "0123456789abcdef", LastSuccessfulDeploy: &domain.LastSuccessfulDeployment{Revision: "0123456789abcdef", VerifiedAt: time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC)}},
			{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown, Deployment: domain.DeploymentUnknown, DeploymentDetail: "current deployment revision is unavailable"},
		},
	}
	var output bytes.Buffer
	HostsText(&output, report)
	for _, expected := range []string{
		"Nixorium computers: PARTIAL",
		"SSH available:      1/2",
		"Deployment:         1 current, 0 outdated, 1 unknown",
		"Desired revision:   0123456789abcdef",
		"History warning:    one old history file was ignored",
		"pc01       10.0.0.1        network=reachable",
		"pc02       10.0.0.2        network=unknown",
		"current: /nix/store/current",
		"current revision: 0123456789abcdef",
		"last verified: 0123456789abcdef at 2026-09-14T10:30:00Z",
		"detail:  current deployment revision is unavailable",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("status output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestDeploymentPlanTextShowsTargetsAndBlockers(t *testing.T) {
	report := domain.DeploymentPlanReport{
		State:           "blocked",
		Repository:      "/deployment",
		Revision:        "abc123",
		ColmenaSelector: "pc01",
		Targets:         []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}},
		Issues:          []domain.ValidationIssue{{Field: "git", Message: "worktree is dirty"}},
	}
	var output bytes.Buffer
	DeploymentPlanText(&output, report)
	for _, expected := range []string{"Deployment plan: BLOCKED", "Targets:         pc01", "pc01       10.0.0.1", "BLOCKED: git: worktree is dirty"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("deployment plan output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestReadyDeploymentPlanShowsRevisionBoundNextCommand(t *testing.T) {
	report := domain.DeploymentPlanReport{
		State:           "ready",
		Revision:        "0123456789abcdef",
		ColmenaSelector: "@lab",
		Targets:         []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}},
	}
	output := &bytes.Buffer{}
	DeploymentPlanText(output, report)
	if !strings.Contains(output.String(), "nixorium deploy apply --on @lab --expect 0123456789abcdef") {
		t.Fatalf("ready plan does not show exact next command:\n%s", output.String())
	}
}

func TestDeploymentExecutionTextShowsFailureAndRecovery(t *testing.T) {
	report := domain.DeploymentExecutionReport{
		State:           "failed",
		Phase:           domain.DeploymentPhaseApply,
		Revision:        "0123456789abcdef",
		ColmenaSelector: "pc01,pc02",
		BuildCompleted:  true,
		RetrySafe:       true,
		LogPath:         "/state/deploy.log",
		Message:         "apply failed; some targets may already have changed",
		Verification: domain.DeploymentVerificationSummary{
			Attempted: 2,
			Verified:  1,
			Recorded:  1,
			Targets: []domain.DeploymentTargetVerification{
				{Name: "pc01", State: "verified"},
				{Name: "pc02", State: "unverified", Detail: "authentication failed"},
			},
		},
	}
	output := &bytes.Buffer{}
	DeploymentExecutionText(output, report)
	for _, expected := range []string{"FAILED", "apply", "pc01,pc02", "Verified targets: 1/2 (recorded 1)", "pc02       unverified: authentication failed", "/state/deploy.log", "some targets may already have changed", "Retry:"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("deployment output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestControllerRebuildTextShowsReviewAndVerifiedResult(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	planOutput := &bytes.Buffer{}
	ControllerRebuildPlanText(planOutput, domain.ControllerRebuildPlanReport{
		State: "ready", Repository: "/deployment", Controller: "pc99", Revision: revision,
		CurrentDetail: "not active", Confirmation: "REBUILD pc99", Issues: []domain.ValidationIssue{},
	})
	for _, expected := range []string{"READY", "pc99", revision, "controller apply --expect"} {
		if !strings.Contains(planOutput.String(), expected) {
			t.Fatalf("controller plan omits %q:\n%s", expected, planOutput.String())
		}
	}
	resultOutput := &bytes.Buffer{}
	ControllerRebuildExecutionText(resultOutput, domain.ControllerRebuildExecutionReport{
		State: "completed", Controller: "pc99", Revision: revision,
		Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true,
	})
	for _, expected := range []string{"COMPLETED", "complete", "Applied:    true", "Verified:   true"} {
		if !strings.Contains(resultOutput.String(), expected) {
			t.Fatalf("controller result omits %q:\n%s", expected, resultOutput.String())
		}
	}
}
