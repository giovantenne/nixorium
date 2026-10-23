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

type controllerInspectionSource interface {
	InspectController(context.Context, string) (domain.ControllerInspection, error)
}

// controllerActivationSource confirms an activation from the record the apply
// unit writes only after the active system is the one it built from the
// reviewed revision, instead of evaluating the controller once more.
type controllerActivationSource interface {
	ControllerActivated(ctx context.Context, revision string) (bool, string, error)
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
	var inspection *domain.ControllerInspection
	if source, ok := m.source.(controllerInspectionSource); ok {
		observed, inspectErr := source.InspectController(ctx, root)
		if inspectErr != nil {
			return controllerIssue(report, "controller", inspectErr.Error())
		}
		inspection = &observed
	}
	var meta domain.LabMeta
	if inspection != nil {
		meta = inspection.Meta
	} else {
		meta, err = m.source.LabMeta(ctx, root)
	}
	if err != nil {
		report.Issues = append(report.Issues, domain.EvaluationIssue("configuration", "labMeta", err))
		return report
	}
	report.Controller = meta.Controller.Name
	var deployment domain.DeploymentStatus
	if inspection != nil {
		deployment = inspection.Deployment
	} else {
		deployment, err = m.source.DeploymentStatus(ctx, root)
	}
	if err != nil {
		report = controllerIssue(report, "readiness", fmt.Sprintf("evaluate deploymentStatus: %v", err))
	} else if readiness := deployment.ControllerReadiness(); !readiness.Ready {
		for _, issue := range readiness.Issues {
			report = controllerIssue(report, "readiness", issue)
		}
		if len(readiness.Issues) == 0 {
			report = controllerIssue(report, "readiness", "controller configuration is not ready")
		}
	}
	gitState, err := m.source.GitState(ctx, root)
	if err != nil {
		report = controllerIssue(report, "git", fmt.Sprintf("inspect worktree: %v", err))
	} else if gitState.Dirty {
		report.Issues = append(report.Issues, domain.BlockedIssue("git", "GIT-DIRTY", fmt.Sprintf("deployment worktree has %d changed path(s); save or inspect them in Maintenance → Review Git changes (`nixorium git review`)", gitState.Changes)))
	}
	if revision, revisionErr := m.source.GitRevision(ctx, root); revisionErr != nil {
		report = controllerIssue(report, "git", fmt.Sprintf("resolve revision: %v", revisionErr))
	} else if !fullGitRevisionPattern.MatchString(revision) {
		report = controllerIssue(report, "git", "resolved revision is not a full Git object ID")
	} else {
		report.Revision = revision
	}
	if inspection != nil {
		report.Current, report.CurrentDetail = inspection.Current, inspection.CurrentDetail
		report.Changes, report.ChangesKnown = inspection.Changes, inspection.ChangesKnown
		err = nil
	} else {
		report.Current, report.CurrentDetail, err = m.source.ControllerState(ctx, root)
	}
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
	return m.apply(ctx, m.Plan(ctx, repository), expectedRevision)
}

// ApplyReviewed applies a plan the caller has just reviewed. The plan's
// evaluation is not repeated: only the cheap Git checks that tie it to the
// deployment are, and the apply unit validates the configuration again
// before it builds anything.
func (m *ControllerManager) ApplyReviewed(ctx context.Context, repository string, plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
	root, err := filepath.Abs(repository)
	if err != nil || plan.Repository != root {
		plan = controllerIssue(plan, "review", "the reviewed controller plan belongs to another deployment")
	} else if state, stateErr := m.source.GitState(ctx, root); stateErr != nil || state.Dirty {
		plan = controllerIssue(plan, "git", "deployment worktree changed after the controller review")
	} else if revision, revisionErr := m.source.GitRevision(ctx, root); revisionErr != nil || revision != plan.Revision {
		plan = controllerIssue(plan, "review", "deployment revision differs from the reviewed controller plan")
	}
	return m.apply(ctx, plan, plan.Revision)
}

func (m *ControllerManager) apply(ctx context.Context, plan domain.ControllerRebuildPlanReport, expectedRevision string) domain.ControllerRebuildExecutionReport {
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
	active := false
	if source, ok := m.source.(controllerActivationSource); ok {
		recorded, _, recordErr := source.ControllerActivated(ctx, expectedRevision)
		active = recorded && recordErr == nil
	} else {
		verified := m.Plan(ctx, plan.Repository)
		active = !verified.HasErrors() && verified.Revision == expectedRevision && verified.Current
	}
	if !active {
		report.State = "failed"
		report.Message = "controller activation completed but the reviewed revision is not the active system; inspect the unit journal and create a fresh plan"
		return report
	}
	report.Verified = true
	report.State = "completed"
	report.Phase = domain.ControllerRebuildPhaseComplete
	report.Message = "The controller now uses the saved configuration."
	return report
}

func ControllerApplyUnit(revision string) string {
	return "nixorium-apply-controller@" + revision + ".service"
}

func controllerIssue(report domain.ControllerRebuildPlanReport, field, message string) domain.ControllerRebuildPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}
