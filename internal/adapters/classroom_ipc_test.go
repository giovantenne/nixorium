package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestClassroomIPCExchangesOnlyTypedRequests(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "classroom")
	if err := os.Mkdir(directory, 0770); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0770); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "control.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewClassroomIPCServer(path, func(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
		// The worker learns who asks from the kernel, not from the request.
		uid, ok := ClassroomPeerUID(ctx)
		if !ok || uid != os.Getuid() {
			return domain.ClassroomResponse{State: "completed", Message: "unknown peer"}
		}
		return domain.ClassroomResponse{State: "completed", Message: string(request.Operation)}
	})
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(path); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	request, err := domain.NewClassroomRequest(domain.ClassroomOverviewOperation)
	if err != nil {
		t.Fatal(err)
	}
	response, err := ClassroomIPCRequest(context.Background(), path, request)
	if err != nil || response.Message != string(domain.ClassroomOverviewOperation) || response.RequestID != request.RequestID {
		t.Fatalf("response = %+v, error = %v", response, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClassroomIPCRejectsPublicRuntimeDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "classroom")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	server := NewClassroomIPCServer(filepath.Join(directory, "control.sock"), func(context.Context, domain.ClassroomRequest) domain.ClassroomResponse {
		return domain.ClassroomResponse{}
	})
	if err := server.Serve(context.Background()); err == nil {
		t.Fatal("public classroom runtime directory was accepted")
	}
}
