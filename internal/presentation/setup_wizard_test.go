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
			VeyonNativeHosts: []string{},
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
	for range settingsFields {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model = updated.(settingsWizardModel)
	}
	if !model.accepted || model.cancelled {
		t.Fatalf("wizard did not accept valid defaults: %+v", model)
	}
	if model.settings.Lab.VeyonNativeHosts == nil {
		t.Fatal("empty Veyon host list became null")
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
	for _, expected := range []string{"Network", "Laboratory", "Accounts", "Regional settings", "Preferences", "Classroom"} {
		if !groups[expected] {
			t.Fatalf("first-run group %q is missing: %v", expected, groups)
		}
	}
}

func TestRegionalFieldsUseSearchableSuggestedValues(t *testing.T) {
	if localeChoices[0].value != "en_US.UTF-8" {
		t.Fatalf("first locale choice = %q, want en_US.UTF-8", localeChoices[0].value)
	}
	model := newSettingsWizardModel(wizardSettings())
	model = model.moveToField(settingsFieldIndex("lab.defaultLocale"))
	if !model.selector.FilteringEnabled() || !strings.Contains(model.View().Content, "press / to filter") {
		t.Fatalf("locale selector is not searchable:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	model = updated.(settingsWizardModel)
	if model.selector.FilterState() != list.Filtering {
		t.Fatalf("slash did not open locale filtering: %s", model.selector.FilterState())
	}
	model.selector.SetFilterText("United States")
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	if model.settings.Lab.DefaultLocale != "en_US.UTF-8" {
		t.Fatalf("selected locale was not applied: %+v", model.settings.Lab)
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
