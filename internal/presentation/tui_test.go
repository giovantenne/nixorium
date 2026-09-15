package presentation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func testDashboardReport(mode string) domain.StatusReport {
	report := domain.StatusReport{
		Deployment:     domain.DeploymentStatus{Ready: true},
		PXE:            domain.PXELifecycleState{Mode: mode},
		PXEPreparation: domain.PXEPreparationState{Present: true, Ready: true},
		Services: []domain.ServiceState{
			{Name: "nixorium-harmonia.service", State: "active", Active: true},
		},
	}
	report.Meta.Clients.Count = 2
	report.Meta.Controller.Name = "pc99"
	report.Meta.Controller.DHCPIP = "192.0.2.10"
	report.Meta.Network.Interface = "enp1s0"
	return report
}

func TestDashboardLoadsAndRefreshesComputerInventory(t *testing.T) {
	loads := 0
	actions := DashboardActions{
		LoadHosts: func() (domain.HostsReport, error) {
			loads++
			report := domain.HostsReport{State: "partial", Deployment: domain.HostDeploymentSummary{Current: 1, Unknown: 1}, Hosts: []domain.HostStatus{
				{Name: "pc01", IP: "10.0.0.1", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Deployment: domain.DeploymentCurrent, LastSuccessfulDeploy: &domain.LastSuccessfulDeployment{Revision: "0123456789abcdef", VerifiedAt: time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC)}},
				{Name: "pc02", IP: "10.0.0.2", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown, Deployment: domain.DeploymentUnknown},
			}}
			if loads > 1 {
				report.State = "available"
				report.Deployment = domain.HostDeploymentSummary{Current: 2}
				report.Hosts[1].Reachability = domain.ReachabilityReachable
				report.Hosts[1].SSH = domain.SSHAvailable
				report.Hosts[1].Deployment = domain.DeploymentCurrent
			}
			return report, nil
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	if !strings.Contains(model.View().Content, "Computers            2 configured") || !strings.Contains(model.View().Content, "View computers") {
		t.Fatalf("home omits computer summary:\n%s", model.View().Content)
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "h"})
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View().Content, "Checking configured computers") {
		t.Fatalf("opening inventory did not start explicit probe:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardHosts || !strings.Contains(model.View().Content, "pc02") || !strings.Contains(model.View().Content, "unreachable  unknown") || !strings.Contains(model.View().Content, "1 current, 0 outdated, 1 unknown") || !strings.Contains(model.View().Content, "2026-09-14 10:30Z") || !strings.Contains(model.View().Content, "never") {
		t.Fatalf("computer inventory is incomplete:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command == nil || !strings.Contains(model.View().Content, "Refreshing computer status") {
		t.Fatalf("refresh did not enter busy state:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.screen != dashboardHosts || !strings.Contains(model.View().Content, "SSH available: 2/2") {
		t.Fatalf("load count = %d, screen = %d:\n%s", loads, model.screen, model.View().Content)
	}
}

func TestDashboardOffersPXEWorkflowFromReconciledState(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready")}
	view := model.View().Content
	if !strings.Contains(view, "Installation mode    ready") || !strings.Contains(view, "Install computers over network") {
		t.Fatalf("dashboard omits PXE workflow:\n%s", view)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	view = model.View().Content
	if model.screen != dashboardPXE || !strings.Contains(view, "Prepared artifacts: ready") || !strings.Contains(view, "Start installation mode") {
		t.Fatalf("PXE screen is incomplete:\n%s", view)
	}
}

func TestDashboardReviewsAndRunsAllClientDeployment(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc02", IP: "10.0.0.2"},
	}
	planned := ""
	applied := 0
	actions := DashboardActions{
		PlanDeployment: func(requested string) domain.DeploymentPlanReport {
			planned = requested
			return domain.DeploymentPlanReport{
				Operation:       "deploy-plan",
				State:           "ready",
				Requested:       requested,
				Revision:        "0123456789abcdef",
				ColmenaSelector: "@lab",
				Targets:         []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc02"}},
			}
		},
		ApplyDeployment: func(plan domain.DeploymentPlanReport) domain.DeploymentExecutionReport {
			applied++
			return domain.DeploymentExecutionReport{
				Operation:       "deploy-apply",
				State:           "completed",
				Phase:           domain.DeploymentPhaseComplete,
				ColmenaSelector: plan.ColmenaSelector,
				BuildCompleted:  true,
				ApplyCompleted:  true,
				Verification:    domain.DeploymentVerificationSummary{Attempted: 2, Verified: 2, Recorded: 2},
				LogPath:         "/state/deploy.log",
				Message:         "all selected targets were built and applied successfully",
			}
		},
	}
	model := dashboardModel{report: report, actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "d"})
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "[ ] pc01") {
		t.Fatalf("deployment selection not shown:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != "@lab" || model.screen != dashboardDeployReview || !strings.Contains(model.View().Content, "Revision: 0123456789abcdef") {
		t.Fatalf("planned = %q, screen = %d:\n%s", planned, model.screen, model.View().Content)
	}

	for _, key := range []tea.KeyPressMsg{
		{Text: "DEPLOY"},
		{Code: tea.KeySpace},
		{Text: "@lab"},
	} {
		updated, _ = model.Update(key)
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.deploying || !strings.Contains(model.View().Content, "Closing is disabled") {
		t.Fatalf("confirmed deployment did not enter protected busy state:\n%s", model.View().Content)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "wait for its result") {
		t.Fatal("dashboard allowed quit while deployment was running")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.deploying || model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "Last result: completed") || !strings.Contains(model.View().Content, "Authenticated: 2/2   Recorded: 2") || !strings.Contains(model.View().Content, "/state/deploy.log") {
		t.Fatalf("deployment result missing: applied=%d\n%s", applied, model.View().Content)
	}
}

func TestDashboardReviewsAndRunsControllerRebuild(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	applied := 0
	actions := DashboardActions{
		PlanController: func() domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-plan",
				State:         "ready",
				Controller:    "pc99",
				Revision:      revision,
				Confirmation:  "REBUILD pc99",
				Issues:        []domain.ValidationIssue{},
			}
		},
		ApplyController: func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			applied++
			return domain.ControllerRebuildExecutionReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "controller-apply",
				State:         "completed",
				Controller:    plan.Controller,
				Revision:      plan.Revision,
				Phase:         domain.ControllerRebuildPhaseComplete,
				Applied:       true,
				Verified:      true,
			}
		},
	}
	model := dashboardModel{actions: actions}
	updated, command := model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardControllerReview || !strings.Contains(model.View().Content, revision) || !strings.Contains(model.View().Content, "REBUILD pc99") {
		t.Fatalf("controller review missing:\n%s", model.View().Content)
	}
	for _, character := range "REBUILD pc99" {
		updated, _ = model.Update(tea.KeyPressMsg{Code: character, Text: string(character)})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("controller apply did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.screen != dashboardController || !strings.Contains(model.View().Content, "Applied: true   Verified: true") {
		t.Fatalf("controller result missing: applied=%d\n%s", applied, model.View().Content)
	}
}

