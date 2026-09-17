package presentation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

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

func ServicesText(writer io.Writer, report domain.ServicesReport) {
	fmt.Fprintf(writer, "Nixorium services: %s\n", strings.ToUpper(report.State))
	for _, service := range report.Services {
		fmt.Fprintf(writer, "  %s (%s): %s\n", service.Name, service.Mode, service.State)
		if service.Detail != "" {
			fmt.Fprintf(writer, "    %s\n", service.Detail)
		}
		for _, unit := range service.Units {
			fmt.Fprintf(writer, "    %-32s %s\n", unit.Name, unit.State)
		}
		for _, command := range service.Commands {
			fmt.Fprintf(writer, "    workflow: %s\n", command)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func ServiceActionText(writer io.Writer, report domain.ServiceActionReport) {
	fmt.Fprintf(writer, "Service action: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Action:         %s %s\n", report.Action, report.Service)
	if report.Unit != "" {
		fmt.Fprintf(writer, "Unit:           %s\n", report.Unit)
	}
	fmt.Fprintf(writer, "Verified:       %t\n", report.Verified)
	if report.Current.State != "" {
		fmt.Fprintf(writer, "Current state:  %s\n", report.Current.State)
	}
	if report.Message != "" {
		fmt.Fprintln(writer, report.Message)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func OperationLogsText(writer io.Writer, report domain.OperationLogsReport) {
	fmt.Fprintf(writer, "Nixorium operation logs: %s\n", strings.ToUpper(report.State))
	if len(report.Records) == 0 {
		fmt.Fprintln(writer, "No recorded operation outcomes are available.")
	} else {
		fmt.Fprintln(writer, "Recent actions:")
		for _, record := range report.Records {
			fmt.Fprintf(writer, "  %s  %-20s %-11s %s\n", record.RecordedAt.UTC().Format("2006-01-02 15:04:05Z"), record.Operation, record.State, record.Subject)
			fmt.Fprintf(writer, "    %s\n", record.Summary)
		}
	}
	fmt.Fprintln(writer, "Deployment logs:")
	if len(report.Logs) == 0 {
		fmt.Fprintln(writer, "No deployment operation logs are available.")
	}
	for _, entry := range report.Logs {
		fmt.Fprintf(writer, "  %s  %-10s %-11s %d bytes\n", entry.StartedAt.UTC().Format("2006-01-02 15:04:05Z"), entry.Kind, entry.State, entry.SizeBytes)
		fmt.Fprintf(writer, "    %s\n", entry.ID)
		if entry.Detail != "" {
			fmt.Fprintf(writer, "    unavailable: %s\n", entry.Detail)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "WARNING: %s: %s\n", issue.Field, issue.Message)
	}
}

func OperationLogText(writer io.Writer, report domain.OperationLogReport) {
	if report.Log == nil {
		fmt.Fprintf(writer, "Operation log: %s\n", strings.ToUpper(report.State))
	} else {
		fmt.Fprintf(writer, "Operation log: %s\n", report.Log.ID)
		fmt.Fprintf(writer, "Started:       %s\n", report.Log.StartedAt.UTC().Format(time.RFC3339Nano))
		fmt.Fprintf(writer, "Kind:          %s\n", report.Log.Kind)
		fmt.Fprintf(writer, "Result:        %s\n", report.Log.State)
		fmt.Fprintf(writer, "Size:          %d bytes\n", report.Log.SizeBytes)
		if report.Truncated {
			fmt.Fprintln(writer, "Content:       tail only (earlier output was truncated)")
		} else {
			fmt.Fprintln(writer, "Content:")
		}
		fmt.Fprint(writer, report.Content)
		if report.Content != "" && !strings.HasSuffix(report.Content, "\n") {
			fmt.Fprintln(writer)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func GitReviewText(writer io.Writer, report domain.GitReviewReport) {
	fmt.Fprintf(writer, "Nixorium Git review: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:      %s\n", report.Repository)
	if report.Revision != "" {
		fmt.Fprintf(writer, "HEAD revision:   %s\n", report.Revision)
	}
	fmt.Fprintf(writer, "Changed paths:   %d\n", len(report.Changes))
	fmt.Fprintf(writer, "Scopes:          %d staged, %d unstaged, %d untracked\n", report.Summary.Staged, report.Summary.Unstaged, report.Summary.Untracked)
	fmt.Fprintf(writer, "Ownership:       %d managed, %d unexpected, %d private\n", report.Summary.Managed, report.Summary.Unexpected, report.Summary.Private)
	if len(report.Changes) == 0 {
		fmt.Fprintln(writer, "The deployment worktree is clean.")
	} else {
		fmt.Fprintln(writer, "Changes:")
		for _, change := range report.Changes {
			fmt.Fprintf(writer, "  %-10s %-10s %-9s %s\n", gitChangeOwnership(change), gitChangeIndex(change), gitChangeWorktree(change), change.Path)
			if change.OriginalPath != "" {
				fmt.Fprintf(writer, "    from %s\n", change.OriginalPath)
			}
		}
	}
	for _, diff := range report.Diffs {
		fmt.Fprintf(writer, "\n%s diff%s:\n", gitScopeTitle(diff.Scope), truncatedGitDiffLabel(diff.Truncated))
		fmt.Fprint(writer, diff.Content)
		if diff.Content != "" && !strings.HasSuffix(diff.Content, "\n") {
			fmt.Fprintln(writer)
		}
	}
	if report.Summary.Untracked > 0 {
		fmt.Fprintln(writer, "\nUntracked file contents are not opened automatically.")
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func GitCommitPlanText(writer io.Writer, report domain.GitCommitPlanReport) {
	fmt.Fprintf(writer, "Nixorium Git commit plan: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:      %s\n", report.Repository)
	if report.Revision != "" {
		fmt.Fprintf(writer, "HEAD revision:   %s\n", report.Revision)
	}
	if len(report.Paths) > 0 {
		fmt.Fprintf(writer, "Selected paths:  %s\n", strings.Join(report.Paths, ", "))
	}
	if report.CommitMessage != "" {
		fmt.Fprintf(writer, "Commit message:  %s\n", report.CommitMessage)
		fmt.Fprintf(writer, "Review token:    %s\n", report.ReviewToken)
		fmt.Fprintf(writer, "Confirmation:    %s\n", report.Confirmation)
		fmt.Fprintln(writer, "No remote or push is part of this plan.")
	}
	if report.Diff.Content != "" {
		fmt.Fprintln(writer, "\nProposed commit diff:")
		fmt.Fprint(writer, report.Diff.Content)
		if !strings.HasSuffix(report.Diff.Content, "\n") {
			fmt.Fprintln(writer)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func GitCommitText(writer io.Writer, report domain.GitCommitReport) {
	fmt.Fprintf(writer, "Nixorium Git commit: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Paths:          %s\n", strings.Join(report.Paths, ", "))
	if report.PreviousRevision != "" {
		fmt.Fprintf(writer, "Previous HEAD:  %s\n", report.PreviousRevision)
	}
	if report.Revision != "" {
		fmt.Fprintf(writer, "Current HEAD:   %s\n", report.Revision)
	}
	if report.CommitMessage != "" {
		fmt.Fprintf(writer, "Commit message: %s\n", report.CommitMessage)
	}
	fmt.Fprintf(writer, "Committed:      %t\n", report.Committed)
	if report.Message != "" {
		fmt.Fprintln(writer, report.Message)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func UpdatePlanText(writer io.Writer, report domain.UpdatePlanReport) {
	fmt.Fprintf(writer, "Nixorium update plan: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:       %s\n", report.Repository)
	if report.Revision != "" {
		fmt.Fprintf(writer, "Deployment HEAD:  %s\n", report.Revision)
	}
	if report.CurrentRef != "" {
		fmt.Fprintf(writer, "Current upstream: %s (%s; %s)\n", report.CurrentRef, report.CurrentChannel, report.CurrentRev)
	}
	if report.Target != "" {
		fmt.Fprintf(writer, "Target release:   %s (%s)\n", report.Target, report.TargetChannel)
	}
	for _, check := range report.Checks {
		fmt.Fprintf(writer, "  %-18s %-8s %s\n", check.ID, strings.ToUpper(check.State), check.Message)
	}
	if report.ReviewToken != "" {
		fmt.Fprintf(writer, "Review token:     %s\n", report.ReviewToken)
		fmt.Fprintf(writer, "Confirmation:     %s\n", report.Confirmation)
		fmt.Fprintln(writer, "No branch, commit, push, activation, PXE action, or deployment is part of this plan.")
	}
	if report.Diff.Content != "" {
		fmt.Fprintln(writer, "\nProposed flake.nix/flake.lock changes:")
		fmt.Fprint(writer, report.Diff.Content)
		if !strings.HasSuffix(report.Diff.Content, "\n") {
			fmt.Fprintln(writer)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func UpdateCheckText(writer io.Writer, report domain.UpdateCheckReport) {
	fmt.Fprintf(writer, "Nixorium release check: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:       %s\n", report.Repository)
	if report.Upstream != "" {
		fmt.Fprintf(writer, "Public upstream:  %s\n", report.Upstream)
	}
	if report.CurrentRef != "" {
		fmt.Fprintf(writer, "Current upstream: %s (%s; %s)\n", report.CurrentRef, report.CurrentChannel, report.CurrentRev)
	}
	printUpdateReleases(writer, "Development branch", report.Development)
	printUpdateReleases(writer, "Stable releases", report.Stable)
	printUpdateReleases(writer, "Prereleases", report.Prerelease)
	if report.Truncated {
		fmt.Fprintln(writer, "Only the newest 20 releases per channel are shown.")
	}
	fmt.Fprintln(writer, "This explicit check is the only update operation that enumerates the remote.")
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "FAILED: %s: %s\n", issue.Field, issue.Message)
	}
}

func printUpdateReleases(writer io.Writer, title string, releases []domain.UpdateRelease) {
	fmt.Fprintf(writer, "%s:\n", title)
	if len(releases) == 0 {
		fmt.Fprintln(writer, "  none")
		return
	}
	for _, release := range releases {
		fmt.Fprintf(writer, "  %-24s %s\n", release.Tag, release.ObjectID)
	}
}

func UpdateApplyText(writer io.Writer, report domain.UpdateApplyReport) {
	fmt.Fprintf(writer, "Nixorium update: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Target release:  %s\n", report.Target)
	fmt.Fprintf(writer, "Files updated:   %t\n", report.Updated)
	if report.Message != "" {
		fmt.Fprintln(writer, report.Message)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func gitChangeOwnership(change domain.GitChange) string {
	if change.Private {
		return "private"
	}
	if change.Managed {
		return "managed"
	}
	return "unexpected"
}

func gitChangeIndex(change domain.GitChange) string {
	if change.Untracked {
		return "-"
	}
	if change.Staged == "" {
		return "-"
	}
	return change.Staged
}

func gitChangeWorktree(change domain.GitChange) string {
	if change.Untracked {
		return "untracked"
	}
	if change.Unstaged == "" {
		return "-"
	}
	return change.Unstaged
}

func truncatedGitDiffLabel(truncated bool) string {
	if truncated {
		return " (first 256 KiB; remaining output omitted)"
	}
	return ""
}

func gitScopeTitle(scope string) string {
	if scope == "staged" {
		return "Staged"
	}
	if scope == "unstaged" {
		return "Unstaged"
	}
	return scope
}

func HostsText(writer io.Writer, report domain.HostsReport) {
	available, total := hostAvailability(report.Hosts)
	fmt.Fprintf(writer, "Nixorium computers: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "SSH available:      %d/%d\n", available, total)
	fmt.Fprintf(writer, "Deployment:         %d current, %d outdated, %d unknown\n", report.Deployment.Current, report.Deployment.Outdated, report.Deployment.Unknown)
	if report.DesiredRevision != "" {
		fmt.Fprintf(writer, "Desired revision:   %s\n", report.DesiredRevision)
	}
	if report.HistoryDetail != "" {
		fmt.Fprintf(writer, "History warning:    %s\n", report.HistoryDetail)
	}
	for _, host := range report.Hosts {
		fmt.Fprintf(writer, "  %-10s %-15s network=%-12s ssh=%-11s deployment=%s\n", host.Name, host.IP, host.Reachability, host.SSH, host.Deployment)
		if host.CurrentSystem != "" {
			fmt.Fprintf(writer, "    current: %s\n", host.CurrentSystem)
		}
		if host.CurrentRevision != "" {
			fmt.Fprintf(writer, "    current revision: %s\n", host.CurrentRevision)
		}
		if host.LastSuccessfulDeploy != nil {
			fmt.Fprintf(writer, "    last verified: %s at %s\n", host.LastSuccessfulDeploy.Revision, host.LastSuccessfulDeploy.VerifiedAt.UTC().Format(time.RFC3339))
		}
		if host.DeploymentDetail != "" {
			fmt.Fprintf(writer, "    detail:  %s\n", host.DeploymentDetail)
		}
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
		if report.State == "ready" && report.Revision != "" {
			fmt.Fprintf(writer, "Next:            nixorium deploy apply --on %s --expect %s\n", report.ColmenaSelector, report.Revision)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func DeploymentExecutionText(writer io.Writer, report domain.DeploymentExecutionReport) {
	fmt.Fprintf(writer, "Deployment:      %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Phase:           %s\n", report.Phase)
	if report.Revision != "" {
		fmt.Fprintf(writer, "Revision:        %s\n", report.Revision)
	}
	if report.ColmenaSelector != "" {
		fmt.Fprintf(writer, "Targets:         %s\n", report.ColmenaSelector)
	}
	fmt.Fprintf(writer, "Build completed: %t\n", report.BuildCompleted)
	fmt.Fprintf(writer, "Apply completed: %t\n", report.ApplyCompleted)
	if report.Verification.Attempted > 0 {
		fmt.Fprintf(writer, "Verified targets: %d/%d (recorded %d)\n", report.Verification.Verified, report.Verification.Attempted, report.Verification.Recorded)
		for _, target := range report.Verification.Targets {
			if target.State != "verified" {
				fmt.Fprintf(writer, "  %-10s %s: %s\n", target.Name, target.State, target.Detail)
			}
		}
	}
	if report.LogPath != "" {
		fmt.Fprintf(writer, "Detailed log:    %s\n", report.LogPath)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:          %s\n", report.Message)
	}
	if report.HasErrors() && report.RetrySafe {
		fmt.Fprintln(writer, "Retry:           safe after reviewing current host state and a fresh deploy plan")
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "BLOCKED: %s: %s\n", issue.Field, issue.Message)
	}
}

func ControllerRebuildPlanText(writer io.Writer, report domain.ControllerRebuildPlanReport) {
	fmt.Fprintf(writer, "Controller rebuild plan: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Repository:              %s\n", report.Repository)
	if report.Controller != "" {
		fmt.Fprintf(writer, "Controller:              %s\n", report.Controller)
	}
	if report.Revision != "" {
		fmt.Fprintf(writer, "Reviewed revision:       %s\n", report.Revision)
	}
	fmt.Fprintf(writer, "Already current:         %t\n", report.Current)
	if report.CurrentDetail != "" {
		fmt.Fprintf(writer, "Current-state detail:    %s\n", report.CurrentDetail)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %-12s %s\n", issue.Field, issue.Message)
	}
	if !report.HasErrors() {
		fmt.Fprintf(writer, "Apply: nixorium controller apply --expect %s\n", report.Revision)
	}
}

func ControllerRebuildExecutionText(writer io.Writer, report domain.ControllerRebuildExecutionReport) {
	fmt.Fprintf(writer, "Controller rebuild: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Phase:      %s\n", report.Phase)
	fmt.Fprintf(writer, "Controller: %s\n", report.Controller)
	fmt.Fprintf(writer, "Revision:   %s\n", report.Revision)
	fmt.Fprintf(writer, "Applied:    %t\n", report.Applied)
	fmt.Fprintf(writer, "Verified:   %t\n", report.Verified)
	if report.Unit != "" {
		fmt.Fprintf(writer, "Unit:       %s\n", report.Unit)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %-12s %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:     %s\n", report.Message)
	}
	if report.HasErrors() && report.RetrySafe {
		fmt.Fprintln(writer, "Retry: inspect the unit journal, create a fresh plan, and retry the full workflow")
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

func SoftwareCatalogText(writer io.Writer, report domain.SoftwareCatalogReport) {
	fmt.Fprintf(writer, "Software catalog: %s\n", strings.ToUpper(report.State))
	if report.ManagedFile != "" {
		fmt.Fprintf(writer, "Managed file:     %s\n", report.ManagedFile)
	}
	for _, item := range report.Catalog {
		fmt.Fprintf(writer, "  %-18s %-20s %s\n", item.ID, item.Label, item.Summary)
	}
	if len(report.Packages) > 0 {
		fmt.Fprintln(writer, "Configured declarations:")
		for _, entry := range report.Packages {
			fmt.Fprintf(writer, "  %-18s %s\n", entry.Package, softwareScopeText(entry.Scope))
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:           %s\n", report.Message)
	}
}

func SoftwareSearchText(writer io.Writer, report domain.SoftwareSearchReport) {
	fmt.Fprintf(writer, "Software search: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Pinned query:    %s\n", report.Query)
	for _, item := range report.Results {
		version := item.Version
		if version == "" {
			version = "version unavailable"
		}
		fmt.Fprintf(writer, "  %-32s %-18s %-22s %s\n", item.ID, item.Availability, version, item.Summary)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:          %s\n", report.Message)
	}
}

func SoftwareChangePlanText(writer io.Writer, report domain.SoftwareChangePlanReport) {
	fmt.Fprintf(writer, "Software proposal: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Package:           %s\n", report.Request.Package)
	fmt.Fprintf(writer, "Managed file:      %s\n", report.ManagedFile)
	fmt.Fprintf(writer, "Scope:             %s\n", softwareScopeText(report.Request.Scope))
	if report.AffectedController != "" {
		fmt.Fprintf(writer, "Controller:        %s (configuration only; not activated)\n", report.AffectedController)
	}
	if len(report.AffectedClients) > 0 {
		fmt.Fprintf(writer, "Configuration:     %s\n", strings.Join(report.AffectedClients, ", "))
	}
	if report.ReviewToken != "" {
		fmt.Fprintf(writer, "Review token:      %s\nConfirmation:      %s\n", report.ReviewToken, report.Confirmation)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:            %s\n", report.Message)
	}
}

func softwareScopeText(scope domain.SoftwareScope) string {
	switch scope.Kind {
	case domain.SoftwareScopeGroup:
		return "group:" + scope.Group
	case domain.SoftwareScopeClients:
		return "clients:" + strings.Join(scope.Clients, ",")
	default:
		return scope.Kind
	}
}

func SoftwareChangeApplyText(writer io.Writer, report domain.SoftwareChangeApplyReport) {
	fmt.Fprintf(writer, "Software change: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Managed file:    %s\n", report.ManagedFile)
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:          %s\n", report.Message)
	}
}

func ShutdownPlanText(writer io.Writer, report domain.ShutdownPlanReport) {
	fmt.Fprintf(writer, "Shutdown plan: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Targets:       %s\n", report.Requested)
	fmt.Fprintf(writer, "Eligible:      %d/%d\n", report.Eligible, len(report.Targets))
	fmt.Fprintf(writer, "Session policy: %s\n", report.Policy)
	for _, target := range report.Targets {
		status := "not eligible"
		if target.Eligible {
			status = "eligible"
		}
		fmt.Fprintf(writer, "  %-10s %-12s session=%-7s %s\n", target.Name, status, target.Session, target.Detail)
	}
	if report.ReviewToken != "" {
		fmt.Fprintf(writer, "Review token:  %s\n", report.ReviewToken)
		fmt.Fprintf(writer, "Expires:       %s\n", report.ExpiresAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(writer, "Confirmation:  %s\n", report.Confirmation)
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail:        %s\n", report.Message)
	}
}

func ShutdownApplyText(writer io.Writer, report domain.ShutdownApplyReport) {
	fmt.Fprintf(writer, "Shutdown requests: %s\n", strings.ToUpper(report.State))
	fmt.Fprintf(writer, "Accepted: %d  Not sent: %d  Unconfirmed: %d\n", report.Accepted, report.NotSent, report.Unconfirmed)
	for _, target := range report.Targets {
		fmt.Fprintf(writer, "  %-10s %-11s %s\n", target.Name, target.State, target.Detail)
		if target.TechnicalDetail != "" {
			fmt.Fprintf(writer, "    technical: %s\n", target.TechnicalDetail)
		}
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(writer, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
	if report.Message != "" {
		fmt.Fprintf(writer, "Detail: %s\n", report.Message)
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
