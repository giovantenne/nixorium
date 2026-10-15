package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Message and exam mode in the classroom view page. The page reviews a
// message through the worker's own message plan, and applies it once after
// the confirmation, like a lock. Exam mode is the page's own composition of
// reviews of Internet, lock and message; the worker remembers which steps
// it applied for it (exam.go).

const (
	actionMessage     = "message"
	actionMessageHide = "message-hide"
)

// planMessage reviews showing (or removing) the message the page sent along.
func (server *viewServer) planMessage(ctx context.Context, operation domain.ClassroomRequest, body actionPlanRequest) (actionReview, pendingAction) {
	operation.Operation = domain.ClassroomMessagePlanOperation
	operation.MessageAction = domain.MessageShow
	operation.MessageContent = body.Message
	if body.Action == actionMessageHide {
		operation.MessageAction = domain.MessageHide
	}
	response := server.actions(ctx, operation)
	if response.MessagePlan == nil {
		return actionReview{Title: "Message", Message: response.Message, Rows: []actionRow{}}, pendingAction{}
	}
	return messageReview(*response.MessagePlan)
}

func messageReview(plan domain.MessagePlan) (actionReview, pendingAction) {
	hide := plan.Action == domain.MessageHide
	review := actionReview{Title: "Message", Message: plan.Message, Rows: []actionRow{}}
	if hide {
		review.Title = "Remove message"
	}
	eligible := 0
	for _, target := range plan.Targets {
		review.Rows = append(review.Rows, actionRow{Name: target.Name, Note: target.Detail, Skip: !target.Eligible})
		if target.Eligible {
			eligible++
		}
	}
	for _, issue := range plan.Issues {
		review.Warnings = append(review.Warnings, issue.Message)
	}
	review.Ready = !plan.HasErrors() && eligible > 0
	if review.Ready {
		// The review shows the text itself, ahead of the controller's own
		// sentences, which never repeat it.
		if !hide {
			review.Message = "“" + plan.Content.Text + "”\n\n" + plan.Message
			review.Confirm = "Send the message to " + plural(eligible, "computer")
		} else {
			review.Confirm = "Remove the message from " + plural(eligible, "computer")
		}
	}
	return review, pendingAction{message: &plan, expires: plan.ExpiresAt}
}

// applyMessage shows or removes the reviewed message; each computer is
// counted as "shown" or "removed" in the page's notice.
func (server *viewServer) applyMessage(ctx context.Context, plan *domain.MessagePlan) actionResult {
	result := actionResult{Rows: []actionRow{}, DoneWord: "shown"}
	if plan.Action == domain.MessageHide {
		result.DoneWord = "removed"
	}
	operation, err := domain.NewClassroomRequest(domain.ClassroomMessageApplyOperation)
	if err != nil {
		result.State, result.Message = "failed", "The action could not start."
		return result
	}
	operation.MessagePlan = plan
	response := server.actions(ctx, operation)
	if response.MessageReport == nil {
		result.State, result.Message = "failed", response.Message
		return result
	}
	report := response.MessageReport
	result.State, result.Message = report.State, report.Message
	planned := map[string]plannedTarget{}
	for _, target := range plan.Targets {
		planned[target.Name] = plannedTarget{eligible: target.Eligible, detail: target.Detail}
	}
	for _, target := range report.Targets {
		result.Rows = append(result.Rows, resultRow(target.Name, target.State == domain.MessageDone, target.Detail, planned[target.Name]))
	}
	return result
}
