package presentation

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type backupModel struct {
	choosing, archive                   bool
	field                               int
	remote, branch, destination         string
	passphrase, repeat                  []rune
	confirmation                        string
	plan                                domain.GitBackupPlan
	reviewed, restore, standalone, done bool
	result                              domain.BackupReport
}
type backupResultMsg struct{ report domain.BackupReport }
type gitBackupPlanMsg struct {
	plan domain.GitBackupPlan
	err  error
}

func RunRestoreLab(actions DashboardActions) error {
	model := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, actions, false)
	model.screen = dashboardBackup
	model.backup = backupModel{restore: true, standalone: true, branch: "main"}
	_, err := tea.NewProgram(model).Run()
	return err
}

func (model dashboardModel) openBackup() (tea.Model, tea.Cmd) {
	model.screen = dashboardBackup
	model.message = ""
	model.backup = backupModel{choosing: true, branch: "main"}
	if model.actions.GitBackupDestination != nil {
		model.backup.remote, model.backup.branch = model.actions.GitBackupDestination()
	}
	return model, nil
}
func (model dashboardModel) openRestoreLab() (tea.Model, tea.Cmd) {
	next, _ := model.openBackup()
	model = next.(dashboardModel)
	model.backup.restore = true
	model.backup.choosing = false
	return model, nil
}
func (b *backupModel) clearSecrets() {
	clear(b.passphrase)
	clear(b.repeat)
	b.passphrase = nil
	b.repeat = nil
}

func (model dashboardModel) updateBackup(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := &model.backup
	if key.String() == "esc" || (b.done && key.String() == "enter") {
		done := b.done
		b.clearSecrets()
		if !done && !b.restore && !b.choosing {
			return model.openBackup()
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
		switch key.String() {
		case "up", "down", "j", "k", "tab", "shift+tab":
			b.field = 1 - b.field
		case "f", "g", "enter":
			if key.String() == "f" {
				b.field = 0
			}
			if key.String() == "g" {
				b.field = 1
			}
			b.archive = b.field == 0
			b.choosing, b.field = false, 0
		}
		return model, nil
	}
	fields := 2
	if b.restore {
		fields = 4
	} else if b.archive || b.reviewed {
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
			if b.remote == "" || b.branch == "" || b.destination == "" || len(b.passphrase) == 0 {
				model.message = "Enter the SSH repository URL, branch, new directory and passphrase."
				return model, nil
			}
			b.reviewed = true
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
			if !b.archive && b.confirmation != "PUSH" {
				model.message = "Confirm that this is your PRIVATE repository, then type PUSH."
				return model, nil
			}
			if (b.archive && model.actions.CreateBackup == nil) || (!b.archive && model.actions.PublishGitBackup == nil) {
				model.message = "Backup is unavailable in this session."
				return model, nil
			}
		} else if model.actions.RestoreLab == nil {
			model.message = "Restore is unavailable in this session."
			return model, nil
		}
		passphrase := []byte(string(b.passphrase))
		b.clearSecrets()
		model.message = ""
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
		if b.restore {
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
		} else if b.archive {
			switch b.field {
			case 0:
				plain = &b.destination
			case 1:
				secret = &b.passphrase
			case 2:
				secret = &b.repeat
			}
		} else if b.reviewed {
			switch b.field {
			case 0:
				secret = &b.passphrase
			case 1:
				secret = &b.repeat
			case 2:
				plain = &b.confirmation
			}
		} else if b.field == 0 {
			plain = &b.remote
		} else {
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
	actions := []tuiAction{{key: "Tab", label: "Next field"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	if b.done {
		lines = append(lines, "", tuiResult(b.result.Message, !b.result.HasErrors(), model.isDark))
		if b.result.Path != "" {
			lines = append(lines, "Location: "+b.result.Path)
		}
		if b.result.Revision != "" {
			lines = append(lines, "Revision: "+shortRevision(b.result.Revision))
		}
		actions = []tuiAction{{key: "Enter", label: "Return"}, {key: "F1", label: "Help"}}
	} else if b.choosing {
		lines = append(lines, "Backup is optional. Choose where to save it:", "",
			tuiSelection("[f] File / USB — encrypted archive in a folder", b.field == 0, model.isDark),
			tuiSelection("[g] Remote Git — private repository with encrypted secrets", b.field == 1, model.isDark))
		actions = []tuiAction{{key: "↑/↓", label: "Choose"}, {key: "Enter", label: "Continue"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else {
		type field struct{ label, value string }
		fields := []field{}
		if b.restore {
			if b.reviewed {
				lines = append(lines, "", "Repository: "+b.remote, "Branch: "+b.branch, "New folder: "+b.destination, "", "Restore original keys and trusted computers. Existing folders are never replaced.", "No system will be activated.")
				actions = []tuiAction{{key: "Enter", label: "Restore lab"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
			} else {
				lines = append(lines, "Restore into a new folder using independent SSH access.", "No password or Git token belongs in the repository URL.", "")
				fields = []field{{"SSH repository", b.remote}, {"Branch", b.branch}, {"New folder", b.destination}, {"Passphrase", strings.Repeat("•", len(b.passphrase))}}
			}
		} else if b.archive {
			if b.reviewed {
				lines = append(lines, "", "Destination folder: "+b.destination,
					"The entire deployment, Git history, private keys and account credentials",
					"will be encrypted in one new .age file. Nothing will be pushed.",
					"Keep the backup away from this controller and the passphrase separately.")
				actions = []tuiAction{{key: "Enter", label: "Create backup"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
			} else {
				lines = append(lines, "Choose an existing folder outside the deployment.",
					"For USB, mount the drive first and enter its folder here.",
					"Use a passphrase of at least 12 characters. The whole archive is encrypted.", "")
				fields = []field{{"Folder", b.destination}, {"Passphrase", strings.Repeat("•", len(b.passphrase))}, {"Repeat", strings.Repeat("•", len(b.repeat))}}
			}
		} else if b.reviewed {
			lines = append(lines, "Repository: "+b.plan.Remote, "Branch: "+b.plan.Branch+" · revision "+shortRevision(b.plan.Revision), fmt.Sprintf("%d tracked files and Git history; ignored files excluded.", b.plan.Files), "Configuration stays readable; password hashes and private keys are encrypted.", "Confirm the repository is PRIVATE on GitHub/GitLab. Type PUSH below.", "")
			fields = []field{{"Passphrase", strings.Repeat("•", len(b.passphrase))}, {"Repeat", strings.Repeat("•", len(b.repeat))}, {"Confirmation", b.confirmation}}
			actions[1].label = "Encrypt and push"
		} else {
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
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
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
