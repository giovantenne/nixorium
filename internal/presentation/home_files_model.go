package presentation

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// The editor holds declarations and capture IDs only. Captured bytes stay in
// the application-owned snapshot and are never read by presentation.
type homeFilesModel struct {
	stage                        string
	loaded                       domain.HomeFilesPlanReport
	draft                        domain.HomeFilesDraft
	plan                         domain.HomeFilesPlanReport
	result                       domain.HomeFilesApplyReport
	pending                      domain.HomeFilesCaptureReport
	cursor, scroll               int
	source, destination, replace string
	completions                  []string
	sizes                        map[string]int64
	requestID                    uint64
	saving                       bool
}

func (m homeFilesModel) textEntry() bool {
	return m.stage == "source" || m.stage == "destination" || m.stage == "review"
}

func cloneHomeFilesDraft(d domain.HomeFilesDraft) domain.HomeFilesDraft {
	d.Profile.Files = slices.Clone(d.Profile.Files)
	d.Profile.Directories = slices.Clone(d.Profile.Directories)
	d.CaptureIDs = slices.Clone(d.CaptureIDs)
	return d
}

func (m homeFilesModel) destinations() []string {
	entries := []string{}
	for _, file := range m.draft.Profile.Files {
		entries = append(entries, file.Destination)
	}
	entries = append(entries, m.draft.Profile.Directories...)
	slices.Sort(entries)
	return entries
}

func homeFilesUnder(path, root string) bool { return path == root || strings.HasPrefix(path, root+"/") }

func homeFilesRemove(d domain.HomeFilesDraft, destination string) domain.HomeFilesDraft {
	d = cloneHomeFilesDraft(d)
	d.Profile.Files = slices.DeleteFunc(d.Profile.Files, func(f domain.HomeFile) bool { return homeFilesUnder(f.Destination, destination) })
	d.Profile.Directories = slices.DeleteFunc(d.Profile.Directories, func(path string) bool { return homeFilesUnder(path, destination) })
	return d
}

func (m *homeFilesModel) acceptCapture() string {
	draft := cloneHomeFilesDraft(m.draft)
	if m.replace != "" {
		draft = homeFilesRemove(draft, m.replace)
	}
	existing := append([]string{}, draft.Profile.Directories...)
	for _, f := range draft.Profile.Files {
		existing = append(existing, f.Destination)
	}
	incoming := append([]string{}, m.pending.Directories...)
	for _, f := range m.pending.Files {
		incoming = append(incoming, f.Destination)
	}
	for _, path := range incoming {
		for _, old := range existing {
			if path == old {
				return "A declaration already uses " + safeWorkspaceText(path) + ". Cancel and select it, then use u to choose its source again."
			}
		}
	}
	draft.Profile.Files = append(draft.Profile.Files, m.pending.Files...)
	draft.Profile.Directories = append(draft.Profile.Directories, m.pending.Directories...)
	if !slices.Contains(draft.CaptureIDs, m.pending.CaptureID) {
		draft.CaptureIDs = append(draft.CaptureIDs, m.pending.CaptureID)
	}
	// The domain decoder is the authority for protected paths and tree conflicts.
	if _, err := domain.MarshalHomeFilesProfile(draft.Profile); err != nil {
		return safeWorkspaceText(err.Error()) + " Cancel and choose a different destination."
	}
	m.draft = draft
	if m.sizes == nil {
		m.sizes = map[string]int64{}
	}
	for _, entry := range m.pending.Entries {
		m.sizes[entry.Destination] = entry.Size
	}
	m.plan = domain.HomeFilesPlanReport{}
	m.pending = domain.HomeFilesCaptureReport{}
	m.source, m.destination, m.replace = "", "", ""
	m.cursor = 0
	m.stage = "overview"
	return "Captured copies added to the draft; nothing has been saved."
}

func homeFilesEditText(value string, key tea.KeyPressMsg) string {
	if key.String() == "backspace" {
		_, size := utf8.DecodeLastRuneInString(value)
		if size > 0 {
			return value[:len(value)-size]
		}
		return value
	}
	if key.Text != "" && len(value)+len(key.Text) <= 4096 {
		return value + key.Text
	}
	return value
}

