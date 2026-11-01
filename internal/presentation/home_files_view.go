package presentation

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) homeFilesView() string {
	m := model.homeFiles
	width := min(110, max(24, model.width-10))
	capacity := max(2, model.height-ctaRows(model.height)-16)
	lines := []string{tuiTitle("Initial files", model.isDark)}
	actions := []tuiAction{{key: "Esc", label: "Workspace"}, {key: "F1", label: "Help"}}
	fixed := ""
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		if m.saving {
			actions = []tuiAction{{key: "F1", label: "Help"}}
		}
	} else if m.loaded.HasErrors() || m.loaded.Candidate == nil {
		lines = append(lines, "", "Initial files could not be loaded.", "If this lab runs an older Nixorium, update it:", "Maintenance → Update Nixorium.")
		actions = append([]tuiAction{{key: "r", label: "Retry"}}, actions...)
	} else {
		switch m.stage {
		case "overview":
			lines = append(lines, "Files and folders that the student "+safeWorkspaceText(m.loaded.StudentUser)+" finds in their home,", "on the controller and on every client computer.", "")
			entries := m.destinations()
			if len(entries) == 0 {
				lines = append(lines, "No files yet. Press a to add a file or a folder from this computer.")
			}
			start, end := listWindow(len(entries), m.cursor, max(1, (capacity-2)/2))
			for i := start; i < end; i++ {
				label := entries[i] + "/"
				detail := "Folder"
				for _, f := range m.draft.Profile.Files {
					if f.Destination == entries[i] {
						label = f.Destination
						size := m.sizes[f.Destination]
						if f.Text != nil {
							size = int64(len(*f.Text))
						}
						detail = homeFilesSize(size) + " · " + homeFilesMode(f)
						break
					}
				}
				lines = append(lines, tuiSelection(workspaceShort(label, width-2), i == m.cursor, model.isDark), tuiMuted("  "+detail, model.isDark))
			}
			if len(entries) > 0 {
				lines = append(lines, fmt.Sprintf("%d–%d of %d", start+1, end, len(entries)))
			}
			fixed = "Changes stay in a draft until you review and save them with v.\nAfter you update the computers, students find them at the next start."
			actions = []tuiAction{{key: "Enter", label: "Add"}, {key: "a", label: "Add file or folder"}, {key: "v", label: "Review"}, {key: "Esc", label: "Workspace"}, {key: "F1", label: "Help"}}
			if len(entries) > 0 {
				actions[0].label = "Replace"
				actions = append([]tuiAction{{key: "↑/↓", label: "Select"}, {key: "u", label: "Replace"}, {key: "d", label: "Remove"}, {key: "Space", label: "Executable"}}, actions...)
			}
		case "source":
			lines = append(lines, "Choose a file or a folder on this computer.", "Nixorium keeps a copy in the lab configuration and on every computer:", "add teaching materials only, never passwords or keys.", "", "Path: "+homeFilesInput(m.source, width-7))
			if m.replace != "" {
				lines = append(lines, "Replaces: "+safeWorkspaceText(m.replace))
			}
			if m.source == "" {
				lines = append(lines, tuiMuted("Example: /home/admin/Teaching/Lessons", model.isDark))
			}
			for _, path := range m.completions[:min(4, len(m.completions))] {
				lines = append(lines, tuiMuted(safeWorkspaceText(path), model.isDark))
			}
			if len(m.completions) > 4 {
				lines = append(lines, "More matches; type a longer path.")
			}
			actions = []tuiAction{{key: "Enter", label: "Continue"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
			if model.actions.CompleteHomeFilesPath != nil {
				actions = append([]tuiAction{{key: "Tab", label: "Complete path"}}, actions...)
			}
		case "destination":
			lines = append(lines, "Where should it appear in the student's home?", "Write a path inside the home.", "", "In the home: "+homeFilesInput(m.destination, width-14))
			if m.destination == "" {
				lines = append(lines, tuiMuted("Example: Documents/Lessons", model.isDark))
			}
			actions = []tuiAction{{key: "Enter", label: "Copy"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		case "capture":
			total := int64(0)
			for _, e := range m.pending.Entries {
				total += e.Size
			}
			lines = append(lines, fmt.Sprintf("Copied %s, %s, %s", countNoun(len(m.pending.Files), "file"), homeFilesDirectoryCount(len(m.pending.Directories)), homeFilesSize(total)))
			if m.replace != "" {
				lines = append(lines, "Replaces: "+safeWorkspaceText(m.replace))
			}
			lines = append(lines, "Check the list. Files start as not executable: press Space on programs.", "")
			count := len(m.pending.Files) + len(m.pending.Directories)
			start, end := listWindow(count, m.cursor, max(1, capacity-4))
			for i := start; i < end; i++ {
				label := ""
				if i < len(m.pending.Files) {
					f := m.pending.Files[i]
					size := int64(0)
					for _, e := range m.pending.Entries {
						if e.Destination == f.Destination {
							size = e.Size
						}
					}
					label = f.Destination + " · " + homeFilesSize(size) + " · " + homeFilesMode(f)
				} else {
					label = m.pending.Directories[i-len(m.pending.Files)] + "/"
				}
				lines = append(lines, tuiSelection(workspaceShort(label, width-2), i == m.cursor, model.isDark))
			}
			lines = append(lines, fmt.Sprintf("%d–%d of %d", start+1, end, count))
			fixed = "The copies belong to the student. Original owners and permissions\nare not kept; links and special files are refused."
			actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Space", label: "Executable"}, {key: "Enter", label: "Add to draft"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		case "review":
			var review bytes.Buffer
			displayed := m.plan
			displayed.ReviewToken = ""
			HomeFilesPlanText(&review, displayed)
			wrapped := strings.Split(lipgloss.NewStyle().Width(width).Render(strings.TrimSpace(review.String())), "\n")
			start := min(m.scroll, max(0, len(wrapped)-capacity))
			lines = append(lines, wrapped[start:min(len(wrapped), start+capacity)]...)
			lines = append(lines, fmt.Sprintf("Review lines %d–%d of %d", start+1, min(len(wrapped), start+capacity), len(wrapped)))
			fixed = "Enter saves the files in the lab configuration. Computers change\nonly when you update them."
			label := "Save"
			if m.plan.State == "unchanged" {
				fixed = "Nothing changed."
				label = "Back"
			}
			actions = []tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "Enter", label: label}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		case "result":
			level := tuiStatusNeutral
			if m.result.State == "saved" && m.result.Recorded && !m.result.RecoveryRequired {
				level = tuiStatusSuccess
			}
			if m.result.HasErrors() {
				level = tuiStatusAttention
			}
			lines = append(lines, tuiStatus(strings.ToUpper(m.result.State), level, model.isDark), "", safeWorkspaceText(m.result.Message))
			lines = append(lines, saveStatusLines(m.result.State, m.result.RecoveryRequired, true, true, controllerVerifiedForSave(m.result.Revision, model.controller.result))...)
			for _, issue := range m.result.Issues {
				lines = append(lines, safeWorkspaceText(issue.Message))
			}
			fixed = "Students who are signed in keep their current files. After you update\nthe computers, each one gets the new files at its next start."
			actions = model.saveFollowupActions("Workspace")
			if homeFilesNeedsInspection(m.result) && model.actions.LoadGitReview != nil {
				lines = append(lines, "Check the saved configuration before updating the computers; do not save again.")
				actions = append([]tuiAction{{key: "g", label: "Inspect Git state"}}, actions...)
			} else if m.result.State != "saved" && !m.result.RecoveryRequired {
				actions = append([]tuiAction{{key: "r", label: "Reload"}}, actions...)
			}
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notice := model.messageNotice()
		notice.title = safeWorkspaceText(model.message)
		notices = append(notices, notice)
	}
	return model.renderShell(tuiShell{path: []string{"Settings", "Student workspace", "Initial files"}, body: strings.Join(lines, "\n"), fixedBody: fixed, notices: notices, actions: actions})
}

func HomeFilesPlanText(writer io.Writer, report domain.HomeFilesPlanReport) {
	fmt.Fprintf(writer, "Initial files review: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Student: %s · the controller and %s\n", safeWorkspaceText(report.StudentUser), countNoun(max(0, len(report.Targets)-1), "client"))
	fmt.Fprintln(writer, "Changes:")
	if len(report.Changes) == 0 {
		if report.State == "ready" {
			fmt.Fprintln(writer, "  Only the list was rewritten; the files stay the same.")
		} else {
			fmt.Fprintln(writer, "  Nothing changed.")
		}
	}
	for _, change := range report.Changes {
		name := safeWorkspaceText(change.Destination)
		before, after := homeFilesChangeMode(change.BeforeDirectory, change.BeforeExecutable), homeFilesChangeMode(change.AfterDirectory, change.AfterExecutable)
		if change.AfterDirectory || (change.Kind == "removed" && change.BeforeDirectory) {
			name += "/"
		}
		switch change.Kind {
		case "declaration":
			fmt.Fprintln(writer, "  Only the list was rewritten; the files stay the same.")
		case "added":
			if change.AfterDirectory {
				fmt.Fprintf(writer, "  add %s\n", name)
			} else {
				fmt.Fprintf(writer, "  add %s · %s · %s\n", name, homeFilesSize(change.AfterSize), after)
			}
		case "removed":
			if change.BeforeDirectory {
				fmt.Fprintf(writer, "  remove %s\n", name)
			} else {
				fmt.Fprintf(writer, "  remove %s · %s · %s\n", name, homeFilesSize(change.BeforeSize), before)
			}
		case "permissions":
			fmt.Fprintf(writer, "  change %s · %s → %s\n", name, before, after)
		default:
			mode := ""
			if before != after {
				mode = " · " + before + " → " + after
			}
			fmt.Fprintf(writer, "  replace %s · %s → %s%s\n", name, homeFilesSize(change.BeforeSize), homeFilesSize(change.AfterSize), mode)
		}
	}
	fmt.Fprintln(writer, "The copies belong to the student; original owners, permissions and links are not kept.")
	fmt.Fprintln(writer, "Copies are kept in the lab configuration and on every computer: never add passwords or keys.")
	fmt.Fprintln(writer, "Saving changes only the configuration. After you update the computers, students get the files at the next start.")
	if report.Candidate != nil {
		profile := report.Candidate.Profile
		total := int64(0)
		sizes := map[string]int64{}
		for _, e := range report.Entries {
			sizes[e.Destination] = e.Size
			total += e.Size
		}
		fmt.Fprintf(writer, "All initial files: %s, %s, %s\n", countNoun(len(profile.Files), "file"), homeFilesDirectoryCount(len(profile.Directories)), homeFilesSize(total))
		for _, f := range profile.Files {
			fmt.Fprintf(writer, "  %s · %s · %s\n", safeWorkspaceText(f.Destination), homeFilesSize(sizes[f.Destination]), homeFilesMode(f))
		}
		for _, dir := range profile.Directories {
			fmt.Fprintf(writer, "  %s/\n", safeWorkspaceText(dir))
		}
	}
	if len(report.Targets) > 0 {
		fmt.Fprintln(writer, "Computers:")
		for _, target := range report.Targets {
			fmt.Fprintf(writer, "  %s (%s)\n", safeWorkspaceText(target.Name), safeWorkspaceText(target.Role))
		}
	}
	if report.ReviewToken != "" {
		fmt.Fprintf(writer, "Review token: %s\n", report.ReviewToken)
	}
	fmt.Fprintln(writer, safeWorkspaceText(report.Message))
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", safeWorkspaceText(issue.Field), safeWorkspaceText(issue.Message))
	}
}

func HomeFilesApplyText(writer io.Writer, report domain.HomeFilesApplyReport) {
	fmt.Fprintf(writer, "Initial files save: %s\n", strings.ToUpper(report.State))
	fmt.Fprintln(writer, safeWorkspaceText(report.Message))
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", safeWorkspaceText(issue.Field), safeWorkspaceText(issue.Message))
	}
}

func homeFilesDirectoryCount(n int) string {
	if n == 1 {
		return "1 folder"
	}
	return fmt.Sprintf("%d folders", n)
}

func homeFilesInput(value string, width int) string {
	value = safeWorkspaceText(value)
	if lipgloss.Width(value)+1 <= width {
		return value + "_"
	}
	chars := []rune(value)
	for len(chars) > 0 && lipgloss.Width(string(chars))+2 > width {
		chars = chars[1:]
	}
	return "…" + string(chars) + "_"
}

func homeFilesChangeMode(directory, executable bool) string {
	if directory {
		return "folder"
	}
	return homeFilesMode(domain.HomeFile{Executable: executable})
}
