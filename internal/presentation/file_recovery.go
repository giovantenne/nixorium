package presentation

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// FileRecoveryActions recover a student's files from the sessions a
// student computer saved at its last startups. The string argument is the
// computer's name.
type FileRecoveryActions struct {
	Sessions func(context.Context, string) ([]domain.FileRecoverySession, error)
	List     func(context.Context, string, int, string) (domain.FileRecoveryListing, error)
	Plan     func(context.Context, string, int, []string) domain.FileRecoveryPlan
	Apply    func(domain.FileRecoveryPlan) domain.FileRecoveryReport
}

var recoverFilesTask = dashboardTask{id: "recover", shortcut: "r", title: "Recover student files", description: "Copy files from a past session back to a student's desktop"}

// Stages of Recover student files.
const (
	recoveryStageComputer = iota
	recoveryStageSession
	recoveryStageBrowse
	recoveryStageReview
	recoveryStageResult
)

type fileRecoveryModel struct {
	stage int
	// The computer, chosen with the cursor of the inventory.
	computerCursor int
	computer       string
	sessions       []domain.FileRecoverySession
	sessionCursor  int
	session        domain.FileRecoverySession
	listing        domain.FileRecoveryListing
	cursor         int
	// chosen holds the chosen files and folders by path; never one inside
	// another.
	chosen map[string]bool
	plan   domain.FileRecoveryPlan
	result domain.FileRecoveryReport
}

func newFileRecoveryModel() fileRecoveryModel {
	return fileRecoveryModel{chosen: map[string]bool{}}
}

type fileRecoverySessionsMsg struct {
	computer string
	sessions []domain.FileRecoverySession
	err      error
}
type fileRecoveryListMsg struct {
	listing domain.FileRecoveryListing
	err     error
}
type fileRecoveryPlanMsg struct{ plan domain.FileRecoveryPlan }
type fileRecoveryApplyMsg struct{ report domain.FileRecoveryReport }

// choose toggles one path. Choosing a folder drops the choices inside it,
// and choosing something inside a chosen folder drops that folder.
func (m *fileRecoveryModel) choose(name string) {
	if m.chosen[name] {
		delete(m.chosen, name)
		return
	}
	for other := range m.chosen {
		if strings.HasPrefix(other, name+"/") || strings.HasPrefix(name, other+"/") {
			delete(m.chosen, other)
		}
	}
	m.chosen[name] = true
}

func (m fileRecoveryModel) chosenPaths() []string {
	paths := make([]string, 0, len(m.chosen))
	for name := range m.chosen {
		paths = append(paths, name)
	}
	slices.Sort(paths)
	return paths
}

func (m fileRecoveryModel) entryPath(entry domain.FileRecoveryEntry) string {
	if m.listing.Path == "" {
		return entry.Name
	}
	return m.listing.Path + "/" + entry.Name
}

func recoverySessionLabel(session domain.FileRecoverySession) string {
	if session.SavedAt.IsZero() {
		return fmt.Sprintf("Session %d", session.Number)
	}
	return "Session before " + session.SavedAt.Local().Format("Mon 2 Jan 2006, 15:04")
}

func (model dashboardModel) openFileRecovery() (tea.Model, tea.Cmd) {
	if model.actions.FileRecovery == nil {
		return model, nil
	}
	model.screen = dashboardFileRecovery
	model.fileRecovery = newFileRecoveryModel()
	model.message = ""
	return model, nil
}

func (model dashboardModel) listRecoveryFolder(folder string) (tea.Model, tea.Cmd) {
	m := model.fileRecovery
	actions := model.actions.FileRecovery
	model.busy = "Reading the saved session on " + m.computer
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		listing, err := actions.List(ctx, m.computer, m.session.Number, folder)
		return fileRecoveryListMsg{listing, err}
	})
}

