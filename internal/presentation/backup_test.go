package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
	"os"
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

func TestBackupScreensFitAndKeepActions(t *testing.T) {
	var gallery strings.Builder
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
			m.screen = dashboardBackup
			m.width = size[0]
			m.height = size[1]
			m.isDark = dark
			for _, b := range []backupModel{
				{choosing: true},
				{archive: true, destination: "/media/usb"},
				{archive: true, reviewed: true, destination: "/media/usb"},
				{branch: "main"},
				{restore: true, branch: "main"},
				{reviewed: true, plan: domain.GitBackupPlan{Remote: "git@github.com:school/lab.git", Branch: "main", Revision: strings.Repeat("a", 40), Files: 20}},
				{done: true, result: domain.BackupReport{State: "blocked", Message: "SSH access failed. Check access and retry Back up lab."}},
			} {
				m.backup = b
				view := m.View().Content
				if size[0] == 80 && dark {
					gallery.WriteString(demoANSI.ReplaceAllString(view, "") + "\n\n")
				}
				if !strings.Contains(view, "Enter") || !strings.Contains(view, "F1") {
					t.Fatalf("actions missing at %v", size)
				}
				if strings.Count(view, "\n")+1 > size[1] {
					t.Fatalf("screen overflow at %v", size)
				}
			}
		}
	}
	if path := os.Getenv("NIXORIUM_BACKUP_GALLERY"); path != "" {
		if err := os.WriteFile(path, []byte(gallery.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBackupChoiceAndArchiveReview(t *testing.T) {
	calls, pushes := 0, 0
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		CreateBackup: func(destination string, pass []byte) domain.BackupReport {
			calls++
			if destination != "/media/usb" || string(pass) != "quiet passphrase!" {
				t.Fatal("wrong archive input")
			}
			return domain.BackupReport{State: "completed", Message: "Archive complete", Path: "/media/usb/backup.age"}
		},
		PublishGitBackup: func(domain.GitBackupPlan, []byte) domain.BackupReport {
			pushes++
			return domain.BackupReport{}
		},
	}, false)
	next, _ := m.openBackup()
	m = next.(dashboardModel)
	if view := m.View().Content; !strings.Contains(view, "Encrypted file") || !strings.Contains(view, "USB drive") || !strings.Contains(view, "Private Git repository") || !strings.Contains(view, "optional") {
		t.Fatal("backup choices missing")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if !m.backup.archive || m.backup.choosing {
		t.Fatal("default choice did not open archive")
	}
	for _, field := range []string{"/media/usb", "quiet passphrase!", "mismatch"} {
		next, _ = m.Update(tea.KeyPressMsg{Text: field})
		m = next.(dashboardModel)
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(dashboardModel)
		if cmd != nil || calls != 0 {
			t.Fatal("archive created before review")
		}
	}
	if m.backup.reviewed {
		t.Fatal("mismatched passphrases accepted")
	}
	m.backup.repeat = append([]rune{}, m.backup.passphrase...)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd != nil || calls != 0 || !m.backup.reviewed {
		t.Fatal("archive skipped review")
	}
	if strings.Contains(m.View().Content, "quiet passphrase!") {
		t.Fatal("passphrase exposed")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd == nil || len(m.backup.passphrase) != 0 || len(m.backup.repeat) != 0 {
		t.Fatal("secret not consumed")
	}
	next, _ = m.Update(cmd())
	m = next.(dashboardModel)
	if calls != 1 || pushes != 0 || !m.backup.done {
		t.Fatal("archive did not complete independently of Git")
	}
}

func TestBackupChoiceCancellationClearsSecrets(t *testing.T) {
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
	next, _ := m.openBackup()
	m = next.(dashboardModel)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = next.(dashboardModel)
	if m.backup.choosing || m.backup.archive {
		t.Fatal("Git shortcut did not select remote")
	}
	m.backup.passphrase = []rune("quiet passphrase!")
	secret := m.backup.passphrase
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(dashboardModel)
	if cmd != nil || !m.backup.choosing || len(m.backup.passphrase) > 0 {
		t.Fatal("cancel did not return to choice")
	}
	for _, r := range secret {
		if r != 0 {
			t.Fatal("cancel retained secret")
		}
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || next.(dashboardModel).screen != dashboardAdministration {
		t.Fatal("cannot leave backup without creating one")
	}
}

func TestConfiguredGitBackupPushesWithoutPassphraseUnlessKeysChanged(t *testing.T) {
	state := "completed"
	syncs := 0
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		GitBackupDestination: func() (string, string) { return "git@example.invalid:school/lab.git", "main" },
		SyncGitBackup: func(_ context.Context, skip string) domain.BackupReport {
			syncs++
			if skip != "" {
				t.Fatal("manual backup skipped a failed revision")
			}
			return domain.BackupReport{State: state, Message: "Backed up", Path: "git@example.invalid:school/lab.git"}
		},
		PlanGitBackup: func(context.Context, string, string) (domain.GitBackupPlan, error) {
			return domain.GitBackupPlan{}, nil
		},
	}, false)
	next, _ := m.openBackup()
	m = next.(dashboardModel)
	view := m.View().Content
	if !strings.Contains(view, "pushed automatically") || !strings.Contains(view, "Back up now to git@example.invalid:school/lab.git") || !strings.Contains(view, "different Git repository") {
		t.Fatalf("configured Git backup not explained:\n%s", view)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Text: "g", Code: 'g'})
	m = next.(dashboardModel)
	if cmd == nil {
		t.Fatal("back up now did not start a push")
	}
	next, _ = m.Update(backupSyncMsg{report: domain.BackupReport{State: "completed", Message: "Backed up"}, manual: true})
	m = next.(dashboardModel)
	if !m.backup.done || m.backup.result.Message != "Backed up" {
		t.Fatalf("push result not shown: %+v", m.backup)
	}
	// Changed keys continue into the reviewed, passphrase-protected backup.
	next, _ = m.openBackup()
	m = next.(dashboardModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "g", Code: 'g'})
	m = next.(dashboardModel)
	next, _ = m.Update(backupSyncMsg{report: domain.BackupReport{State: "needs-passphrase", Message: "Keys changed"}, manual: true})
	m = next.(dashboardModel)
	if m.backup.done || m.backup.choosing || m.backup.archive || m.message != "Keys changed" {
		t.Fatalf("passphrase flow not offered: %+v %q", m.backup, m.message)
	}
	if view := m.View().Content; !strings.Contains(view, "SSH repository") || !strings.Contains(view, "git@example.invalid:school/lab.git") {
		t.Fatal("Git destination not prefilled")
	}
}

