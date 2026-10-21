package presentation

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSettingsPasswordCommandKeepsPlaintextOutsideDashboardModel(t *testing.T) {
	current := wizardSettings()
	command := &settingsPasswordCommand{
		account:  "teacher",
		settings: current,
		action: func(account string, settings domain.LabSettingsFile, _ *os.File, _ io.Writer) (domain.LabSettingsFile, error) {
			if account != "teacher" {
				t.Fatalf("account = %q", account)
			}
			settings.Lab.TeacherPassword = "$6$new$teacher"
			return settings, nil
		},
	}
	command.SetStdin(os.Stdin)
	command.SetStdout(&bytes.Buffer{})
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	if command.candidate.Lab.TeacherPassword != "$6$new$teacher" || current.Lab.TeacherPassword == command.candidate.Lab.TeacherPassword {
		t.Fatalf("password candidate was not isolated: current=%q candidate=%q", current.Lab.TeacherPassword, command.candidate.Lab.TeacherPassword)
	}
}

func TestSettingsPasswordReviewIsRedacted(t *testing.T) {
	current := wizardSettings()
	candidate := current
	candidate.Lab.TeacherPassword = "$6$new$teacher"
	actions := DashboardActions{
		PlanSettings: func(ctx context.Context, received domain.LabSettingsFile) domain.ConfigPlanReport {
			if received.Lab.TeacherPassword != candidate.Lab.TeacherPassword {
				t.Fatalf("candidate hash was not passed to application plan")
			}
			return domain.ConfigPlanReport{
				Operation:       "config-plan",
				State:           "valid",
				BaseFingerprint: "sha256:reviewed",
				Changes: []domain.SettingChange{{
					Field:     "lab.teacherPassword",
					Before:    "$6$old$teacher",
					After:     "$6$new$teacher",
					Sensitive: true,
				}},
			}
		},
	}
	model := dashboardModel{settings: settingsModel{current: current}, actions: actions, screen: dashboardSettingsPasswords}
	updated, command := model.Update(dashboardSettingsPasswordMsg{candidate: candidate})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("password plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if model.screen != dashboardSettingsReview || !strings.Contains(view, "Teacher password: configured → updated") || strings.Contains(view, "$6$") {
		t.Fatalf("password review was not redacted:\n%s", view)
	}
}

func TestUninitializedPasswordMenuIgnoresResumeEvents(t *testing.T) {
	model := dashboardModel{screen: dashboardSettingsPasswords}
	updated, command := model.Update(struct{}{})
	if command != nil || updated.(dashboardModel).screen != dashboardSettingsPasswords {
		t.Fatalf("unexpected resume handling: command=%v screen=%d", command, updated.(dashboardModel).screen)
	}
}

func TestFirstSetupValidatesAndSavesTheCompleteCandidateOnce(t *testing.T) {
	plans, saves, refreshes := 0, 0, 0
	candidate := wizardSettings()
	candidate.Lab.AdminPassword = "$6$new$admin"
	candidate.Lab.TeacherPassword = "$6$new$teacher"
	candidate.Lab.StudentPassword = "$6$new$student"
	model := dashboardModel{
		screen: dashboardSettingsPasswords,
		settings: settingsModel{
			returnScreen: dashboardSetup,
			candidate:    candidate,
		},
		actions: DashboardActions{
			PlanSettings: func(ctx context.Context, received domain.LabSettingsFile) domain.ConfigPlanReport {
				plans++
				if received.Lab.StudentPassword != candidate.Lab.StudentPassword {
					t.Fatal("complete password candidate was not preserved")
				}
				return domain.ConfigPlanReport{Operation: "config-plan", State: "valid", Changes: []domain.SettingChange{{Field: "lab.studentPassword", Sensitive: true}}}
			},
			SaveSettings: func(received domain.LabSettingsFile, _ domain.ConfigPlanReport) domain.ConfigurationSaveReport {
				saves++
				return domain.ConfigurationSaveReport{Operation: "configuration-save", State: "saved"}
			},
			LoadSetup: func(ctx context.Context) domain.SetupReport {
				refreshes++
				return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageKeys}
			},
		},
	}

	updated, command := model.Update(dashboardSettingsPasswordMsg{candidate: candidate})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("complete candidate validation did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if plans != 1 || model.screen != dashboardSettingsReview {
		t.Fatalf("complete candidate was not reviewed once: plans=%d screen=%d", plans, model.screen)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "y"})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("reviewed setup configuration did not start saving")
	}
	updated, command = model.Update(command())
	model = updated.(dashboardModel)
	if saves != 1 || command == nil || model.screen != dashboardSetup {
		t.Fatalf("setup save did not continue automatically: saves=%d screen=%d", saves, model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if refreshes != 1 || model.setup.CurrentStage != domain.SetupStageKeys {
		t.Fatalf("setup did not resume after one save: refreshes=%d setup=%+v", refreshes, model.setup)
	}
}

func TestSettingsBlockedNetworkChangeReturnsWithoutSaving(t *testing.T) {
	base := wizardSettings()
	base.Lab.PCCount = 20
	candidate := base
	candidate.Lab.NetworkBase = "10.1.0.0"
	model := dashboardModel{screen: dashboardSettings, width: 120, height: 30,
		settings: settingsModel{current: base, candidate: candidate, menu: newRoutineSettingsMenu(true, 120, 30)},
		actions: DashboardActions{SaveSettings: func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			t.Fatal("blocked network change tried to save")
			return domain.ConfigurationSaveReport{}
		}},
	}
	next, command := model.Update(dashboardSettingsPlanMsg{report: domain.ConfigPlanReport{
		State: "invalid", Issues: domain.SettingsTransitionIssues(base, candidate),
	}})
	model = next.(dashboardModel)
	if command != nil || model.screen != dashboardSettings || model.settings.current.Lab.NetworkBase != base.Lab.NetworkBase {
		t.Fatal("blocked change did not preserve saved settings")
	}
	if !strings.Contains(model.message, "keep the saved value") || strings.Contains(model.message, "lab.networkBase:") {
		t.Fatalf("unclear refusal: %s", model.message)
	}
}

