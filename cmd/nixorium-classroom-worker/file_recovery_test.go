package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// recoverySource is a laboratory of two computers whose helper answers
// with two saved sessions.
type recoverySource struct {
	restored [][]string
}

func (source *recoverySource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	meta := domain.LabMeta{}
	meta.Controller.Name = "controller"
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}, {Name: "pc02", IP: "10.42.0.12"}}
	return meta, nil
}

func (source *recoverySource) FileRecovery(_ context.Context, _ domain.HostMeta, request domain.FileRecoveryHelperRequest) (domain.FileRecoveryHelperAnswer, error) {
	saved := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	switch request.Operation {
	case domain.FileRecoveryHelperSessions:
		return domain.FileRecoveryHelperAnswer{Sessions: []domain.FileRecoverySession{{Number: 1, SavedAt: saved}, {Number: 2, SavedAt: saved.Add(-time.Hour)}}}, nil
	case domain.FileRecoveryHelperList:
		if request.Path == "Missing" {
			return domain.FileRecoveryHelperAnswer{Error: "This folder is not in the saved session.", Code: domain.FileRecoveryCodeNotFound}, nil
		}
		return domain.FileRecoveryHelperAnswer{Listing: &domain.FileRecoveryListing{Session: request.Session, Path: request.Path, Entries: []domain.FileRecoveryEntry{{Name: "notes.txt", Size: 4}}}}, nil
	case domain.FileRecoveryHelperRestore:
		source.restored = append(source.restored, request.Paths)
		return domain.FileRecoveryHelperAnswer{Restored: &domain.FileRecoveryRestored{Folder: "Desktop/Recovered files", Placed: request.Paths, Files: 1, Bytes: 4}}, nil
	}
	return domain.FileRecoveryHelperAnswer{Error: "unknown"}, nil
}

func TestTeacherRecoversFilesThroughTheClassroomWorker(t *testing.T) {
	source := &recoverySource{}
	worker := classroomWorker{repository: "/home/admin/nixorium-deployment", plans: newPlanStore(), viewOn: func() bool { return false }, recovery: app.NewFileRecoveryManager(source)}
	ctx := context.Background()
	request := func(operation domain.ClassroomOperation, configure func(*domain.ClassroomRequest)) domain.ClassroomResponse {
		t.Helper()
		value, err := domain.NewClassroomRequest(operation)
		if err != nil {
			t.Fatal(err)
		}
		configure(&value)
		return worker.handle(ctx, value)
	}
	// Recovering works with the classroom view off.
	sessions := request(domain.ClassroomFileRecoverySessionsOperation, func(r *domain.ClassroomRequest) { r.Requested = "pc02" })
	if sessions.State != "completed" || len(sessions.FileRecoverySessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	listing := request(domain.ClassroomFileRecoveryListOperation, func(r *domain.ClassroomRequest) {
		r.Requested, r.FileRecoverySession, r.FileRecoveryPath = "pc02", 2, "Desktop"
	})
	if listing.FileRecoveryListing == nil || listing.FileRecoveryListing.Path != "Desktop" {
		t.Fatalf("listing = %+v", listing)
	}
	missing := request(domain.ClassroomFileRecoveryListOperation, func(r *domain.ClassroomRequest) {
		r.Requested, r.FileRecoverySession, r.FileRecoveryPath = "pc02", 2, "Missing"
	})
	if missing.State != "failed" || missing.Message != "This folder is not in the saved session." {
		t.Fatalf("refusal = %+v", missing)
	}
	controller := request(domain.ClassroomFileRecoverySessionsOperation, func(r *domain.ClassroomRequest) { r.Requested = "controller" })
	if controller.State != "failed" {
		t.Fatalf("the controller was accepted: %+v", controller)
	}
	plan := request(domain.ClassroomFileRecoveryPlanOperation, func(r *domain.ClassroomRequest) {
		r.Requested, r.FileRecoverySession, r.FileRecoveryPaths = "pc02", 2, []string{"Desktop/notes.txt"}
	})
	if plan.FileRecoveryPlan == nil || plan.FileRecoveryPlan.HasErrors() {
		t.Fatalf("plan = %+v", plan)
	}
	// A plan changed by the caller is never applied: the worker applies its
	// own copy, found by the review token.
	forged := *plan.FileRecoveryPlan
	forged.Paths = []string{"Desktop"}
	forged.ReviewToken = "forged"
	if response := request(domain.ClassroomFileRecoveryApplyOperation, func(r *domain.ClassroomRequest) { r.FileRecoveryPlan = &forged }); response.FileRecoveryReport != nil || len(source.restored) != 0 {
		t.Fatalf("forged plan applied: %+v", response)
	}
	copied := *plan.FileRecoveryPlan
	copied.Paths = []string{"Desktop"}
	response := request(domain.ClassroomFileRecoveryApplyOperation, func(r *domain.ClassroomRequest) { r.FileRecoveryPlan = &copied })
	if response.FileRecoveryReport == nil || response.FileRecoveryReport.HasErrors() || len(source.restored) != 1 || strings.Join(source.restored[0], ",") != "Desktop/notes.txt" {
		t.Fatalf("apply = %+v, restored %v", response, source.restored)
	}
	if again := request(domain.ClassroomFileRecoveryApplyOperation, func(r *domain.ClassroomRequest) { r.FileRecoveryPlan = &copied }); again.FileRecoveryReport != nil || len(source.restored) != 1 {
		t.Fatalf("a review was applied twice: %+v", again)
	}
}
