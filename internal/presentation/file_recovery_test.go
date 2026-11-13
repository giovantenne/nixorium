package presentation

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type recoveryCalls struct {
	listed  []string
	planned []string
	applied int
}

func recoveryActions(t *testing.T, calls *recoveryCalls) *FileRecoveryActions {
	saved := time.Date(2026, 11, 10, 8, 12, 0, 0, time.UTC)
	return &FileRecoveryActions{
		Sessions: func(_ context.Context, computer string) ([]domain.FileRecoverySession, error) {
			if computer != "pc02" {
				t.Fatalf("sessions of %s", computer)
			}
			return []domain.FileRecoverySession{{Number: 1, SavedAt: saved}, {Number: 2, SavedAt: saved.Add(-24 * time.Hour)}}, nil
		},
		List: func(_ context.Context, computer string, session int, folder string) (domain.FileRecoveryListing, error) {
			calls.listed = append(calls.listed, folder)
			if computer != "pc02" || session != 2 {
				t.Fatalf("list %s session %d", computer, session)
			}
			switch folder {
			case "":
				return domain.FileRecoveryListing{Session: 2, Entries: []domain.FileRecoveryEntry{{Name: "Desktop", Dir: true}, {Name: "notes.txt", Size: 2048}}}, nil
			case "Desktop":
				return domain.FileRecoveryListing{Session: 2, Path: "Desktop", Entries: []domain.FileRecoveryEntry{{Name: "essay.odt", Size: 10}}}, nil
			}
			return domain.FileRecoveryListing{}, errors.New("This folder is not in the saved session.")
		},
		Plan: func(_ context.Context, computer string, session int, paths []string) domain.FileRecoveryPlan {
			calls.planned = paths
			return domain.FileRecoveryPlan{State: "ready", Session: session, SavedAt: saved.Add(-24 * time.Hour), Paths: paths, ReviewToken: "test", Message: "Copies 2 chosen item(s) into the folder “Recovered files” on the desktop of pc02. Nothing there is replaced.", Target: domain.FileRecoveryTarget{HostMeta: domain.HostMeta{Name: computer}}}
		},
		Apply: func(plan domain.FileRecoveryPlan) domain.FileRecoveryReport {
			calls.applied++
			return domain.FileRecoveryReport{State: "completed", Computer: "pc02", Message: "2 file(s) copied into “Desktop/Recovered files” on pc02.", Restored: &domain.FileRecoveryRestored{Folder: "Desktop/Recovered files", Placed: []string{"essay.odt", "notes.txt"}, Files: 2}}
		},
	}
}

func TestRecoverStudentFilesBrowseChooseReviewAndApply(t *testing.T) {
	calls := &recoveryCalls{}
	actions := DashboardActions{ClassroomMode: true, FileRecovery: recoveryActions(t, calls)}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	model.width, model.height = 120, 30
	if !slices.ContainsFunc(model.availableComputerTasks(), func(task dashboardTask) bool { return task.id == "recover" }) {
		t.Fatal("Recover student files is not listed for the teacher")
	}
	opened, _ := model.openComputerTask("r")
	model = opened.(dashboardModel)
	if model.screen != dashboardFileRecovery {
		t.Fatal("Recover student files did not open")
	}
	key := func(press tea.KeyPressMsg) {
		t.Helper()
		updated, cmd := model.updateFileRecovery(press)
		model = updated.(dashboardModel)
		if cmd != nil {
			updated, _ = model.Update(cmd())
			model = updated.(dashboardModel)
		}
	}
	code := func(value rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: value} }
	text := func(value string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(value[0]), Text: value} }
	key(code(tea.KeyDown))
	key(code(tea.KeyEnter))
	if model.fileRecovery.stage != recoveryStageSession || model.fileRecovery.computer != "pc02" || !strings.Contains(model.View().Content, "Last change") {
		t.Fatalf("sessions: stage %d, %q\n%s", model.fileRecovery.stage, model.message, model.View().Content)
	}
	key(code(tea.KeyDown))
	key(code(tea.KeyEnter))
	if model.fileRecovery.stage != recoveryStageBrowse || len(calls.listed) != 1 || calls.listed[0] != "" {
		t.Fatalf("browse: stage %d, listed %v, %q", model.fileRecovery.stage, calls.listed, model.message)
	}
	key(text("r"))
	if model.fileRecovery.stage != recoveryStageBrowse || !strings.Contains(model.message, "Choose at least one") {
		t.Fatalf("empty review: %q", model.message)
	}
	// Choose the home's notes, then open Desktop and choose a file there.
	key(code(tea.KeyDown))
	key(code(tea.KeySpace))
	key(code(tea.KeyUp))
	key(code(tea.KeyEnter))
	if model.fileRecovery.listing.Path != "Desktop" {
		t.Fatalf("Desktop not opened: %+v", model.fileRecovery.listing)
	}
	key(code(tea.KeySpace))
	key(code(tea.KeyBackspace))
	if model.fileRecovery.listing.Path != "" || calls.listed[len(calls.listed)-1] != "" {
		t.Fatalf("not back in the home: %v", calls.listed)
	}
	// Choosing Desktop drops the file chosen inside it; choosing it again
	// clears it.
	key(code(tea.KeySpace))
	if !model.fileRecovery.chosen["Desktop"] || model.fileRecovery.chosen["Desktop/essay.odt"] || !model.fileRecovery.chosen["notes.txt"] {
		t.Fatalf("chosen = %v", model.fileRecovery.chosen)
	}
	key(code(tea.KeySpace))
	key(code(tea.KeyEnter))
	key(code(tea.KeySpace))
	key(code(tea.KeyBackspace))
	key(text("r"))
	if model.fileRecovery.stage != recoveryStageReview || strings.Join(calls.planned, ",") != "Desktop/essay.odt,notes.txt" || calls.applied != 0 {
		t.Fatalf("review: stage %d, planned %v", model.fileRecovery.stage, calls.planned)
	}
	key(code(tea.KeyEscape))
	if model.fileRecovery.stage != recoveryStageBrowse || calls.applied != 0 {
		t.Fatal("cancelling the review applied it")
	}
	key(text("r"))
	key(code(tea.KeyEnter))
	if calls.applied != 1 || model.fileRecovery.stage != recoveryStageResult {
		t.Fatalf("not applied: stage %d", model.fileRecovery.stage)
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		model.width, model.height = size[0], size[1]
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overflow %dx%d:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "Recover student files") || !strings.Contains(view, "Recovered files") {
			t.Fatalf("result view %dx%d:\n%s", size[0], size[1], view)
		}
	}
	key(code(tea.KeyEnter))
	if model.screen != dashboardComputersArea {
		t.Fatal("Enter did not return to Computers")
	}
}

