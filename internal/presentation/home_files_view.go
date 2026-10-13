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
	capacity := max(2, model.height-16)
	lines := []string{tuiTitle("Initial files", model.isDark)}
	actions := []tuiAction{{key: "Esc", label: "Workspace"}, {key: "F1", label: "Help"}}
	fixed := ""
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		if m.saving {
			actions = []tuiAction{{key: "F1", label: "Help"}}
		}
	} else if m.loaded.HasErrors() || m.loaded.Candidate == nil {
		lines = append(lines, "", "Initial files could not be loaded.", "If the deployment pin lacks this feature, update Nixorium:", "Maintenance → Update system and packages.")
		actions = append([]tuiAction{{key: "r", label: "Retry"}}, actions...)
	} else {
		switch m.stage {
		case "overview":
			lines = append(lines, "Student: "+safeWorkspaceText(m.loaded.StudentUser)+" · controller and all client computers", "Draft declarations; current sessions stay unchanged.", "")
			entries := m.destinations()
			if len(entries) == 0 {
				lines = append(lines, "No initial files declared.", "Import a file or a clean folder of educational materials.")
			}
			start, end := listWindow(len(entries), m.cursor, max(1, (capacity-2)/2))
			for i := start; i < end; i++ {
				label := entries[i] + "/"
				detail := "Directory · 0700"
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
				lines = append(lines, fmt.Sprintf("%d–%d of %d declarations", start+1, end, len(entries)))
			}
			fixed = "Student-owned files 0600; executable files and directories 0700.\nChanges take effect at the next boot after system application."
			actions = []tuiAction{{key: "Enter", label: "Choose source"}, {key: "a", label: "Import"}, {key: "v", label: "Review"}, {key: "Esc", label: "Workspace"}, {key: "F1", label: "Help"}}
			if len(entries) > 0 {
				actions[0].label = "Update source"
				actions = append([]tuiAction{{key: "↑/↓", label: "Select"}, {key: "u", label: "Update"}, {key: "d", label: "Remove"}, {key: "Space", label: "Executable"}}, actions...)
			}
		case "source":
			lines = append(lines, "Choose a local controller file or directory.", "Import only non-secret educational materials.", "Copies are saved in Git, the Nix store, installer and cache.", "", "Source: "+homeFilesInput(m.source, width-9))
			if m.replace != "" {
				lines = append(lines, "Replace draft declaration: "+safeWorkspaceText(m.replace))
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
			lines = append(lines, "Where should the copies appear in the student's home?", "Enter a relative file name or folder path.", "", "Destination: "+homeFilesInput(m.destination, width-14))
			if m.destination == "" {
				lines = append(lines, tuiMuted("Example: Documents/Lessons", model.isDark))
			}
			actions = []tuiAction{{key: "Enter", label: "Capture copies"}, {key: "Esc", label: "Source"}, {key: "F1", label: "Help"}}
		case "capture":
			total := int64(0)
			for _, e := range m.pending.Entries {
				total += e.Size
			}
			lines = append(lines, fmt.Sprintf("Captured %s · %s · %s", countNoun(len(m.pending.Files), "file"), homeFilesDirectoryCount(len(m.pending.Directories)), homeFilesSize(total)))
			if m.replace != "" {
				lines = append(lines, "Replaces the declared subtree: "+safeWorkspaceText(m.replace))
			}
			lines = append(lines, "Captured copies are reviewed below; executability starts off.", "")
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
					label = m.pending.Directories[i-len(m.pending.Files)] + "/ · 0700"
				}
				lines = append(lines, tuiSelection(workspaceShort(label, width-2), i == m.cursor, model.isDark))
			}
			lines = append(lines, fmt.Sprintf("%d–%d of %d captured entries", start+1, end, count))
			fixed = "Student ownership; source owner/group, permissions, ACLs and attributes\nare not inherited. Links and special files are rejected."
			actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Space", label: "Executable"}, {key: "Enter", label: "Keep copies in draft"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		case "review":
			var review bytes.Buffer
			displayed := m.plan
			displayed.ReviewToken = ""
			HomeFilesPlanText(&review, displayed)
			wrapped := strings.Split(lipgloss.NewStyle().Width(width).Render(strings.TrimSpace(review.String())), "\n")
			start := min(m.scroll, max(0, len(wrapped)-capacity))
			lines = append(lines, wrapped[start:min(len(wrapped), start+capacity)]...)
			lines = append(lines, fmt.Sprintf("Review lines %d–%d of %d", start+1, min(len(wrapped), start+capacity), len(wrapped)))
			fixed = "Save and record the manifest and reviewed copies locally.\nType SAVE: " + m.confirmation + "_"
			label := "Save copies"
			if m.plan.State == "unchanged" {
				fixed = "Declarations unchanged; student homes were not checked."
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
			fixed = "Current student sessions stay unchanged. After system application,\nthe saved files are restored at the next boot."
			actions = model.saveFollowupActions("Workspace")
			if homeFilesNeedsInspection(m.result) && model.actions.LoadGitReview != nil {
				lines = append(lines, "Inspect the local record before applying systems; never replay this save.")
				actions = append([]tuiAction{{key: "g", label: "Inspect Git state"}}, actions...)
			} else if m.result.State != "saved" && !m.result.RecoveryRequired {
				actions = append([]tuiAction{{key: "r", label: "Reload"}}, actions...)
			}
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: safeWorkspaceText(model.message)})
	}
	return model.renderShell(tuiShell{path: []string{"Settings", "Student workspace", "Initial files"}, body: strings.Join(lines, "\n"), fixedBody: fixed, notices: notices, actions: actions})
}

