package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDashboardReviewsAndAppliesValidatedNixoriumUpdate(t *testing.T) {
	target := "v2.3.0"
	confirmation := "UPDATE"
	token := "sha256:" + strings.Repeat("d", 64)
	checked := 0
	planned := 0
	applied := 0
	actions := DashboardActions{
		CheckUpdate: func(ctx context.Context) domain.UpdateCheckReport {
			checked++
			return domain.UpdateCheckReport{
				SchemaVersion: domain.SchemaVersion,
				Operation:     "update-check",
				State:         "available",
				Upstream:      "github:giovantenne/nixorium",
				CurrentRef:    "v2.2.0",
				Development:   []domain.UpdateRelease{{Tag: "master", Channel: domain.UpdateChannelMoving}},
				Stable: []domain.UpdateRelease{
					{Tag: target, Channel: domain.UpdateChannelStable},
					{Tag: "v2.2.0", Channel: domain.UpdateChannelStable},
				},
				Prerelease: []domain.UpdateRelease{{Tag: "v2.4.0-beta.1", Channel: domain.UpdateChannelPrerelease}},
			}
		},
		PlanUpdate: func(ctx context.Context, received string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			planned++
			if received != target || allowPrerelease || allowDowngrade {
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
				TargetChannel:  domain.UpdateChannelStable,
				ReviewToken:    token,
				Confirmation:   confirmation,
				Checks:         []domain.UpdateCheck{{ID: "controller", State: "passed", Message: "candidate controller built"}},
				Diff:           domain.GitDiff{Content: strings.Repeat("+ reviewed update\n", 20)},
			}
		},
		SaveUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			applied++
			if plan.ReviewToken != token || plan.Target != target {
				t.Fatalf("applied unexpected update plan: %+v", plan)
			}
			return domain.UpdateApplyReport{Revision: strings.Repeat("b", 40), Operation: "update-save", State: "saved", Target: target, Updated: true, Message: "Nixorium update saved locally. Running systems were not changed."}
		},
		PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: strings.Repeat("b", 40)}
		},
		ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{Revision: strings.Repeat("b", 40), Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true}
		},
	}
	actions.RunningVersion = "2.0.0-test"
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	if strings.Contains(model.View().Content, "Update Nixorium") || !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("home omits navigable task menu:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardUpdate || !strings.Contains(model.View().Content, "Fetching available Nixorium updates") {
		t.Fatalf("release discovery did not start:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if checked != 1 || !strings.Contains(model.View().Content, target) || !strings.Contains(model.View().Content, "master") || !strings.Contains(model.View().Content, "Development branch") || !strings.Contains(model.View().Content, "Running interface   2.0.0-test") || strings.Contains(model.View().Content, "v2.4.0-beta.1") || strings.Contains(model.View().Content, "Target: >") {
		t.Fatalf("fetched stable release list is incorrect:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "v2.4.0-beta.1") {
		t.Fatalf("prerelease disclosure missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("update plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardUpdateReview || !strings.Contains(model.View().Content, "Validated release") || !strings.Contains(model.View().Content, "candidate controller built") || strings.Contains(model.View().Content, confirmation) || !strings.Contains(model.View().Content, "Press Enter") || !strings.Contains(model.View().Content, "No push, PXE action") {
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
	if model.updates.scroll == 0 {
		t.Fatal("update diff did not scroll")
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.updates.applying || !strings.Contains(model.View().Content, "Wait for the atomic two-file result") {
		t.Fatalf("update apply did not enter protected busy state:\n%s", model.View().Content)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "Wait for the result") {
		t.Fatal("dashboard allowed quit while update apply was running")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil || !model.updates.applying {
		t.Fatal("saved update did not start controller activation")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.updates.applying || model.screen != dashboardUpdate || !strings.Contains(model.View().Content, "Nixorium and this controller are updated") || strings.Contains(model.View().Content, "review Git changes") || !strings.Contains(model.View().Content, "Running interface: 2.0.0-test") || !strings.Contains(model.View().Content, "Reopen Nixorium") || !strings.Contains(model.View().Content, "New update") {
		t.Fatalf("update result missing: applied=%d\n%s", applied, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("update result did not return to Maintenance: screen=%d", model.screen)
	}
}

func TestDashboardHidesNixoriumDowngrades(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), actions: DashboardActions{}}
	report := domain.UpdateCheckReport{
		State: "available", CurrentRef: "v3.2.0-beta.10", CurrentChannel: domain.UpdateChannelPrerelease,
		Development: []domain.UpdateRelease{{Tag: "master", Channel: domain.UpdateChannelMoving}},
		Stable:      []domain.UpdateRelease{{Tag: "v3.1.1", Channel: domain.UpdateChannelStable, Downgrade: true}},
		Prerelease: []domain.UpdateRelease{
			{Tag: "v3.2.0-beta.10", Channel: domain.UpdateChannelPrerelease},
			{Tag: "v3.2.0-beta.2", Channel: domain.UpdateChannelPrerelease, Downgrade: true},
		},
	}
	updated, _ := model.Update(dashboardUpdateCheckMsg{report: report})
	model = updated.(dashboardModel)
	releases := model.availableUpdateReleases()
	if len(releases) != 2 || model.updates.cursor != 1 || releases[1].Tag != report.CurrentRef {
		t.Fatalf("selectable releases = %+v, cursor = %d", releases, model.updates.cursor)
	}
	view := model.View().Content
	if strings.Contains(view, "v3.1.1") || strings.Contains(view, "v3.2.0-beta.2") {
		t.Fatalf("downgrades appear in the TUI:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardUpdate {
		t.Fatal("the current release started an update")
	}
}

func TestDashboardHidesReleasesOlderThanRunningVersionOnMasterPins(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), actions: DashboardActions{RunningVersion: "3.5.0"}}
	report := domain.UpdateCheckReport{
		State: "available", CurrentRef: "master", CurrentChannel: domain.UpdateChannelMoving, VersionFloor: "v3.5.0",
		Development: []domain.UpdateRelease{{Tag: "master", Channel: domain.UpdateChannelMoving}},
		Stable: []domain.UpdateRelease{
			{Tag: "v3.6.0", Channel: domain.UpdateChannelStable},
			{Tag: "v3.5.0", Channel: domain.UpdateChannelStable},
			{Tag: "v3.4.0", Channel: domain.UpdateChannelStable, Downgrade: true},
		},
	}
	updated, _ := model.Update(dashboardUpdateCheckMsg{report: report})
	model = updated.(dashboardModel)
	releases := model.availableUpdateReleases()
	if len(releases) != 3 || releases[2].Tag != "v3.5.0" {
		t.Fatalf("selectable releases = %+v", releases)
	}
	view := model.View().Content
	if strings.Contains(view, "v3.4.0") || !strings.Contains(view, "Running version     3.5.0 (older releases are hidden)") || !strings.Contains(view, "Configured target   master") {
		t.Fatalf("master-pinned update screen does not apply the version floor:\n%s", view)
	}
	for index, release := range releases {
		if release.Tag == "v3.5.0" && updateReleaseStatus(report, release) != "" {
			t.Fatalf("floor release %d is marked as configured: %q", index, updateReleaseStatus(report, release))
		}
	}
}

func TestDashboardRefusesOlderThanRunningVersionPlanOnMasterPins(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready")}
	updated, command := model.Update(dashboardUpdatePlanMsg{report: domain.UpdatePlanReport{
		State: "blocked", Target: "v3.4.0", CurrentRef: "master", CurrentChannel: domain.UpdateChannelMoving, Downgrade: true,
		Issues: []domain.ValidationIssue{{Field: "target", Message: "target release is older than the running version v3.5.0; downgrade targets require explicit --allow-downgrade"}},
	}})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardUpdate || !strings.Contains(model.message, "downgrades are not available in the TUI") || strings.Contains(model.message, "--allow-downgrade") {
		t.Fatalf("older release reached review from a master pin: %+v", model)
	}
}

func TestDashboardRefusesNixoriumDowngradeBeforeReview(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready")}
	updated, command := model.Update(dashboardUpdatePlanMsg{report: domain.UpdatePlanReport{
		State: "ready", Target: "v3.1.1", Downgrade: true,
	}})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardUpdate || !strings.Contains(model.message, "downgrades are not available") {
		t.Fatalf("downgrade reached review: %+v", model)
	}
}

func TestUpdatePlanningShowsRealCandidatePhasesAndBuildCount(t *testing.T) {
	target := "v2.3.0"
	model := dashboardModel{
		screen: dashboardUpdate,
		updates: updateModel{
			check: domain.UpdateCheckReport{
				Operation:  "update-check",
				State:      "available",
				CurrentRef: "v2.2.0",
				Stable:     []domain.UpdateRelease{{Tag: target, Channel: domain.UpdateChannelStable}},
			},
		},
		actions: DashboardActions{
			PlanUpdateWithProgress: func(ctx context.Context, received string, allowPrerelease, allowDowngrade bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
				if received != target || allowPrerelease || allowDowngrade {
					t.Fatalf("update plan input = %q, prerelease=%t, downgrade=%t", received, allowPrerelease, allowDowngrade)
				}
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseInspect, Detail: "Checking the deployment repository and selected release"})
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseLock, Detail: "Resolving the selected release and generating its candidate lock"})
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseEvaluate, Detail: "Evaluating candidate deployment readiness"})
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseBuild, Detail: "Building the controller", Current: 2, Total: 5})
				return domain.UpdatePlanReport{Operation: "update-plan", State: "ready", Target: target, TargetChannel: domain.UpdateChannelStable}
			},
		},
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.updates.planning || !strings.Contains(model.View().Content, "Target: "+target) {
		t.Fatalf("update planning did not start with visible context:\n%s", model.View().Content)
	}
	foundBuild := false
	for command != nil && model.screen != dashboardUpdateReview {
		message := command()
		updated, command = model.Update(message)
		model = updated.(dashboardModel)
		payload := message
		if wrapped, ok := message.(activityResultMsg); ok {
			payload = wrapped.message
		}
		if progress, ok := payload.(dashboardUpdatePlanProgressMsg); ok && progress.progress.Phase == domain.UpdatePlanPhaseBuild {
			view := model.View().Content
			foundBuild = strings.Contains(view, "Test systems before saving") && strings.Contains(view, "Testing the controller system") && strings.Contains(view, "Safety check 2/5") && strings.Contains(view, "Elapsed:") && strings.Contains(view, "current deployment remains unchanged")
		}
	}
	if !foundBuild || model.updates.planning || model.screen != dashboardUpdateReview {
		t.Fatalf("update progress did not reach a visible representative build: found=%t planning=%t screen=%d\n%s", foundBuild, model.updates.planning, model.screen, model.View().Content)
	}
}

