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
	model.backup = backupModel{branch: "main"}
	if model.actions.GitBackupDestination != nil {
		model.backup.remote, model.backup.branch = model.actions.GitBackupDestination()
	}
	return model, nil
}
func (model dashboardModel) openRestoreLab() (tea.Model, tea.Cmd) {
	next, _ := model.openBackup()
	model = next.(dashboardModel)
	model.backup.restore = true
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
	fields := 2
	if b.restore {
		fields = 4
	} else if b.reviewed {
		fields = 3
	}
	switch key.String() {
	case "tab", "down":
		b.field = (b.field + 1) % fields
	case "shift+tab", "up":
		b.field = (b.field + fields - 1) % fields
	case "enter":
		if b.field < fields-1 {
			b.field++
			return model, nil
		}
		if !b.restore && !b.reviewed {
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
			if b.confirmation != "PUSH" {
				model.message = "Confirm that this is your PRIVATE repository, then type PUSH."
				return model, nil
			}
			if model.actions.PublishGitBackup == nil {
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
		action, plan := model.actions.PublishGitBackup, b.plan
		model.busy = "Saving encrypted keys, pushing and verifying the remote backup"
		return model, func() tea.Msg { defer clear(passphrase); return backupResultMsg{action(plan, passphrase)} }
	default:
		if b.restore && b.reviewed {
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
