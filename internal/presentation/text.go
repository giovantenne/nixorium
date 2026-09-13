package presentation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func JSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func StatusText(writer io.Writer, report domain.StatusReport) {
	fmt.Fprintf(writer, "Nixorium %s — %s\n", report.Meta.Version, report.Meta.Controller.Name)
	fmt.Fprintf(writer, "Repository:     %s\n", report.Repository)
	fmt.Fprintf(writer, "Configuration:  %s\n", readyText(report.Deployment.Ready))
	fmt.Fprintf(writer, "Clients:        %d configured\n", report.Meta.Clients.Count)
	fmt.Fprintf(writer, "Network:        %s/%d on %s\n", report.Meta.Network.Base, report.Meta.Network.PrefixLength, report.Meta.Network.Interface)
	fmt.Fprintf(writer, "Git worktree:   %s\n", cleanText(report.Git.Dirty, report.Git.Changes))
	fmt.Fprintf(writer, "PXE mode:       %s\n", report.PXE.Mode)
	fmt.Fprintln(writer, "Services:")
	for _, service := range report.Services {
		fmt.Fprintf(writer, "  %-28s %s\n", service.Name, service.State)
	}
	fmt.Fprintln(writer, "Installation artifacts:")
	for _, artifact := range report.Artifacts {
		state := "missing"
		if artifact.Present {
			state = "ready"
		}
		fmt.Fprintf(writer, "  %-28s %s\n", artifact.Name, state)
	}
	for _, issue := range report.Deployment.Issues {
		fmt.Fprintf(writer, "ACTION: %s\n", issue)
	}
}

func HostsText(writer io.Writer, report domain.HostsReport) {
	available, total := hostAvailability(report.Hosts)
	fmt.Fprintf(writer, "Nixorium computers: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "SSH available:      %d/%d\n", available, total)
	for _, host := range report.Hosts {
		fmt.Fprintf(writer, "  %-10s %-15s network=%-12s ssh=%s\n", host.Name, host.IP, host.Reachability, host.SSH)
	}
}

func DeploymentPlanText(writer io.Writer, report domain.DeploymentPlanReport) {
	fmt.Fprintf(writer, "Deployment plan: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:      %s\n", report.Repository)
	if report.Revision != "" {
		fmt.Fprintf(writer, "Revision:        %s\n", report.Revision)
	}
	if len(report.Targets) > 0 {
		fmt.Fprintf(writer, "Targets:         %s\n", report.ColmenaSelector)
		for _, target := range report.Targets {
			fmt.Fprintf(writer, "  %-10s %s\n", target.Name, target.IP)
		}
		fmt.Fprintln(writer, "Plan:            build selected configurations, then deploy with Colmena")
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func DoctorText(writer io.Writer, report domain.DoctorReport) {
	fmt.Fprintf(writer, "Nixorium doctor: %s\n", strings.ToUpper(report.State))
	for _, finding := range report.Findings {
		fmt.Fprintf(writer, "%-7s %-24s %s\n", finding.Level, finding.ID, finding.Summary)
		if finding.Evidence != "" {
			fmt.Fprintf(writer, "        Evidence: %s\n", finding.Evidence)
		}
		if finding.Remediation != "" {
			fmt.Fprintf(writer, "        Next: %s\n", finding.Remediation)
		}
	}
}

func ConfigValidationText(writer io.Writer, report domain.ConfigValidationReport) {
	fmt.Fprintf(writer, "Configuration: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:    %s\n", report.Repository)
	fmt.Fprintf(writer, "Managed file:  %s\n", report.File)
	if len(report.Issues) == 0 {
		fmt.Fprintln(writer, "Validation:    management schema and Nix evaluation passed")
		return
	}
	fmt.Fprintln(writer, "Issues:")
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  - %s: %s\n", issue.Field, issue.Message)
	}
}

func ConfigPlanText(writer io.Writer, report domain.ConfigPlanReport) {
	fmt.Fprintf(writer, "Configuration plan: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:         %s\n", report.Repository)
	if report.BaseFingerprint != "" {
		fmt.Fprintf(writer, "Base fingerprint:   %s\n", report.BaseFingerprint)
	}
	configChangesText(writer, report.Changes, report.Issues)
}

func ConfigApplyText(writer io.Writer, report domain.ConfigApplyReport) {
	fmt.Fprintf(writer, "Configuration apply: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:          %s\n", report.Repository)
	configChangesText(writer, report.Changes, report.Issues)
}

func configChangesText(writer io.Writer, changes []domain.SettingChange, issues []domain.ValidationIssue) {
	if len(changes) > 0 {
		fmt.Fprintln(writer, "Changes:")
		for _, change := range changes {
			fmt.Fprintf(writer, "  - %s: %v -> %v\n", change.Field, change.Before, change.After)
		}
	}
	if len(issues) > 0 {
		fmt.Fprintln(writer, "Issues:")
		for _, issue := range issues {
			fmt.Fprintf(writer, "  - %s: %s\n", issue.Field, issue.Message)
		}
	}
}

func SetupText(writer io.Writer, report domain.SetupReport) {
	fmt.Fprintf(writer, "First-run setup: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:      %s\n", report.Repository)
	if report.CurrentStage != "" {
		fmt.Fprintf(writer, "Current stage:   %s\n", report.CurrentStage)
	}
	for _, stage := range report.Stages {
		fmt.Fprintf(writer, "  %-8s %-28s", strings.ToUpper(string(stage.State)), stage.Title)
		if stage.Detail != "" {
			fmt.Fprintf(writer, " %s", stage.Detail)
		}
		fmt.Fprintln(writer)
	}
}

func KeyReconcileText(writer io.Writer, report domain.KeyReconcileReport) {
	fmt.Fprintf(writer, "Key reconciliation: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:         %s\n", report.Repository)
	for _, key := range report.Keys {
		state := "ready"
		if !key.Ready() {
			state = key.Problem
			if state == "" {
				state = "action required"
			}
		}
		fmt.Fprintf(writer, "  %-8s %s\n", key.Name, state)
	}
}

func ActionText(writer io.Writer, report domain.ActionReport) {
	fmt.Fprintf(writer, "%s: %s\n", report.Operation, strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Unit: %s\n", report.Unit)
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail: %s\n", report.Message)
	}
}

func PXELifecycleText(writer io.Writer, report domain.PXELifecycleReport) {
	fmt.Fprintf(writer, "%s: %s\n", report.Operation, strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Mode: %s\n", report.Mode)
	if report.Interface != "" {
		fmt.Fprintf(writer, "Network: %s via %s (normal address %s)\n", report.Interface, report.DHCPAddress, report.StaticCIDR)
	}
	for _, service := range report.Services {
		fmt.Fprintf(writer, "  %-30s %s\n", service.Name, service.State)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail: %s\n", report.Message)
	}
}

func readyText(ready bool) string {
	if ready {
		return "ready"
	}
	return "action required"
}

func cleanText(dirty bool, changes int) string {
	if dirty {
		return fmt.Sprintf("%d changed path(s)", changes)
	}
	return "clean"
}