func TestNixoriumUpdateDiscoveryFailureHasNoEditableFallback(t *testing.T) {
	model := dashboardModel{
		report: testDashboardReport("ready"),
		actions: DashboardActions{CheckUpdate: func(ctx context.Context) domain.UpdateCheckReport {
			return domain.UpdateCheckReport{
				Operation: "update-check",
				State:     "failed",
				Issues:    []domain.ValidationIssue{{Field: "network", Message: "upstream unavailable"}},
			}
		}},
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if !strings.Contains(view, "Updates could not be fetched") || !strings.Contains(view, "No update can be selected") || strings.Contains(view, "Target: >") {
		t.Fatalf("failed discovery exposed an unsafe fallback:\n%s", view)
	}
	updated, planCommand := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if planCommand != nil || model.busy != "" {
		t.Fatal("failed discovery could start planning")
	}
}

func TestNixoriumMovingMasterCanAdvanceToNewRevision(t *testing.T) {
	currentRevision := strings.Repeat("a", 40)
	remoteRevision := strings.Repeat("b", 40)
	planned := 0
	model := dashboardModel{
		screen: dashboardUpdate,
		report: testDashboardReport("ready"),
		updates: updateModel{check: domain.UpdateCheckReport{
			Operation:  "update-check",
			State:      "available",
			CurrentRef: "master",
			CurrentRev: currentRevision,
			Development: []domain.UpdateRelease{{
				Tag: "master", ObjectID: remoteRevision, Channel: domain.UpdateChannelMoving,
			}},
		}},
		actions: DashboardActions{PlanUpdate: func(ctx context.Context, target string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
			planned++
			if target != "master" || allowPrerelease || allowDowngrade {
				t.Fatalf("moving update input = %q, prerelease=%t, downgrade=%t", target, allowPrerelease, allowDowngrade)
			}
			return domain.UpdatePlanReport{Operation: "update-plan", State: "ready", Target: target, TargetChannel: domain.UpdateChannelMoving}
		}},
	}
	if view := model.View().Content; !strings.Contains(view, "New revision available") {
		t.Fatalf("moving update is not visible:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("new master revision did not start validation: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardUpdateReview {
		t.Fatalf("new master revision was not planned: calls=%d screen=%d", planned, model.screen)
	}
}

func TestNixoriumMovingMasterBlocksSameRevision(t *testing.T) {
	revision := strings.Repeat("a", 40)
	model := dashboardModel{
		screen: dashboardUpdate,
		report: testDashboardReport("ready"),
		updates: updateModel{check: domain.UpdateCheckReport{
			Operation:  "update-check",
			State:      "available",
			CurrentRef: "master",
			CurrentRev: revision,
			Development: []domain.UpdateRelease{{
				Tag: "master", ObjectID: revision, Channel: domain.UpdateChannelMoving,
			}},
		}},
	}
	if view := model.View().Content; !strings.Contains(view, "Current revision") {
		t.Fatalf("current master revision is not identified:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || !strings.Contains(model.message, "current upstream revision") {
		t.Fatalf("current master revision was not blocked clearly: command=%v message=%q", command, model.message)
	}
}

func TestNixoriumUpdateInputFailureIsNotPresentedAsNetworkFailure(t *testing.T) {
	model := dashboardModel{
		report: testDashboardReport("ready"),
		actions: DashboardActions{CheckUpdate: func(ctx context.Context) domain.UpdateCheckReport {
			return domain.UpdateCheckReport{
				Operation: "update-check",
				State:     "failed",
				Issues:    []domain.ValidationIssue{{Field: "input", Message: "unsupported flake input"}},
			}
		}},
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	view := updated.(dashboardModel).View().Content
	if !strings.Contains(view, "Update setup needs attention") ||
		!strings.Contains(view, "supported update source from this deployment") ||
		strings.Contains(view, "could not be fetched") {
		t.Fatalf("input failure was presented as a network error:\n%s", view)
	}
}

func TestNixoriumUpdatePartialSaveOffersInPlaceRecovery(t *testing.T) {
	calls := 0
	model := dashboardModel{
		report: testDashboardReport("ready"),
		screen: dashboardUpdate,
		updates: updateModel{
			plan: domain.UpdatePlanReport{
				Operation: "update-plan", State: "ready", Target: "v2.3.0", ReviewToken: "sha256:review",
			},
			result: domain.UpdateApplyReport{
				Operation: "update-save", State: "partial", Target: "v2.3.0", Updated: true,
				RecoveryRequired: true, Message: "The update files are ready, but saving needs recovery.",
			},
		},
		actions: DashboardActions{SaveUpdate: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
			calls++
			return domain.UpdateApplyReport{Revision: strings.Repeat("c", 40), Operation: "update-save", State: "saved", Target: plan.Target, Updated: true, Message: "Nixorium update saved locally."}
		}, PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
			return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: strings.Repeat("c", 40)}
		}, ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			return domain.ControllerRebuildExecutionReport{Revision: strings.Repeat("c", 40), Operation: "controller-apply", State: "completed", Phase: domain.ControllerRebuildPhaseComplete, Applied: true, Verified: true}
		}},
	}
	if !strings.Contains(model.View().Content, "Complete save") || strings.Contains(model.View().Content, "review Git changes") {
		t.Fatalf("partial save does not expose bounded recovery:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	if command == nil || !model.updates.applying || !strings.Contains(model.View().Content, "Completing the local update save") {
		t.Fatalf("recovery did not start in place: %+v", model)
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("recovered update did not start controller activation")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if calls != 1 || model.updates.applying || model.updates.result.State != "saved" || !strings.Contains(model.View().Content, "Nixorium and this controller are updated") {
		t.Fatalf("recovery result missing: calls=%d\n%s", calls, model.View().Content)
	}
}

func TestUpdateDiscoverySelectsStableOrCurrentTarget(t *testing.T) {
	for _, test := range []struct {
		name   string
		report domain.UpdateCheckReport
		target string
	}{
		{"stable", domain.UpdateCheckReport{State: "available", Development: []domain.UpdateRelease{{Tag: "master"}}, Stable: []domain.UpdateRelease{{Tag: "v2.0.1"}, {Tag: "v2.0.0"}}}, "v2.0.1"},
		{"current", domain.UpdateCheckReport{State: "available", CurrentRef: "v2.1.0-beta.1", CurrentChannel: domain.UpdateChannelPrerelease, Development: []domain.UpdateRelease{{Tag: "master"}}, Prerelease: []domain.UpdateRelease{{Tag: "v2.1.0-beta.1"}}}, "v2.1.0-beta.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := dashboardModel{}
			updated, _ := model.Update(dashboardUpdateCheckMsg{report: test.report})
			model = updated.(dashboardModel)
			releases := model.availableUpdateReleases()
			if releases[model.updates.cursor].Tag != test.target {
				t.Fatalf("selected %s, want %s", releases[model.updates.cursor].Tag, test.target)
			}
		})
	}
}
