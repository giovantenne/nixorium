package presentation

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Back up lab and Restore lab share one screen. Both start by choosing where
// the backup lives: an encrypted file (for example on a USB drive) or a private
// Git repository. A configured Git backup is then kept current automatically.
type backupModel struct {
	choosing, archive                   bool
	field                               int
	remote, branch, destination, source string
	passphrase, repeat                  []rune
	plan                                domain.GitBackupPlan
	reviewed, restore, standalone, done bool
	result                              domain.BackupReport
	// found lists backup files on connected drives, newest first.
	found []string
	// adopted holds the result of using the restored lab on this controller.
	adopted     bool
	adoptResult domain.BackupReport
}
type backupResultMsg struct{ report domain.BackupReport }
type backupAdoptMsg struct{ report domain.BackupReport }
type gitBackupPlanMsg struct {
	plan domain.GitBackupPlan
	err  error
}

// backupSyncState tracks the background push of saved changes to a configured
// Git backup. failed holds a revision whose push already failed, so a refresh
// does not retry it until the configuration changes or the administrator asks.
type backupSyncState struct {
	running bool
	failed  string
}
type backupSyncMsg struct {
	report   domain.BackupReport
	recovery domain.RecoveryReport
	manual   bool
}

func RunRestoreLab(actions DashboardActions) error {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, actions, false)
	model.screen = dashboardBackup
	model.backup = backupModel{restore: true, choosing: true, standalone: true, branch: "master"}
	_, err := tea.NewProgram(model).Run()
	return err
}

func (model dashboardModel) openBackup() (tea.Model, tea.Cmd) {
	model.screen = dashboardBackup
	model.message = ""
	model.backup = backupModel{choosing: true, branch: "master"}
	if model.actions.GitBackupDestination != nil {
		model.backup.remote, model.backup.branch = model.actions.GitBackupDestination()
	}
	return model, nil
}
func (model dashboardModel) openRestoreLab() (tea.Model, tea.Cmd) {
	next, _ := model.openBackup()
	model = next.(dashboardModel)
	model.backup.restore = true
	if model.actions.FindBackupFiles != nil {
		model.backup.found = model.actions.FindBackupFiles()
	}
	return model, nil
}

// offersAdopt reports a completed restore that can become the lab this
// controller manages, the step a new or replaced controller needs.
func (model dashboardModel) offersAdopt() bool {
	b := model.backup
	return b.done && b.restore && !b.standalone && !b.result.HasErrors() && b.result.Path != "" && model.actions.UseRestoredLab != nil
}

func (model dashboardModel) useRestoredLab() (tea.Model, tea.Cmd) {
	action, restored, source := model.actions.UseRestoredLab, model.backup.result, ""
	if model.backup.archive {
		source = strings.TrimSpace(model.backup.source)
	}
	model.busy = "Putting the restored lab in place and installing its keys"
	model.message = ""
	return model, func() tea.Msg { return backupAdoptMsg{action(restored, source)} }
}
func (b *backupModel) clearSecrets() {
	clear(b.passphrase)
	clear(b.repeat)
	b.passphrase = nil
	b.repeat = nil
}

// gitConfigured reports a Git backup that was published from this deployment.
func (b backupModel) gitConfigured() bool { return !b.restore && b.remote != "" }

func (b backupModel) choices() int {
	if b.gitConfigured() {
		return 3
	}
	return 2
}

// withBackupSync starts the background push when it is useful, keeping any
// command already returned by the update.
func (model dashboardModel) withBackupSync(command tea.Cmd) (dashboardModel, tea.Cmd) {
	action := model.actions.SyncGitBackup
	if action == nil || model.actions.ClassroomMode || model.backupSync.running {
		return model, command
	}
	model.backupSync.running = true
	skip, observe := model.backupSync.failed, model.observeRecovery
	return model, tea.Batch(command, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		report := action(ctx, skip)
		message := backupSyncMsg{report: report}
		if report.State == "completed" {
			message.recovery = observe(ctx)
		}
		return message
	})
}

func (model dashboardModel) syncBackupNow() (tea.Model, tea.Cmd) {
	action := model.actions.SyncGitBackup
	model.busy = "Pushing saved changes to the Git backup"
	return model.startBoundedRead(6*time.Minute, func(ctx context.Context) tea.Msg {
		return backupSyncMsg{report: action(ctx, ""), manual: true}
	})
}

