package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func WorkspacePlanText(writer io.Writer, report domain.WorkspacePlanReport) {
	fmt.Fprintf(writer, "Student workspace review: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository: %s\nFile: %s\n", safeWorkspaceText(report.Repository), report.ManagedFile)
	if report.Inspection != nil {
		inspection := report.Inspection
		fmt.Fprintf(writer, "Revision: %s\nStudent: %s\n", inspection.Snapshot.Revision, safeWorkspaceText(inspection.Resolution.StudentUser))
		fmt.Fprintf(writer, "Runtime opt-in: %t (not changed by saving)\n", inspection.Resolution.RuntimeEnabled)
		fmt.Fprintln(writer, "Destinations:")
		for _, target := range inspection.Resolution.Targets {
			fmt.Fprintf(writer, "  %s (%s)\n", safeWorkspaceText(target.Name), safeWorkspaceText(target.Role))
		}
		if inspection.Base == nil {
			fmt.Fprintln(writer, "Current declaration: absent (legacy mode)")
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

func safeWorkspaceText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, value)
}
