package presentation

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func wizardSettings() domain.LabSettingsFile {
	return domain.LabSettingsFile{
		SchemaVersion: domain.SettingsSchemaVersion,
		Lab: domain.LabSettings{
			MasterDHCPIP: "192.0.2.10", NetworkBase: "10.0.0.0", NetworkPrefix: 24,
			PCCount: 20, MasterHostNumber: 99, InterfaceName: "eth0", TeacherUser: "teacher", StudentUser: "student",
			TeacherPassword: "$6$salt$teacher", StudentPassword: "$6$salt$student", AdminPassword: "$6$salt$admin",
			HomepageURL: "https://example.org", StudentGitName: "Student", StudentGitEmail: "student@example.org",
			AdminGitName: "Admin", AdminGitEmail: "admin@example.org", TimeZone: "Europe/Rome",
			DefaultLocale: "en_US.UTF-8", ExtraLocale: "it_IT.UTF-8", KeyboardLayout: "it", ConsoleKeyMap: "it2",
		},
	}
}

func TestSettingsWizardPreservesValuesWhenNavigatingBack(t *testing.T) {
	model := newSettingsWizardModel(wizardSettings())
	index := settingsFieldIndex("lab.networkBase")
	model = model.moveToField(index)
	model.drafts[index] = "10.1.0.0"
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	model = updated.(settingsWizardModel)
	if model.index != index || model.drafts[index] != "10.1.0.0" || model.settings.Lab.NetworkBase != "10.1.0.0" {
		t.Fatalf("wizard lost state: %+v", model)
	}
}

func settingsFieldIndex(id string) int {
	for index, field := range settingsFields {
		if field.id == id {
			return index
		}
	}
	return -1
}

func TestSettingsWizardCanAcceptAllDefaults(t *testing.T) {
	model := newSettingsWizardModel(wizardSettings())
	model.helpOpen = true
	if !strings.Contains(model.View().Content, "All settings are reviewed before saving") {
		t.Fatalf("first-run help is missing:\n%s", model.View().Content)
	}
	model.helpOpen = false
	for range settingsFields {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model = updated.(settingsWizardModel)
	}
	if !model.accepted || model.cancelled {
		t.Fatalf("wizard did not accept valid defaults: %+v", model)
	}
}

func TestInstallationInterfaceFieldEditsClientOverride(t *testing.T) {
	settings := wizardSettings()
	settings.Lab.ControllerInterfaceName = "enp8s0"
	model := newSettingsEditorModel(settings, installationSettingsFields, "Install computers")
	model.helpOpen = true
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	model.helpOpen = false
	if !strings.Contains(view, "Client computers' network interface") || !strings.Contains(view, "Controller network interface: enp8s0") {
		t.Fatalf("interface roles are unclear: %s", view)
	}
	model.drafts[0] = "enp2s0"
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	if model.settings.Lab.ClientInterfaceName != "enp2s0" || model.settings.Lab.ControllerInterfaceName != "enp8s0" || model.settings.Lab.InterfaceName != "eth0" {
		t.Fatalf("client question changed another interface: %+v", model.settings.Lab)
	}
}

func TestFirstRunOmitsGitIdentityAndGroupsEssentialFields(t *testing.T) {
	groups := map[string]bool{}
	for _, field := range settingsFields {
		groups[field.group] = true
		if strings.Contains(field.id, "Git") {
			t.Fatalf("first-run still asks for Git identity: %+v", field)
		}
	}
	for _, expected := range []string{"Network", "Laboratory", "Accounts", "Regional settings", "Preferences"} {
		if !groups[expected] {
			t.Fatalf("first-run group %q is missing: %v", expected, groups)
		}
	}
}

