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

func TestServicesTextShowsModesWorkflowsAndVerifiedAction(t *testing.T) {
	servicesOutput := &bytes.Buffer{}
	ServicesText(servicesOutput, domain.ServicesReport{
		State: "healthy",
		Services: []domain.ManagedService{
			{ID: "cache", Name: "Binary cache", Mode: "persistent", State: "healthy", Detail: "HTTP-ready", Units: []domain.ServiceState{{Name: "nixorium-harmonia.service", State: "active"}}},
			{ID: "pxe", Name: "PXE installation mode", Mode: "on-demand", State: "ready", Commands: []string{"nixorium pxe start"}, Units: []domain.ServiceState{{Name: "nixorium-pxe.service", State: "inactive"}}},
		},
	})
	for _, expected := range []string{"HEALTHY", "Binary cache (persistent): healthy", "nixorium-harmonia.service", "PXE installation mode (on-demand): ready", "workflow: nixorium pxe start"} {
		if !strings.Contains(servicesOutput.String(), expected) {
			t.Fatalf("services output omits %q:\n%s", expected, servicesOutput.String())
		}
	}
	actionOutput := &bytes.Buffer{}
	ServiceActionText(actionOutput, domain.ServiceActionReport{State: "completed", Action: "restart", Service: "cache", Unit: "nixorium-restart-cache.service", Verified: true, Current: domain.ServiceState{State: "active"}})
	for _, expected := range []string{"COMPLETED", "restart cache", "Verified:       true", "Current state:  active"} {
		if !strings.Contains(actionOutput.String(), expected) {
			t.Fatalf("service action output omits %q:\n%s", expected, actionOutput.String())
		}
	}
}

func TestOperationLogTextShowsBoundedListAndTail(t *testing.T) {
	id := "deploy-20260914T113000.000000000Z-11.log"
	entry := domain.OperationLogEntry{
		ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC),
		SizeBytes: 70000, State: "partial", Available: true,
	}
	listOutput := &bytes.Buffer{}
	OperationLogsText(listOutput, domain.OperationLogsReport{
		State:   "available",
		Records: []domain.OperationRecord{{RecordedAt: time.Date(2026, 9, 14, 11, 31, 0, 0, time.UTC), Operation: "pxe-start", State: "completed", Subject: "active", Summary: "PXE lifecycle transition finished"}},
		Logs:    []domain.OperationLogEntry{entry},
	})
	for _, expected := range []string{"AVAILABLE", "Recent actions", "pxe-start", "PXE lifecycle transition finished", "Deployment logs", "2026-09-14 11:30:00Z", "deployment", "partial", id} {
		if !strings.Contains(listOutput.String(), expected) {
			t.Fatalf("log list omits %q:\n%s", expected, listOutput.String())
		}
	}
	detailOutput := &bytes.Buffer{}
	OperationLogText(detailOutput, domain.OperationLogReport{State: "available", Log: &entry, Content: "Result: partial\n", Truncated: true})
	for _, expected := range []string{id, "tail only", "Result:        partial", "Result: partial"} {
		if !strings.Contains(detailOutput.String(), expected) {
			t.Fatalf("log detail omits %q:\n%s", expected, detailOutput.String())
		}
	}
}

