package main

import (
	"context"
	"errors"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// handleCollect answers collecting students' desktops. The classroom
// socket admits only the teacher and the administrator; the collected
// files are kept for the user who asked (the socket's kernel peer
// credentials), who saves them like any save (desktop_save.go).
func (worker classroomWorker) handleCollect(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	response := domain.ClassroomResponse{State: "completed"}
	failed := func(message string) domain.ClassroomResponse {
		return domain.ClassroomResponse{State: "failed", Message: message}
	}
	uid, known := adapters.ClassroomPeerUID(ctx)
	if !known || worker.view == nil || worker.view.desktops == nil {
		return failed("Collecting files is not available.")
	}
	switch request.Operation {
	case domain.ClassroomCollectPlanOperation:
		if worker.viewOn == nil || !worker.viewOn() {
			return failed(errViewDisabled.Error())
		}
		if worker.collect == nil {
			return failed("Collecting files is not available.")
		}
		plan := worker.collect.Plan(ctx, worker.repository, request.Requested, request.CollectFolder)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		response.CollectPlan = &plan
	case domain.ClassroomCollectApplyOperation:
		if worker.collect == nil || worker.collected == nil {
			return failed("Collecting files is not available.")
		}
		if request.CollectPlan == nil || request.CollectPlan.Repository != worker.repository {
			return classroomFailure(errors.New("collect review does not belong to the fixed deployment"))
		}
		report := worker.collect.Apply(ctx, *request.CollectPlan, request.CollectPlan.ReviewToken)
		if report.Save != "" {
			collection := report.Save
			report.Save = ""
			id, err := worker.offerCollection(uid, collection)
			if err != nil {
				worker.collected.Remove(collection)
				report.State, report.Folder = "partial", ""
				report.Message += " The collected files could not be kept: " + err.Error() + "."
				for index := range report.Targets {
					if report.Targets[index].State == domain.CollectCollected {
						report.Targets[index].State, report.Targets[index].Detail = domain.CollectFailed, "Collected, but the files could not be kept; collect again."
					}
				}
			} else {
				report.Save = id
			}
		}
		report.Message = teacherMessage(report.Message)
		response.CollectReport = &report
	default:
		return classroomFailure(errors.New("unsupported classroom operation"))
	}
	return response
}

// offerCollection keeps a collection for the user uid to save; the
// collection's files are removed once saved or after saveIdle.
func (worker classroomWorker) offerCollection(uid int, collection string) (string, error) {
	items, err := worker.collected.Items(collection)
	if err != nil {
		return "", err
	}
	files := make([]domain.ShareFile, len(items))
	for index, item := range items {
		files[index] = item.ShareFile
	}
	read := func(index int, offset int64, size int) ([]byte, error) {
		return worker.collected.Read(collection, items[index], offset, size)
	}
	return worker.view.desktops.Offer(uid, domain.DesktopSaveCollected, files, read, func() { worker.collected.Remove(collection) })
}