func TestInstallationSettingsReuseControllerAccountsAndRegionalSettings(t *testing.T) {
	settings := wizardSettings()
	settings.Lab.TeacherUser, settings.Lab.StudentUser = "professor", "pupil"
	model := newSettingsEditorModel(settings, installationSettingsFields, "Install computers")
	for _, field := range model.fields {
		if field.id == "lab.timeZone" || field.id == "lab.keyboardLayout" || field.id == "lab.teacherUser" || field.id == "lab.studentUser" {
			t.Fatalf("installation asks for existing controller setting %q", field.id)
		}
	}
	for range model.fields {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model = updated.(settingsWizardModel)
	}
	if !model.accepted || model.settings.Lab.TimeZone != settings.Lab.TimeZone || model.settings.Lab.KeyboardLayout != settings.Lab.KeyboardLayout || model.settings.Lab.ConsoleKeyMap != settings.Lab.ConsoleKeyMap {
		t.Fatalf("installation did not preserve controller regional settings: %+v", model.settings.Lab)
	}
	if model.settings.Lab.TeacherUser != settings.Lab.TeacherUser || model.settings.Lab.StudentUser != settings.Lab.StudentUser || model.settings.Lab.TeacherPassword != settings.Lab.TeacherPassword || model.settings.Lab.StudentPassword != settings.Lab.StudentPassword {
		t.Fatalf("installation did not preserve saved accounts: %+v", model.settings.Lab)
	}
}