func (model dashboardModel) updateFileRecovery(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.fileRecovery
	actions := model.actions.FileRecovery
	if actions == nil {
		model.screen = dashboardComputersArea
		return model, nil
	}
	hosts := model.report.Meta.Clients.Hosts
	switch m.stage {
	case recoveryStageComputer:
		switch key.String() {
		case "esc":
			model.screen = dashboardComputersArea
			model.message = ""
		case "up", "k":
			m.computerCursor = max(0, m.computerCursor-1)
		case "down", "j":
			m.computerCursor = min(max(0, len(hosts)-1), m.computerCursor+1)
		case "enter":
			if len(hosts) == 0 {
				return model, nil
			}
			computer := hosts[m.computerCursor].Name
			model.busy = "Reading the saved sessions of " + computer
			model.message = ""
			return model.startRead(func(ctx context.Context) tea.Msg {
				sessions, err := actions.Sessions(ctx, computer)
				return fileRecoverySessionsMsg{computer, sessions, err}
			})
		}
	case recoveryStageSession:
		switch key.String() {
		case "esc":
			m.stage = recoveryStageComputer
			model.message = ""
		case "up", "k":
			m.sessionCursor = max(0, m.sessionCursor-1)
		case "down", "j":
			m.sessionCursor = min(max(0, len(m.sessions)-1), m.sessionCursor+1)
		case "enter":
			if len(m.sessions) == 0 {
				return model, nil
			}
			m.session = m.sessions[m.sessionCursor]
			m.chosen = map[string]bool{}
			return model.listRecoveryFolder("")
		}
	case recoveryStageBrowse:
		entries := m.listing.Entries
		switch key.String() {
		case "esc":
			m.stage = recoveryStageSession
			model.message = ""
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(max(0, len(entries)-1), m.cursor+1)
		case "space":
			if len(entries) > 0 {
				m.choose(m.entryPath(entries[m.cursor]))
			}
		case "enter", "right":
			if len(entries) > 0 && entries[m.cursor].Dir {
				return model.listRecoveryFolder(m.entryPath(entries[m.cursor]))
			}
			if len(entries) > 0 {
				m.choose(m.entryPath(entries[m.cursor]))
			}
		case "backspace", "left":
			if m.listing.Path != "" {
				parent := path.Dir(m.listing.Path)
				if parent == "." {
					parent = ""
				}
				return model.listRecoveryFolder(parent)
			}
		case "r":
			paths := m.chosenPaths()
			if len(paths) == 0 {
				model.message = "Choose at least one file or folder with Space."
				return model, nil
			}
			computer, session := m.computer, m.session.Number
			model.busy = "Checking " + computer
			model.message = ""
			return model.startRead(func(ctx context.Context) tea.Msg {
				return fileRecoveryPlanMsg{actions.Plan(ctx, computer, session, paths)}
			})
		}
	case recoveryStageReview:
		switch key.String() {
		case "esc":
			m.stage = recoveryStageBrowse
			model.message = ""
		case "enter":
			if m.plan.HasErrors() {
				return model, nil
			}
			plan := m.plan
			model.busy = "Copying the files onto the desktop of " + m.computer
			model.message = ""
			return model, func() tea.Msg { return fileRecoveryApplyMsg{actions.Apply(plan)} }
		}
	case recoveryStageResult:
		switch key.String() {
		case "n":
			// Another recovery from the same session.
			m.chosen = map[string]bool{}
			m.stage = recoveryStageBrowse
			model.message = ""
		case "enter", "esc":
			model.screen = dashboardComputersArea
			model.message = ""
		}
	}
	return model, nil
}

// handleFileRecoveryMessage applies the answers of the reads above.
func (model dashboardModel) handleFileRecoveryMessage(message tea.Msg) (dashboardModel, bool) {
	m := &model.fileRecovery
	switch message := message.(type) {
	case fileRecoverySessionsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = message.err.Error()
			return model, true
		}
		m.computer, m.sessions, m.sessionCursor = message.computer, message.sessions, 0
		m.stage = recoveryStageSession
		model.message = ""
	case fileRecoveryListMsg:
		model.busy = ""
		if message.err != nil {
			model.message = message.err.Error()
			return model, true
		}
		m.listing, m.cursor = message.listing, 0
		m.stage = recoveryStageBrowse
		model.message = ""
	case fileRecoveryPlanMsg:
		model.busy = ""
		m.plan = message.plan
		m.stage = recoveryStageReview
		model.message = ""
	case fileRecoveryApplyMsg:
		model.busy = ""
		m.result = message.report
		m.stage = recoveryStageResult
		model.message = ""
	default:
		return model, false
	}
	return model, true
}

