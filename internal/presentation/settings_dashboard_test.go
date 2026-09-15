package presentation

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

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
