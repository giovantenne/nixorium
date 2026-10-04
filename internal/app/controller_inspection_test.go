package app

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type inspectingControllerSource struct {
	*fakeControllerSource
	inspections int
	inspectErr  error
}

func (f *inspectingControllerSource) InspectController(context.Context, string) (domain.ControllerInspection, error) {
	f.inspections++
	return domain.ControllerInspection{Meta: f.meta, Deployment: f.deployment, Current: f.current, CurrentDetail: f.detail}, f.inspectErr
}

func (*inspectingControllerSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	panic("duplicate metadata evaluation")
}

func (*inspectingControllerSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	panic("duplicate readiness evaluation")
}

func (*inspectingControllerSource) ControllerState(context.Context, string) (bool, string, error) {
	panic("duplicate controller evaluation")
}

func TestControllerInspectionIsFreshBeforeAndAfterApply(t *testing.T) {
	source := &inspectingControllerSource{fakeControllerSource: readyControllerSource()}
	manager := NewControllerManager(source)
	plan := manager.Plan(t.Context(), "/deployment")
	if plan.HasErrors() || source.inspections != 1 {
		t.Fatalf("plan=%+v inspections=%d", plan, source.inspections)
	}
	result := manager.Apply(t.Context(), plan.Repository, plan.Revision)
	if !result.Verified || result.HasErrors() || source.inspections != 3 {
		t.Fatalf("apply did not recheck before and after activation: %+v inspections=%d", result, source.inspections)
	}
}

func TestControllerInspectionCannotBypassFreshReadinessOrRevision(t *testing.T) {
	for _, change := range []string{"readiness", "revision", "dirty", "evaluation"} {
		t.Run(change, func(t *testing.T) {
			source := &inspectingControllerSource{fakeControllerSource: readyControllerSource()}
			manager := NewControllerManager(source)
			plan := manager.Plan(t.Context(), "/deployment")
			switch change {
			case "readiness":
				source.deployment.Controller = &domain.ControllerReadiness{Issues: []string{"controller is not ready"}}
			case "revision":
				source.revision = "fedcba9876543210fedcba9876543210fedcba98"
			case "dirty":
				source.git.Dirty = true
			case "evaluation":
				source.inspectErr = errors.New("prerequisite failure")
			}
			result := manager.Apply(t.Context(), plan.Repository, plan.Revision)
			if !result.HasErrors() || result.Applied || source.unit != "" || source.inspections != 2 {
				t.Fatalf("changed preflight permitted activation: %+v", result)
			}
		})
	}
}
