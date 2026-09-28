package presentation

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
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
		fmt.Fprintf(writer, "%s: student %s, runtime opt-in %t\n", side.label, safeWorkspaceText(w.StudentUser), w.RuntimeEnabled)
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

func updateReviewLines(report domain.UpdatePlanReport) []string {
	var content bytes.Buffer
	workspaceUpdateText(&content, report.Workspace)
	if report.Workspace != nil {
		fmt.Fprintln(&content, "\nProposed flake.nix/flake.lock changes:")
	}
	content.WriteString(report.Diff.Content)
	return strings.Split(strings.TrimSuffix(content.String(), "\n"), "\n")
}

func (model dashboardModel) updateReviewContent() []string {
	lines := updateReviewLines(model.updates.plan)
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
	return strings.Split(lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n")), "\n")
}

func (model dashboardModel) workspaceUpdateReviewView() string {
	content := model.updateReviewContent()
	start := min(model.updates.scroll, max(0, len(content)-model.updateReviewHeight()))
	end := min(len(content), start+model.updateReviewHeight())
	lines := []string{tuiTitle("Review update and student workspace", model.isDark),
		"Builds passed; runtime and plugin loading remain unverified.",
		fmt.Sprintf("Workspace and diff lines %d-%d of %d", start+1, end, len(content))}
	lines = append(lines, content[start:end]...)
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: safeWorkspaceText(model.message)})
	}
	return model.renderShell(tuiShell{path: []string{"Maintenance", model.updateTitle(), "Review"}, body: strings.Join(lines, "\n"),
		fixedBody: "Enter saves/records the pin and activates this controller.\nNo client deploy or home reset. Verify runtime on one client.", notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Scroll"}, {key: "F4", label: "Details"}, {key: "Enter", label: "Apply update"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}})
}