func (model dashboardModel) finishBackupSync(message backupSyncMsg) (tea.Model, tea.Cmd) {
	report := message.report
	if message.manual {
		model.busy = ""
		b := &model.backup
		switch report.State {
		case "completed", "current":
			model.backupSync.failed = ""
			b.done, b.result = true, report
		case "waiting":
			model.message = "Save or discard the deployment changes first: Maintenance → Review Git changes."
		case "needs-passphrase", "unconfigured":
			// Fall through to the reviewed Git backup, which encrypts again.
			b.choosing, b.archive, b.field = false, false, 0
			model.message = report.Message
		default:
			model.backupSync.failed = report.Revision
			b.done, b.result = true, report
		}
		return model, nil
	}
	model.backupSync.running = false
	switch report.State {
	case "completed":
		model.backupSync.failed = ""
		model.observeRecoveryController(message.recovery)
		if model.message == "" && model.screen == dashboardHome {
			model.message = report.Message
		}
	case "blocked":
		model.backupSync.failed = report.Revision
		if model.message == "" && model.screen == dashboardHome {
			model.message = report.Message
		}
	}
	return model, nil
}

func (model dashboardModel) updateBackup(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := &model.backup
	if b.done && key.String() == "enter" && model.offersAdopt() && !b.adopted {
		return model.useRestoredLab()
	}
	if b.adopted && key.String() == "enter" && !b.adoptResult.HasErrors() && model.actions.PlanController != nil {
		model.backup = backupModel{}
		return model.openControllerReview()
	}
	if key.String() == "esc" || (b.done && key.String() == "enter") {
		done := b.done
		b.clearSecrets()
		if !done && !b.choosing {
			// Return to the first choice instead of leaving the screen.
			restore, standalone := b.restore, b.standalone
			next, _ := model.openBackup()
			model = next.(dashboardModel)
			model.backup.restore, model.backup.standalone = restore, standalone
			return model, nil
		}
		if b.standalone {
			return model, tea.Quit
		}
		model.screen = dashboardAdministration
		model.backup = backupModel{}
		if done {
			return model.refreshOverview()
		}
		return model, nil
	}
	if b.done {
		return model, nil
	}
	if b.choosing {
		choices := b.choices()
		switch key.String() {
		case "up", "k", "shift+tab":
			b.field = (b.field + choices - 1) % choices
		case "down", "j", "tab":
			b.field = (b.field + 1) % choices
		case "f", "g", "r", "enter":
			switch key.String() {
			case "f":
				b.field = 0
			case "g":
				b.field = 1
			case "r":
				if choices < 3 {
					return model, nil
				}
				b.field = 2
			}
			choice := b.field
			b.archive = choice == 0
			b.choosing, b.field = false, 0
			model.message = ""
			if b.archive && !b.restore && b.destination == "" && model.actions.BackupFileFolder != nil {
				b.destination = model.actions.BackupFileFolder()
			}
			if b.restore && b.destination == "" && model.actions.RestoreFolder != nil {
				b.destination = model.actions.RestoreFolder()
			}
			if b.restore && b.archive && b.source == "" && len(b.found) > 0 {
				b.source = b.found[0]
			}
			if b.restore && b.archive && b.source != "" {
				// The newest backup is proposed: the passphrase comes next.
				b.field = 1
			}
			if choice == 1 && b.gitConfigured() && model.actions.SyncGitBackup != nil {
				return model.syncBackupNow()
			}
		}
		return model, nil
	}
	fields := 2
	switch {
	case b.restore && b.archive:
		fields = 3
	case b.restore:
		fields = 4
	case b.archive:
		fields = 3
	}
	switch key.String() {
	case "tab", "down":
		if (b.archive || b.restore) && b.reviewed {
			return model, nil
		}
		b.field = (b.field + 1) % fields
	case "shift+tab", "up":
		if (b.archive || b.restore) && b.reviewed {
			return model, nil
		}
		b.field = (b.field + fields - 1) % fields
	case "enter":
		if b.field < fields-1 {
			b.field++
			return model, nil
		}
		if !b.restore && !b.archive && !b.reviewed {
			if model.actions.PlanGitBackup == nil {
				model.message = "Backup is unavailable in this session."
				return model, nil
			}
			remote, branch, action := strings.TrimSpace(b.remote), strings.TrimSpace(b.branch), model.actions.PlanGitBackup
			model.busy = "Checking saved configuration and recovery keys"
			return model.startRead(func(ctx context.Context) tea.Msg {
				plan, err := action(ctx, remote, branch)
				return gitBackupPlanMsg{plan, err}
			})
		}
		if b.restore && !b.reviewed {
			if b.archive && (strings.TrimSpace(b.source) == "" || strings.TrimSpace(b.destination) == "" || len(b.passphrase) == 0) {
				model.message = "Enter the backup file, its passphrase and the folder to restore into."
				return model, nil
			}
			if !b.archive && (b.remote == "" || b.branch == "" || b.destination == "" || len(b.passphrase) == 0) {
				model.message = "Enter the SSH repository URL, branch, the folder to restore into and the passphrase."
				return model, nil
			}
			b.reviewed = true
			model.message = ""
			return model, nil
		}
		if !b.restore {
			if len(b.passphrase) < domain.BackupMinimumPassphrase || string(b.passphrase) != string(b.repeat) {
				model.message = "Use at least 12 characters and repeat the same passphrase."
				return model, nil
			}
			if b.archive && !b.reviewed {
				if strings.TrimSpace(b.destination) == "" {
					model.message = "Enter an existing destination folder outside the deployment."
					return model, nil
				}
				b.reviewed = true
				model.message = ""
				return model, nil
			}
			if (b.archive && model.actions.CreateBackup == nil) || (!b.archive && model.actions.PublishGitBackup == nil) {
				model.message = "Backup is unavailable in this session."
				return model, nil
			}
		} else if (b.archive && model.actions.RestoreLabArchive == nil) || (!b.archive && model.actions.RestoreLab == nil) {
			model.message = "Restore is unavailable in this session."
			return model, nil
		}
		passphrase := []byte(string(b.passphrase))
		b.clearSecrets()
		model.message = ""
		if b.restore && b.archive {
			action, source, target := model.actions.RestoreLabArchive, strings.TrimSpace(b.source), strings.TrimSpace(b.destination)
			model.busy = "Restoring the laboratory from the backup file"
			return model, func() tea.Msg {
				defer clear(passphrase)
				return backupResultMsg{action(source, target, passphrase)}
			}
		}
		if b.restore {
			action, remote, branch, target := model.actions.RestoreLab, b.remote, b.branch, b.destination
			model.busy = "Restoring the laboratory and verifying its original keys"
			return model, func() tea.Msg {
				defer clear(passphrase)
				return backupResultMsg{action(remote, branch, target, passphrase)}
			}
		}
		if b.archive {
			action, destination := model.actions.CreateBackup, strings.TrimSpace(b.destination)
			model.busy = "Creating an encrypted backup file"
			return model, func() tea.Msg {
				defer clear(passphrase)
				return backupResultMsg{action(destination, passphrase)}
			}
		}
		action, plan := model.actions.PublishGitBackup, b.plan
		model.busy = "Saving encrypted keys, pushing and verifying the remote backup"
		return model, func() tea.Msg { defer clear(passphrase); return backupResultMsg{action(plan, passphrase)} }
	default:
		if (b.restore || b.archive) && b.reviewed {
			return model, nil
		}
		var plain *string
		var secret *[]rune
		switch {
		case b.restore && b.archive:
			switch b.field {
			case 0:
				plain = &b.source
			case 1:
				secret = &b.passphrase
			case 2:
				plain = &b.destination
			}
		case b.restore:
			switch b.field {
			case 0:
				plain = &b.remote
			case 1:
				plain = &b.branch
			case 2:
				plain = &b.destination
			case 3:
				secret = &b.passphrase
			}
		case b.archive:
			switch b.field {
			case 0:
				plain = &b.destination
			case 1:
				secret = &b.passphrase
			case 2:
				secret = &b.repeat
			}
		case b.reviewed:
			switch b.field {
			case 0:
				secret = &b.passphrase
			case 1:
				secret = &b.repeat
			}
		case b.field == 0:
			plain = &b.remote
		default:
			plain = &b.branch
		}
		if key.String() == "backspace" {
			if plain != nil {
				r := []rune(*plain)
				if len(r) > 0 {
					*plain = string(r[:len(r)-1])
				}
			}
			if secret != nil && len(*secret) > 0 {
				(*secret)[len(*secret)-1] = 0
				*secret = (*secret)[:len(*secret)-1]
			}
		} else if key.Text != "" {
			if plain != nil && len(*plain)+len(key.Text) <= 4096 {
				*plain += key.Text
			}
			if secret != nil && len(*secret)+len([]rune(key.Text)) <= 1024 {
				*secret = append(*secret, []rune(key.Text)...)
			}
		}
	}
	return model, nil
}

