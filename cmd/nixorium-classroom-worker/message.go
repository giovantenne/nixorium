package main

import (
	"context"
	"errors"

	"github.com/giovantenne/nixorium/internal/domain"
)

// handleMessage answers showing and removing a message on students' screens
// with the same expiring, content-bound review as the lock. The classroom
// socket admits only the teacher and the administrator.
func (worker classroomWorker) handleMessage(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	response := domain.ClassroomResponse{State: "completed"}
	if worker.message == nil {
		return domain.ClassroomResponse{State: "failed", Message: "Messages are not available."}
	}
	switch request.Operation {
	case domain.ClassroomMessagePlanOperation:
		if worker.viewOn == nil || !worker.viewOn() {
			return domain.ClassroomResponse{State: "failed", Message: errViewDisabled.Error()}
		}
		content := domain.MessageContent{}
		if request.MessageContent != nil {
			content = *request.MessageContent
		}
		plan := worker.message.Plan(ctx, worker.repository, request.Requested, request.MessageAction, content)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		response.MessagePlan = &plan
	case domain.ClassroomMessageApplyOperation:
		if request.MessagePlan == nil || request.MessagePlan.Repository != worker.repository {
			return classroomFailure(errors.New("message review does not belong to the fixed deployment"))
		}
		report := worker.message.Apply(ctx, *request.MessagePlan, request.MessagePlan.ReviewToken)
		report.Message = teacherMessage(report.Message)
		response.MessageReport = &report
	default:
		return classroomFailure(errors.New("unsupported classroom operation"))
	}
	return response
}