func HomeFilesPlanText(writer io.Writer, report domain.HomeFilesPlanReport) {
	fmt.Fprintf(writer, "Initial files review: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Student: %s · controller + %s (shared seed)\n", safeWorkspaceText(report.StudentUser), countNoun(max(0, len(report.Targets)-1), "client"))
	fmt.Fprintln(writer, "Declaration changes:")
	if len(report.Changes) == 0 {
		if report.State == "ready" {
			fmt.Fprintln(writer, "  Declaration metadata changed; file bytes and modes stay the same.")
		} else {
			fmt.Fprintln(writer, "  No declarations changed.")
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
			fmt.Fprintln(writer, "  Declaration metadata changed; file bytes and permissions stay the same.")
		case "added":
			fmt.Fprintf(writer, "  add %s · %s · %s\n", name, homeFilesSize(change.AfterSize), after)
		case "removed":
			fmt.Fprintf(writer, "  remove %s · %s · %s\n", name, homeFilesSize(change.BeforeSize), before)
		case "permissions":
			fmt.Fprintf(writer, "  permissions %s · %s → %s\n", name, before, after)
		default:
			fmt.Fprintf(writer, "  replace %s · %s → %s · %s → %s\n", name, homeFilesSize(change.BeforeSize), homeFilesSize(change.AfterSize), before, after)
		}
	}
	fmt.Fprintln(writer, "Student-owned files 0600; executable files 0700; directories 0700.")
	fmt.Fprintln(writer, "Source owner/group, permissions, ACLs, attributes and links are not inherited.")
	fmt.Fprintln(writer, "Copies enter Git, the Nix store, installer and cache. Never import secrets.")
	fmt.Fprintln(writer, "Save changes local configuration only. Apply systems separately; files restore at next boot.")
	if report.Candidate != nil {
		profile := report.Candidate.Profile
		total := int64(0)
		sizes := map[string]int64{}
		for _, e := range report.Entries {
			sizes[e.Destination] = e.Size
			total += e.Size
		}
		fmt.Fprintf(writer, "Complete draft: %s · %s · %s\n", countNoun(len(profile.Files), "file"), homeFilesDirectoryCount(len(profile.Directories)), homeFilesSize(total))
		for _, f := range profile.Files {
			fmt.Fprintf(writer, "  %s · %s · %s\n", safeWorkspaceText(f.Destination), homeFilesSize(sizes[f.Destination]), homeFilesMode(f))
		}
		for _, dir := range profile.Directories {
			fmt.Fprintf(writer, "  %s/ · 0700\n", safeWorkspaceText(dir))
		}
	}
	if len(report.Targets) > 0 {
		fmt.Fprintln(writer, "Target computers:")
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
		return "1 directory"
	}
	return fmt.Sprintf("%d directories", n)
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
		return "0700 directory"
	}
	return homeFilesMode(domain.HomeFile{Executable: executable})
}