func (model dashboardModel) backupView() string {
	b := model.backup
	title := "Back up lab"
	if b.restore {
		title = "Restore lab"
	}
	shell := tuiShell{path: []string{"Maintenance", title}}
	if b.standalone {
		shell.path = []string{title}
	}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{tuiTitle(title, model.isDark)}
	actions := []tuiAction{{key: "Tab", label: "Next field"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	var notices []tuiNotice
	if b.adopted {
		lines = append(lines, "", tuiResult(b.adoptResult.Message, !b.adoptResult.HasErrors(), model.isDark))
		if b.adoptResult.Previous != "" {
			lines = append(lines, "Previous lab folder, kept unchanged: "+b.adoptResult.Previous)
		}
		actions = []tuiAction{{key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
		if !b.adoptResult.HasErrors() {
			lines = append(lines, "", tuiSection("Next: apply the restored lab to this controller", model.isDark),
				"  The review shows what changes on this controller; nothing changes until you confirm it.",
				"  Afterwards, computers reinstalled since the backup was made show \"SSH key changed\":",
				"  compare the fingerprint on their screen before trusting them.")
			if model.actions.PlanController != nil {
				actions = []tuiAction{{key: "Enter", label: "Review controller"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
			}
		}
	} else if b.done {
		lines = append(lines, "", tuiResult(b.result.Message, !b.result.HasErrors(), model.isDark))
		if b.result.Path != "" && !b.restore {
			lines = append(lines, "Location: "+b.result.Path)
		}
		if !b.result.CreatedAt.IsZero() {
			created := "Created:  " + b.result.CreatedAt.Local().Format("2006-01-02 15:04")
			if b.restore {
				created = "Backup made: " + b.result.CreatedAt.Local().Format("2006-01-02 15:04")
			}
			if strings.HasSuffix(b.result.Path, ".age") {
				created += " (local time; the file name uses UTC)"
			}
			lines = append(lines, created)
		}
		if b.result.Revision != "" {
			lines = append(lines, "Revision: "+shortRevision(b.result.Revision))
		}
		actions = []tuiAction{{key: "Enter", label: "Return"}, {key: "F1", label: "Help"}}
		switch {
		case model.offersAdopt():
			lines = append(lines, "", tuiSection("Next: use this lab on this controller", model.isDark),
				"  For a new or replaced controller. Nixorium then manages the restored lab:",
				"  • the current lab folder is kept beside it under a new name; nothing is deleted",
				"  • the restored lab takes its place and its keys are installed on this controller",
				"  • nothing is built yet: you review and apply the controller next")
			if model.report.Meta.Clients.Count > 0 {
				notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: fmt.Sprintf("This controller already manages %s.", countNoun(model.report.Meta.Clients.Count, "computer")), detail: "Use the restored lab only if it replaces the lab of this controller."})
			}
			actions = []tuiAction{{key: "Enter", label: "Use this lab on this controller"}, {key: "Esc", label: "Not now"}, {key: "F1", label: "Help"}}
		case b.restore && !b.result.HasErrors():
			lines = append(lines, "", "To make this controller use it, follow \"Replacing the controller\" in the troubleshooting guide.")
		}
	} else if b.choosing && b.restore {
		lines = append(lines, "Brings back a lab from its backup, for example on a new or replaced controller.", "Existing folders are never replaced and no system is activated.", "", "Where is the backup?", "",
			tuiSelection("[f] Backup file — for example on a USB drive", b.field == 0, model.isDark),
			tuiSelection("[g] Private Git repository", b.field == 1, model.isDark))
		actions = []tuiAction{{key: "↑/↓", label: "Choose"}, {key: "Enter", label: "Continue"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else if b.choosing {
		lines = append(lines, "Backup is optional. It saves the configuration, private keys, account", "passwords and trusted computers, encrypted with your passphrase.", "")
		git := "[g] Private Git repository — after the first backup, saved changes are pushed automatically"
		if b.gitConfigured() {
			lines = append(lines, "Git backup: "+b.remote+" ("+b.branch+"). Saved changes are pushed automatically", "while Nixorium is open.", "")
			git = "[g] Back up now to " + b.remote
		}
		lines = append(lines, tuiSelection("[f] Encrypted file — in a folder or on a USB drive", b.field == 0, model.isDark),
			tuiSelection(git, b.field == 1, model.isDark))
		if b.gitConfigured() {
			lines = append(lines, tuiSelection("[r] Use a different Git repository", b.field == 2, model.isDark))
		}
		actions = []tuiAction{{key: "↑/↓", label: "Choose"}, {key: "Enter", label: "Continue"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else {
		type field struct{ label, value string }
		fields := []field{}
		dots := func(r []rune) string { return strings.Repeat("•", len(r)) }
		switch {
		case b.restore && b.archive && b.reviewed:
			lines = append(lines, "", "Backup file: "+b.source, "Restore into: "+b.destination, "", "Restore the deployment with its private keys, account passwords and trusted computers.", "Existing folders are never replaced. No system will be activated.")
			actions = []tuiAction{{key: "Enter", label: "Restore lab"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		case b.restore && b.archive:
			lines = append(lines, "Choose the backup file (nixorium-backup-….age) and enter the passphrase used", "when it was made. The lab is restored into a new folder; nothing is replaced.")
			switch len(b.found) {
			case 0:
				lines = append(lines, "No backup file was found on a connected drive: connect the USB drive and", "open Restore lab again, or type the file's full path.")
			case 1:
				lines = append(lines, "Found on a connected drive: "+b.found[0])
			default:
				lines = append(lines, fmt.Sprintf("Found %d backup files; the newest is proposed.", len(b.found)))
			}
			lines = append(lines, "")
			fields = []field{{"Backup file", b.source}, {"Passphrase", dots(b.passphrase)}, {"Restore into", b.destination}}
		case b.restore && b.reviewed:
			lines = append(lines, "", "Repository: "+b.remote, "Branch: "+b.branch, "Restore into: "+b.destination, "", "Restore original keys and trusted computers. Existing folders are never replaced.", "No system will be activated.")
			actions = []tuiAction{{key: "Enter", label: "Restore lab"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		case b.restore:
			lines = append(lines, "Restore into a new folder using independent SSH access.", "No password or Git token belongs in the repository URL.", "")
			fields = []field{{"SSH repository", b.remote}, {"Branch", b.branch}, {"Restore into", b.destination}, {"Passphrase", dots(b.passphrase)}}
		case b.archive && b.reviewed:
			lines = append(lines, "", "Destination folder: "+b.destination,
				"The entire deployment, Git history, private keys, account passwords and",
				"trusted computers will be encrypted in one new .age file. Nothing will be pushed.",
				"Keep the backup away from this controller and the passphrase separately.")
			actions = []tuiAction{{key: "Enter", label: "Create backup"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		case b.archive:
			proposal := "A connected USB drive is proposed; keep the backup away from this controller."
			if !strings.HasPrefix(b.destination, "/run/media/") {
				proposal = "Your home folder is proposed. For a USB drive, connect it and open Back up lab again."
			}
			lines = append(lines, "Choose an existing folder outside the deployment.", proposal,
				"Use a passphrase of at least 12 characters. The whole archive is encrypted.", "")
			fields = []field{{"Folder", b.destination}, {"Passphrase", dots(b.passphrase)}, {"Repeat", dots(b.repeat)}}
		case b.reviewed:
			lines = append(lines, "Repository: "+b.plan.Remote, "Branch: "+b.plan.Branch+" · revision "+shortRevision(b.plan.Revision), fmt.Sprintf("%d tracked files and Git history; ignored files excluded.", b.plan.Files), "Configuration stays readable; password hashes and private keys are encrypted.", "Later saved changes are pushed automatically. The passphrase is needed again only", "when keys, account passwords or trusted computers change.", "Confirm the repository is PRIVATE on GitHub/GitLab, then press Enter to encrypt and push.", "")
			fields = []field{{"Passphrase", dots(b.passphrase)}, {"Repeat", dots(b.repeat)}}
			actions[1].label = "Encrypt and push"
		default:
			lines = append(lines, "Save changes through Review Git changes first.", "Use a PRIVATE repository and independent SSH access; F1 explains setup.", "")
			fields = []field{{"SSH repository", b.remote}, {"Branch", b.branch}}
		}
		for i, f := range fields {
			value := f.value
			if i == b.field {
				value += "_"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%-16s %s", f.label, value), i == b.field, model.isDark))
		}
	}
	shell.body = strings.Join(lines, "\n")
	shell.actions = actions
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	shell.notices = notices
	return model.renderShell(shell)
}

func BackupText(w io.Writer, report domain.BackupReport) {
	fmt.Fprintln(w, report.Message)
	if report.Path != "" {
		fmt.Fprintf(w, "Path: %s\n", report.Path)
	}
	if report.Bytes > 0 {
		fmt.Fprintf(w, "Size: %s\n", humanBytes(uint64(report.Bytes)))
	}
	if !report.CreatedAt.IsZero() {
		fmt.Fprintf(w, "Created: %s, revision %s, %d entries\n", report.CreatedAt.Local().Format("2006-01-02 15:04"), report.Revision, report.Files)
		fmt.Fprintf(w, "Private keys: %s\n", strings.Join(report.PrivateKeys, ", "))
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(w, "  %s: %s\n", issue.Field, issue.Message)
	}
}
