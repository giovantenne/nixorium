package presentation

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSetupRendersBusyActivity(t *testing.T) {
	model := experienceFixture(2)
	model.screen = dashboardSetup
	model.setupMode = true
	model.busy = "Refreshing first-run progress"

	if view := model.View().Content; !strings.Contains(view, model.busy) {
		t.Fatalf("setup hid its active operation:\n%s", view)
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
			model := experienceFixture(2)
			model.report.PXE.Mode = "active"
			model.screen = scenario.screen

			updated, command := model.Update(scenario.key)
			model = updated.(dashboardModel)
			if command != nil || model.screen != dashboardPXELeaveReview {
				t.Fatalf("exit bypassed active-PXE review: screen=%d", model.screen)
			}
			view := model.View().Content
			if !strings.Contains(view, "Stop installation mode") || !strings.Contains(view, "LEAVE PXE ACTIVE") {
				t.Fatalf("exit choices are incomplete:\n%s", view)
			}
		})
	}
}

func TestActivePXELeaveReviewCannotBeBypassedWithControlC(t *testing.T) {
	model := experienceFixture(2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.confirmation = "LEAVE PXE"

	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXELeaveReview || model.confirmation != "LEAVE PXE" {
		t.Fatalf("control-c bypassed protected PXE exit: screen=%d confirmation=%q", model.screen, model.confirmation)
	}
}

func TestActivePXECanBeStoppedAndVerifiedBeforeExit(t *testing.T) {
	stops := 0
	model := experienceFixture(2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.actions.StopPXE = func() domain.PXELifecycleReport {
		stops++
		return domain.PXELifecycleReport{State: "stopped", Mode: "stopped", Message: "Installation mode stopped."}
	}
	model.actions.Refresh = func() (domain.StatusReport, error) {
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
	model := experienceFixture(2)
	model.report.PXE.Mode = "active"
	model.screen = dashboardPXELeaveReview
	model.actions.StopPXE = func() domain.PXELifecycleReport {
		return domain.PXELifecycleReport{State: "failed", Mode: "active", Message: "network service did not stop"}
	}
	model.actions.Refresh = func() (domain.StatusReport, error) {
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

func TestCancelledSoftwareRemovalReturnsToSelectedPackage(t *testing.T) {
	model := experienceFixture(2)
	model.screen = dashboardSoftware
	model.softwareCatalog = testSoftwareCatalogReport()
	model.softwareCursor = 1
	model.actions.PlanSoftware = func(request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
		return domain.SoftwareChangePlanReport{State: "ready", Request: request, Confirmation: "SAVE SOFTWARE token"}
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	if model.screen != dashboardSoftware || model.softwareSelected != "vlc" || model.softwareCursor != 1 {
		t.Fatalf("removal cancellation lost its origin: screen=%d selected=%q cursor=%d", model.screen, model.softwareSelected, model.softwareCursor)
	}
	if strings.Contains(model.View().Content, "Add VLC") {
		t.Fatalf("removal cancellation opened an add flow:\n%s", model.View().Content)
	}
}

func TestRoutineFlowsStartWithoutStaleResults(t *testing.T) {
	model := experienceFixture(2)
	model.updateResult = domain.UpdateApplyReport{Operation: "update-apply", State: "applied"}
	model.actions.CheckUpdate = func() domain.UpdateCheckReport {
		return domain.UpdateCheckReport{Operation: "update-check", State: "current", CurrentRef: "v2.0.0"}
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "u"})
	model = updated.(dashboardModel)
	if model.updateResult.Operation != "" || command == nil {
		t.Fatal("Update retained a result from the previous session")
	}

	model.screen = dashboardHome
	model.busy = ""
	model.settingsResult = domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved"}
	model.actions.LoadSettings = func() (domain.LabSettingsFile, error) { return domain.LabSettingsFile{}, nil }
	updated, _ = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if model.settingsResult.Operation != "" || command == nil {
		t.Fatal("Settings retained a result from the previous session")
	}
}

func TestIncompleteInitialConfigurationOpensSetupAndRemainsReachable(t *testing.T) {
	setup := domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}
	model := newDashboardModel(testDashboardReport("stopped"), setup, DashboardActions{}, false)
	if model.screen != dashboardSetup || !model.setupMode {
		t.Fatalf("incomplete initial configuration opened screen=%d setupMode=%t", model.screen, model.setupMode)
	}

	ready := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "ready"}, DashboardActions{}, false)
	found := false
	for _, task := range dashboardTasks {
		if task.id == "installation" && task.shortcut == "n" {
			found = true
		}
	}
	if !found || !strings.Contains(ready.View().Content, "Installation") {
		t.Fatal("configured labs cannot reopen computer installation from the intervention menu")
	}

	maintenance := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}, DashboardActions{}, false)
	if maintenance.screen != dashboardHome || !strings.Contains(maintenance.View().Content, "Computer installation is not configured yet") {
		t.Fatalf("a later readiness issue hijacked navigation or became invisible: screen=%d\n%s", maintenance.screen, maintenance.View().Content)
	}
}

