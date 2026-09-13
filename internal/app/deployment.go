package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type DeploymentSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error)
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
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