func TestDashboardReviewsAndAppliesValidatedNixoriumUpdate(t *testing.T) {
	target := "v2.1.0-beta.1"
	confirmation := "DOWNGRADE NIXORIUM TO " + target
	token := "sha256:" + strings.Repeat("d", 64)
	planned := 0
	applied := 0
	actions := DashboardActions{
		PlanUpdate: func(received string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			planned++
			if received != target || !allowPrerelease || !allowDowngrade {
				t.Fatalf("update plan input = %q, prerelease=%t, downgrade=%t", received, allowPrerelease, allowDowngrade)
			}
			return domain.UpdatePlanReport{
				SchemaVersion:  domain.SchemaVersion,
				Operation:      "update-plan",
				State:          "ready",
				Revision:       strings.Repeat("a", 40),
				CurrentRef:     "v2.2.0",
				CurrentChannel: domain.UpdateChannelStable,
				Target:         received,
				TargetChannel:  domain.UpdateChannelPrerelease,
				Downgrade:      true,
				ReviewToken:    token,
				Confirmation:   confirmation,
				Checks:         []domain.UpdateCheck{{ID: "controller", State: "passed", Message: "candidate controller built"}},
				Diff:           domain.GitDiff{Content: strings.Repeat("+ reviewed update\n", 20)},
			}
		},
		ApplyUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			applied++
			if plan.ReviewToken != token || plan.Target != target {
				t.Fatalf("applied unexpected update plan: %+v", plan)
			}
			return domain.UpdateApplyReport{Operation: "update-apply", State: "completed", Target: target, Updated: true, Message: "review and commit the two updated files"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	if !strings.Contains(model.View().Content, "Update Nixorium") {
		t.Fatalf("home omits update task:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	if model.screen != dashboardUpdate || !strings.Contains(model.View().Content, "Allow prerelease: [ ]") {
		t.Fatalf("update input missing:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.View().Content, "explicit release tag") {
		t.Fatalf("empty update target was planned:\n%s", model.View().Content)
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyF2}, {Code: tea.KeyF3}, {Text: target}} {
		updated, _ = model.Update(key)
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("update plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardUpdateReview || !strings.Contains(model.View().Content, "Validated release review") || !strings.Contains(model.View().Content, "candidate controller built") || !strings.Contains(model.View().Content, confirmation) || !strings.Contains(model.View().Content, "No commit, push, activation") {
		t.Fatalf("update review missing: planned=%d\n%s", planned, model.View().Content)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Height: 40})
	model = updated.(dashboardModel)
	if lines := strings.Count(model.View().Content, "\n"); lines > 40 {
		t.Fatalf("update review exceeds terminal height: got %d lines\n%s", lines, model.View().Content)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Height: 24})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	model = updated.(dashboardModel)
	if model.updateScroll == 0 {
		t.Fatal("update diff did not scroll")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "wrong"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || applied != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatalf("inexact update confirmation applied: %d", applied)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: confirmation})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.updating || !strings.Contains(model.View().Content, "Wait for the atomic two-file result") {
		t.Fatalf("update apply did not enter protected busy state:\n%s", model.View().Content)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "wait for its result") {
		t.Fatal("dashboard allowed quit while update apply was running")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.updating || model.screen != dashboardUpdate || !strings.Contains(model.View().Content, "Last result: completed; files updated=true") || !strings.Contains(model.View().Content, "review and commit") {
		t.Fatalf("update result missing: applied=%d\n%s", applied, model.View().Content)
	}
}

func TestDashboardReviewsAndRestartsOnlyCacheService(t *testing.T) {
	restarts := 0
	serviceReport := domain.ServicesReport{
		Operation: "services",
		State:     "healthy",
		Services: []domain.ManagedService{
			{ID: "cache", Name: "Binary cache", State: "healthy", Detail: "HTTP-ready", Units: []domain.ServiceState{{Name: "nixorium-harmonia.service", Loaded: true, Active: true, State: "active"}}},
			{ID: "pxe", Name: "PXE installation mode", State: "ready", Detail: "prepared", Units: []domain.ServiceState{{Name: "nixorium-pxe.service", Loaded: true, State: "inactive"}}},
		},
	}
	actions := DashboardActions{
		LoadServices: func() domain.ServicesReport { return serviceReport },
		RestartService: func(service string) domain.ServiceActionReport {
			restarts++
			if service != "cache" {
				t.Fatalf("restarted unexpected service %q", service)
			}
			return domain.ServiceActionReport{Operation: "service-restart", State: "completed", Service: service, Verified: true, Message: "cache healthy"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, command := model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardServices || !strings.Contains(model.View().Content, "Binary cache — healthy") || !strings.Contains(model.View().Content, "Managed through the Install computers workflow") {
		t.Fatalf("services screen missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if model.screen != dashboardServicesRestartReview || !strings.Contains(model.View().Content, "RESTART CACHE") {
		t.Fatalf("restart review missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "restart cache"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || restarts != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatalf("inexact restart was accepted: restarts=%d", restarts)
	}
	for _, key := range []tea.KeyPressMsg{{Text: "RESTART"}, {Code: tea.KeySpace}, {Text: "CACHE"}} {
		updated, _ = model.Update(key)
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if restarts != 1 || model.screen != dashboardServices || !strings.Contains(model.View().Content, "verified=true") || !strings.Contains(model.View().Content, "cache healthy") {
		t.Fatalf("verified restart result missing: restarts=%d\n%s", restarts, model.View().Content)
	}
}

func TestDashboardBrowsesBoundedOperationLogTail(t *testing.T) {
	id := "deploy-20260914T113000.000000000Z-11.log"
	loaded := ""
	actions := DashboardActions{
		LoadLogs: func() domain.OperationLogsReport {
			return domain.OperationLogsReport{Operation: "logs-list", State: "available", Records: []domain.OperationRecord{
				{RecordedAt: time.Date(2026, 9, 14, 11, 31, 0, 0, time.UTC), Operation: "pxe-start", State: "completed", Subject: "active", Summary: "PXE lifecycle transition finished"},
			}, Logs: []domain.OperationLogEntry{
				{ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC), SizeBytes: 70000, State: "partial", Available: true},
			}}
		},
		LoadLog: func(selected string) domain.OperationLogReport {
			loaded = selected
			lines := make([]string, 20)
			for index := range lines {
				lines[index] = fmt.Sprintf("line-%02d", index+1)
			}
			entry := domain.OperationLogEntry{ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC), SizeBytes: 70000, State: "partial", Available: true}
			return domain.OperationLogReport{Operation: "logs-show", State: "available", Log: &entry, Content: strings.Join(lines, "\n") + "\n", Truncated: true}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "l"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardLogs || !strings.Contains(model.View().Content, "Recent actions") || !strings.Contains(model.View().Content, "pxe-start") || !strings.Contains(model.View().Content, id) || !strings.Contains(model.View().Content, "partial") {
		t.Fatalf("operation log list missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loaded != id || model.screen != dashboardLogDetail || !strings.Contains(model.View().Content, "earlier bytes omitted") || !strings.Contains(model.View().Content, "line-20") || strings.Contains(model.View().Content, "line-01") {
		t.Fatalf("bounded tail detail missing: loaded=%q\n%s", loaded, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "line-01") {
		t.Fatalf("log detail did not scroll to the beginning:\n%s", model.View().Content)
	}
}

func TestDashboardShowsScrollableReadOnlyGitReview(t *testing.T) {
	loads := 0
	actions := DashboardActions{
		LoadGitReview: func() domain.GitReviewReport {
			loads++
			return domain.GitReviewReport{
				Operation: "git-review",
				State:     "changes",
				Summary:   domain.GitChangeSummary{Staged: 1, Untracked: 1, Managed: 1, Unexpected: 1},
				Changes: []domain.GitChange{
					{Path: "lab-settings.json", Staged: "modified", Managed: true},
					{Path: "notes", Untracked: true},
				},
				Diffs: []domain.GitDiff{{Scope: "staged", Content: "line-01\nline-02\nline-03\nline-04\nline-05\nline-06\nline-07\n"}},
			}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("Git review did not load: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View().Content, "Git change review") || !strings.Contains(model.View().Content, "lab-settings.json") {
		t.Fatalf("Git review missing: loads=%d\n%s", loads, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "line-07") || strings.Contains(model.View().Content, "lab-settings.json") {
		t.Fatalf("Git review did not scroll:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "f"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 2 || model.gitScroll != 0 {
		t.Fatalf("Git review refresh = loads %d, scroll %d", loads, model.gitScroll)
	}
}

func TestDashboardPlansAndCreatesExactLocalGitCommit(t *testing.T) {
	revision := strings.Repeat("a", 40)
	token := "sha256:" + strings.Repeat("b", 64)
	confirmation := "COMMIT " + strings.Repeat("b", 12)
	applied := 0
	review := domain.GitReviewReport{
		Operation: "git-review",
		State:     "changes",
		Summary:   domain.GitChangeSummary{Unstaged: 2, Managed: 1, Unexpected: 1},
		Changes: []domain.GitChange{
			{Path: "lab-settings.json", Unstaged: "modified", Managed: true},
			{Path: "modules/site.nix", Unstaged: "modified"},
		},
	}
	actions := DashboardActions{
		LoadGitReview: func() domain.GitReviewReport { return review },
		PlanGitCommit: func(paths string) domain.GitCommitPlanReport {
			if paths != "lab-settings.json" {
				t.Fatalf("planned paths = %q", paths)
			}
			return domain.GitCommitPlanReport{Operation: "git-commit-plan", State: "ready", Revision: revision, Paths: []string{"lab-settings.json"}, ReviewToken: token, CommitMessage: "chore: update laboratory settings", Confirmation: confirmation, Diff: domain.GitDiff{Content: "+settings\n"}}
		},
		ApplyGitCommit: func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
			applied++
			if plan.ReviewToken != token {
				t.Fatalf("applied token = %q", plan.ReviewToken)
			}
			return domain.GitCommitReport{Operation: "git-commit", State: "completed", Paths: plan.Paths, PreviousRevision: revision, Revision: strings.Repeat("c", 40), Committed: true, Message: "no remote push was attempted"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, command := model.Update(tea.KeyPressMsg{Text: "g"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if model.screen != dashboardGitCommitSelect || !strings.Contains(model.View().Content, "Select Git commit paths") {
		t.Fatalf("commit selection missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardGitCommitReview || !strings.Contains(model.View().Content, confirmation) || !strings.Contains(model.View().Content, "No hooks, signing actions, remote operations, or push") {
		t.Fatalf("commit review missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "COMMIT"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: strings.Repeat("b", 12)})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("commit apply did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.screen != dashboardGitReview || !strings.Contains(model.View().Content, "Last commit: completed; committed=true") || !strings.Contains(model.View().Content, "no remote push was attempted") {
		t.Fatalf("commit result missing: applied=%d\n%s", applied, model.View().Content)
	}
}

func TestGitCommitSelectionPreservesReviewOrderAndSkipsPrivatePaths(t *testing.T) {
	changes := []domain.GitChange{{Path: "z"}, {Path: "secret-key", Private: true}, {Path: "a"}}
	chosen := toggleAllGitCommitPaths(changes, nil)
	if chosen["secret-key"] || selectedGitCommitPaths(changes, chosen) != "z,a" {
		t.Fatalf("unsafe or reordered selection = %v / %q", chosen, selectedGitCommitPaths(changes, chosen))
	}
}

func TestDashboardDeploymentRejectsEmptySelectionAndBlockedPlan(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	actions := DashboardActions{
		PlanDeployment: func(string) domain.DeploymentPlanReport {
			return domain.DeploymentPlanReport{State: "blocked", Issues: []domain.ValidationIssue{{Field: "git", Message: "worktree is dirty"}}}
		},
	}
	model := dashboardModel{report: report, actions: actions, screen: dashboardDeploy, deployChosen: map[string]bool{}}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.View().Content, "Select at least one") {
		t.Fatalf("empty selection was planned:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardDeploy || !strings.Contains(model.View().Content, "git: worktree is dirty") {
		t.Fatalf("blocked plan was not surfaced:\n%s", model.View().Content)
	}
}

func TestSelectedDeploymentTargetsPreservesInventoryOrder(t *testing.T) {
	hosts := []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}, {Name: "pc03"}}
	if got := selectedDeploymentTargets(hosts, map[string]bool{"pc03": true, "pc01": true}); got != "pc01,pc03" {
		t.Fatalf("selected targets = %q, want pc01,pc03", got)
	}
	if got := selectedDeploymentTargets(hosts, map[string]bool{"pc01": true, "pc02": true, "pc03": true}); got != "@lab" {
		t.Fatalf("all targets = %q, want @lab", got)
	}
}

func TestDashboardPXEStartRequiresExactTypedConfirmation(t *testing.T) {
	starts := 0
	actions := DashboardActions{
		Refresh: func() (domain.StatusReport, error) { return testDashboardReport("active"), nil },
		PlanPXEStart: func() domain.PXELifecycleReport {
			return domain.PXELifecycleReport{
				State:       "ready",
				Mode:        "ready",
				Interface:   "enp1s0",
				DHCPAddress: "192.0.2.10",
				StaticCIDR:  "10.0.0.99/24",
			}
		},
		StartPXE: func() domain.PXELifecycleReport {
			starts++
			return domain.PXELifecycleReport{State: "completed", Mode: "active", Message: "PXE active"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, screen: dashboardPXE}

	updated, command := model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardPXEStartReview || !strings.Contains(model.View().Content, "Temporarily remove 10.0.0.99/24") {
		t.Fatalf("start review not shown:\n%s", model.View().Content)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "start pxe"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || starts != 0 || !strings.Contains(model.View().Content, "did not match") {
		t.Fatalf("inexact confirmation started PXE: starts=%d", starts)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "START PXE"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if starts != 1 || model.report.PXE.Mode != "active" || !strings.Contains(model.View().Content, "PXE active") {
		t.Fatalf("confirmed start did not refresh state: starts=%d view=%s", starts, model.View().Content)
	}
}

func TestDashboardPXEConfirmationAcceptsTerminalSpaceEvent(t *testing.T) {
	model := dashboardModel{screen: dashboardPXEStartReview}
	for _, key := range []tea.KeyPressMsg{
		{Text: "START"},
		{Code: tea.KeySpace},
		{Text: "PXE"},
	} {
		updated, _ := model.Update(key)
		model = updated.(dashboardModel)
	}
	if model.confirmation != "START PXE" {
		t.Fatalf("terminal confirmation = %q, want START PXE", model.confirmation)
	}
}

func TestDashboardPXEPrepareStopAndRecoverUseCallbacks(t *testing.T) {
	called := ""
	actions := DashboardActions{
		Refresh: func() (domain.StatusReport, error) { return testDashboardReport("ready"), nil },
		PreparePXE: func() domain.ActionReport {
			called = "prepare"
			return domain.ActionReport{Message: "prepared"}
		},
		StopPXE: func() domain.PXELifecycleReport {
			called = "stop"
			return domain.PXELifecycleReport{Message: "stopped"}
		},
		RecoverPXE: func() domain.PXELifecycleReport {
			called = "recover"
			return domain.PXELifecycleReport{Message: "recovered"}
		},
	}
	for key, expected := range map[string]string{"p": "prepare", "x": "stop", "r": "recover"} {
		called = ""
		model := dashboardModel{report: testDashboardReport("active"), actions: actions, screen: dashboardPXE}
		updated, command := model.Update(tea.KeyPressMsg{Text: key})
		model = updated.(dashboardModel)
		if command == nil {
			t.Fatalf("%s did not schedule an operation", key)
		}
		if key == "p" && !strings.Contains(model.View().Content, "journalctl -fu nixorium-prepare-pxe.service") {
			t.Fatalf("PXE preparation omits detailed live-log guidance:\n%s", model.View().Content)
		}
		updated, _ = model.Update(command())
		if called != expected {
			t.Errorf("%s called %q, want %q", key, called, expected)
		}
	}
}