func TestOpeningCachedSetupDoesNotRefreshIt(t *testing.T) {
	loads := 0
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}, DashboardActions{
		LoadSetup: func() domain.SetupReport {
			loads++
			return domain.SetupReport{State: "ready"}
		},
	}, false)
	for index, task := range dashboardTasks {
		if task.id == "installation" {
			model.homeMenu.list.Select(index)
			break
		}
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardInstallationArea {
		t.Fatalf("installation area did not open: screen=%d", model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || loads != 0 || model.screen != dashboardSetup {
		t.Fatalf("opening cached setup triggered a refresh: loads=%d screen=%d", loads, model.screen)
	}
}

func TestInstallNewComputersConvertsControllerModeThroughOneNetworkForm(t *testing.T) {
	settings := wizardSettings()
	settings.Lab.DeploymentMode = "controller"
	settings.Lab.PCCount = 0
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}, DashboardActions{
		LoadSettings: func() (domain.LabSettingsFile, error) { return settings, nil },
	}, false)
	for index, task := range dashboardTasks {
		if task.id == "installation" {
			model.homeMenu.list.Select(index)
			break
		}
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardInstallationArea {
		t.Fatalf("installation area did not open: screen=%d", model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || !model.startingLabSetup {
		t.Fatal("computer installation settings did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || model.settingsEditor.settings.Lab.DeploymentMode != "laboratory" || model.settingsEditor.settings.Lab.PCCount != 20 || len(model.settingsEditor.fields) != len(clientSetupFields) {
		t.Fatalf("client setup editor = %+v", model.settingsEditor)
	}
	view := model.View().Content
	if !strings.Contains(view, "Install new computers / Laboratory network") || strings.Contains(view, "Teacher user name") || strings.Contains(view, "Time zone") {
		t.Fatalf("client setup repeats controller choices:\n%s", view)
	}
}

func TestSetupEditsConfigurationWithoutLeavingTheTUI(t *testing.T) {
	loaded := 0
	model := experienceFixture(2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}
	model.actions.LoadSettings = func() (domain.LabSettingsFile, error) {
		loaded++
		return wizardSettings(), nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSettings || model.settingsReturn != dashboardSetup {
		t.Fatalf("setup did not open in-TUI settings: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loaded != 1 || model.screen != dashboardSettingsEdit || len(model.settingsEditor.fields) != len(settingsFields) {
		t.Fatalf("setup did not open one complete settings sequence: loaded=%d screen=%d fields=%d", loaded, model.screen, len(model.settingsEditor.fields))
	}
	if view := model.View().Content; !strings.Contains(view, "First setup / Laboratory settings") || !strings.Contains(view, "one complete validation") || strings.Contains(view, "exit and run") {
		t.Fatalf("setup settings are not a continuous English flow:\n%s", view)
	}
}

func TestSetupCredentialsFollowTheSameSettingsAndPasswordSequence(t *testing.T) {
	model := experienceFixture(2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageCredentials}
	model.actions.LoadSettings = func() (domain.LabSettingsFile, error) { return wizardSettings(), nil }
	model.actions.ChangePassword = func(string, domain.LabSettingsFile, *os.File, io.Writer) (domain.LabSettingsFile, error) {
		return domain.LabSettingsFile{}, nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || len(model.settingsEditor.fields) != len(settingsFields) {
		t.Fatalf("credential stage did not start the unified settings sequence: screen=%d fields=%d", model.screen, len(model.settingsEditor.fields))
	}

	model.settingsEditor.index = len(model.settingsEditor.fields) - 1
	model.settingsEditor.prepareCurrentField()
	updated, passwordCommand := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsPasswords || passwordCommand == nil || !model.settingsPasswordMenu.initialized {
		t.Fatalf("completed settings did not open one protected password session: screen=%d initialized=%t", model.screen, model.settingsPasswordMenu.initialized)
	}
	updated, _ = model.Update(struct{}{})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsPasswords {
		t.Fatalf("resume event left the protected password step: screen=%d", model.screen)
	}
}

func TestSetupValidationRetryKeepsAcceptedPasswords(t *testing.T) {
	plans := 0
	model := experienceFixture(2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageValidate}
	model.actions.LoadSettings = func() (domain.LabSettingsFile, error) { return wizardSettings(), nil }
	model.actions.PlanSettings = func(domain.LabSettingsFile) domain.ConfigPlanReport {
		plans++
		return domain.ConfigPlanReport{Operation: "config-plan", State: "unchanged"}
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.settingsCollectPasswords {
		t.Fatal("validation retry would collect already accepted passwords again")
	}
	model.settingsEditor.index = len(model.settingsEditor.fields) - 1
	model.settingsEditor.prepareCurrentField()
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.screen == dashboardSettingsPasswords || model.busy == "" {
		t.Fatalf("validation retry did not proceed directly to one plan: screen=%d busy=%q", model.screen, model.busy)
	}
	updated, _ = model.Update(command())
	if plans != 1 {
		t.Fatalf("validation plans = %d, want 1", plans)
	}
}

func TestSetupPreparesSavesAndInstallsKeysThroughTypedActions(t *testing.T) {
	reconciles, saves, installs, refreshes := 0, 0, 0, 0
	model := experienceFixture(2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}
	model.actions.LoadSetupKeys = func() domain.KeyReconcileReport {
		return domain.KeyReconcileReport{Operation: "setup-keys-verify", State: "action-required", Keys: []domain.KeyMaterialState{
			{Name: "cache", Problem: "missing"}, {Name: "ssh", Problem: "missing"}, {Name: "veyon", Problem: "missing"},
		}}
	}
	model.actions.ReconcileSetupKeys = func() (domain.KeyReconcileReport, error) {
		reconciles++
		return domain.KeyReconcileReport{Operation: "setup-keys", State: "ready"}, nil
	}
	model.actions.SaveSetupConfiguration = func() domain.ConfigurationSaveReport {
		saves++
		return domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved"}
	}
	model.actions.InstallSetupSecrets = func() domain.ActionReport {
		installs++
		return domain.ActionReport{Operation: "setup-install-secrets", State: "completed", Message: "installed"}
	}
	model.actions.LoadSetup = func() domain.SetupReport {
		refreshes++
		return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageReview}
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSetupKeys || !strings.Contains(model.View().Content, "import an existing private key") {
		t.Fatalf("key choices are not explicit:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, refresh := model.Update(command())
	model = updated.(dashboardModel)
	if refresh == nil {
		t.Fatal("successful key preparation did not refresh setup")
	}
	updated, _ = model.Update(refresh())
	model = updated.(dashboardModel)
	if reconciles != 1 || saves != 1 || installs != 1 || refreshes != 1 || model.setup.CurrentStage != domain.SetupStageReview {
		t.Fatalf("key continuation failed: reconcile=%d save=%d install=%d refresh=%d setup=%+v", reconciles, saves, installs, refreshes, model.setup)
	}
	if !strings.Contains(model.message, "keys are ready") {
		t.Fatalf("key result is unclear: %q", model.message)
	}
}

func TestSetupImportsSelectedExistingKeyWithoutExposingMaterial(t *testing.T) {
	importedName, importedPath := "", ""
	model := experienceFixture(2)
	model.screen = dashboardSetupKeys
	model.setupKeyCursor = 1
	model.setupKeys = domain.KeyReconcileReport{State: "action-required", Keys: []domain.KeyMaterialState{
		{Name: "cache", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
		{Name: "ssh", Problem: "private and public keys are missing"},
		{Name: "veyon", Problem: "private and public keys are missing"},
	}}
	model.actions.ImportSetupKey = func(name, path string) (domain.KeyImportReport, error) {
		importedName, importedPath = name, path
		return domain.KeyImportReport{Operation: "setup-key-import", State: "imported", Key: name, Source: path, Fingerprint: "SHA256:verified", Message: "Existing key imported and verified."}, nil
	}
	model.actions.LoadSetupKeys = func() domain.KeyReconcileReport {
		return domain.KeyReconcileReport{State: "action-required", Keys: []domain.KeyMaterialState{{Name: "ssh", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true}}}
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "i"})
	model = updated.(dashboardModel)
	if !model.setupKeyImporting || !strings.Contains(model.View().Content, "leaves the source unchanged") {
		t.Fatalf("import explanation missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "/secure/existing-admin-key"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if importedName != "ssh" || importedPath != "/secure/existing-admin-key" || !strings.Contains(model.message, "SHA256:verified") || strings.Contains(model.View().Content, "private-material") {
		t.Fatalf("import result is unclear or unsafe: name=%q path=%q message=%q\n%s", importedName, importedPath, model.message, model.View().Content)
	}
}

func TestLoadingDashboardRendersBeforeInspectionAndThenRoutes(t *testing.T) {
	loads := 0
	setup := domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		LoadInitial: func() (domain.StatusReport, domain.SetupReport, error) {
			loads++
			return testDashboardReport("stopped"), setup, nil
		},
	}, false)
	model.initializing = true
	model.busy = "Opening the laboratory and checking setup progress"
	if view := model.View().Content; !strings.Contains(view, model.busy) || strings.Contains(view, "Restore computers") || strings.Contains(view, "Laboratory overview") {
		t.Fatalf("startup activity is not visible before inspection:\n%s", view)
	}

	updated, command := model.Update(model.loadInitial()())
	model = updated.(dashboardModel)
	if command != nil || loads != 1 || model.initializing || model.screen != dashboardSetup || !model.setupMode {
		t.Fatalf("initial result did not route to setup: loads=%d model=%+v", loads, model)
	}
}

func TestLoadingDashboardFailureHasInPlaceRetry(t *testing.T) {
	attempts := 0
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		LoadInitial: func() (domain.StatusReport, domain.SetupReport, error) {
			attempts++
			if attempts == 1 {
				return domain.StatusReport{}, domain.SetupReport{}, errors.New("evaluation unavailable")
			}
			return testDashboardReport("stopped"), domain.SetupReport{State: "ready"}, nil
		},
	}, false)
	model.initializing = true
	model.busy = "Opening the laboratory and checking setup progress"

	updated, _ := model.Update(model.loadInitial()())
	model = updated.(dashboardModel)
	failureView := model.View().Content
	if !model.initialError || !strings.Contains(failureView, "Enter") || !strings.Contains(failureView, "Try again") {
		t.Fatalf("startup failure has no recovery:\n%s", model.View().Content)
	}
	updated, retry := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if retry == nil || !model.initializing {
		t.Fatal("startup retry did not begin")
	}
	updated, _ = model.Update(retry())
	model = updated.(dashboardModel)
	if attempts != 2 || model.initialError || model.screen != dashboardHome || model.busy != "" {
		t.Fatalf("startup retry did not recover: attempts=%d model=%+v", attempts, model)
	}
}