func TestSettingsInterfaceReviewShowsBothValuesAndWarning(t *testing.T) {
	base := wizardSettings()
	base.Lab.ControllerInterfaceName = "enp0s3"
	candidate := base
	candidate.Lab.ControllerInterfaceName = "enp1s0"
	changes := domain.DiffLabSettings(base, candidate)
	model := dashboardModel{screen: dashboardSettingsReview, width: 120, height: 45,
		settings: settingsModel{plan: domain.ConfigPlanReport{State: "valid", Changes: changes, Impacts: domain.SettingsImpacts(base, candidate, changes)}},
	}
	view := model.View().Content
	for _, want := range []string{"enp0s3", "enp1s0", "disconnect", "local console", "Save", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in review:\n%s", want, view)
		}
	}
}

func TestMaintenanceNoLongerListsAccountPasswords(t *testing.T) {
	for _, task := range administrationTasks {
		if task.id == "passwords" || task.shortcut == "p" || strings.Contains(task.title, "password") {
			t.Fatalf("Maintenance still lists a passwords entry: %+v", task)
		}
	}
	model := dashboardModel{report: testDashboardReport("ready"), width: 100, height: 30, screen: dashboardAdministration}
	if view := model.View().Content; strings.Contains(view, "Change account passwords") {
		t.Fatalf("Maintenance view still offers account passwords:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "p"})
	if command != nil || updated.(dashboardModel).screen != dashboardAdministration {
		t.Fatalf("p still opens a Maintenance task: screen=%d", updated.(dashboardModel).screen)
	}
}

// openAccountsSettings follows Maintenance → Change settings → Accounts.
func openAccountsSettings(t *testing.T) dashboardModel {
	t.Helper()
	current := wizardSettings()
	actions := DashboardActions{LoadSettings: func(context.Context) (domain.LabSettingsFile, error) { return current, nil }}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 100, height: 30, screen: dashboardAdministration}
	updated, command := model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("settings load did not start: screen=%d", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettings {
		t.Fatalf("Change settings opened screen %d", model.screen)
	}
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	if strings.Contains(view, "p Passwords") {
		t.Fatalf("Settings still offers the old p Passwords shortcut:\n%s", view)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "a"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardSettingsAccounts {
		t.Fatalf("Accounts did not open its submenu: screen=%d", model.screen)
	}
	view = demoANSI.ReplaceAllString(model.View().Content, "")
	for _, want := range []string{"Maintenance  /  Settings  /  Accounts", "[n] Change account names", "[p] Change account passwords", "Esc Settings"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Accounts submenu omits %q:\n%s", want, view)
		}
	}
	return model
}

func TestSettingsPasswordsShortcutIsRemoved(t *testing.T) {
	model := openAccountsSettings(t)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettings {
		t.Fatalf("Esc from Accounts = screen %d", model.screen)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardSettings {
		t.Fatalf("p on Settings still opens passwords: screen=%d", model.screen)
	}
}

func TestSettingsAccountsNamesOpensNameFormAndEscReturns(t *testing.T) {
	model := openAccountsSettings(t)
	updated, _ := model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || len(model.settings.editor.fields) != 2 || model.settings.editor.fields[0].id != "lab.teacherUser" || model.settings.editor.fields[1].id != "lab.studentUser" {
		t.Fatalf("names form = screen %d fields %+v", model.screen, model.settings.editor.fields)
	}
	if view := demoANSI.ReplaceAllString(model.View().Content, ""); !strings.Contains(view, "Settings  /  Accounts  /  Names") {
		t.Fatalf("names form path is wrong:\n%s", view)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsAccounts {
		t.Fatalf("Esc from names form = screen %d", model.screen)
	}
}

func TestSettingsAccountsNamesKeepReviewAndSave(t *testing.T) {
	model := openAccountsSettings(t)
	planned := 0
	model.actions.PlanSettings = func(_ context.Context, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
		planned++
		if candidate.Lab.TeacherUser != "docente" {
			t.Fatalf("candidate teacher = %q", candidate.Lab.TeacherUser)
		}
		return domain.ConfigPlanReport{Operation: "config-plan", State: "valid", Changes: []domain.SettingChange{{Field: "lab.teacherUser", Before: "teacher", After: "docente"}}}
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "n"})
	model = updated.(dashboardModel)
	model.settings.editor.drafts[0] = "docente"
	var command tea.Cmd
	for step := 0; step < 4 && command == nil; step++ {
		updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model = updated.(dashboardModel)
	}
	if command == nil {
		t.Fatalf("names form did not submit: screen=%d", model.screen)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardSettingsReview || !strings.Contains(model.View().Content, "Teacher user name: teacher → docente") {
		t.Fatalf("names change was not reviewed: planned=%d screen=%d\n%s", planned, model.screen, model.View().Content)
	}
}

func TestSettingsAccountsPasswordsOpensPasswordListAndEscReturns(t *testing.T) {
	model := openAccountsSettings(t)
	updated, _ := model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsPasswords || !model.settings.passwordMenu.initialized {
		t.Fatalf("passwords did not open: screen=%d", model.screen)
	}
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	for _, want := range []string{"Settings  /  Accounts  /  Passwords", "Change account passwords", "update the computers", "Esc Accounts"} {
		if !strings.Contains(view, want) {
			t.Fatalf("passwords view omits %q:\n%s", want, view)
		}
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsAccounts {
		t.Fatalf("Esc from passwords = screen %d", model.screen)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettings {
		t.Fatalf("second Esc = screen %d", model.screen)
	}
}

func TestSettingsAccountsPasswordsChooseAccountRunsProtectedInput(t *testing.T) {
	model := openAccountsSettings(t)
	model.actions.ChangePassword = func(string, domain.LabSettingsFile, *os.File, io.Writer) (domain.LabSettingsFile, error) {
		return domain.LabSettingsFile{}, nil
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "p"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "t"})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardSettingsPasswords {
		t.Fatalf("choosing the teacher did not start protected input: screen=%d", model.screen)
	}
}

func TestFirstSetupPasswordStepKeepsItsPath(t *testing.T) {
	model := dashboardModel{width: 100, height: 30, screen: dashboardSettingsPasswords, settings: settingsModel{returnScreen: dashboardSetup}}
	view := demoANSI.ReplaceAllString(model.View().Content, "")
	if !strings.Contains(view, "Installation  /  Setup  /  Settings  /  Passwords") || !strings.Contains(view, "Collect passwords") || strings.Contains(view, "Accounts") {
		t.Fatalf("first-setup password step changed:\n%s", view)
	}
}

func TestSettingsAccountsHelpNamesBothEntries(t *testing.T) {
	model := openAccountsSettings(t)
	help := demoANSI.ReplaceAllString(model.helpView(), "")
	for _, want := range []string{"n  Change account names", "p  Change account passwords", "update the computers"} {
		if !strings.Contains(help, want) {
			t.Fatalf("Accounts help omits %q:\n%s", want, help)
		}
	}
	model.screen = dashboardSettings
	if help := demoANSI.ReplaceAllString(model.helpView(), ""); strings.Contains(help, "p  Account passwords") {
		t.Fatalf("Settings help still lists the removed p shortcut:\n%s", help)
	}
}

func TestHelpShowsContactAddressOnEveryScreen(t *testing.T) {
	for _, screen := range []dashboardScreen{dashboardHome, dashboardSettings, dashboardHosts} {
		model := dashboardModel{width: 100, height: 30, screen: screen}
		help := demoANSI.ReplaceAllString(model.helpView(), "")
		if strings.Count(help, "hello@nixorium.org") != 1 || !strings.Contains(help, "Esc / ? / F1 close help") {
			t.Fatalf("help for screen %d should show the contact address once:\n%s", screen, help)
		}
	}
}

func TestDashboardEditsReviewsAndAppliesManagedSettings(t *testing.T) {
	current := wizardSettings()
	planned := 0
	applied := 0
	actions := DashboardActions{
		LoadSettings: func(ctx context.Context) (domain.LabSettingsFile, error) {
			return current, nil
		},
		PlanSettings: func(ctx context.Context, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
			planned++
			if candidate.Lab.StudentGitName != "Lab Student" {
				t.Fatalf("unexpected settings candidate: %+v", candidate.Lab)
			}
			return domain.ConfigPlanReport{
				Operation:       "config-plan",
				State:           "valid",
				BaseFingerprint: "sha256:reviewed",
				Changes: []domain.SettingChange{{
					Field:  "lab.studentGitName",
					Before: current.Lab.StudentGitName,
					After:  candidate.Lab.StudentGitName,
				}},
			}
		},
		SaveSettings: func(candidate domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			applied++
			if candidate.Lab.StudentGitName != "Lab Student" || plan.BaseFingerprint != "sha256:reviewed" {
				t.Fatalf("unexpected reviewed apply: candidate=%+v plan=%+v", candidate.Lab, plan)
			}
			return domain.ConfigurationSaveReport{
				Operation: "configuration-save",
				State:     "saved",
				Changes:   plan.Changes,
				Message:   "Configuration saved locally. No computer was changed.",
			}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions, width: 100, height: 30}
	if !strings.Contains(model.View().Content, "Maintenance") {
		t.Fatalf("home omits settings task:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("settings load did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardSettings || len(model.settings.menu.list.Items()) != len(routineSettingsGroups) || !strings.Contains(model.View().Content, "Regional") {
		t.Fatalf("settings categories missing:\n%s", model.View().Content)
	}
	model.settings.menu.list.Select(5)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardSettingsEdit || !strings.Contains(demoANSI.ReplaceAllString(model.View().Content, ""), "Settings  /  Git") {
		t.Fatalf("Git settings editor missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "Lab Student"})
	model = updated.(dashboardModel)
	for range 3 {
		updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("settings plan did not start: %+v", model)
	}
	if !strings.Contains(model.View().Content, "Checking that the proposed settings") {
		t.Fatalf("settings editor could not render while validation starts:\n%s", model.View().Content)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if planned != 1 || model.screen != dashboardSettingsReview || !strings.Contains(model.View().Content, "Student Git author name: Student → Lab Student") || !strings.Contains(model.View().Content, "Only the lab settings file") {
		t.Fatalf("settings review missing: planned=%d\n%s", planned, model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "y"})
	model = updated.(dashboardModel)
	if command == nil || !model.settings.applying {
		t.Fatalf("settings apply did not start: %+v", model)
	}
	updated, quitCommand := model.Update(tea.KeyPressMsg{Text: "q"})
	model = updated.(dashboardModel)
	if quitCommand != nil || !strings.Contains(model.message, "Wait for the result") {
		t.Fatal("dashboard allowed quit while settings apply was running")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if applied != 1 || model.settings.applying || model.screen != dashboardSettings || model.settings.current.Lab.StudentGitName != "Lab Student" || !strings.Contains(model.View().Content, "Configuration saved") || strings.Contains(model.View().Content, "Git") {
		t.Fatalf("settings result missing: applied=%d model=%+v\n%s", applied, model, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "e"})
	model = updated.(dashboardModel)
	if model.settings.result.Operation != "" || !strings.Contains(model.View().Content, "Choose one area to edit") {
		t.Fatalf("settings result did not open another edit:\n%s", model.View().Content)
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
		LoadServices: func(ctx context.Context) domain.ServicesReport { return serviceReport },
		RestartService: func(service string) domain.ServiceActionReport {
			restarts++
			if service != "cache" {
				t.Fatalf("restarted unexpected service %q", service)
			}
			return domain.ServiceActionReport{Operation: "service-restart", State: "completed", Service: service, Verified: true, Message: "cache healthy"}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "s"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardServices || !strings.Contains(model.View().Content, "healthy") || !strings.Contains(model.View().Content, "Installation → Network boot (PXE)") {
		t.Fatalf("services screen missing:\n%s", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	if model.screen != dashboardServicesRestartReview || !strings.Contains(model.View().Content, "Enter restarts the software cache") {
		t.Fatalf("restart review missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if command != nil || restarts != 0 || model.screen != dashboardServices {
		t.Fatalf("cancelled restart changed something: restarts=%d screen=%d", restarts, model.screen)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "c"})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if restarts != 1 || model.screen != dashboardServices || !strings.Contains(model.View().Content, "Binary cache restarted and verified") || !strings.Contains(model.View().Content, "cache healthy") || !strings.Contains(model.View().Content, "Restart again") {
		t.Fatalf("verified restart result missing: restarts=%d\n%s", restarts, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if model.screen != dashboardAdministration {
		t.Fatalf("service result did not return to Maintenance: screen=%d", model.screen)
	}
}