func homeFilesIssueText(message string, issues []domain.ValidationIssue) string {
	parts := []string{message}
	for _, issue := range issues {
		parts = append(parts, issue.Message)
	}
	value := strings.TrimSpace(safeWorkspaceText(strings.Join(parts, " ")))
	if value == "" {
		value = "Initial files could not be checked; no configuration changed."
	}
	return value + " Return to Initial files and retry, or use Maintenance → Diagnostics."
}

type homeFilesReadMsg struct {
	id          uint64
	kind        string
	plan        domain.HomeFilesPlanReport
	capture     domain.HomeFilesCaptureReport
	completions []string
	err         error
}
type homeFilesSaveMsg struct {
	id     uint64
	report domain.HomeFilesApplyReport
}

func (model dashboardModel) openHomeFiles() (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode {
		model.message = "Ask the administrator to manage initial student-home files."
		return model, nil
	}
	if model.actions.LoadHomeFiles == nil || model.actions.PlanHomeFiles == nil || model.actions.CaptureHomeFiles == nil || model.actions.SaveHomeFiles == nil {
		model.message = "Initial files are unavailable. Ask the administrator to use Maintenance → Update Nixorium."
		return model, nil
	}
	model.screen = dashboardHomeFiles
	model.homeFiles = homeFilesModel{stage: "overview", requestID: model.homeFiles.requestID + 1, sizes: map[string]int64{}}
	return model.startHomeFilesRead("load")
}

func (model dashboardModel) startHomeFilesRead(kind string) (tea.Model, tea.Cmd) {
	model.homeFiles.requestID++
	id := model.homeFiles.requestID
	draft := cloneHomeFilesDraft(model.homeFiles.draft)
	source, destination := model.homeFiles.source, model.homeFiles.destination
	model.message = ""
	model.busy = map[string]string{"load": "Reading saved initial-file declarations", "plan": "Checking captured copies and the complete student-home draft", "capture": "Capturing local materials for review", "complete": "Completing the local source path"}[kind]
	return model.startRead(func(ctx context.Context) tea.Msg {
		msg := homeFilesReadMsg{id: id, kind: kind}
		switch kind {
		case "load":
			msg.plan = model.actions.LoadHomeFiles(ctx)
		case "plan":
			msg.plan = model.actions.PlanHomeFiles(ctx, draft)
		case "capture":
			msg.capture = model.actions.CaptureHomeFiles(ctx, source, destination)
		case "complete":
			msg.completions, msg.err = model.actions.CompleteHomeFilesPath(ctx, source)
		}
		return msg
	})
}

func (model dashboardModel) finishHomeFilesRead(msg homeFilesReadMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardHomeFiles || msg.id != model.homeFiles.requestID {
		return model, nil
	}
	model.busy = ""
	switch msg.kind {
	case "load":
		model.homeFiles.loaded = msg.plan
		if msg.plan.HasErrors() || msg.plan.Candidate == nil {
			model.message = homeFilesIssueText(msg.plan.Message, msg.plan.Issues)
			return model, nil
		}
		model.homeFiles.draft = cloneHomeFilesDraft(*msg.plan.Candidate)
		if msg.plan.Snapshot != nil {
			model.homeFiles.draft.BaseFingerprint = msg.plan.Snapshot.BaseFingerprint
		}
		for _, e := range msg.plan.Entries {
			model.homeFiles.sizes[e.Destination] = e.Size
		}
	case "plan":
		if msg.plan.HasErrors() || msg.plan.Candidate == nil {
			model.message = homeFilesIssueText(msg.plan.Message, msg.plan.Issues)
			return model, nil
		}
		if model.homeFiles.loaded.Snapshot != nil && (msg.plan.Snapshot == nil || msg.plan.Snapshot.BaseFingerprint != model.homeFiles.loaded.Snapshot.BaseFingerprint) {
			model.message = "The saved initial files changed while editing. Leave and reopen Initial files before creating a new review."
			return model, nil
		}
		model.homeFiles.plan = msg.plan
		model.homeFiles.scroll = 0
		model.homeFiles.stage = "review"
	case "capture":
		if msg.capture.HasErrors() || msg.capture.CaptureID == "" {
			model.message = homeFilesIssueText(msg.capture.Message, msg.capture.Issues) + " Choose a clean materials folder; select a source without keys, credentials, links or special files."
			model.homeFiles.stage = "source"
			return model, nil
		}
		model.homeFiles.pending = msg.capture
		model.homeFiles.pending.Files = slices.Clone(msg.capture.Files)
		for i := range model.homeFiles.pending.Files {
			model.homeFiles.pending.Files[i].Executable = false
		}
		model.homeFiles.cursor = 0
		model.homeFiles.stage = "capture"
	case "complete":
		if msg.err != nil {
			model.message = safeWorkspaceText(msg.err.Error()) + " Type the full local path, or choose another folder."
			return model, nil
		}
		if len(msg.completions) == 0 {
			model.message = "No matching local path. Type the full file or directory path."
		}
		model.homeFiles.completions = slices.Clone(msg.completions)
		if len(msg.completions) == 1 {
			model.homeFiles.source = msg.completions[0]
		}
	case "discard":
		if msg.err != nil {
			model.message = "The temporary captured copy could not be discarded. No declaration changed; reopen Initial files and retry the source selection. " + safeWorkspaceText(msg.err.Error())
		}
	}
	return model, nil
}

