package main

import (
	"context"
	_ "embed"
	"net/http"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Schedule: switching the student computers on and off by themselves. The
// classroom service keeps the schedule and shows and sets it for the
// teacher's dashboard and command line (classroom socket) and for the
// classroom view page's Schedule dialog. The controller's timer runs it
// (nixorium schedule run); the service only keeps the record.

//go:embed view/schedule.js
var scheduleScript []byte

func init() {
	// Saving from the dialog is a POST of this page to its own endpoint.
	postPaths["/api/schedule"] = true
}

func (worker classroomWorker) handleSchedule(_ context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	if worker.schedule == nil {
		return domain.ClassroomResponse{State: "failed", Message: "The schedule is not available."}
	}
	var report domain.ScheduleReport
	if request.Operation == domain.ClassroomScheduleSetOperation {
		if request.Schedule == nil {
			return domain.ClassroomResponse{State: "failed", Message: "The request has no schedule."}
		}
		report = worker.schedule.Set(*request.Schedule)
	} else {
		report = worker.schedule.Show()
	}
	report.Message, report.Issues = teacherMessage(report.Message), teacherIssues(report.Issues)
	return domain.ClassroomResponse{State: "completed", Schedule: &report}
}

// scheduleAsset serves the dialog's script.
func (server *viewServer) scheduleAsset(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = writer.Write(scheduleScript)
}

// schedulePage reads the schedule (GET) or sets it (POST of the schedule;
// one without times clears it) for the dialog, and answers with the
// resulting schedule report.
func (server *viewServer) schedulePage(writer http.ResponseWriter, request *http.Request) {
	if server.actions == nil {
		http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
		return
	}
	operation, err := domain.NewClassroomRequest(domain.ClassroomScheduleGetOperation)
	if err != nil {
		http.Error(writer, "The action could not start.", http.StatusInternalServerError)
		return
	}
	if request.Method == http.MethodPost {
		var body domain.Schedule
		if !decodePageJSON(writer, request, &body) {
			return
		}
		operation.Operation, operation.Schedule = domain.ClassroomScheduleSetOperation, &body
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	response := server.actions(ctx, operation)
	if response.Schedule == nil {
		writePageJSON(writer, domain.ScheduleReport{SchemaVersion: domain.SchemaVersion, Operation: string(operation.Operation), State: "blocked", Schedule: domain.Schedule{Days: []string{}}, Message: response.Message, Issues: []domain.ValidationIssue{}})
		return
	}
	writePageJSON(writer, response.Schedule)
}