func TestAutomaticGitBackupRunsOnOverviewAndRemembersFailures(t *testing.T) {
	var skips []string
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		SyncGitBackup: func(_ context.Context, skip string) domain.BackupReport {
			skips = append(skips, skip)
			return domain.BackupReport{State: "blocked", Revision: "abc123", Message: "Automatic Git backup failed: offline"}
		},
	}, false)
	m.screen = dashboardAdministration
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(dashboardModel)
	if m.screen != dashboardHome || cmd == nil || !m.backupSync.running {
		t.Fatalf("returning to the overview did not start a push: screen=%v running=%v", m.screen, m.backupSync.running)
	}
	next, _ = m.Update(backupSyncMsg{report: domain.BackupReport{State: "blocked", Revision: "abc123", Message: "Automatic Git backup failed: offline"}})
	m = next.(dashboardModel)
	if m.backupSync.running || m.backupSync.failed != "abc123" || !strings.Contains(m.message, "offline") {
		t.Fatalf("failure not remembered: %+v %q", m.backupSync, m.message)
	}
	m, cmd = m.withBackupSync(nil)
	if cmd == nil {
		t.Fatal("no follow-up push")
	}
	for _, message := range drainBackupSync(cmd) {
		_ = message
	}
	if len(skips) == 0 || skips[len(skips)-1] != "abc123" {
		t.Fatalf("failed revision retried automatically: %v", skips)
	}
}

func TestRestoreLabFromBackupFileReview(t *testing.T) {
	restores := 0
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		RestoreLabArchive: func(source, target string, pass []byte) domain.BackupReport {
			restores++
			if source != "/media/usb/nixorium-backup.age" || target != "/home/admin/lab" || string(pass) != "long passphrase" {
				t.Fatalf("wrong restore input %q %q", source, target)
			}
			return domain.BackupReport{State: "completed", Message: "Lab restored"}
		},
	}, false)
	next, _ := m.openRestoreLab()
	m = next.(dashboardModel)
	if view := m.View().Content; !strings.Contains(view, "Backup file") || !strings.Contains(view, "Private Git repository") {
		t.Fatal("restore choices missing")
	}
	next, _ = m.Update(tea.KeyPressMsg{Text: "f", Code: 'f'})
	m = next.(dashboardModel)
	for _, field := range []string{"/media/usb/nixorium-backup.age", "/home/admin/lab", "long passphrase"} {
		next, _ = m.Update(tea.KeyPressMsg{Text: field})
		m = next.(dashboardModel)
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(dashboardModel)
	}
	if !m.backup.reviewed || restores != 0 || strings.Contains(m.View().Content, "long passphrase") {
		t.Fatal("restore skipped review or exposed the passphrase")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(dashboardModel)
	if cmd == nil {
		t.Fatal("restore did not start")
	}
	next, _ = m.Update(cmd())
	m = next.(dashboardModel)
	if restores != 1 || !m.backup.done || m.backup.result.Message != "Lab restored" {
		t.Fatalf("restore result = %+v", m.backup)
	}
}

// drainBackupSync runs a batched command tree and returns its messages.
func drainBackupSync(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		var messages []tea.Msg
		for _, inner := range batch {
			messages = append(messages, drainBackupSync(inner)...)
		}
		return messages
	}
	return []tea.Msg{message}
}

func TestBackupFileProposesTheHomeFolder(t *testing.T) {
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		BackupFileFolder: func() string { return "/home/admin" },
	}, false)
	next, _ := m.openBackup()
	m = next.(dashboardModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "f", Code: 'f'})
	m = next.(dashboardModel)
	if m.backup.destination != "/home/admin" || !strings.Contains(m.View().Content, "/home/admin") {
		t.Fatalf("home folder not proposed: %q", m.backup.destination)
	}
}
