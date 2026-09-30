package presentation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func WorkspacePlanText(writer io.Writer, report domain.WorkspacePlanReport) {
	fmt.Fprintf(writer, "Student workspace review: %s\n", strings.ToUpper(report.State))
	if report.Inspection != nil {
		workspaceChangesText(writer, *report.Inspection)
		fmt.Fprintln(writer, workspaceApplicationText(report.Inspection.Resolution.RuntimeEnabled))
		fmt.Fprintln(writer, "Saving does not apply systems or reset a home.")
	}
	fmt.Fprintf(writer, "Repository: %s\nFile: %s\n", safeWorkspaceText(report.Repository), report.ManagedFile)
	if report.Inspection != nil {
		inspection := report.Inspection
		fmt.Fprintf(writer, "Revision: %s\nStudent: %s\n", inspection.Snapshot.Revision, safeWorkspaceText(inspection.Resolution.StudentUser))
		fmt.Fprintln(writer, "Destinations:")
		for _, target := range inspection.Resolution.Targets {
			fmt.Fprintf(writer, "  %s (%s)\n", safeWorkspaceText(target.Name), safeWorkspaceText(target.Role))
		}
		if inspection.Base == nil {
			fmt.Fprintln(writer, "Current declaration: no saved profile")
		} else {
			workspaceProfileText(writer, "Current declaration", *inspection.Base)
		}
		workspaceProfileText(writer, "Proposed declaration", inspection.Resolution.Declared)
		workspaceProfileText(writer, "Effective preferences after baseline", inspection.Resolution.Effective)
		for _, item := range inspection.Resolution.Packages {
			fmt.Fprintf(writer, "Required package: %s @ %s\n", safeWorkspaceText(item.Package), safeWorkspaceText(item.Version))
		}
		for _, item := range inspection.Resolution.Extensions {
			fmt.Fprintf(writer, "Extension: %s @ %s\n", safeWorkspaceText(item.ID), safeWorkspaceText(item.Version))
			fmt.Fprintf(writer, "  Required packages: %s\n  Required extensions: %s\n", safeWorkspaceText(strings.Join(item.RequiredPackages, ", ")), safeWorkspaceText(strings.Join(item.RequiredExtensions, ", ")))
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

func WorkspaceApplyText(writer io.Writer, report domain.WorkspaceApplyReport) {
	fmt.Fprintf(writer, "Student workspace save: %s\n", strings.ToUpper(report.State))
	fmt.Fprintln(writer, safeWorkspaceText(report.Message))
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", safeWorkspaceText(issue.Field), safeWorkspaceText(issue.Message))
	}
}

func ConfirmWorkspace(input io.Reader, output io.Writer, report domain.WorkspacePlanReport) (bool, error) {
	if report.HasErrors() || report.State != "ready" || report.Confirmation != "SAVE" {
		return false, nil
	}
	WorkspacePlanText(output, report)
	fmt.Fprint(output, "Type SAVE to write only workspace-profile.json: ")
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "SAVE", nil
}

func workspaceProfileText(writer io.Writer, label string, profile domain.WorkspaceProfile) {
	data, err := domain.MarshalWorkspaceProfile(profile)
	if err != nil {
		return
	}
	fmt.Fprintln(writer, label+":")
	fmt.Fprint(writer, string(data))
}

func workspaceApplicationText(enabled bool) string {
	if !enabled {
		return "These preferences are not configured to apply to student homes."
	}
	return "Preferences take effect at the next computer start after system application."
}

func workspaceChangesText(writer io.Writer, inspection domain.WorkspaceInspection) {
	base := domain.WorkspaceProfile{SchemaVersion: 1}
	if inspection.Base != nil {
		base = *inspection.Base
	}
	fmt.Fprintln(writer, "Preference changes:")
	changes := 0
	for _, field := range workspaceFields {
		before := workspaceValue(base, field)
		after := workspaceValue(inspection.Resolution.Declared, field)
		if reflect.DeepEqual(before, after) {
			continue
		}
		if field.kind == "settings" {
			// Name each changed setting rather than a count.
			for _, line := range workspaceEntryChanges(field, before, after) {
				changes++
				fmt.Fprintf(writer, "  %s / %s / %s\n", field.group, field.label, safeWorkspaceText(line))
			}
			continue
		}
		changes++
		fmt.Fprintf(writer, "  %s / %s: %s → %s\n", field.group, field.label,
			safeWorkspaceText(workspaceValueText(before)), safeWorkspaceText(workspaceValueText(after)))
	}
	if changes == 0 {
		if reflect.DeepEqual(workspaceObject(base), workspaceObject(inspection.Resolution.Declared)) {
			fmt.Fprintln(writer, "  No preference overrides changed.")
		} else {
			fmt.Fprintln(writer, "  Additional profile settings changed; see the declarations below.")
		}
	}
}

// workspaceEntryChanges compares extra settings by name.
func workspaceEntryChanges(field workspaceField, before, after any) []string {
	entries := func(value any) map[string]string {
		result := map[string]string{}
		switch items := value.(type) {
		case map[string]any:
			for name, item := range items {
				result[name] = workspaceShortValue(item)
			}
		}
		return result
	}
	old, updated := entries(before), entries(after)
	names := []string{}
	for name := range old {
		names = append(names, name)
	}
	for name := range updated {
		if _, ok := old[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	lines := []string{}
	for _, name := range names {
		previous, had := old[name]
		next, has := updated[name]
		switch {
		case !had:
			previous = "(none)"
		case !has:
			next = "removed"
		case previous == next:
			continue
		}
		lines = append(lines, name+": "+previous+" → "+next)
	}
	return lines
}

func workspaceShortValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "?"
	}
	text := string(data)
	if len(text) > 60 {
		text = strings.ToValidUTF8(text[:60], "") + "…"
	}
	return text
}

func safeWorkspaceText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, value)
}
