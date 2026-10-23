package presentation

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

func workspaceUpdateText(writer io.Writer, impact *domain.WorkspaceUpdateImpact) {
	if impact == nil {
		return
	}
	fmt.Fprintln(writer, "Student workspace: current pin -> proposed pin (not live versions)")
	fmt.Fprintln(writer, "This is a system/package update, not an isolated extension update.")
	fmt.Fprintln(writer, "Builds do not certify plugin loading or the latest vendor release.")
	versions := func(w *domain.WorkspaceResolution) map[string]string {
		result := map[string]string{}
		if w != nil {
			for _, p := range w.Packages {
				result["Package "+p.Package] = p.Version
			}
			for _, e := range w.Extensions {
				result["Extension "+e.ID] = e.Version
			}
		}
		return result
	}
	before, after := versions(impact.Current), versions(impact.Proposed)
	names := map[string]bool{}
	for name := range before {
		names[name] = true
	}
	for name := range after {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		old, currentOK := before[name]
		next, proposedOK := after[name]
		if !currentOK {
			old = "not selected"
		}
		if !proposedOK {
			next = "not selected"
		}
		fmt.Fprintf(writer, "%s: %s -> %s\n", safeWorkspaceText(name), safeWorkspaceText(old), safeWorkspaceText(next))
	}
	for _, side := range []struct {
		label string
		value *domain.WorkspaceResolution
	}{{"Current pin", impact.Current}, {"Proposed pin", impact.Proposed}} {
		if side.value == nil {
			fmt.Fprintln(writer, side.label+": no workspace metadata")
			continue
		}
		w := side.value
		fmt.Fprintf(writer, "%s: student %s\n", side.label, safeWorkspaceText(w.StudentUser))
		for _, host := range w.Targets {
			fmt.Fprintf(writer, "  %s (%s)\n", safeWorkspaceText(host.Name), safeWorkspaceText(host.Role))
		}
		for _, e := range w.Extensions {
			fmt.Fprintf(writer, "  %s requires packages [%s], extensions [%s]\n", safeWorkspaceText(e.ID), safeWorkspaceText(strings.Join(e.RequiredPackages, ", ")), safeWorkspaceText(strings.Join(e.RequiredExtensions, ", ")))
		}
		workspaceProfileText(writer, side.label+" effective preferences", w.Effective)
	}
	fmt.Fprintln(writer, "Verify the controller and one client before fleet distribution; preferences change at boot reset, not at profile save.")
}

// workspaceVersionChanges lists the applications whose pinned version the
// update changes, in the order of the full comparison.
func workspaceVersionChanges(impact *domain.WorkspaceUpdateImpact) []string {
	if impact == nil {
		return nil
	}
	var full bytes.Buffer
	workspaceUpdateText(&full, impact)
	changes := []string{}
	for _, line := range strings.Split(full.String(), "\n") {
		if !strings.HasPrefix(line, "Package ") && !strings.HasPrefix(line, "Extension ") {
			continue
		}
		name, versions, found := strings.Cut(line, ": ")
		old, next, arrow := strings.Cut(versions, " -> ")
		if found && arrow && old != next {
			changes = append(changes, name+": "+old+" -> "+next)
		}
	}
	return changes
}

