package main

import (
	"context"
	"net/http"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Collect from desktop: the page reviews the selected computers through
// the worker's own collect plan, and after the confirmation the worker
// copies their desktops into its private temporary folder and the desktop
// helper of the page's user saves them as
// ~/Nixorium/Collected/<YYYY-MM-DD HH.MM>/<computer>/….

const actionCollect = "collect"

// Outcomes of a collection that the page counts besides done, off,
// skipped and failed.
const (
	outcomeEmpty   = "empty"
	outcomeTooMuch = "too-much"
)

const (
	// collectPageDestination is the folder the page's review names.
	collectPageDestination = "Nixorium/Collected in your home folder"
	// collectPageTime bounds collecting and saving from the page, and
	// collectHelperWait the wait for a helper busy with another job.
	collectPageTime   = 45 * time.Minute
	collectHelperWait = time.Minute
)

func (server *viewServer) planCollect(ctx context.Context, request *http.Request, operation domain.ClassroomRequest) (actionReview, pendingAction) {
	grant, signed := server.session(request)
	if !signed || server.desktops == nil || !server.desktops.Available(grant.uid) {
		return actionReview{Title: "Collect", Message: errNoDesktopHelper.Error(), Rows: []actionRow{}}, pendingAction{}
	}
	operation.Operation = domain.ClassroomCollectPlanOperation
	operation.CollectFolder = collectPageDestination
	response := server.actions(adapters.WithClassroomPeerUID(ctx, grant.uid), operation)
	if response.CollectPlan == nil {
		return actionReview{Title: "Collect", Message: response.Message, Rows: []actionRow{}}, pendingAction{}
	}
	return collectReview(*response.CollectPlan)
}

func collectReview(plan domain.CollectPlan) (actionReview, pendingAction) {
	review := actionReview{Title: "Collect", Message: plan.Message, Rows: []actionRow{}}
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
		review.Confirm = "Collect from " + plural(eligible, "computer")
	}
	return review, pendingAction{collect: &plan, expires: plan.ExpiresAt}
}

// applyCollect collects the reviewed computers and has the page user's
// desktop helper save the result.
func (server *viewServer) applyCollect(parent context.Context, request *http.Request, plan *domain.CollectPlan) actionResult {
	result := actionResult{State: "failed", Rows: []actionRow{}}
	grant, signed := server.session(request)
	if !signed || server.desktops == nil {
		result.Message = errNoDesktopHelper.Error()
		return result
	}
	ctx, cancel := context.WithTimeout(parent, collectPageTime)
	defer cancel()
	ctx = adapters.WithClassroomPeerUID(ctx, grant.uid)
	operation, err := domain.NewClassroomRequest(domain.ClassroomCollectApplyOperation)
	if err != nil {
		result.Message = "The action could not start."
		return result
	}
	operation.CollectPlan = plan
	response := server.actions(ctx, operation)
	if response.CollectReport == nil {
		result.Message = response.Message
		return result
	}
	report := response.CollectReport
	result.State, result.Message = report.State, report.Message
	folder, problem := "", ""
	if report.Save != "" {
		outcome, err := server.desktops.Deliver(ctx, report.Save, collectHelperWait, collectPageTime)
		switch {
		case err != nil:
			server.desktops.Withdraw(report.Save)
			problem = err.Error()
		case len(outcome.names) != 1 && outcome.message != "":
			problem = outcome.message
		case len(outcome.names) != 1:
			problem = "The files arrived incomplete."
		default:
			folder = outcome.names[0]
		}
		if problem != "" {
			result.State = "partial"
			result.Message = "The files were collected but not saved in your home folder: " + problem
		}
	}
	planned := map[string]plannedTarget{}
	for _, target := range plan.Targets {
		planned[target.Name] = plannedTarget{eligible: target.Eligible, detail: target.Detail}
	}
	for _, target := range report.Targets {
		row := actionRow{Name: target.Name, Note: target.Detail}
		switch target.State {
		case domain.CollectCollected:
			row.Outcome = outcomeDone
			if problem != "" {
				row.Skip, row.Outcome, row.Note = true, outcomeFailed, "Collected, but not saved in your home folder."
			} else if folder != "" {
				row.Note += " In " + folder + "/" + target.Name + "."
			}
		case domain.CollectEmpty:
			row.Outcome = outcomeEmpty
		case domain.CollectTooMuch:
			row.Skip, row.Outcome = true, outcomeTooMuch
		case domain.CollectNotSent:
			row.Skip, row.Outcome = true, outcomeSkipped
			if target, found := planned[target.Name]; found && !target.eligible && target.detail != "" {
				row.Note = target.detail
			}
			if row.Note == app.ClassroomOffDetail {
				row.Outcome = outcomeOff
			}
		default:
			row.Skip, row.Outcome = true, outcomeFailed
		}
		result.Rows = append(result.Rows, row)
	}
	return result
}
