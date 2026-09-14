package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type DeploymentSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error)
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
	InterfaceAddresses(string) ([]string, error)
	RunDeploymentPhase(context.Context, string, domain.DeploymentPhase, string, io.Writer) error
}

func (m *DeploymentManager) Execute(ctx context.Context, repository, requested, expectedRevision, logPath string, progress io.Writer) domain.DeploymentExecutionReport {
	plan := m.Plan(ctx, repository, requested)
	report := executionFromPlan(plan, logPath)
	if plan.HasErrors() {
		return report
	}
	if expectedRevision == "" {
		return deploymentExecutionIssue(report, "review", "expected revision is required; copy it from deploy plan")
	}
	if plan.Revision != expectedRevision {
		return deploymentExecutionIssue(report, "review", fmt.Sprintf("reviewed revision %s does not match current revision %s", expectedRevision, plan.Revision))
	}
	if progress == nil {
		progress = io.Discard
	}

	report.State = "running"
	report.Phase = domain.DeploymentPhaseBuild
	fmt.Fprintf(progress, "Deployment revision: %s\nTargets: %s\n\n", report.Revision, report.ColmenaSelector)
	fmt.Fprintln(progress, "==> Building selected configurations with Colmena")
	if err := m.source.RunDeploymentPhase(ctx, report.Repository, domain.DeploymentPhaseBuild, report.ColmenaSelector, progress); err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("build failed before any deployment was started: %v", err)
		return report
	}
	report.BuildCompleted = true

	verified := m.Plan(ctx, repository, requested)
	if verified.HasErrors() {
		report.State = "failed"
		report.Phase = domain.DeploymentPhasePreflight
		report.Message = "post-build validation failed; no apply was started"
		report.Issues = append(report.Issues, verified.Issues...)
		return report
	}
	if verified.Revision != expectedRevision || verified.ColmenaSelector != report.ColmenaSelector || !sameDeploymentTargets(verified.Targets, report.Targets) {
		report.State = "failed"
		report.Phase = domain.DeploymentPhasePreflight
		report.Message = "reviewed revision or targets changed after build; no apply was started"
		return deploymentExecutionIssue(report, "review", "run deploy plan again and review the new revision and targets")
	}

	report.Phase = domain.DeploymentPhaseApply
	fmt.Fprintln(progress, "\n==> Applying the built configurations with Colmena")
	if err := m.source.RunDeploymentPhase(ctx, report.Repository, domain.DeploymentPhaseApply, report.ColmenaSelector, progress); err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("apply failed; some targets may already have changed: %v", err)
		return report
	}
	report.ApplyCompleted = true
	report.Phase = domain.DeploymentPhaseComplete
	report.State = "completed"
	report.Message = "all selected targets were built and applied successfully"
	return report
}

func executionFromPlan(plan domain.DeploymentPlanReport, logPath string) domain.DeploymentExecutionReport {
	return domain.DeploymentExecutionReport{
		SchemaVersion:   domain.SchemaVersion,
		Operation:       "deploy-apply",
		State:           "blocked",
		Repository:      plan.Repository,
		Requested:       plan.Requested,
		Revision:        plan.Revision,
		ColmenaSelector: plan.ColmenaSelector,
		Targets:         plan.Targets,
		Phase:           domain.DeploymentPhasePreflight,
		RetrySafe:       true,
		LogPath:         logPath,
		Issues:          append([]domain.ValidationIssue{}, plan.Issues...),
	}
}

func deploymentExecutionIssue(report domain.DeploymentExecutionReport, field, message string) domain.DeploymentExecutionReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}

func sameDeploymentTargets(left, right []domain.DeploymentTarget) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type DeploymentManager struct {
	source DeploymentSource
}

func NewDeploymentManager(source DeploymentSource) *DeploymentManager {
	return &DeploymentManager{source: source}
}

func (m *DeploymentManager) Plan(ctx context.Context, repository, requested string) domain.DeploymentPlanReport {
	report := domain.DeploymentPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "deploy-plan",
		State:         "blocked",
		Requested:     requested,
		BuildFirst:    true,
		Targets:       []domain.DeploymentTarget{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return deploymentIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return deploymentIssue(report, "configuration", fmt.Sprintf("evaluate labMeta: %v", err))
	}
	deployment, err := m.source.DeploymentStatus(ctx, root)
	if err != nil {
		return deploymentIssue(report, "readiness", fmt.Sprintf("evaluate deploymentStatus: %v", err))
	}
	if !deployment.Ready {
		for _, issue := range deployment.Issues {
			report = deploymentIssue(report, "readiness", issue)
		}
	}
	gitState, err := m.source.GitState(ctx, root)
	if err != nil {
		report = deploymentIssue(report, "git", fmt.Sprintf("inspect worktree: %v", err))
	} else if gitState.Dirty {
		report = deploymentIssue(report, "git", fmt.Sprintf("deployment worktree has %d changed path(s)", gitState.Changes))
	}
	if revision, revisionErr := m.source.GitRevision(ctx, root); revisionErr != nil {
		report = deploymentIssue(report, "git", fmt.Sprintf("resolve revision: %v", revisionErr))
	} else {
		report.Revision = revision
	}
	if addresses, addressErr := m.source.InterfaceAddresses(meta.Network.Interface); addressErr != nil {
		report = deploymentIssue(report, "network", fmt.Sprintf("inspect %s: %v", meta.Network.Interface, addressErr))
	} else if !containsIP(addresses, meta.Controller.StaticIP) {
		report = deploymentIssue(report, "network", fmt.Sprintf("controller deployment address %s is not assigned to %s; stop or recover PXE mode first", meta.Controller.StaticIP, meta.Network.Interface))
	}

	targets, selector, selectionIssues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	report.Targets = targets
	report.ColmenaSelector = selector
	for _, issue := range selectionIssues {
		report = deploymentIssue(report, "selector", issue)
	}
	if !report.HasErrors() {
		report.State = "ready"
	}
	return report
}

func selectDeploymentTargets(hosts []domain.HostMeta, requested string) ([]domain.DeploymentTarget, string, []string) {
	available := make(map[string]domain.HostMeta, len(hosts))
	for _, host := range hosts {
		available[host.Name] = host
	}
	names := []string{}
	issues := []string{}
	if requested == "@lab" {
		for _, host := range hosts {
			names = append(names, host.Name)
		}
	} else {
		seen := map[string]bool{}
		for _, raw := range strings.Split(requested, ",") {
			name := strings.TrimSpace(raw)
			if name == "" {
				issues = append(issues, "target list contains an empty hostname")
				continue
			}
			if _, found := available[name]; !found {
				issues = append(issues, fmt.Sprintf("%s is not a configured client", name))
				continue
			}
			if seen[name] {
				issues = append(issues, fmt.Sprintf("%s is selected more than once", name))
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	targets := make([]domain.DeploymentTarget, 0, len(names))
	for _, name := range names {
		host := available[name]
		targets = append(targets, domain.DeploymentTarget{Name: host.Name, IP: host.IP})
	}
	selector := strings.Join(names, ",")
	if requested == "@lab" && len(issues) == 0 {
		selector = "@lab"
	}
	if len(targets) == 0 && len(issues) == 0 {
		issues = append(issues, "no clients were selected")
	}
	return targets, selector, issues
}

func deploymentIssue(report domain.DeploymentPlanReport, field, message string) domain.DeploymentPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}
