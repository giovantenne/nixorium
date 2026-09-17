package presentation

import (
	"bytes"
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
		PlanSettings: func(received domain.LabSettingsFile) domain.ConfigPlanReport {
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
	model := dashboardModel{settings: current, actions: actions, screen: dashboardSettingsPasswords}
	updated, command := model.Update(dashboardSettingsPasswordMsg{candidate: candidate})
	model = updated.(dashboardModel)
	if command == nil || model.busy == "" {
		t.Fatalf("password plan did not start: %+v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	view := model.View().Content
	if model.screen != dashboardSettingsReview || !strings.Contains(view, "lab.teacherPassword: configured → updated") || strings.Contains(view, "$6$") {
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
		screen:            dashboardSettingsPasswords,
		settingsReturn:    dashboardSetup,
		settingsCandidate: candidate,
		actions: DashboardActions{
			PlanSettings: func(received domain.LabSettingsFile) domain.ConfigPlanReport {
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
			LoadSetup: func() domain.SetupReport {
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