func TestGitReviewTextShowsScopesOwnershipAndRedactedDiff(t *testing.T) {
	report := domain.GitReviewReport{
		State:      "changes",
		Repository: "/deployment",
		Revision:   "0123456789abcdef0123456789abcdef01234567",
		Summary:    domain.GitChangeSummary{Staged: 1, Unstaged: 1, Untracked: 1, Managed: 1, Unexpected: 2},
		Changes: []domain.GitChange{
			{Path: "lab-settings.json", Staged: "modified", Managed: true},
			{Path: "module.nix", Unstaged: "modified"},
			{Path: "notes", Untracked: true},
		},
		Diffs: []domain.GitDiff{{Scope: "staged", Content: "+  \"adminPassword\": \"<redacted>\"\n", Truncated: true}},
	}
	var output bytes.Buffer
	GitReviewText(&output, report)
	for _, expected := range []string{"Git review: CHANGES", "1 staged, 1 unstaged, 1 untracked", "managed", "unexpected", "lab-settings.json", "first 256 KiB", "<redacted>", "Untracked file contents are not opened"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("Git review output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestGitCommitTextShowsPlanAndLocalResult(t *testing.T) {
	plan := domain.GitCommitPlanReport{
		State:         "ready",
		Repository:    "/deployment",
		Revision:      strings.Repeat("a", 40),
		Paths:         []string{"lab-settings.json"},
		ReviewToken:   "sha256:review",
		CommitMessage: "chore: update laboratory settings",
		Confirmation:  "COMMIT abcdef012345",
		Diff:          domain.GitDiff{Content: "+password: <redacted>\n"},
	}
	var output bytes.Buffer
	GitCommitPlanText(&output, plan)
	for _, expected := range []string{"commit plan: READY", "lab-settings.json", "sha256:review", "COMMIT abcdef012345", "No remote or push", "<redacted>"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("commit plan output omits %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	GitCommitText(&output, domain.GitCommitReport{State: "completed", Paths: plan.Paths, PreviousRevision: plan.Revision, Revision: strings.Repeat("b", 40), CommitMessage: plan.CommitMessage, Committed: true, Message: "no remote push was attempted"})
	for _, expected := range []string{"Git commit: COMPLETED", "Committed:      true", "no remote push was attempted"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("commit result output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestUpdateTextShowsValidatedPlanAndReviewableResult(t *testing.T) {
	plan := domain.UpdatePlanReport{
		State: "ready", Repository: "/deployment", Revision: strings.Repeat("a", 40),
		CurrentRef: "v2.0.0", CurrentRev: strings.Repeat("b", 40), CurrentChannel: domain.UpdateChannelStable,
		Target: "v2.1.0-beta.1", TargetChannel: domain.UpdateChannelPrerelease,
		ReviewToken: "sha256:review", Confirmation: "UPDATE NIXORIUM TO v2.1.0-beta.1",
		Checks: []domain.UpdateCheck{{ID: "client", State: "passed", Message: "built"}},
		Diff:   domain.GitDiff{Content: "+github:owner/repo/v2.1.0-beta.1\n"},
	}
	var output bytes.Buffer
	UpdatePlanText(&output, plan)
	for _, expected := range []string{"update plan: READY", "v2.0.0", "prerelease", "client", "PASSED", "sha256:review", plan.Confirmation, "No branch, commit, push", "flake.nix/flake.lock"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("update plan output omits %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	UpdateApplyText(&output, domain.UpdateApplyReport{State: "completed", Target: plan.Target, Updated: true, Message: "review and commit it separately"})
	for _, expected := range []string{"update: COMPLETED", "Files updated:   true", "review and commit it separately"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("update apply output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestUpdateCheckTextSeparatesReleaseChannels(t *testing.T) {
	report := domain.UpdateCheckReport{
		State: "available", Repository: "/deployment", Upstream: "github:owner/repo",
		CurrentRef: "v2.0.0", CurrentRev: strings.Repeat("a", 40), CurrentChannel: domain.UpdateChannelStable,
		Stable:     []domain.UpdateRelease{{Tag: "v2.1.0", ObjectID: strings.Repeat("b", 40), Channel: domain.UpdateChannelStable}},
		Prerelease: []domain.UpdateRelease{{Tag: "v2.2.0-beta.1", ObjectID: strings.Repeat("c", 40), Channel: domain.UpdateChannelPrerelease}},
		Truncated:  true,
	}
	var output bytes.Buffer
	UpdateCheckText(&output, report)
	for _, expected := range []string{"release check: AVAILABLE", "github:owner/repo", "Stable releases", "v2.1.0", "Prereleases", "v2.2.0-beta.1", "newest 20", "only update operation"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("update check output omits %q:\n%s", expected, output.String())
		}
	}
}

func TestSoftwareTextSeparatesDeclarationFromBuildAndDeployment(t *testing.T) {
	catalog := domain.SoftwareCatalogReport{
		State: "ready", ManagedFile: "lab-software.json",
		Catalog:  []domain.SoftwareCatalogItem{{ID: "vlc", Label: "VLC", Summary: "Play media", Availability: "available"}},
		Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"}},
		Message:  "resolved from pinned inputs",
	}
	var output bytes.Buffer
	SoftwareCatalogText(&output, catalog)
	for _, expected := range []string{"Software catalog: READY", "lab-software.json", "VLC", "all-clients", "pinned inputs"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("software catalog omits %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	plan := domain.SoftwareChangePlanReport{
		State: "ready", ManagedFile: "lab-software.json",
		Request:         domain.SoftwareChangeRequest{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: "media"}},
		AffectedClients: []string{"pc01", "pc02"}, ReviewToken: "sha256:review", Confirmation: "SAVE SOFTWARE abcdef012345",
		Message: "no system has been built or changed",
	}
	SoftwareChangePlanText(&output, plan)
	for _, expected := range []string{"Software proposal: READY", "vlc", "media", "pc01, pc02", "sha256:review", plan.Confirmation, "no system has been built or changed"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("software plan omits %q:\n%s", expected, output.String())
		}
	}
	output.Reset()
	SoftwareChangeApplyText(&output, domain.SoftwareChangeApplyReport{State: "applied", ManagedFile: "lab-software.json", Message: "Review and commit before preparing or distributing systems."})
	if text := output.String(); !strings.Contains(text, "Software change: APPLIED") || !strings.Contains(text, "Review and commit") {
		t.Fatalf("software result is incomplete:\n%s", text)
	}
}