func TestRecoverStudentFilesShowsRefusals(t *testing.T) {
	calls := &recoveryCalls{}
	actions := recoveryActions(t, calls)
	actions.Sessions = func(context.Context, string) ([]domain.FileRecoverySession, error) {
		return nil, errors.New("pc01 could not be reached: the computer did not answer; check that it is on.")
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{FileRecovery: actions}, false)
	model.width, model.height = 120, 30
	opened, _ := model.openComputerTask("r")
	model = opened.(dashboardModel)
	updated, cmd := model.updateFileRecovery(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(cmd())
	model = updated.(dashboardModel)
	if model.fileRecovery.stage != recoveryStageComputer || !strings.Contains(model.View().Content, "could not be reached") {
		t.Fatalf("refusal not shown:\n%s", model.View().Content)
	}
	// A failed copy offers to read the sessions again, not the old session.
	calls2 := &recoveryCalls{}
	actions2 := recoveryActions(t, calls2)
	actions2.Apply = func(domain.FileRecoveryPlan) domain.FileRecoveryReport {
		return domain.FileRecoveryReport{State: "failed", Message: "The computer restarted since the review and its saved sessions moved. Choose the session again."}
	}
	model = newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{FileRecovery: actions2}, false)
	model.width, model.height = 120, 30
	model.screen = dashboardFileRecovery
	model.fileRecovery = newFileRecoveryModel()
	model.fileRecovery.stage, model.fileRecovery.computer = recoveryStageReview, "pc02"
	model.fileRecovery.plan = domain.FileRecoveryPlan{State: "ready", ReviewToken: "t", Paths: []string{"a"}}
	press := func(code rune, text string) {
		updated, cmd := model.updateFileRecovery(tea.KeyPressMsg{Code: code, Text: text})
		model = updated.(dashboardModel)
		if cmd != nil {
			updated, _ = model.Update(cmd())
			model = updated.(dashboardModel)
		}
	}
	press(tea.KeyEnter, "")
	if !strings.Contains(model.View().Content, "Choose the session again") {
		t.Fatalf("failed copy view:\n%s", model.View().Content)
	}
	press('n', "n")
	if model.fileRecovery.stage != recoveryStageSession {
		t.Fatalf("n after a failure: stage %d", model.fileRecovery.stage)
	}
	model = dashboardModel{report: testDashboardReport("ready"), width: 120, height: 30, screen: dashboardComputersArea}
	if slices.ContainsFunc(model.availableComputerTasks(), func(task dashboardTask) bool { return task.id == "recover" }) {
		t.Fatal("Recover student files listed without its actions")
	}
}
