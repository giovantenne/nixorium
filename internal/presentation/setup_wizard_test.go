package presentation

import (
	"strings"
	"testing"

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
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	model.drafts[1] = "10.1.0.0"
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsWizardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	model = updated.(settingsWizardModel)
	if model.index != 1 || model.drafts[1] != "10.1.0.0" || model.settings.Lab.NetworkBase != "10.1.0.0" {
		t.Fatalf("wizard lost state: %+v", model)
	}
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

func TestConfigReviewNeverRendersPasswordHash(t *testing.T) {
	plan := domain.ConfigPlanReport{Changes: []domain.SettingChange{{Field: "lab.adminPassword", Before: "configured", After: "updated", Sensitive: true}}}
	view := (configReviewModel{plan: plan, git: domain.GitState{Dirty: true, Changes: 2, Paths: []string{"keys/admin-ssh.pub", "modules/local.nix"}}}).View()
	if strings.Contains(view.Content, "$6$") || !strings.Contains(view.Content, "Setup-generated") || !strings.Contains(view.Content, "other existing") {
		t.Fatalf("unsafe or incomplete review:\n%s", view.Content)
	}
}
