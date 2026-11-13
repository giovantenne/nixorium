package main

import (
	"context"
	"errors"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Recovering a student's files from a saved session, for the teacher's
// dashboard (classroom socket). It works whether the classroom view is on
// or off: the controller reaches the computer over SSH, not through the
// classroom agent.

func (worker classroomWorker) handleFileRecovery(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	if worker.recovery == nil {
		return domain.ClassroomResponse{State: "failed", Message: "Recovering student files is not available."}
	}
	refused := func(err error) domain.ClassroomResponse {
		return domain.ClassroomResponse{State: "failed", Message: teacherMessage(err.Error())}
	}
	response := domain.ClassroomResponse{State: "completed"}
	switch request.Operation {
	case domain.ClassroomFileRecoverySessionsOperation:
		sessions, err := worker.recovery.Sessions(ctx, worker.repository, request.Requested)
		if err != nil {
			return refused(err)
		}
		response.FileRecoverySessions = sessions
	case domain.ClassroomFileRecoveryListOperation:
		listing, err := worker.recovery.List(ctx, worker.repository, request.Requested, request.FileRecoverySession, request.FileRecoveryPath)
		if err != nil {
			return refused(err)
		}
		response.FileRecoveryListing = &listing
	case domain.ClassroomFileRecoveryPlanOperation:
		plan := worker.recovery.Plan(ctx, worker.repository, request.Requested, request.FileRecoverySession, request.FileRecoveryPaths)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		rememberPlan(worker.plans, "file-recovery", plan.ReviewToken, plan.ExpiresAt, plan)
		response.FileRecoveryPlan = &plan
	case domain.ClassroomFileRecoveryApplyOperation:
		// Only a plan this service issued is applied, never the request's copy.
		if request.FileRecoveryPlan == nil || request.FileRecoveryPlan.Repository != worker.repository {
			return classroomFailure(errors.New("file recovery review does not belong to the fixed deployment"))
		}
		plan, err := takePlan[domain.FileRecoveryPlan](worker.plans, "file-recovery", request.FileRecoveryPlan.ReviewToken)
		if err != nil {
			return classroomFailure(err)
		}
		report := worker.recovery.Apply(ctx, plan, plan.ReviewToken)
		report.Message = teacherMessage(report.Message)
		response.FileRecoveryReport = &report
	default:
		return classroomFailure(errors.New("unsupported classroom operation"))
	}
	return response
}
