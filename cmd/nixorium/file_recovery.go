package main

import (
	"context"
	"errors"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// fileRecoveryApplyTimeout covers copying up to 500 MB onto a student's
// desktop, longer than the classroom socket's default wait.
const fileRecoveryApplyTimeout = 17 * time.Minute

// classroomFileRecovery recovers files through the classroom service, for
// the teacher, who has no access to the student computers.
func classroomFileRecovery(ctx context.Context) *presentation.FileRecoveryActions {
	failure := func(response domain.ClassroomResponse, err error, fallback string) string {
		if err != nil {
			return err.Error()
		}
		if response.Message != "" {
			return response.Message
		}
		return fallback
	}
	return &presentation.FileRecoveryActions{
		Sessions: func(ctx context.Context, computer string) ([]domain.FileRecoverySession, error) {
			response, err := classroomRequest(ctx, domain.ClassroomFileRecoverySessionsOperation, func(request *domain.ClassroomRequest) {
				request.Requested = computer
			})
			if err != nil || response.FileRecoverySessions == nil {
				return nil, errors.New(failure(response, err, "The saved sessions could not be read."))
			}
			return response.FileRecoverySessions, nil
		},
		List: func(ctx context.Context, computer string, session int, folder string) (domain.FileRecoveryListing, error) {
			response, err := classroomRequest(ctx, domain.ClassroomFileRecoveryListOperation, func(request *domain.ClassroomRequest) {
				request.Requested = computer
				request.FileRecoverySession = session
				request.FileRecoveryPath = folder
			})
			if err != nil || response.FileRecoveryListing == nil {
				return domain.FileRecoveryListing{}, errors.New(failure(response, err, "The folder could not be read."))
			}
			return *response.FileRecoveryListing, nil
		},
		Plan: func(ctx context.Context, computer string, session int, paths []string) domain.FileRecoveryPlan {
			response, err := classroomRequest(ctx, domain.ClassroomFileRecoveryPlanOperation, func(request *domain.ClassroomRequest) {
				request.Requested = computer
				request.FileRecoverySession = session
				request.FileRecoveryPaths = paths
			})
			if err != nil || response.FileRecoveryPlan == nil {
				message := failure(response, err, "Recovering files is unavailable.")
				return domain.FileRecoveryPlan{SchemaVersion: domain.SchemaVersion, Operation: "file-recovery-plan", State: "blocked", Requested: computer, Session: session, Paths: paths, Message: message, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
			}
			return *response.FileRecoveryPlan
		},
		Apply: func(plan domain.FileRecoveryPlan) domain.FileRecoveryReport {
			ctx, cancel := context.WithTimeout(ctx, fileRecoveryApplyTimeout)
			defer cancel()
			response, err := classroomRequest(ctx, domain.ClassroomFileRecoveryApplyOperation, func(request *domain.ClassroomRequest) {
				request.FileRecoveryPlan = &plan
			})
			if err != nil || response.FileRecoveryReport == nil {
				return domain.FileRecoveryReport{SchemaVersion: domain.SchemaVersion, Operation: "file-recovery-apply", State: "blocked", Computer: plan.Target.Name, Message: failure(response, err, "The reviewed recovery could not be applied.")}
			}
			return *response.FileRecoveryReport
		},
	}
}

// directFileRecovery recovers files with the administrator's own access to
// the student computers.
func directFileRecovery(ctx context.Context, manager *app.FileRecoveryManager, repository string) *presentation.FileRecoveryActions {
	return &presentation.FileRecoveryActions{
		Sessions: func(ctx context.Context, computer string) ([]domain.FileRecoverySession, error) {
			return manager.Sessions(ctx, repository, computer)
		},
		List: func(ctx context.Context, computer string, session int, folder string) (domain.FileRecoveryListing, error) {
			return manager.List(ctx, repository, computer, session, folder)
		},
		Plan: func(ctx context.Context, computer string, session int, paths []string) domain.FileRecoveryPlan {
			return manager.Plan(ctx, repository, computer, session, paths)
		},
		Apply: func(plan domain.FileRecoveryPlan) domain.FileRecoveryReport {
			return manager.Apply(ctx, plan, plan.ReviewToken)
		},
	}
}
