package presentation

import (
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
	"strings"
	"testing"
)

func TestGitBackupRequiresReviewAndConfirmationAndMasksSecrets(t *testing.T) {
	calls := 0
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{PublishGitBackup: func(p domain.GitBackupPlan, pass []byte) domain.BackupReport {
		calls++
		if string(pass) != "quiet passphrase!" || p.Revision != "reviewed" {
			t.Fatal("wrong reviewed input")
		}
		return domain.BackupReport{State: "completed", Operation: "backup-publish"}
	}}, false)
	m.screen = dashboardBackup
	m.backup = backupModel{reviewed: true, plan: domain.GitBackupPlan{Revision: "reviewed"}}
	for _, char := range "quiet passphrase!" {
		next, _ := m.Update(tea.KeyPressMsg{Code: char, Text: string(char)})
		m = next.(dashboardModel)
	}
	if string(m.backup.passphrase) != "quiet passphrase!" {
		t.Fatal("global shortcut consumed passphrase")
	}
	if strings.Contains(m.View().Content, "quiet passphrase!") {
		t.Fatal("passphrase shown")
	}
	m.backup.repeat = append([]rune{}, m.backup.passphrase...)
	m.backup.field = 2
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd != nil || calls != 0 {
		t.Fatal("push without confirmation")
	}
	m.backup.confirmation = "PUSH"
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	m = next.(dashboardModel)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd != nil || calls != 0 {
		t.Fatal("help allowed a push")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(dashboardModel)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd == nil || len(m.backup.passphrase) > 0 {
		t.Fatal("push did not consume secret")
	}
	message := cmd()
	if calls != 1 {
		t.Fatal("push not called exactly once")
	}
	next, _ = m.Update(message)
	m = next.(dashboardModel)
	if !m.backup.done {
		t.Fatal("result missing")
	}
}

func TestRestoreLabReviewAndCancellation(t *testing.T) {
	called := false
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{RestoreLab: func(string, string, string, []byte) domain.BackupReport {
		called = true
		return domain.BackupReport{State: "completed"}
	}}, false)
	m.screen = dashboardBackup
	m.backup = backupModel{restore: true, standalone: true, field: 3, remote: "git@host:lab.git", branch: "main", destination: "/new/lab", passphrase: []rune("restore password")}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd != nil || called || !m.backup.reviewed {
		t.Fatal("restore skipped review")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(dashboardModel)
	if called || len(m.backup.passphrase) > 0 {
		t.Fatal("cancel restored or retained secret")
	}
}

func TestGitBackupScreensFitAndKeepActions(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
			m.screen = dashboardBackup
			m.width = size[0]
			m.height = size[1]
			m.isDark = dark
			for _, b := range []backupModel{
				{branch: "main"},
				{restore: true, branch: "main"},
				{reviewed: true, plan: domain.GitBackupPlan{Remote: "git@github.com:school/lab.git", Branch: "main", Revision: strings.Repeat("a", 40), Files: 20}},
				{done: true, result: domain.BackupReport{State: "blocked", Message: "SSH access failed. Check access and retry Back up lab."}},
			} {
				m.backup = b
				view := m.View().Content
				if !strings.Contains(view, "Enter") || !strings.Contains(view, "F1") {
					t.Fatalf("actions missing at %v", size)
				}
				if strings.Count(view, "\n")+1 > size[1] {
					t.Fatalf("screen overflow at %v", size)
				}
			}
		}
	}
}