// updateReviewLines is the review of an update. The summary shows what
// changes: the release, the deployment file migrations and the applications
// whose version changes. full (F4 Details, and the command-line plan) adds
// every pinned version, target and preference, and the file diff, which is
// also saved in the Git history with the update.
func updateReviewLines(report domain.UpdatePlanReport, full bool) []string {
	var content bytes.Buffer
	if full {
		workspaceUpdateText(&content, report.Workspace)
	} else if report.Workspace != nil {
		if report.CurrentRef != "" || report.Target != "" {
			fmt.Fprintln(&content, "Nixorium: "+safeWorkspaceText(updateRefChange(report)))
		}
		if changes := workspaceVersionChanges(report.Workspace); len(changes) > 0 {
			fmt.Fprintln(&content, "Applications whose version changes:")
			for _, change := range changes {
				fmt.Fprintln(&content, "  "+change)
			}
		} else {
			fmt.Fprintln(&content, "No application version changes.")
		}
	}
	migrations := deploymentMigrationLines(report.Migrations)
	if len(migrations) > 0 {
		fmt.Fprintln(&content, strings.Join(migrations, "\n"))
	}
	if !full {
		if report.Workspace != nil {
			fmt.Fprintln(&content, "F4 shows every version and the changes to the deployment files.")
		} else {
			fmt.Fprintln(&content, "F4 shows the changes to the deployment files.")
		}
		return strings.Split(strings.TrimSuffix(content.String(), "\n"), "\n")
	}
	if len(migrations) > 0 {
		fmt.Fprintln(&content, "\nProposed changes to deployment files:")
	} else if report.Workspace != nil {
		fmt.Fprintln(&content, "\nProposed flake.nix/flake.lock changes:")
	}
	content.WriteString(report.Diff.Content)
	return strings.Split(strings.TrimSuffix(content.String(), "\n"), "\n")
}

// updateRefChange names the Nixorium change. A moving target such as master
// keeps its name, so the revisions say what changes.
func updateRefChange(report domain.UpdatePlanReport) string {
	if report.CurrentRef == report.Target && report.CurrentRev != "" && report.TargetRev != "" {
		return fmt.Sprintf("%s %s -> %s", report.Target, shortRevision(report.CurrentRev), shortRevision(report.TargetRev))
	}
	return report.CurrentRef + " -> " + report.Target
}

// hangingWrap wraps a "Label:   text" line, or an indented one, so its
// continuation lines start under the text instead of at the margin.
func hangingWrap(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent == 0 {
		if colon := strings.Index(line, ":"); colon > 0 && colon < 24 {
			rest := line[colon+1:]
			indent = colon + 1 + len(rest) - len(strings.TrimLeft(rest, " "))
		}
	}
	if indent == 0 || indent > width/2 {
		return line
	}
	wrapped := strings.Split(ansi.Wordwrap(line[indent:], width-indent, ""), "\n")
	pad := strings.Repeat(" ", indent)
	for index := 1; index < len(wrapped); index++ {
		wrapped[index] = pad + wrapped[index]
	}
	return line[:indent] + strings.Join(wrapped, "\n")
}

func (model dashboardModel) updateReviewContent() []string {
	lines := updateReviewLines(model.updates.plan, model.updateDetails)
	if model.updates.plan.Workspace == nil {
		return lines
	}
	if model.updateDetails {
		details := []string{"Revision: " + model.updates.plan.Revision}
		for _, check := range model.updates.plan.Checks {
			details = append(details, safeWorkspaceText(check.ID+" · "+check.State+" · "+check.Message))
		}
		lines = append(details, lines...)
	}
	width := min(110, max(24, model.width-10))
	if !model.updateDetails {
		for index, line := range lines {
			lines[index] = hangingWrap(line, width)
		}
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n")), "\n")
}

func (model dashboardModel) workspaceUpdateReviewView() string {
	content := model.updateReviewContent()
	start := min(model.updates.scroll, max(0, len(content)-model.updateReviewHeight()))
	end := min(len(content), start+model.updateReviewHeight())
	lines := []string{tuiTitle("Review update", model.isDark),
		"Builds passed; programs are checked when they run on a computer."}
	if len(content) > model.updateReviewHeight() {
		lines = append(lines, fmt.Sprintf("Lines %d-%d of %d", start+1, end, len(content)))
	}
	lines = append(lines, content[start:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: safeWorkspaceText(model.message)})
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", model.updateTitle(), "Review"}, body: strings.Join(lines, "\n"),
		fixedBody: "Enter saves the update and applies it to this controller.\nComputers are updated afterwards, from Computers → Update computers.", notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "F4", label: "Details"}, {key: "Enter", label: "Apply update"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}})
}