func (model dashboardModel) fileRecoveryView() string {
	m := model.fileRecovery
	shell := tuiShell{path: []string{"Computers", recoverFilesTask.title}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	dark := model.isDark
	lines := []string{tuiTitle(recoverFilesTask.title, dark)}
	visible := max(3, model.height-ctaRows(model.height)-14)
	switch m.stage {
	case recoveryStageComputer:
		lines = append(lines, tuiMuted("At every startup a student computer saves the previous session. Choose the computer the student used.", dark), "")
		hosts := model.report.Meta.Clients.Hosts
		if len(hosts) == 0 {
			lines = append(lines, "No client computers configured.")
		}
		start, end := listWindow(len(hosts), m.computerCursor, visible)
		for index := start; index < end; index++ {
			lines = append(lines, tuiSelection(fmt.Sprintf("%-10s %s", hosts[index].Name, hosts[index].IP), index == m.computerCursor, dark))
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Choose"}, {key: "↑/↓", label: "Move"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	case recoveryStageSession:
		lines = append(lines, tuiMuted("Saved sessions of "+m.computer+", the most recent first. Each is what the computer held when it was next switched on.", dark), "")
		if len(m.sessions) == 0 {
			lines = append(lines, "This computer has no saved session. The number of saved sessions is set in Change settings → Student snapshots.")
		}
		start, end := listWindow(len(m.sessions), m.sessionCursor, visible)
		for index := start; index < end; index++ {
			lines = append(lines, tuiSelection(recoverySessionLabel(m.sessions[index]), index == m.sessionCursor, dark))
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Open"}, {key: "↑/↓", label: "Move"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	case recoveryStageBrowse:
		folder := "Home"
		if m.listing.Path != "" {
			folder = "Home/" + m.listing.Path
		}
		lines = append(lines, tuiMuted(m.computer+" · "+recoverySessionLabel(m.session), dark), tuiSection(folder, dark), "")
		if len(m.listing.Entries) == 0 {
			lines = append(lines, "This folder is empty.")
		}
		start, end := listWindow(len(m.listing.Entries), m.cursor, visible)
		for index := start; index < end; index++ {
			entry := m.listing.Entries[index]
			mark := "[ ]"
			if m.chosen[m.entryPath(entry)] {
				mark = "[x]"
			}
			label := entry.Name
			if entry.Dir {
				label += "/"
			} else {
				label += "  " + humanBytes(uint64(max(0, entry.Size)))
			}
			if !entry.Modified.IsZero() {
				label += "  " + entry.Modified.Local().Format("2 Jan 15:04")
			}
			lines = append(lines, tuiSelection(mark+" "+label, index == m.cursor, dark))
		}
		if m.listing.Truncated {
			lines = append(lines, tuiMuted(fmt.Sprintf("Only the first %d entries are shown.", len(m.listing.Entries)), dark))
		}
		if count := len(m.chosen); count > 0 {
			lines = append(lines, "", fmt.Sprintf("Chosen: %d", count))
		}
		shell.actions = []tuiAction{{key: "Space", label: "Choose"}, {key: "Enter", label: "Open folder"}, {key: "Backspace", label: "Up"}, {key: "r", label: "Review"}, {key: "Esc", label: "Sessions"}, {key: "F1", label: "Help"}}
	case recoveryStageReview:
		lines = append(lines, "", m.plan.Message, "")
		if !m.plan.HasErrors() {
			lines = append(lines, tuiMuted(recoverySessionLabel(domain.FileRecoverySession{Number: m.plan.Session, SavedAt: m.plan.SavedAt}), dark))
			for _, name := range m.plan.Paths {
				lines = append(lines, "  "+name)
			}
			lines = append(lines, "", tuiMuted("The student's own files are not changed. The copies stay until the computer restarts.", dark))
		}
		shell.actions = []tuiAction{{key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		if !m.plan.HasErrors() {
			shell.actions = append([]tuiAction{{key: "Enter", label: "Copy files"}}, shell.actions...)
		}
	case recoveryStageResult:
		lines = append(lines, "", tuiResult(m.result.Message, !m.result.HasErrors(), dark))
		if m.result.Restored != nil {
			for _, name := range m.result.Restored.Placed {
				lines = append(lines, "  "+m.result.Restored.Folder+"/"+name)
			}
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Computers"}, {key: "n", label: "More from this session"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
