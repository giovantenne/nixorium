package presentation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestPXEHasOneEntryAndStateSpecificPrimaryAction(t *testing.T) {
	if len(installationAreaTasks) != 2 || installationAreaTasks[0].shortcut != "p" || installationAreaTasks[1].shortcut != "u" {
		t.Fatal("Installation must offer only PXE and USB methods")
	}
	for _, tc := range []struct {
		mode  string
		ready bool
		want  string
	}{
		{"stopped", false, "configure"}, {"ready", true, "start-review"},
		{"active", true, "stop"}, {"recovery-required", true, "recover"}, {"degraded", false, "recover"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			called := ""
			report := testDashboardReport(tc.mode)
			report.PXEPreparation.Ready = tc.ready
			m := experienceFixture(t, 2)
			m.screen = dashboardInstallationArea
			m.actions = DashboardActions{
				LoadRemoteInstall: func(ctx context.Context) (domain.RemoteInstallResponse, error) {
					t.Fatal("PXE must not wait for the USB worker")
					return domain.RemoteInstallResponse{}, nil
				},
				Refresh: func(ctx context.Context) (domain.StatusReport, error) { return report, nil },
				LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) {
					called = "configure"
					return wizardSettings(), nil
				},
				PlanPXEStart: func(ctx context.Context) domain.PXELifecycleReport {
					called = "start-review"
					return domain.PXELifecycleReport{State: "ready"}
				},
				StartPXE: func() domain.PXELifecycleReport {
					t.Fatal("entry/primary action bypassed typed start confirmation")
					return domain.PXELifecycleReport{}
				},
				StopPXE:    func() domain.PXELifecycleReport { called = "stop"; return domain.PXELifecycleReport{} },
				RecoverPXE: func() domain.PXELifecycleReport { called = "recover"; return domain.PXELifecycleReport{} },
			}
			next, command := m.Update(tea.KeyPressMsg{Text: "p"})
			m = next.(dashboardModel)
			if command == nil || called != "" {
				t.Fatal("entry must only observe current state")
			}
			next, _ = m.Update(command())
			m = next.(dashboardModel)
			if m.screen != dashboardPXE || !strings.Contains(m.View().Content, m.pxePrimaryAction().label) {
				t.Fatal("missing primary action")
			}
			next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if command == nil {
				t.Fatal("Enter did not schedule the visible action")
			}
			next, _ = next.Update(command())
			if called != tc.want {
				t.Fatalf("called %q, want %q", called, tc.want)
			}
			if tc.want == "start-review" && next.(dashboardModel).screen != dashboardPXEStartReview {
				t.Fatal("start must open the existing review")
			}
		})
	}
}

func TestPXEFailedObservationCannotActOnPreviousReadiness(t *testing.T) {
	m := experienceFixture(t, 2)
	m.screen = dashboardInstallationArea
	m.report.PXEPreparation.Ready = true
	m.actions.Refresh = func(ctx context.Context) (domain.StatusReport, error) {
		return domain.StatusReport{}, errors.New("unavailable")
	}
	next, command := m.Update(tea.KeyPressMsg{Text: "p"})
	next, _ = next.Update(command())
	m = next.(dashboardModel)
	if !m.installation.stateError || !strings.Contains(m.View().Content, "Retry status") {
		t.Fatal("observation failure is not actionable")
	}
	for _, key := range []string{"p", "s", "x", "n"} {
		_, command = m.Update(tea.KeyPressMsg{Text: key})
		if command != nil {
			t.Fatalf("stale state authorized %s", key)
		}
	}
	_, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("retry missing")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next.(dashboardModel).screen != dashboardInstallationArea {
		t.Fatal("lost Installation parent")
	}
}

func TestDashboardOffersPXEWorkflowFromReconciledState(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready")}
	view := model.View().Content
	if !strings.Contains(view, "Installation") || !strings.Contains(view, "Laboratory overview") {
		t.Fatalf("dashboard omits PXE workflow:\n%s", view)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	view = model.View().Content
	if model.screen != dashboardPXE || !strings.Contains(demoANSI.ReplaceAllString(view, ""), "Installation files    ✓ ready") || !strings.Contains(view, "Next: start network installation") {
		t.Fatalf("PXE screen is incomplete:\n%s", view)
	}
}

func TestReinstallReviewsConsequencesBeforeLeavingPXEActive(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("active"), screen: dashboardPXE}
	updated, command := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXELeaveReview || !strings.Contains(model.View().Content, "Enter closes Nixorium") {
		t.Fatalf("reinstall quit bypassed the active-PXE review:\n%s", model.View().Content)
	}
}

