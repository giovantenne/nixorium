package main

import (
	"context"
	"errors"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Switch on: Wake-on-LAN packets for student computers that are off, for
// the teacher's dashboard and command line (classroom socket) and for the
// classroom view page. A packet sent is never shown as a computer started.

const actionWake = "wake"

// Outcomes of Switch on, which the page counts in its notice as sent,
// already on and address unknown.
const (
	outcomeSent    = "sent"
	outcomeOn      = "on"
	outcomeUnknown = "unknown"
)

// wakeLabel stays on the card of a computer that was sent a packet until
// it answers, for at most powerLabelTime.
const wakeLabel = "Starting…"

func (worker classroomWorker) handleWake(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	if worker.wake == nil {
		return domain.ClassroomResponse{State: "failed", Message: "Switching computers on is not available."}
	}
	response := domain.ClassroomResponse{State: "completed"}
	if request.Operation == domain.ClassroomWakePlanOperation {
		plan := worker.wake.Plan(ctx, worker.repository, request.Requested)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		response.WakePlan = &plan
		return response
	}
	// The plan names computers only: the addresses come from the service's
	// own record, and the inventory is checked again.
	if request.WakePlan == nil || request.WakePlan.Repository != worker.repository {
		return classroomFailure(errors.New("switch-on review does not belong to the fixed deployment"))
	}
	report := worker.wake.Apply(ctx, *request.WakePlan)
	report.Message = teacherMessage(report.Message)
	response.WakeReport = &report
	return response
}

func (server *viewServer) planWake(ctx context.Context, operation domain.ClassroomRequest) (actionReview, pendingAction) {
	operation.Operation = domain.ClassroomWakePlanOperation
	response := server.actions(ctx, operation)
	if response.WakePlan == nil {
		return actionReview{Title: "Switch on", Message: response.Message}, pendingAction{}
	}
	return wakeReview(*response.WakePlan)
}

// wakeReview lists the computers that receive the packet, and those
// skipped because they are already on or their address is not known yet.
func wakeReview(plan domain.WakePlan) (actionReview, pendingAction) {
	review := actionReview{Title: "Switch on", Message: plan.Message, Rows: []actionRow{}}
	for _, target := range plan.Targets {
		review.Rows = append(review.Rows, actionRow{Name: target.Name, Note: target.Detail, Skip: target.State != domain.WakeTargetSend})
	}
	for _, issue := range plan.Issues {
		review.Warnings = append(review.Warnings, issue.Message)
	}
	send := plan.Count(domain.WakeTargetSend)
	review.Ready = !plan.HasErrors() && send > 0
	if review.Ready {
		review.Confirm = "Switch on " + plural(send, "computer")
	}
	return review, pendingAction{wake: &plan, expires: plan.ExpiresAt}
}

func (server *viewServer) applyWake(ctx context.Context, plan domain.WakePlan) actionResult {
	result := actionResult{State: "failed", Rows: []actionRow{}}
	operation, err := domain.NewClassroomRequest(domain.ClassroomWakeApplyOperation)
	if err != nil {
		result.Message = "The action could not start."
		return result
	}
	operation.WakePlan = &plan
	response := server.actions(ctx, operation)
	if response.WakeReport == nil {
		result.Message = response.Message
		return result
	}
	report := response.WakeReport
	result.State, result.Message = report.State, report.Message
	for _, target := range report.Targets {
		row := actionRow{Name: target.Name, Note: target.Detail}
		switch target.State {
		case domain.WakeOutcomeSent:
			row.Outcome = outcomeSent
			server.setLabel(target.Name, func(labels *computerLabels) {
				labels.power, labels.powerExpiry = wakeLabel, server.now().Add(powerLabelTime)
				labels.powerRestart, labels.powerAway, labels.powerStart = false, false, true
			})
		case domain.WakeOutcomeOn:
			row.Outcome = outcomeOn
		case domain.WakeOutcomeUnknown:
			row.Outcome, row.Skip = outcomeUnknown, true
		default:
			row.Outcome, row.Skip = outcomeFailed, true
		}
		result.Rows = append(result.Rows, row)
	}
	return result
}
