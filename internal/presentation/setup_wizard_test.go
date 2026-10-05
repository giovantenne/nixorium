package presentation

import (
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
	if !model.selector.FilteringEnabled() || !strings.Contains(model.View().Content, "press / to filter") {
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

func TestClientInterfaceProposesTheControllerCardOnlyBeforeClientsExist(t *testing.T) {
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
	if got := draft(settings); got != "eno1" {
		t.Fatalf("first laboratory setup proposed %q, want the controller card eno1", got)
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