func TestRegionalFieldsExposeOnlyTimeZoneAndKeyboard(t *testing.T) {
	if timeZoneChoices[0].value != "America/New_York" || keyboardChoices[0].value != "us" {
		t.Fatalf("regional suggestions do not start with US defaults: timezone=%q keyboard=%q", timeZoneChoices[0].value, keyboardChoices[0].value)
	}
	regional := []string{}
	for _, field := range settingsFields {
		if field.group == "Regional settings" {
			regional = append(regional, field.id)
		}
	}
	if strings.Join(regional, ",") != "lab.timeZone,lab.keyboardLayout" {
		t.Fatalf("first setup exposes unnecessary regional choices: %v", regional)
	}
	model := newSettingsWizardModel(wizardSettings())
	model = model.moveToField(settingsFieldIndex("lab.keyboardLayout"))
	if !model.selector.FilteringEnabled() || !strings.Contains(model.View().Content, "press / to search") {
		t.Fatalf("keyboard selector is not searchable:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	model = updated.(settingsWizardModel)
	if model.selector.FilterState() != list.Filtering {
		t.Fatalf("slash did not open keyboard filtering: %s", model.selector.FilterState())
	}
	model.selector.SetFilterText("US English")
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	if model.settings.Lab.KeyboardLayout != "us" || model.settings.Lab.ConsoleKeyMap != "us" || model.settings.Lab.DefaultLocale != "en_US.UTF-8" {
		t.Fatalf("selected keyboard did not update its internal console mapping while preserving the US locale: %+v", model.settings.Lab)
	}
}

func TestRegionalSelectorAllowsValidatedCustomValue(t *testing.T) {
	model := newSettingsWizardModel(wizardSettings())
	index := settingsFieldIndex("lab.timeZone")
	model = model.moveToField(index)
	model.selector.Select(len(model.selector.Items()) - 1)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	if !model.custom || !strings.Contains(model.View().Content, "Custom value") {
		t.Fatalf("custom entry did not open: %+v", model)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	model = updated.(settingsWizardModel)
	updated, _ = model.Update(tea.PasteMsg{Content: "Asia/Tokyo"})
	model = updated.(settingsWizardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	if model.settings.Lab.TimeZone != "Asia/Tokyo" {
		t.Fatalf("custom timezone was not applied: %+v", model.settings.Lab)
	}
}

func TestConfigReviewNeverRendersPasswordHash(t *testing.T) {
	plan := domain.ConfigPlanReport{Changes: []domain.SettingChange{{Field: "lab.adminPassword", Before: "configured", After: "updated", Sensitive: true}}}
	view := (configReviewModel{plan: plan, git: domain.GitState{Dirty: true, Changes: 2, Paths: []string{"keys/admin-ssh.pub", "modules/local.nix"}}}).View()
	if strings.Contains(view.Content, "$6$") || !strings.Contains(view.Content, "Setup-generated") || !strings.Contains(view.Content, "other existing") {
		t.Fatalf("unsafe or incomplete review:\n%s", view.Content)
	}
}

func TestClientInterfaceIsNeverCopiedFromTheControllerCard(t *testing.T) {
	draft := func(settings domain.LabSettingsFile) string {
		model := newSettingsEditorModel(settings, installationSettingsFields, "Install computers")
		for index, field := range model.fields {
			if field.id == "lab.clientIfaceName" {
				return model.drafts[index]
			}
		}
		t.Fatal("installation form has no client interface field")
		return ""
	}
	settings := domain.LabSettingsFile{}
	settings.Lab.DeploymentMode = "controller"
	settings.Lab.InterfaceName = "enp1s0"
	settings.Lab.ControllerInterfaceName = "eno1"
	// Clients are matched by their wired card unless a name is saved; the
	// controller's card name must not become every client's explicit name.
	if got := draft(settings); got != "" {
		t.Fatalf("first laboratory setup proposed %q, want automatic detection", got)
	}
	settings.Lab.DeploymentMode = "laboratory"
	settings.Lab.PCCount = 5
	if got := draft(settings); got != "" {
		t.Fatalf("installed laboratory changed its client fallback to %q", got)
	}
	settings.Lab.ClientInterfaceName = "enp3s0"
	settings.Lab.DeploymentMode = "controller"
	if got := draft(settings); got != "enp3s0" {
		t.Fatalf("saved client card replaced by %q", got)
	}
}

func TestAccountNamesRemainEditableInFirstSetupAndSettings(t *testing.T) {
	var accountFields []settingsField
	for _, group := range routineSettingsGroups {
		if group.id == "accounts" {
			accountFields = group.fields
		}
	}
	for _, fields := range [][]settingsField{settingsFields, accountFields} {
		editor := newSettingsEditorModel(wizardSettings(), fields, "Accounts")
		for i, field := range fields {
			switch field.id {
			case "lab.teacherUser":
				editor.drafts[i] = "professor"
			case "lab.studentUser":
				editor.drafts[i] = "pupil"
			}
		}
		for range fields {
			next, _ := editor.Update(demoCode(tea.KeyEnter))
			editor = next.(settingsWizardModel)
		}
		if !editor.accepted || editor.settings.Lab.TeacherUser != "professor" || editor.settings.Lab.StudentUser != "pupil" {
			t.Fatal("account names cannot be changed in setup or Settings")
		}
	}
}

func TestDashboardGuidesAndResumesFirstSetup(t *testing.T) {
	setup := testSetupReport(false, false, false, false)
	loads := 0
	model := dashboardModel{
		report:    testDashboardReport("ready"),
		setup:     setup,
		screen:    dashboardSetup,
		setupMode: true,
		actions: DashboardActions{
			SaveSetupConfiguration: func() domain.ConfigurationSaveReport {
				return domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved", Message: "Configuration saved locally."}
			},
			LoadSetup: func(ctx context.Context) domain.SetupReport {
				loads++
				return testSetupReport(true, false, false, false)
			},
		},
	}
	if view := model.View().Content; !strings.Contains(view, "Step 1 of 3") || !strings.Contains(view, "Laboratory settings") || !strings.Contains(view, "Save the generated configuration locally") || strings.Contains(view, "› ●") || strings.Contains(view, "! Next step") || !strings.Contains(view, "Continue setup") {
		t.Fatalf("setup progress screen is incomplete:\n%s", model.View().Content)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("setup did not start local save: screen=%d", model.screen)
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSetup {
		t.Fatalf("local save did not refresh setup: screen=%d", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loads != 1 || model.setup.CurrentStage != domain.SetupStageApply || !strings.Contains(model.View().Content, "Review and activate the controller configuration") {
		t.Fatalf("setup did not resume from refreshed state: loads=%d setup=%+v\n%s", loads, model.setup, model.View().Content)
	}
}

func TestCompletedSetupOpensGenericNetworkInstallation(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), setup: testSetupReport(true, true, true, true), setupMode: true, screen: dashboardSetup}
	if !strings.Contains(model.View().Content, "Controller and client systems are ready") || !strings.Contains(model.View().Content, "Any configured computer") {
		t.Fatalf("completed setup omits first installation action:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardPXE || !strings.Contains(model.View().Content, "Install computers") || strings.Contains(strings.ToLower(model.View().Content), "pilot") {
		t.Fatalf("completed setup did not open generic installation:\n%s", model.View().Content)
	}
}

func TestSetupAsksForEnterToLeavePXEActive(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("active"), setupMode: true, screen: dashboardPXE}
	updated, command := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXELeaveReview || !strings.Contains(model.View().Content, "Closing Nixorium will not stop installation mode") {
		t.Fatalf("active PXE quit did not open consequence review:\n%s", model.View().Content)
	}

	if view := model.View().Content; !strings.Contains(view, "Enter closes Nixorium and leaves installation mode active") || strings.Contains(view, "Type LEAVE") {
		t.Fatalf("leave review does not say what Enter does:\n%s", view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXE {
		t.Fatal("Esc did not keep Nixorium open")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	_, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter on the leave review did not quit")
	}
}

func TestSetupRendersBusyActivity(t *testing.T) {
	model := experienceFixture(t, 2)
	model.screen = dashboardSetup
	model.setupMode = true
	model.busy = "Refreshing first-run progress"

	if view := model.View().Content; !strings.Contains(view, model.busy) {
		t.Fatalf("setup hid its active operation:\n%s", view)
	}
}

func TestInstallComputersOpensSettingsWithoutSetupMenu(t *testing.T) {
	setupLoads, settingsLoads := 0, 0
	model := newTestModel(t, withReport(testDashboardReport("stopped")), withSetup(domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}), withActions(DashboardActions{
		LoadSetup: func(ctx context.Context) domain.SetupReport {
			setupLoads++
			return domain.SetupReport{State: "ready"}
		},
		LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) {
			settingsLoads++
			return wizardSettings(), nil
		},
	}))
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
	updated, command = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if command != nil || setupLoads != 0 || model.screen != dashboardPXE {
		t.Fatalf("install did not open method selection: setupLoads=%d screen=%d", setupLoads, model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSettings {
		t.Fatalf("PXE selection did not open common settings: screen=%d", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if settingsLoads != 1 || model.screen != dashboardSettings || !model.installation.savedSummary || strings.Contains(model.View().Content, "Step 1 of 5") {
		t.Fatalf("install exposed the old setup menu: settingsLoads=%d screen=%d\n%s", settingsLoads, model.screen, model.View().Content)
	}
}

func TestInstallNewComputersConvertsControllerModeThroughOneNetworkForm(t *testing.T) {
	settings := wizardSettings()
	settings.Lab.DeploymentMode = "controller"
	settings.Lab.PCCount = 0
	model := newTestModel(t, withReport(testDashboardReport("stopped")), withSetup(domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}), withActions(DashboardActions{
		LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) { return settings, nil },
	}))
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
	updated, command = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardPXE {
		t.Fatal("network installation status did not open")
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if command == nil || !model.installation.startingLabSetup {
		t.Fatal("computer installation settings did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || model.settings.editor.settings.Lab.DeploymentMode != "laboratory" || model.settings.editor.settings.Lab.PCCount != 20 || len(model.settings.editor.fields) != len(installationSettingsFields) {
		t.Fatalf("client setup editor = %+v", model.settings.editor)
	}
	if model.settings.editor.title != "Installation / Laboratory settings" {
		t.Fatalf("complete laboratory settings are not shown: title=%q fields=%+v", model.settings.editor.title, model.settings.editor.fields)
	}
	for _, field := range model.settings.editor.fields {
		if field.id == "lab.timeZone" || field.id == "lab.keyboardLayout" || field.id == "lab.teacherUser" || field.id == "lab.studentUser" {
			t.Fatalf("installation asks for controller regional setting %q", field.id)
		}
	}
	if model.settings.editor.settings.Lab.TimeZone != settings.Lab.TimeZone || model.settings.editor.settings.Lab.KeyboardLayout != settings.Lab.KeyboardLayout {
		t.Fatalf("installation changed controller regional settings: %+v", model.settings.editor.settings.Lab)
	}
}

func TestPXEAndUSBPreparationReuseSavedAccountNames(t *testing.T) {
	for _, method := range []domain.RemoteInstallMethod{"pxe", domain.RemoteInstallUSBSSH} {
		t.Run(string(method), func(t *testing.T) {
			settings := wizardSettings()
			settings.Lab.DeploymentMode = "controller"
			settings.Lab.PCCount = 0
			settings.Lab.TeacherUser, settings.Lab.StudentUser = "professor", "pupil"
			model := experienceFixture(t, 2)
			model.actions.LoadSettings = func(context.Context) (domain.LabSettingsFile, error) { return settings, nil }
			next, command := model.beginComputerInstallation(method)
			model = workspaceComplete(t, next.(dashboardModel), command)
			if model.screen != dashboardSettingsEdit {
				t.Fatal("installation form did not open", model.screen)
			}
			editor := model.settings.editor
			for _, field := range editor.fields {
				if field.id == "lab.teacherUser" || field.id == "lab.studentUser" {
					t.Fatal("installation asks for saved account name", field.id)
				}
			}
			for range editor.fields {
				next, _ := editor.Update(demoCode(tea.KeyEnter))
				editor = next.(settingsWizardModel)
			}
			if !editor.accepted || editor.settings.Lab.TeacherUser != "professor" || editor.settings.Lab.StudentUser != "pupil" {
				t.Fatal("installation did not retain saved account names")
			}
		})
	}
}

func TestInstallComputersSavesValidatedSettingsWithoutReviewScreen(t *testing.T) {
	saves, setupLoads := 0, 0
	candidate := wizardSettings()
	model := dashboardModel{
		screen:       dashboardSettingsEdit,
		installation: installationModel{flow: true, method: "pxe"},
		settings:     settingsModel{candidate: candidate},
		actions: DashboardActions{
			SaveSettings: func(received domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigurationSaveReport {
				saves++
				if received.Lab.PCCount != candidate.Lab.PCCount || plan.State != "valid" {
					t.Fatalf("save received candidate=%+v plan=%+v", received, plan)
				}
				return domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved"}
			},
			LoadSetup: func(ctx context.Context) domain.SetupReport {
				setupLoads++
				return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply}
			},
		},
	}
	plan := domain.ConfigPlanReport{Operation: "config-plan", State: "valid", Changes: []domain.SettingChange{{Field: "lab.pcCount"}}}
	updated, command := model.Update(dashboardSettingsPlanMsg{report: plan})
	model = updated.(dashboardModel)
	if command == nil || model.screen == dashboardSettingsReview || !model.settings.applying {
		t.Fatalf("installation settings stopped for a save review: screen=%d", model.screen)
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if saves != 1 || command == nil || model.screen != dashboardPXE {
		t.Fatalf("validated settings did not save and continue: saves=%d screen=%d", saves, model.screen)
	}
	_ = command()
	if setupLoads != 1 {
		t.Fatalf("setup checks=%d, want 1", setupLoads)
	}
}

func TestInstallComputersAutomaticallyActivatesPreparesAndStopsAtPXEConfirmation(t *testing.T) {
	plans, applies, preparations, starts := 0, 0, 0, 0
	revision := strings.Repeat("a", 40)
	model := dashboardModel{
		report:       testDashboardReport("stopped"),
		screen:       dashboardPXE,
		installation: installationModel{flow: true, method: "pxe"},
		actions: DashboardActions{
			PlanController: func(ctx context.Context) domain.ControllerRebuildPlanReport {
				plans++
				return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: revision}
			},
			ApplyController: func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
				applies++
				return domain.ControllerRebuildExecutionReport{State: "completed", Applied: true, Verified: true, Revision: plan.Revision}
			},
			Refresh: func(ctx context.Context) (domain.StatusReport, error) {
				status := testDashboardReport("stopped")
				status.PXEPreparation.Ready = true
				return status, nil
			},
			LoadSetup: func(ctx context.Context) domain.SetupReport {
				return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageArtifacts}
			},
			PreparePXE: func() domain.ActionReport {
				preparations++
				return domain.ActionReport{Operation: "pxe-prepare", State: "completed", Message: "prepared"}
			},
			LoadPXEProgress: func() (domain.OperationProgress, error) {
				return domain.OperationProgress{Operation: "pxe-prepare", State: "completed"}, nil
			},
			PlanPXEStart: func(ctx context.Context) domain.PXELifecycleReport {
				return domain.PXELifecycleReport{State: "ready", Mode: "stopped", Interface: "enp1s0", StaticCIDR: "10.0.0.99/24", DHCPAddress: "192.0.2.10"}
			},
			StartPXE: func() domain.PXELifecycleReport {
				starts++
				return domain.PXELifecycleReport{State: "active", Mode: "active"}
			},
		},
	}

	updated, command := model.continueComputerInstallation(domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply})
	model = updated.(dashboardModel)
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 || !model.controller.applying {
		t.Fatalf("controller activation was not started automatically")
	}
	updated, command = model.Update(batch[0]())
	model = updated.(dashboardModel)
	if command == nil || plans != 1 || applies != 1 {
		t.Fatalf("controller activation did not finish: plans=%d applies=%d", plans, applies)
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	prepareBatch, ok := command().(tea.BatchMsg)
	if !ok || len(prepareBatch) != 2 || !model.installation.pxePreparing {
		t.Fatalf("client preparation was not started automatically")
	}
	updated, command = model.Update(prepareBatch[0]())
	model = updated.(dashboardModel)
	if preparations != 1 || command == nil {
		t.Fatalf("client preparation did not finish: preparations=%d", preparations)
	}
	planBatch, ok := command().(tea.BatchMsg)
	if !ok || len(planBatch) != 2 {
		t.Fatalf("PXE validation was not scheduled after preparation")
	}
	var planMessage tea.Msg
	for _, next := range planBatch {
		message := next()
		payload := message
		if wrapped, ok := message.(activityResultMsg); ok {
			payload = wrapped.message
		}
		if _, matches := payload.(dashboardPlanMsg); matches {
			planMessage = message
		}
	}
	if planMessage == nil {
		t.Fatal("PXE plan message is missing")
	}
	updated, _ = model.Update(planMessage)
	model = updated.(dashboardModel)
	view := model.View().Content
	if model.screen != dashboardPXEStartReview || starts != 0 || !strings.Contains(view, "laboratory address (10.0.0.99/24) is paused") || !strings.Contains(view, "Enter starts network installation mode") || strings.Contains(view, "pilot") {
		t.Fatalf("flow did not stop at the sole PXE review: screen=%d starts=%d\n%s", model.screen, starts, view)
	}
}

func TestSetupEditsConfigurationWithoutLeavingTheTUI(t *testing.T) {
	loaded := 0
	model := experienceFixture(t, 2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}
	model.actions.LoadSettings = func(ctx context.Context) (domain.LabSettingsFile, error) {
		loaded++
		return wizardSettings(), nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSettings || model.settings.returnScreen != dashboardSetup {
		t.Fatalf("setup did not open in-TUI settings: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loaded != 1 || model.screen != dashboardSettingsEdit || len(model.settings.editor.fields) != len(settingsFields) {
		t.Fatalf("setup did not open one complete settings sequence: loaded=%d screen=%d fields=%d", loaded, model.screen, len(model.settings.editor.fields))
	}
	if view := demoANSI.ReplaceAllString(model.View().Content, ""); !strings.Contains(view, "First setup  /  Laboratory settings") || !strings.Contains(view, "Step 1 of 11") || strings.Contains(view, "exit and run") {
		t.Fatalf("setup settings are not a continuous English flow:\n%s", view)
	}
}

func TestSetupCredentialsFollowTheSameSettingsAndPasswordSequence(t *testing.T) {
	model := experienceFixture(t, 2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageCredentials}
	model.actions.LoadSettings = func(ctx context.Context) (domain.LabSettingsFile, error) { return wizardSettings(), nil }
	model.actions.ChangePassword = func(string, domain.LabSettingsFile, *os.File, io.Writer) (domain.LabSettingsFile, error) {
		return domain.LabSettingsFile{}, nil
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || len(model.settings.editor.fields) != len(settingsFields) {
		t.Fatalf("credential stage did not start the unified settings sequence: screen=%d fields=%d", model.screen, len(model.settings.editor.fields))
	}

	model.settings.editor.index = len(model.settings.editor.fields) - 1
	model.settings.editor.prepareCurrentField()
	updated, passwordCommand := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsPasswords || passwordCommand == nil || !model.settings.passwordMenu.initialized {
		t.Fatalf("completed settings did not open one protected password session: screen=%d initialized=%t", model.screen, model.settings.passwordMenu.initialized)
	}
	updated, _ = model.Update(struct{}{})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsPasswords {
		t.Fatalf("resume event left the protected password step: screen=%d", model.screen)
	}
}

func TestSetupValidationRetryKeepsAcceptedPasswords(t *testing.T) {
	plans := 0
	model := experienceFixture(t, 2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageValidate}
	model.actions.LoadSettings = func(ctx context.Context) (domain.LabSettingsFile, error) { return wizardSettings(), nil }
	model.actions.PlanSettings = func(ctx context.Context, _ domain.LabSettingsFile) domain.ConfigPlanReport {
		plans++
		return domain.ConfigPlanReport{Operation: "config-plan", State: "unchanged"}
	}

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.settings.collectPasswords {
		t.Fatal("validation retry would collect already accepted passwords again")
	}
	model.settings.editor.index = len(model.settings.editor.fields) - 1
	model.settings.editor.prepareCurrentField()
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.screen == dashboardSettingsPasswords || model.busy == "" {
		t.Fatalf("validation retry did not proceed directly to one plan: screen=%d busy=%q", model.screen, model.busy)
	}
	_, _ = model.Update(command())
	if plans != 1 {
		t.Fatalf("validation plans = %d, want 1", plans)
	}
}

func TestSetupPreparesSavesAndInstallsKeysThroughTypedActions(t *testing.T) {
	reconciles, saves, installs, refreshes := 0, 0, 0, 0
	model := experienceFixture(t, 2)
	model.setupMode = true
	model.screen = dashboardSetup
	model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}
	model.actions.LoadSetupKeys = func(ctx context.Context) domain.KeyReconcileReport {
		return domain.KeyReconcileReport{Operation: "setup-keys-verify", State: "action-required", Keys: []domain.KeyMaterialState{
			{Name: "cache", Problem: "missing"}, {Name: "ssh", Problem: "missing"},
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
	model.actions.LoadSetup = func(ctx context.Context) domain.SetupReport {
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

func TestInstallComputersCreatesMissingKeysWithoutOpeningKeyChoices(t *testing.T) {
	reconciles, saves, installs, setupLoads := 0, 0, 0, 0
	model := experienceFixture(t, 2)
	model.installation.flow = true
	model.screen = dashboardPXE
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
		return domain.ActionReport{Operation: "setup-install-secrets", State: "completed"}
	}
	model.actions.LoadSetup = func(ctx context.Context) domain.SetupReport {
		setupLoads++
		return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply}
	}

	updated, command := model.continueComputerInstallation(domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys})
	model = updated.(dashboardModel)
	if command == nil || model.screen == dashboardSetupKeys || !strings.Contains(model.busy, "Preparing and installing controller keys") {
		t.Fatalf("installation exposed key choices instead of preparing automatically: screen=%d busy=%q", model.screen, model.busy)
	}
	updated, refresh := model.Update(command())
	model = updated.(dashboardModel)
	if reconciles != 1 || saves != 1 || installs != 1 || refresh == nil || model.screen == dashboardSetupKeys {
		t.Fatalf("automatic key preparation failed: reconcile=%d save=%d install=%d screen=%d", reconciles, saves, installs, model.screen)
	}
	_ = refresh()
	if setupLoads != 1 {
		t.Fatalf("setup refreshes=%d, want 1", setupLoads)
	}
}

func TestExistingKeyImportLivesUnderAdvancedSettings(t *testing.T) {
	loads := 0
	model := experienceFixture(t, 2)
	model.screen = dashboardSettings
	model.settings.menu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
	model.actions.LoadSetupKeys = func(ctx context.Context) domain.KeyReconcileReport {
		loads++
		return domain.KeyReconcileReport{State: "action-required", Keys: []domain.KeyMaterialState{
			{Name: "cache", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
			{Name: "ssh", Problem: "private and public keys are missing"},
		}}
	}

	updated, command := model.Update(tea.KeyPressMsg{Text: "y"})
	model = updated.(dashboardModel)
	if command == nil || model.setupKeysReturn != dashboardSettings {
		t.Fatalf("advanced key settings did not open: return=%d", model.setupKeysReturn)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if loads != 1 || model.screen != dashboardSetupKeys || !strings.Contains(demoANSI.ReplaceAllString(view, ""), "Maintenance  /  Settings  /  Advanced  /  Controller keys") || !strings.Contains(view, "Import") {
		t.Fatalf("existing-key import is not in advanced settings: loads=%d screen=%d\n%s", loads, model.screen, view)
	}
	model.setupKeyCursor = 1
	updated, _ = model.Update(tea.KeyPressMsg{Text: "i"})
	model = updated.(dashboardModel)
	if !model.setupKeyImporting || !strings.Contains(model.View().Content, "Import existing administrator SSH private key") {
		t.Fatalf("advanced import did not start:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettings {
		t.Fatalf("advanced keys returned to screen %d, want settings", model.screen)
	}
}

func TestSetupImportsSelectedExistingKeyWithoutExposingMaterial(t *testing.T) {
	importedName, importedPath := "", ""
	model := experienceFixture(t, 2)
	model.screen = dashboardSetupKeys
	model.setupKeyCursor = 1
	model.setupKeys = domain.KeyReconcileReport{State: "action-required", Keys: []domain.KeyMaterialState{
		{Name: "cache", PrivatePresent: true, PublicPresent: true, Safe: true, Verified: true, Matches: true},
		{Name: "ssh", Problem: "private and public keys are missing"},
	}}
	model.actions.ImportSetupKey = func(name, path string) (domain.KeyImportReport, error) {
		importedName, importedPath = name, path
		return domain.KeyImportReport{Operation: "setup-key-import", State: "imported", Key: name, Source: path, Fingerprint: "SHA256:verified", Message: "Existing key imported and verified."}, nil
	}
	model.actions.LoadSetupKeys = func(ctx context.Context) domain.KeyReconcileReport {
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

func TestSetupPasswordCancellationNamesSetup(t *testing.T) {
	for _, installation := range []bool{false, true} {
		model := newTestModel(t, withReport(testDashboardReport("stopped")))
		model.screen = dashboardSettingsPasswords
		model.settings.returnScreen = dashboardSetup
		model.installation.flow = installation
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		model = updated.(dashboardModel)
		if installation != strings.Contains(model.message, "Computer installation cancelled") || !strings.Contains(model.message, "no setting was changed") {
			t.Fatalf("wrong cancellation context: %s", model.message)
		}
	}
}
