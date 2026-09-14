package app

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/giovantenne/nixorium/internal/domain"
)

var fullGitRevisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type ControllerRebuildSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error)
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
	ControllerState(context.Context, string) (bool, string, error)
	StartSystemUnit(context.Context, string) error
}

type ControllerManager struct {
	source ControllerRebuildSource
}

func NewControllerManager(source ControllerRebuildSource) *ControllerManager {
	return &ControllerManager{source: source}
}

func (m *ControllerManager) Plan(ctx context.Context, repository string) domain.ControllerRebuildPlanReport {
	report := domain.ControllerRebuildPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "controller-plan",
		State:         "blocked",
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return controllerIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return controllerIssue(report, "configuration", fmt.Sprintf("evaluate labMeta: %v", err))
	}
	report.Controller = meta.Controller.Name
	report.Confirmation = "REBUILD " + meta.Controller.Name
	deployment, err := m.source.DeploymentStatus(ctx, root)
	if err != nil {
		report = controllerIssue(report, "readiness", fmt.Sprintf("evaluate deploymentStatus: %v", err))
	} else if !deployment.Ready {
		for _, issue := range deployment.Issues {
			report = controllerIssue(report, "readiness", issue)
		}
	}
	gitState, err := m.source.GitState(ctx, root)
	if err != nil {
		report = controllerIssue(report, "git", fmt.Sprintf("inspect worktree: %v", err))
	} else if gitState.Dirty {
		report = controllerIssue(report, "git", fmt.Sprintf("deployment worktree has %d changed path(s)", gitState.Changes))
	}
	if revision, revisionErr := m.source.GitRevision(ctx, root); revisionErr != nil {
		report = controllerIssue(report, "git", fmt.Sprintf("resolve revision: %v", revisionErr))
	} else if !fullGitRevisionPattern.MatchString(revision) {
		report = controllerIssue(report, "git", "resolved revision is not a full Git object ID")
	} else {
		report.Revision = revision
	}
	report.Current, report.CurrentDetail, err = m.source.ControllerState(ctx, root)
	if err != nil {
		report = controllerIssue(report, "controller", err.Error())
	}
	if !report.HasErrors() {
		report.State = "ready"
		if report.Current {
			report.State = "current"
		}
	}
	return report
}

func (m *ControllerManager) Apply(ctx context.Context, repository, expectedRevision string) domain.ControllerRebuildExecutionReport {
	plan := m.Plan(ctx, repository)
	report := domain.ControllerRebuildExecutionReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "controller-apply",
		State:         "blocked",
		Repository:    plan.Repository,
		Controller:    plan.Controller,
		Revision:      plan.Revision,
		Phase:         domain.ControllerRebuildPhasePreflight,
		RetrySafe:     true,
		Issues:        append([]domain.ValidationIssue(nil), plan.Issues...),
	}
	if plan.HasErrors() {
		report.Message = "controller rebuild preflight failed; no action was started"
		return report
	}
	if expectedRevision == "" || plan.Revision != expectedRevision {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "deployment revision differs from the reviewed controller plan"})
		report.Message = "controller rebuild review is stale; create a fresh plan"
		return report
	}
	report.Unit = ControllerApplyUnit(expectedRevision)
	report.Phase = domain.ControllerRebuildPhaseApply
	if err := m.source.StartSystemUnit(ctx, report.Unit); err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("controller build or activation failed: %v", err)
		return report
	}
	report.Applied = true
	report.Phase = domain.ControllerRebuildPhaseVerify
	verified := m.Plan(ctx, plan.Repository)
	if verified.HasErrors() || verified.Revision != expectedRevision || !verified.Current {
		report.State = "failed"
		report.Message = "controller activation completed but the reviewed revision is not the active system; inspect the unit journal and create a fresh plan"
		return report
	}
	report.Verified = true
	report.State = "completed"
	report.Phase = domain.ControllerRebuildPhaseComplete
	report.Message = "reviewed controller revision built, activated, and verified"
	return report
}

func ControllerApplyUnit(revision string) string {
	return "nixorium-apply-controller@" + revision + ".service"
}

func controllerIssue(report domain.ControllerRebuildPlanReport, field, message string) domain.ControllerRebuildPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}