func (model dashboardModel) finishHomeFilesSave(msg homeFilesSaveMsg) (tea.Model, tea.Cmd) {
	if model.screen != dashboardHomeFiles || msg.id != model.homeFiles.requestID || !model.homeFiles.saving {
		return model, nil
	}
	model.busy, model.message = "", ""
	model.homeFiles.saving = false
	model.homeFiles.result = msg.report
	model.homeFiles.stage = "result"
	if !msg.report.HasErrors() && msg.report.Recorded {
		model.noteControllerSave(msg.report.Revision)
	}
	return model, nil
}

func (model dashboardModel) updateHomeFilesKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.homeFiles
	if m.saving {
		model.message = "The local save cannot be interrupted. Wait for its result; F1 opens help."
		return model, nil
	}
	if m.loaded.HasErrors() || m.loaded.Candidate == nil {
		switch key.String() {
		case "r":
			return model.startHomeFilesRead("load")
		case "esc":
			model.screen = dashboardWorkspace
			model.message = ""
		}
		return model, nil
	}
	switch m.stage {
	case "overview":
		entries := m.destinations()
		switch key.String() {
		case "esc":
			model.screen = dashboardWorkspace
			model.message = "Initial files closed; unsaved changes discarded."
			return model, model.discardHomeFilesCaptures(slices.Clone(m.draft.CaptureIDs))
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(max(0, len(entries)-1), m.cursor+1)
		case "a":
			m.stage = "source"
			m.source, m.destination, m.replace = "", "", ""
			m.completions = nil
			model.message = ""
		case "u", "enter":
			m.stage = "source"
			m.source, m.destination, m.replace = "", "", ""
			m.completions = nil
			if len(entries) > 0 {
				m.destination = entries[m.cursor]
				m.replace = m.destination
			}
			model.message = ""
		case "d":
			if len(entries) > 0 {
				m.draft = homeFilesRemove(m.draft, entries[m.cursor])
				m.cursor = min(m.cursor, max(0, len(m.destinations())-1))
				m.plan = domain.HomeFilesPlanReport{}
				model.message = "Declaration removed from the draft; review before saving."
			}
		case "space":
			if len(entries) > 0 {
				m.draft = cloneHomeFilesDraft(m.draft)
				for i := range m.draft.Profile.Files {
					if m.draft.Profile.Files[i].Destination == entries[m.cursor] {
						m.draft.Profile.Files[i].Executable = !m.draft.Profile.Files[i].Executable
						m.plan = domain.HomeFilesPlanReport{}
					}
				}
			}
		case "v":
			return model.startHomeFilesRead("plan")
		}
	case "source":
		switch key.String() {
		case "esc":
			m.stage = "overview"
			m.source, m.destination, m.replace = "", "", ""
			model.message = "Source selection cancelled; the draft is unchanged."
		case "tab":
			if model.actions.CompleteHomeFilesPath != nil {
				return model.startHomeFilesRead("complete")
			}
		case "enter":
			if strings.TrimSpace(m.source) == "" {
				model.message = "Enter a local controller file or directory."
				break
			}
			m.completions = nil
			if m.replace != "" {
				return model.startHomeFilesRead("capture")
			}
			m.stage = "destination"
			model.message = ""
		default:
			m.source = homeFilesEditText(m.source, key)
			m.completions = nil
		}
	case "destination":
		switch key.String() {
		case "esc":
			m.stage = "source"
			model.message = ""
		case "enter":
			if m.destination == "" {
				model.message = "Enter a destination relative to the student home, such as Documents/Lessons."
			} else {
				return model.startHomeFilesRead("capture")
			}
		default:
			m.destination = homeFilesEditText(m.destination, key)
		}
	case "capture":
		count := len(m.pending.Files) + len(m.pending.Directories)
		switch key.String() {
		case "esc":
			captureID := m.pending.CaptureID
			m.pending = domain.HomeFilesCaptureReport{}
			m.stage = "source"
			model.message = "Captured proposal cancelled; the draft is unchanged."
			return model, model.discardHomeFilesCaptures([]string{captureID})
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(max(0, count-1), m.cursor+1)
		case "space":
			if m.cursor < len(m.pending.Files) {
				m.pending.Files[m.cursor].Executable = !m.pending.Files[m.cursor].Executable
			}
		case "enter":
			model.message = m.acceptCapture()
		}
	case "review":
		switch key.String() {
		case "esc":
			m.stage = "overview"
			model.message = "Review cancelled; no configuration changed."
		case "up":
			m.scroll = max(0, m.scroll-1)
		case "down":
			m.scroll++
		case "pgup":
			m.scroll = max(0, m.scroll-max(1, model.height-ctaRows(model.height)-17))
		case "pgdown":
			m.scroll += max(1, model.height-ctaRows(model.height)-17)
		case "enter":
			if m.plan.State == "unchanged" {
				m.stage = "overview"
				model.message = "Declarations unchanged; student homes were not checked."
				break
			}
			if m.plan.State != "ready" || m.plan.HasErrors() || m.plan.ReviewToken == "" {
				model.message = "This review cannot be saved. Go back and create a new one."
				break
			}
			m.saving = true
			m.requestID++
			id, plan := m.requestID, m.plan
			model.busy = "Rechecking, saving and recording the reviewed initial files"
			model.message = ""
			return model, func() tea.Msg { return homeFilesSaveMsg{id: id, report: model.actions.SaveHomeFiles(plan)} }
		}
	case "result":
		switch key.String() {
		case "enter":
			return model.continueSavedConfiguration()
		case "esc":
			model.screen = dashboardWorkspace
			model.message = ""
		case "r":
			if !m.result.RecoveryRequired && m.result.State != "saved" {
				return model.openHomeFiles()
			}
		case "g":
			if homeFilesNeedsInspection(m.result) && model.actions.LoadGitReview != nil {
				return model.openMaintenanceTask("g")
			}
		}
	}
	return model, nil
}

func homeFilesNeedsInspection(r domain.HomeFilesApplyReport) bool {
	return r.RecoveryRequired || r.State == "conflict" || (r.State == "saved" && !r.Recorded)
}

func (model dashboardModel) discardHomeFilesCaptures(ids []string) tea.Cmd {
	if model.actions.DiscardHomeFilesCapture == nil || len(ids) == 0 {
		return nil
	}
	id := model.homeFiles.requestID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer cancel()
		for _, capture := range ids {
			if err := model.actions.DiscardHomeFilesCapture(ctx, capture); err != nil {
				return homeFilesReadMsg{id: id, kind: "discard", err: err}
			}
		}
		return homeFilesReadMsg{id: id, kind: "discard"}
	}
}
func homeFilesMode(f domain.HomeFile) string {
	if f.Executable {
		return "0700 · executable"
	}
	return "0600 · not executable"
}
func homeFilesSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
}
