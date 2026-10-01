package presentation

import (
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type backupModel struct {
	field       int // 0 destination, 1 passphrase, 2 repeat
	destination string
	passphrase  []rune
	repeat      []rune
	result      domain.BackupReport
	done        bool
}

type backupResultMsg struct{ report domain.BackupReport }

func (model dashboardModel) openBackup() (tea.Model, tea.Cmd) {
	model.screen = dashboardBackup
	model.message = ""
	model.backup = backupModel{}
	if model.actions.BackupDestination != nil {
		model.backup.destination = model.actions.BackupDestination()
	}
	return model, nil
}

func (model dashboardModel) updateBackup(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := &model.backup
	if b.done {
		switch key.String() {
		case "enter", "esc":
			model.screen = dashboardAdministration
			model.backup = backupModel{}
			return model.refreshOverview()
		}
		return model, nil
	}
	switch key.String() {
	case "esc":
		model.screen = dashboardAdministration
		model.backup = backupModel{}
		model.message = "No backup was written."
		return model, nil
	case "tab", "down":
		b.field = (b.field + 1) % 3
	case "shift+tab", "up":
		b.field = (b.field + 2) % 3
	case "backspace":
		switch b.field {
		case 0:
			if value := []rune(b.destination); len(value) > 0 {
				b.destination = string(value[:len(value)-1])
			}
		case 1:
			if len(b.passphrase) > 0 {
				b.passphrase = b.passphrase[:len(b.passphrase)-1]
			}
		default:
			if len(b.repeat) > 0 {
				b.repeat = b.repeat[:len(b.repeat)-1]
			}
		}
	case "enter":
		if b.field < 2 {
			b.field++
			return model, nil
		}
		switch {
		case len(b.passphrase) < domain.BackupMinimumPassphrase:
			model.message = fmt.Sprintf("Use a passphrase of at least %d characters.", domain.BackupMinimumPassphrase)
			return model, nil
		case string(b.passphrase) != string(b.repeat):
			b.repeat = nil
			model.message = "The two passphrases differ; type the second one again."
			return model, nil
		case strings.TrimSpace(b.destination) == "":
			b.field = 0
			model.message = "Choose a directory, ideally on a USB drive or network share."
			return model, nil
		case model.actions.CreateBackup == nil:
			model.message = "Backups are not available in this session."
			return model, nil
		}
		destination, passphrase, action := strings.TrimSpace(b.destination), []byte(string(b.passphrase)), model.actions.CreateBackup
		b.passphrase, b.repeat = nil, nil
		model.busy = "Writing the encrypted backup"
		model.message = ""
		return model, func() tea.Msg {
			report := action(destination, passphrase)
			for index := range passphrase {
				passphrase[index] = 0
			}
			return backupResultMsg{report: report}
		}
	default:
		if key.Text == "" {
			return model, nil
		}
		switch b.field {
		case 0:
			b.destination += key.Text
		case 1:
			b.passphrase = append(b.passphrase, []rune(key.Text)...)
		default:
			b.repeat = append(b.repeat, []rune(key.Text)...)
		}
	}
	return model, nil
}

func (model dashboardModel) backupView() string {
	b := model.backup
	shell := tuiShell{path: []string{"Maintenance", "Back up the controller"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{tuiTitle("Back up the controller", model.isDark)}
	if b.done {
		lines = append(lines, "", tuiResult(b.result.Message, !b.result.HasErrors(), model.isDark), "")
		if b.result.Path != "" {
			lines = append(lines, "File:         "+b.result.Path, fmt.Sprintf("Size:         %s, %d entries", humanBytes(uint64(b.result.Bytes)), b.result.Files))
			lines = append(lines, "Private keys: "+strings.Join(b.result.PrivateKeys, ", "))
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Maintenance"}, {key: "F1", label: "Help"}}
	} else {
		lines = append(lines,
			tuiMuted("One encrypted file with the configuration and its history, the private keys and the trusted computer keys.", model.isDark),
			tuiMuted("Store it away from this controller. Without the passphrase nobody, including you, can read it.", model.isDark), "")
		fields := []struct{ label, value string }{
			{"Directory", b.destination},
			{"Passphrase", strings.Repeat("•", len(b.passphrase))},
			{"Repeat passphrase", strings.Repeat("•", len(b.repeat))},
		}
		for index, field := range fields {
			value := field.value
			if index == b.field {
				value += "_"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%-18s %s", field.label, value), index == b.field, model.isDark))
		}
		shell.actions = []tuiAction{{key: "Tab", label: "Next field"}, {key: "Enter", label: "Write backup"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
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