func TestPXEScreenRecommendsOnlyTheObservedNextStage(t *testing.T) {
	report := testDashboardReport("ready")
	report.PXEPreparation = domain.PXEPreparationState{}
	view := (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	if !strings.Contains(view, "Next: prepare the installation files") || strings.Contains(view, "start network installation") {
		t.Fatalf("unprepared PXE guidance is ambiguous:\n%s", view)
	}

	report = testDashboardReport("active")
	view = (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	for _, expected := range []string{"Next: install computers", "guided installer opens automatically", "Finish installation", "stop network installation"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("active PXE guidance omits %q:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "p prepare") || strings.Contains(view, "s start PXE") {
		t.Fatalf("active PXE guidance offers invalid setup actions:\n%s", view)
	}

	report.PXE.Mode = "recovery-required"
	view = (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	if !strings.Contains(view, "Recover the controller network") || !strings.Contains(view, "What Nixorium found") || !strings.Contains(view, "What you can do") || strings.Contains(view, "start PXE") {
		t.Fatalf("recovery guidance is ambiguous:\n%s", view)
	}
}

func TestDashboardPXEStartIsConfirmedWithEnter(t *testing.T) {
	starts := 0
	actions := DashboardActions{
		Refresh: func(ctx context.Context) (domain.StatusReport, error) { return testDashboardReport("active"), nil },
		PlanPXEStart: func(ctx context.Context) domain.PXELifecycleReport {
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
	if model.screen != dashboardPXEStartReview || !strings.Contains(model.View().Content, "laboratory address (10.0.0.99/24) is paused") {
		t.Fatalf("start review not shown:\n%s", model.View().Content)
	}

	if view := model.View().Content; !strings.Contains(view, "Enter starts network installation mode.") || strings.Contains(view, "Type START") || !strings.Contains(view, "Start PXE") {
		t.Fatalf("start review does not say what Enter does:\n%s", view)
	}
	// Esc cancels without touching the network.
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if command != nil || starts != 0 || model.screen != dashboardPXE {
		t.Fatalf("cancelled start changed something: starts=%d screen=%d", starts, model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)

	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if starts != 1 || model.report.PXE.Mode != "active" || !strings.Contains(model.View().Content, "PXE active") {
		t.Fatalf("confirmed start did not refresh state: starts=%d view=%s", starts, model.View().Content)
	}
}

func TestDashboardPXEStartReviewIgnoresTypedWords(t *testing.T) {
	starts := 0
	model := dashboardModel{screen: dashboardPXEStartReview, actions: DashboardActions{
		StartPXE: func() domain.PXELifecycleReport { starts++; return domain.PXELifecycleReport{Message: "PXE active"} },
	}}
	for _, key := range []tea.KeyPressMsg{{Text: "START"}, {Code: tea.KeySpace}, {Text: "PXE"}} {
		updated, _ := model.Update(key)
		model = updated.(dashboardModel)
	}
	if model.confirmation != "" || starts != 0 || model.screen != dashboardPXEStartReview {
		t.Fatalf("typed text was kept or acted on: confirmation=%q starts=%d", model.confirmation, starts)
	}
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter did not start the reviewed PXE mode")
	}
}

func TestDashboardPXEPrepareStopAndRecoverUseCallbacks(t *testing.T) {
	called := ""
	actions := DashboardActions{
		Refresh: func(ctx context.Context) (domain.StatusReport, error) { return testDashboardReport("ready"), nil },
		LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) {
			called = "configure"
			return wizardSettings(), nil
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
	for key, expected := range map[string]string{"p": "configure", "x": "stop", "n": "recover"} {
		called = ""
		mode := "active"
		if key == "p" {
			mode = "stopped"
		}
		if key == "n" {
			mode = "recovery-required"
		}
		model := dashboardModel{report: testDashboardReport(mode), actions: actions, screen: dashboardPXE}
		updated, command := model.Update(tea.KeyPressMsg{Text: key})
		model = updated.(dashboardModel)
		if command == nil {
			t.Fatalf("%s did not schedule an operation", key)
		}
		message := command()
		_, _ = model.Update(message)
		if called != expected {
			t.Errorf("%s called %q, want %q", key, called, expected)
		}
	}
}

func TestDashboardPXEHidesAndIgnoresRecoveryInNormalStates(t *testing.T) {
	called := false
	model := dashboardModel{
		report: testDashboardReport("ready"),
		screen: dashboardPXE,
		actions: DashboardActions{RecoverPXE: func() domain.PXELifecycleReport {
			called = true
			return domain.PXELifecycleReport{}
		}},
	}
	if strings.Contains(model.View().Content, "Recover") {
		t.Fatalf("normal PXE state offers recovery:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command != nil || called || model.busy != "" {
		t.Fatalf("hidden recovery shortcut ran in a normal state")
	}
}

func TestDashboardPXEProgressShowsPhaseBarAndRecentActivity(t *testing.T) {
	started := time.Now().UTC().Add(-3 * time.Second)
	model := dashboardModel{
		report: testDashboardReport("ready"),
		screen: dashboardPXE,
		busy:   "Preparing netboot artifacts and client closures",
		installation: installationModel{
			pxePreparing:  true,
			pxeStarted:    started,
			pxeProgressID: 7,
		},
		width: 100,
		actions: DashboardActions{
			LoadPXEProgress: func() (domain.OperationProgress, error) {
				return domain.OperationProgress{}, nil
			},
		},
	}
	updated, command := model.Update(dashboardPXEProgressMsg{
		id: 7,
		progress: domain.OperationProgress{
			SchemaVersion: domain.OperationProgressSchemaVersion,
			Operation:     "pxe-prepare",
			State:         "running",
			Phase:         "clients",
			StartedAt:     started,
			UpdatedAt:     time.Now().UTC(),
			Current:       2,
			Total:         10,
			Recent:        []string{"Built netboot kernel (1/4)", "Built client pc02 (2/10)"},
		},
	})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("running preparation did not schedule the next progress poll")
	}
	view := model.View().Content
	for _, expected := range []string{"Building client systems", "2/10", "Progress details", "Built client pc02 (2/10)", "Elapsed:"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("PXE progress omits %q:\n%s", expected, view)
		}
	}
	if strings.Count(view, "Progress details") != 1 || strings.Contains(view, "l progress details") {
		t.Fatalf("PXE progress duplicates the contextual action bar:\n%s", view)
	}
	if strings.Contains(view, "journalctl") {
		t.Fatalf("PXE progress exposes journal access instead of typed progress:\n%s", view)
	}
}

func TestDashboardPXEProgressIgnoresRecordFromPreviousRun(t *testing.T) {
	started := time.Now().UTC()
	model := dashboardModel{
		installation: installationModel{
			pxePreparing:  true,
			pxeStarted:    started,
			pxeProgressID: 9,
		},
		actions: DashboardActions{LoadPXEProgress: func() (domain.OperationProgress, error) {
			return domain.OperationProgress{}, nil
		}},
	}
	updated, command := model.Update(dashboardPXEProgressMsg{
		id: 9,
		progress: domain.OperationProgress{
			Operation: "pxe-prepare",
			StartedAt: started.Add(-time.Second),
			UpdatedAt: started.Add(-500 * time.Millisecond),
			Recent:    []string{"Old preparation"},
		},
	})
	model = updated.(dashboardModel)
	if model.installation.pxeProgress.Operation != "" {
		t.Fatalf("stale progress was rendered: %+v", model.installation.pxeProgress)
	}
	if command == nil {
		t.Fatal("stale progress stopped polling for the current run")
	}
}

func TestPXEScreenShowsTheAddressItServes(t *testing.T) {
	report := testDashboardReport("active")
	report.Meta.Controller.DHCPIP = "192.168.56.99"
	report.PXEPreparation.Ready, report.PXEPreparation.DHCPAddress = true, "192.168.57.99"
	view := (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	if !strings.Contains(view, "192.168.57.99") || strings.Contains(view, "192.168.56.99") {
		t.Fatalf("PXE header shows the saved hint instead of the served address:\n%s", view)
	}
}

func TestFinishedNetworkInstallationPointsToTheInstalledComputers(t *testing.T) {
	report := testDashboardReport("stopped")
	loads := 0
	model := dashboardModel{report: report, screen: dashboardPXE, width: 120, height: 40, actions: DashboardActions{LoadHosts: func(context.Context) (domain.HostsReport, error) {
		loads++
		return domain.HostsReport{}, nil
	}}}
	model.installation.pxeResult = domain.PXELifecycleReport{Operation: "pxe-stop", State: "completed"}
	view := model.View().Content
	if !strings.Contains(view, "Next: check the installed computers") || !strings.Contains(view, "Check computers") || strings.Contains(view, "Next: start network installation") {
		t.Fatalf("finished installation guidance:\n%s", view)
	}
	updated, read := model.Update(demoText("h"))
	if next := updated.(dashboardModel); next.screen != dashboardHosts || read == nil {
		t.Fatalf("h opened %v", next.screen)
	}
}

func TestPXEScreenNeverShowsTheAddressPlaceholder(t *testing.T) {
	report := testDashboardReport("ready")
	report.Meta.Controller.DHCPIP = domain.MasterDHCPPlaceholder
	report.PXEPreparation = domain.PXEPreparationState{}
	view := (dashboardModel{report: report, screen: dashboardPXE}).View().Content
	if strings.Contains(view, domain.MasterDHCPPlaceholder) || !strings.Contains(view, "not set yet") {
		t.Fatalf("placeholder shown:\n%s", view)
	}
}

func TestActivePXEExitUsesOneGlobalReview(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		screen dashboardScreen
		key    tea.KeyPressMsg
	}{
		{name: "home q", screen: dashboardHome, key: tea.KeyPressMsg{Text: "q"}},
		{name: "PXE q", screen: dashboardPXE, key: tea.KeyPressMsg{Text: "q"}},
		{name: "setup ctrl-c", screen: dashboardSetup, key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			model := experienceFixture(t, 2)
			model.report.PXE.Mode = "active"
			model.screen = scenario.screen

			updated, command := model.Update(scenario.key)
			model = updated.(dashboardModel)
			if command != nil || model.screen != dashboardPXELeaveReview {
				t.Fatalf("exit bypassed active-PXE review: screen=%d", model.screen)
			}
			view := model.View().Content
			if !strings.Contains(view, "Stop installation mode") || !strings.Contains(view, "Enter closes Nixorium") {
				t.Fatalf("exit choices are incomplete:\n%s", view)
			}
		})
	}
}

func TestActivePXELeaveReviewCannotBeBypassedWithControlC(t *testing.T) {
	model := experienceFixture(t, 2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview

	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXELeaveReview {
		t.Fatalf("control-c bypassed protected PXE exit: screen=%d", model.screen)
	}
}

func TestActivePXECanBeStoppedAndVerifiedBeforeExit(t *testing.T) {
	stops := 0
	model := experienceFixture(t, 2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.actions.StopPXE = func() domain.PXELifecycleReport {
		stops++
		return domain.PXELifecycleReport{State: "stopped", Mode: "stopped", Message: "Installation mode stopped."}
	}
	model.actions.Refresh = func(ctx context.Context) (domain.StatusReport, error) {
		return testDashboardReport("stopped"), nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatal("stop-and-exit did not start")
	}
	updated, quit := model.Update(command())
	model = updated.(dashboardModel)
	if stops != 1 || quit == nil {
		t.Fatalf("stop-and-exit did not finish: stops=%d", stops)
	}
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("verified PXE stop did not quit")
	}
	if model.report.PXE.Mode != "stopped" {
		t.Fatalf("final PXE state = %q", model.report.PXE.Mode)
	}
}

func TestActivePXEStopFailureKeepsDashboardOpen(t *testing.T) {
	model := experienceFixture(t, 2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.actions.StopPXE = func() domain.PXELifecycleReport {
		return domain.PXELifecycleReport{State: "failed", Mode: "active", Message: "network service did not stop"}
	}
	model.actions.Refresh = func(ctx context.Context) (domain.StatusReport, error) {
		return domain.StatusReport{}, errors.New("unavailable")
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "x"})
	model = updated.(dashboardModel)
	updated, quit := model.Update(command())
	model = updated.(dashboardModel)
	if quit != nil || model.screen != dashboardPXELeaveReview || !strings.Contains(model.message, "could not be stopped") {
		t.Fatalf("failed stop closed or lost recovery: %+v", model)
	}
}

func TestPXEBusyQuitDescribesContinuingWork(t *testing.T) {
	model := dashboardModel{busy: "Preparing", installation: installationModel{pxePreparing: true}}
	// Test actions directly so narrow responsive labels cannot hide the meaning.
	for _, action := range model.pxeActions() {
		if action.key == "q" && strings.Contains(action.label, "Quit Nixorium") && strings.Contains(action.label, "work continues") {
			return
		}
	}
	t.Fatal("PXE detach does not describe its effect")
}
